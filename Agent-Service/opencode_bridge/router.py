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
    """Mongo 세션에서 openCodeSessionId 를 찾거나 새로 생성."""
    doc = agent_sessions().find_one({"_id": gojira_sid}) or {}
    oc_sid = doc.get("openCodeSessionId")
    if oc_sid:
        return oc_sid
    oc_sid = await oc.create_session(title=f"GoJIRA {gojira_sid[:8]}")
    # 사용자 컨텍스트를 noReply 로 주입 — 스킬 bash 에서 $USER 로 사용
    await oc.inject_context(
        oc_sid,
        f"[SYSTEM CONTEXT] 현재 사용자의 knoxID(USER)는 `{user}` 입니다. "
        f"스킬 실행 시 `export USER=\"{user}\"` 를 자동으로 사용하고, "
        f"별도로 knoxID 를 되묻지 마세요.",
    )
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
    # 매 요청마다 사용자 컨텍스트 주입 (기존 세션에도 적용)
    await oc.inject_context(
        oc_sid,
        f"[SYSTEM CONTEXT] 현재 사용자의 knoxID(USER)는 `{x_user_id}` 입니다. "
        f"스킬 실행 시 `export USER=\"{x_user_id}\"` 를 자동으로 사용하고, "
        f"knoxID 를 사용자에게 되묻지 마세요.",
    )

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
            await oc.send_message_async(oc_sid, req.message, agent="gojira")

            final_reply_holder = {"text": ""}

            # Watchdog: 60초 동안 어떤 이벤트도 없으면 권한 대기 등으로 행 된 것으로 간주 → abort
            IDLE_TIMEOUT = 60.0
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
                # done 이벤트의 reply 를 히스토리에 저장하기 위해 가로채기
                if sse.startswith("event: done\n"):
                    try:
                        import json as _j
                        data_line = sse.split("\n", 2)[1]
                        payload = _j.loads(data_line[len("data: "):])
                        final_reply_holder["text"] = payload.get("reply", "")
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
