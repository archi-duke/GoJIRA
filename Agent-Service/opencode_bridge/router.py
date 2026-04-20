"""OpenCode 기반 /api/v2/Agent 라우터.

UI 계약 유지:
  POST /api/v2/Agent/chat/stream  → SSE(session/content/tool_*/done/error)
  GET  /api/v2/Agent/sessions      → Mongo 세션 목록
  GET  /api/v2/Agent/session/{id}  → 세션 복원
"""

import logging
import uuid
from datetime import datetime, timezone

from fastapi import APIRouter, Header, Request
from fastapi.responses import StreamingResponse

from core.models import ChatRequest
from core.database import agent_sessions

from . import client as oc
from .translator import translate, _sse

log = logging.getLogger("agent.opencode_bridge")

router = APIRouter(prefix="/api/v2/Agent", tags=["Agent"])


def _now():
    return datetime.now(timezone.utc)


async def _ensure_opencode_session(gojira_sid: str, user: str) -> str:
    """Mongo 세션에서 openCodeSessionId 를 찾거나 새로 생성. stale 세션은 자동 교체."""
    doc = agent_sessions().find_one({"_id": gojira_sid}) or {}
    oc_sid = doc.get("openCodeSessionId")
    if oc_sid and await oc.session_exists(oc_sid):
        return oc_sid
    if oc_sid:
        log.warning(f"OpenCode 세션 {oc_sid[:16]} 만료 — 새 세션 생성")
    oc_sid = await oc.create_session(title=f"GoJIRA {gojira_sid[:8]}")
    agent_sessions().update_one(
        {"_id": gojira_sid},
        {
            "$set": {
                "openCodeSessionId": oc_sid,
                "userId": user,
                "updatedAt": _now(),
            },
            "$setOnInsert": {"_id": gojira_sid, "messages": [], "createdAt": _now()},
        },
        upsert=True,
    )
    return oc_sid


