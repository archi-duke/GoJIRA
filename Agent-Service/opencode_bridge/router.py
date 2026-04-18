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

            # Watchdog: 60초 동안 어떤 이벤트도 없으면 권한 대기 등으로 행 된 것으로 간주 → abort
            IDLE_TIMEOUT = 300.0
            translated = translate(events_iter, gojira_sid)

            async def _next_sse():
                return await translated.__anext__()

            while True:
                try:
                    sse = await asyncio.wait_for(_next_sse(), timeout=IDLE_TIMEOUT)
                except StopAsyncIteration:
                    break
                except asyncio.TimeoutError:
                    log.warning(f"SSE watchdog timeout — aborting OpenCode session {oc_sid}")
                    try:
                        await oc.abort_session(oc_sid)
                    except Exception:
                        pass
                    yield _sse("error", {"message": f"{IDLE_TIMEOUT}초 동안 응답이 없어 작업을 중단했습니다 (권한 대기 또는 행 상태 의심). 재시도해주세요."})
                    yield _sse("done", {"reply": "", "actions": []})
                    return
                # keepalive 는 UI 로 전달만 하고 로직 건너뛰기
                if sse.startswith(":"):
                    yield sse
                    continue
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

            # assistant 메시지 Mongo 저장
            if final_reply_holder["text"]:
                agent_sessions().update_one(
                    {"_id": gojira_sid},
                    {
                        "$push": {"messages": {"role": "assistant", "content": final_reply_holder["text"], "timestamp": _now()}},
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
            final_reply_holder = {"text": ""}

            async def _next_sse():
                return await translated.__anext__()

            while True:
                try:
                    sse = await asyncio.wait_for(_next_sse(), timeout=IDLE_TIMEOUT)
                except StopAsyncIteration:
                    break
                except asyncio.TimeoutError:
                    log.warning(f"events watchdog timeout — session {session_id}")
                    try:
                        await oc.abort_session(oc_sid)
                    except Exception:
                        pass
                    yield _sse("error", {"message": "응답 대기 시간 초과"})
                    yield _sse("done", {"reply": "", "actions": []})
                    return
                if sse.startswith(":"):
                    yield sse
                    continue
                if sse.startswith("event: done\n"):
                    try:
                        import json as _j
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

            if final_reply_holder["text"]:
                agent_sessions().update_one(
                    {"_id": session_id},
                    {
                        "$push": {"messages": {"role": "assistant", "content": final_reply_holder["text"], "timestamp": _now()}},
                        "$set": {"updatedAt": _now()},
                    },
                )
        except Exception as e:
            log.exception("session_events 실패")
            yield _sse("error", {"message": f"이벤트 스트림 오류: {e}"})
            yield _sse("done", {"reply": "", "actions": []})

    return StreamingResponse(sse_stream(), media_type="text/event-stream")


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