@router.post("/chat/stream")
async def chat_stream(
    req: ChatRequest,
    request: Request,
    x_user_id: str = Header(default="batman", alias="X-User-Id"),
):
    gojira_sid = req.session_id or str(uuid.uuid4())
    oc_sid = await _ensure_opencode_session(gojira_sid, x_user_id)
    # 컨텍스트를 메시지 앞에 프리픽스로 삽입 (noReply 주입 대신 — noReply 가 UI 에 노출되는 문제 방지)
    project_code = (req.context or {}).get("projectCode", "")

    # 메시지를 mongo 에 append (히스토리 보존)
    agent_sessions().update_one(
        {"_id": gojira_sid},
        {
            "$push": {"messages": {"role": "user", "content": req.message, "timestamp": _now()}},
            "$set": {"updatedAt": _now()},
        },
    )

    async def sse_stream():
        try:
            # 이벤트 구독 시작 (먼저 시작해야 첫 이벤트 놓치지 않음)
            import asyncio
            events_iter = oc.stream_events(oc_sid)

            # prompt_async 로 메시지 송신
            await oc.send_message_async(oc_sid, req.message)

            final_reply_holder = {"text": ""}
            process_steps = []

            IDLE_TIMEOUT = 300.0
            POLL_INTERVAL = 5.0
            translated = translate(events_iter, gojira_sid)
            poll_state = {"last_reasoning": "", "last_text": "", "idle_since": 0.0}

            async def _next_sse():
                return await translated.__anext__()

            while True:
                try:
                    sse = await asyncio.wait_for(_next_sse(), timeout=POLL_INTERVAL)
                    poll_state["idle_since"] = 0.0
                except StopAsyncIteration:
                    break
                except asyncio.TimeoutError:
                    # 이벤트 없음 — 세션 메시지 폴링으로 새 내용 확인
                    if poll_state["idle_since"] == 0.0:
                        poll_state["idle_since"] = asyncio.get_event_loop().time()
                    elapsed = asyncio.get_event_loop().time() - poll_state["idle_since"]
                    if elapsed > IDLE_TIMEOUT:
                        log.warning(f"SSE watchdog timeout — aborting OpenCode session {oc_sid}")
                        try:
                            await oc.abort_session(oc_sid)
                        except Exception:
                            pass
                        yield _sse("error", {"message": f"{IDLE_TIMEOUT}초 동안 응답이 없어 작업을 중단했습니다 (권한 대기 또는 행 상태 의심). 재시도해주세요."})
                        yield _sse("done", {"reply": "", "actions": []})
                        return
                    # 폴링: 세션 상태 + 메시지에서 새 내용 확인
                    try:
                        # 세션 상태 확인 — idle이면 완료
                        import httpx as _hx
                        async with _hx.AsyncClient(timeout=3.0) as _pc:
                            sr = await _pc.get(f"{oc._base_url()}/session/{oc_sid}", headers=oc._auth_header())
                            if sr.status_code == 200:
                                sdata = sr.json()
                                sstatus = (sdata.get("status") or {}).get("type") or sdata.get("status")
                                if sstatus == "idle" or sstatus == "stopped":
                                    # 최종 텍스트 수집
                                    msgs = await oc.get_session_messages(oc_sid)
                                    reply_text = ""
                                    if msgs:
                                        for part in reversed(msgs[-1].get("parts") or []):
                                            if part.get("type") == "text":
                                                reply_text = part.get("text", "")
                                                break
                                    if reply_text and reply_text != poll_state["last_text"]:
                                        delta = reply_text[len(poll_state["last_text"]):]
                                        yield _sse("content", {"text": delta})
                                        process_steps.append({"type": "content", "data": {"text": delta}})
                                    final_reply_holder["text"] = reply_text or poll_state["last_text"]
                                    yield _sse("done", {"reply": final_reply_holder["text"], "actions": []})
                                    break

                        # 메시지에서 새 reasoning/text 확인
                        msgs = await oc.get_session_messages(oc_sid)
                        if msgs:
                            last_msg = msgs[-1]
                            for part in (last_msg.get("parts") or []):
                                pt = part.get("type")
                                txt = part.get("text") or part.get("content") or ""
                                if pt == "reasoning" and txt and txt != poll_state["last_reasoning"]:
                                    delta = txt[len(poll_state["last_reasoning"]):]
                                    poll_state["last_reasoning"] = txt
                                    yield _sse("reasoning", {"turn": 0, "step": "thought", "content": delta})
                                    process_steps.append({"type": "reasoning", "data": {"turn": 0, "step": "thought", "content": delta}})
                                elif pt == "text" and txt and txt != poll_state["last_text"]:
                                    delta = txt[len(poll_state["last_text"]):]
                                    poll_state["last_text"] = txt
                                    yield _sse("content", {"text": delta})
                                    process_steps.append({"type": "content", "data": {"text": delta}})
                    except Exception:
                        pass
                    yield ": keepalive\n\n"
                    continue
                # keepalive 는 UI 로 전달만 하고 로직 건너뛰기
                if sse.startswith(":"):
                    yield sse
                    continue
                # process step 수집 (이력 저장용)
                import json as _j
                for evt_type in ("reasoning", "tool_call", "tool_result", "content"):
                    if sse.startswith(f"event: {evt_type}\n"):
                        try:
                            data_line = sse.split("\n", 2)[1]
                            step_data = _j.loads(data_line[len("data: "):])
                            process_steps.append({"type": evt_type, "data": step_data})
                        except Exception:
                            pass
                        break
                # done 이벤트의 reply 를 히스토리에 저장하기 위해 가로채기
                if sse.startswith("event: done\n"):
                    try:
                        import json as _j
                        data_line = sse.split("\n", 2)[1]
                        payload = _j.loads(data_line[len("data: "):])
                        final_reply_holder["text"] = payload.get("reply", "")
                        # translator 가 권한 타임아웃으로 abort 요청한 경우
                        if payload.get("_abort"):
                            try:
                                await oc.abort_session(oc_sid)
                            except Exception:
                                pass
                    except Exception:
                        pass
                yield sse
                if sse.startswith("event: done\n"):
                    break

            # process + assistant 메시지 Mongo 저장
            msgs_to_push = []
            if process_steps:
                msgs_to_push.append({"role": "process", "steps": process_steps, "timestamp": _now()})
            if final_reply_holder["text"]:
                msgs_to_push.append({"role": "assistant", "content": final_reply_holder["text"], "timestamp": _now()})
            if msgs_to_push:
                agent_sessions().update_one(
                    {"_id": gojira_sid},
                    {
                        "$push": {"messages": {"$each": msgs_to_push}},
                        "$set": {"updatedAt": _now()},
                    },
                )
        except Exception as e:
            log.exception("chat_stream 실패")
            yield _sse("error", {"message": f"OpenCode 브리지 오류: {e}"})
            yield _sse("done", {"reply": "", "actions": []})

    return StreamingResponse(sse_stream(), media_type="text/event-stream")


@router.post("/chat/send")
async def chat_send(
    req: ChatRequest,
    request: Request,
    x_user_id: str = Header(default="batman", alias="X-User-Id"),
):
    """메시지 전송만 수행 (이벤트는 /session/{id}/events 에서 수신)."""
    gojira_sid = req.session_id or str(uuid.uuid4())
    oc_sid = await _ensure_opencode_session(gojira_sid, x_user_id)

    agent_sessions().update_one(
        {"_id": gojira_sid},
        {
            "$push": {"messages": {"role": "user", "content": req.message, "timestamp": _now()}},
            "$set": {"updatedAt": _now()},
        },
    )

    await oc.send_message_async(oc_sid, req.message)
    return {"sessionId": gojira_sid, "openCodeSessionId": oc_sid}


@router.get("/session/{session_id}/events")
async def session_events(session_id: str, x_user_id: str = Header(default="batman", alias="X-User-Id")):
    """세션별 SSE 이벤트 스트림 — EventSource 로 연결, 자동 재접속 지원."""
    doc = agent_sessions().find_one({"_id": session_id}) or {}
    oc_sid = doc.get("openCodeSessionId")
    if not oc_sid:
        async def _err():
            yield _sse("error", {"message": "세션에 연결된 OpenCode 세션이 없습니다."})
        return StreamingResponse(_err(), media_type="text/event-stream")

    async def sse_stream():
        import asyncio
        try:
            events_iter = oc.stream_events(oc_sid)
            translated = translate(events_iter, session_id)
            IDLE_TIMEOUT = 300.0
            POLL_INTERVAL = 5.0
            final_reply_holder = {"text": ""}
            process_steps = []
            poll_state = {"last_reasoning": "", "last_text": "", "idle_since": 0.0}

            async def _next_sse():
                return await translated.__anext__()

            while True:
                try:
                    sse = await asyncio.wait_for(_next_sse(), timeout=POLL_INTERVAL)
                    poll_state["idle_since"] = 0.0
                except StopAsyncIteration:
                    break
                except asyncio.TimeoutError:
                    if poll_state["idle_since"] == 0.0:
                        poll_state["idle_since"] = asyncio.get_event_loop().time()
                    elapsed = asyncio.get_event_loop().time() - poll_state["idle_since"]
                    if elapsed > IDLE_TIMEOUT:
                        log.warning(f"events watchdog timeout — session {session_id}")
                        try:
                            await oc.abort_session(oc_sid)
                        except Exception:
                            pass
                        yield _sse("error", {"message": "응답 대기 시간 초과"})
                        yield _sse("done", {"reply": "", "actions": []})
                        return
                    try:
                        import httpx as _hx
                        async with _hx.AsyncClient(timeout=3.0) as _pc:
                            sr = await _pc.get(f"{oc._base_url()}/session/{oc_sid}", headers=oc._auth_header())
                            if sr.status_code == 200:
                                sdata = sr.json()
                                sstatus = (sdata.get("status") or {}).get("type") or sdata.get("status")
                                if sstatus == "idle" or sstatus == "stopped":
                                    msgs = await oc.get_session_messages(oc_sid)
                                    reply_text = ""
                                    if msgs:
                                        for part in reversed(msgs[-1].get("parts") or []):
                                            if part.get("type") == "text":
                                                reply_text = part.get("text", "")
                                                break
                                    if reply_text and reply_text != poll_state["last_text"]:
                                        delta = reply_text[len(poll_state["last_text"]):]
                                        yield _sse("content", {"text": delta})
                                        process_steps.append({"type": "content", "data": {"text": delta}})
                                    final_reply_holder["text"] = reply_text or poll_state["last_text"]
                                    yield _sse("done", {"reply": final_reply_holder["text"], "actions": []})
                                    break

                        msgs = await oc.get_session_messages(oc_sid)
                        if msgs:
                            last_msg = msgs[-1]
                            for part in (last_msg.get("parts") or []):
                                pt = part.get("type")
                                txt = part.get("text") or part.get("content") or ""
                                if pt == "reasoning" and txt and txt != poll_state["last_reasoning"]:
                                    delta = txt[len(poll_state["last_reasoning"]):]
                                    poll_state["last_reasoning"] = txt
                                    yield _sse("reasoning", {"turn": 0, "step": "thought", "content": delta})
                                    process_steps.append({"type": "reasoning", "data": {"turn": 0, "step": "thought", "content": delta}})
                                elif pt == "text" and txt and txt != poll_state["last_text"]:
                                    delta = txt[len(poll_state["last_text"]):]
                                    poll_state["last_text"] = txt
                                    yield _sse("content", {"text": delta})
                                    process_steps.append({"type": "content", "data": {"text": delta}})
                    except Exception:
                        pass
                    yield ": keepalive\n\n"
                    continue
                if sse.startswith(":"):
                    yield sse
                    continue
                # process step 수집
                import json as _j
                for evt_type in ("reasoning", "tool_call", "tool_result", "content"):
                    if sse.startswith(f"event: {evt_type}\n"):
                        try:
                            data_line = sse.split("\n", 2)[1]
                            step_data = _j.loads(data_line[len("data: "):])
                            process_steps.append({"type": evt_type, "data": step_data})
                        except Exception:
                            pass
                        break
                if sse.startswith("event: done\n"):
                    try:
                        data_line = sse.split("\n", 2)[1]
                        payload = _j.loads(data_line[len("data: "):])
                        final_reply_holder["text"] = payload.get("reply", "")
                        if payload.get("_abort"):
                            try:
                                await oc.abort_session(oc_sid)
                            except Exception:
                                pass
                    except Exception:
                        pass
                yield sse
                if sse.startswith("event: done\n"):
                    break

            msgs_to_push = []
            if process_steps:
                msgs_to_push.append({"role": "process", "steps": process_steps, "timestamp": _now()})
            if final_reply_holder["text"]:
                msgs_to_push.append({"role": "assistant", "content": final_reply_holder["text"], "timestamp": _now()})
            if msgs_to_push:
                agent_sessions().update_one(
                    {"_id": session_id},
                    {
                        "$push": {"messages": {"$each": msgs_to_push}},
                        "$set": {"updatedAt": _now()},
                    },
                )
        except Exception as e:
            log.exception("session_events 실패")
            yield _sse("error", {"message": f"이벤트 스트림 오류: {e}"})
            yield _sse("done", {"reply": "", "actions": []})

    return StreamingResponse(sse_stream(), media_type="text/event-stream")


@router.post("/session/{session_id}/terminal")
async def open_terminal(session_id: str):
    """macOS Terminal.app 에서 해당 세션의 OpenCode attach 실행."""
    doc = agent_sessions().find_one({"_id": session_id}) or {}
    oc_sid = doc.get("openCodeSessionId")
    if not oc_sid:
        return {"ok": False, "reason": "openCodeSessionId 없음"}
    base_url = oc._base_url()
    cmd = f'opencode attach {base_url} -s {oc_sid}'
    import subprocess, tempfile
    sh = tempfile.NamedTemporaryFile(mode='w', suffix='.sh', delete=False, prefix='gojira-term-')
    sh.write(f'#!/bin/bash\n')
    sh.write(f'echo -e "\\033[1;36m[GoJIRA] OpenCode 세션 연결 — Ctrl+C 로 안전하게 종료\\033[0m\\n"\n')
    sh.write(f'{cmd}\n')
    sh.write(f'echo -e "\\n\\033[1;33m세션 연결 종료. 창을 닫아도 안전합니다.\\033[0m"\n')
    sh.write(f'sleep 2; exit\n')
    sh.close()
    import os
    os.chmod(sh.name, 0o755)
    subprocess.Popen(["open", "-a", "Terminal", sh.name])
    return {"ok": True, "command": cmd, "openCodeSessionId": oc_sid}


@router.delete("/session/{session_id}")
async def delete_session(session_id: str, x_user_id: str = Header(default="batman", alias="X-User-Id")):
    agent_sessions().delete_one({"_id": session_id})
    return {"ok": True}


@router.delete("/sessions")
async def delete_all_sessions(x_user_id: str = Header(default="batman", alias="X-User-Id")):
    result = agent_sessions().delete_many({"userId": x_user_id})
    return {"ok": True, "deleted": result.deleted_count}


@router.post("/session/{session_id}/cancel")
async def cancel_session(session_id: str):
    """진행 중인 세션 중단 — OpenCode abort 전달."""
    doc = agent_sessions().find_one({"_id": session_id}) or {}
    oc_sid = doc.get("openCodeSessionId")
    if not oc_sid:
        return {"ok": False, "reason": "no openCodeSessionId"}
    ok = await oc.abort_session(oc_sid)
    return {"ok": ok, "openCodeSessionId": oc_sid}


@router.get("/sessions")
async def list_sessions(x_user_id: str = Header(default="batman", alias="X-User-Id")):
    docs = list(agent_sessions().find({"userId": x_user_id}, {"messages": 0}).sort("updatedAt", -1).limit(50))
    return [
        {
            "sessionId": d["_id"],
            "title": d.get("title") or (d.get("messages", [{}])[0].get("content", "")[:40] if d.get("messages") else "New session"),
            "createdAt": d.get("createdAt"),
            "updatedAt": d.get("updatedAt"),
        }
        for d in docs
    ]


@router.get("/session/{session_id}")
async def get_session(session_id: str):
    doc = agent_sessions().find_one({"_id": session_id})
    if not doc:
        return {"sessionId": session_id, "messages": []}
    return {
        "sessionId": doc["_id"],
        "messages": doc.get("messages", []),
    }


@router.get("/health")
async def health():
    try:
        import httpx
        async with httpx.AsyncClient(timeout=3.0) as c:
            r = await c.get(f"{oc._base_url()}/global/health", headers=oc._auth_header())
            up = r.status_code == 200
    except Exception:
        up = False
    return {"bridge": "ok", "opencode": "up" if up else "down"}
