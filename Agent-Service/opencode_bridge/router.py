"""OpenCode 기반 /api/v2/Agent 라우터.

UI 계약 유지:
  POST /api/v2/Agent/chat/send   → 메시지 전송 + 백그라운드 구독 시작
  GET  /api/v2/Agent/session/{id}/events → SSE (버퍼에서 읽기, 재연결 지원)
  GET  /api/v2/Agent/sessions    → Mongo 세션 목록
  GET  /api/v2/Agent/session/{id} → 세션 복원
"""

import asyncio
import json
import logging
import time
import uuid
from datetime import datetime, timezone

from fastapi import APIRouter, Header, Request
from fastapi.responses import StreamingResponse

from core.models import ChatRequest
from core.database import agent_sessions

from . import client as oc
from .translator import _sse
from . import event_buffer as eb

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


# ── 메시지 전송 ──

@router.post("/chat/send")
async def chat_send(
    req: ChatRequest,
    request: Request,
    x_user_id: str = Header(default="batman", alias="X-User-Id"),
):
    """메시지 전송 + 백그라운드 구독자 시작."""
    gojira_sid = req.session_id or str(uuid.uuid4())
    oc_sid = await _ensure_opencode_session(gojira_sid, x_user_id)

    agent_sessions().update_one(
        {"_id": gojira_sid},
        {
            "$push": {"messages": {"role": "user", "content": req.message, "timestamp": _now()}},
            "$set": {"updatedAt": _now()},
        },
    )

    # 이전 턴 이벤트 버퍼 초기화 (중복 방지)
    eb.clear(gojira_sid)

    # 턴 경계에서 구독자 리셋 (이전 턴 구독자가 stale 인 경우 대비)
    await eb.reset_subscriber(oc_sid, gojira_sid)

    # 메시지 전송
    await oc.send_message_async(oc_sid, req.message)

    return {"sessionId": gojira_sid, "openCodeSessionId": oc_sid}


# ── SSE 이벤트 스트림 (버퍼에서 읽기) ──

@router.get("/session/{session_id}/events")
async def session_events(
    session_id: str,
    request: Request,
    x_user_id: str = Header(default="batman", alias="X-User-Id"),
):
    """세션별 SSE 이벤트 스트림 — 버퍼에서 읽기, Last-Event-ID로 재연결 지원."""
    doc = agent_sessions().find_one({"_id": session_id}) or {}
    oc_sid = doc.get("openCodeSessionId")
    if not oc_sid:
        async def _err():
            yield _sse("error", {"message": "세션에 연결된 OpenCode 세션이 없습니다."})
        return StreamingResponse(_err(), media_type="text/event-stream")

    last_event_id = int(request.headers.get("Last-Event-ID", "0") or "0")

    # 재연결 시 백그라운드 구독자가 없으면 시작
    await eb.ensure_subscriber(oc_sid, session_id)

    async def sse_stream():
        cursor = last_event_id
        IDLE_TIMEOUT = 180.0
        idle_start = asyncio.get_event_loop().time()
        last_keepalive = asyncio.get_event_loop().time()

        while True:
            # 버퍼에서 새 이벤트 읽기
            events = eb.read_since(session_id, cursor)
            if events:
                idle_start = asyncio.get_event_loop().time()
                last_keepalive = idle_start
                for evt in events:
                    cursor = evt["id"]
                    yield f"id: {evt['id']}\nevent: {evt['type']}\ndata: {evt['data']}\n\n"
                    if evt["type"] == "done":
                        return
            else:
                now = asyncio.get_event_loop().time()
                # 타임아웃 체크
                if now - idle_start > IDLE_TIMEOUT:
                    log.warning(f"events idle timeout — session {session_id}")
                    yield _sse("error", {"message": "응답 대기 시간 초과"})
                    yield _sse("done", {"reply": "", "actions": []})
                    return
                # keepalive (3초마다)
                if now - last_keepalive > 3.0:
                    last_keepalive = now
                    yield ": keepalive\n\n"

            # 짧은 polling (0.1초) — notifier 대신 안정적
            await asyncio.sleep(0.1)

    return StreamingResponse(sse_stream(), media_type="text/event-stream")


# ── 레거시 chat/stream (하위 호환) ──

@router.post("/chat/stream")
async def chat_stream(
    req: ChatRequest,
    request: Request,
    x_user_id: str = Header(default="batman", alias="X-User-Id"),
):
    """레거시: 메시지 전송 + SSE 스트림 (chat/send + events 조합)."""
    gojira_sid = req.session_id or str(uuid.uuid4())
    oc_sid = await _ensure_opencode_session(gojira_sid, x_user_id)

    agent_sessions().update_one(
        {"_id": gojira_sid},
        {
            "$push": {"messages": {"role": "user", "content": req.message, "timestamp": _now()}},
            "$set": {"updatedAt": _now()},
        },
    )

    await eb.ensure_subscriber(oc_sid, gojira_sid)
    await oc.send_message_async(oc_sid, req.message)

    async def sse_stream():
        cursor = 0
        IDLE_TIMEOUT = 180.0
        idle_start = asyncio.get_event_loop().time()
        last_keepalive = idle_start

        while True:
            events = eb.read_since(gojira_sid, cursor)
            if events:
                idle_start = asyncio.get_event_loop().time()
                last_keepalive = idle_start
                for evt in events:
                    cursor = evt["id"]
                    yield f"id: {evt['id']}\nevent: {evt['type']}\ndata: {evt['data']}\n\n"
                    if evt["type"] == "done":
                        return
            else:
                now = asyncio.get_event_loop().time()
                if now - idle_start > IDLE_TIMEOUT:
                    yield _sse("error", {"message": f"{IDLE_TIMEOUT}초 동안 응답이 없어 작업을 중단했습니다."})
                    yield _sse("done", {"reply": "", "actions": []})
                    return
                if now - last_keepalive > 3.0:
                    last_keepalive = now
                    yield ": keepalive\n\n"

            await asyncio.sleep(0.1)

    return StreamingResponse(sse_stream(), media_type="text/event-stream")


# ── 세션 관리 ──

@router.post("/session/{session_id}/terminal")
async def open_terminal(session_id: str):
    """macOS Terminal.app 에서 해당 세션의 OpenCode attach 실행."""
    doc = agent_sessions().find_one({"_id": session_id}) or {}
    oc_sid = doc.get("openCodeSessionId")
    if not oc_sid:
        return {"ok": False, "reason": "openCodeSessionId 없음"}
    base_url = oc._base_url()
    cmd = f'opencode attach {base_url} -s {oc_sid}'
    import subprocess, tempfile, os
    sh = tempfile.NamedTemporaryFile(mode='w', suffix='.sh', delete=False, prefix='gojira-term-')
    sh.write(f'#!/bin/bash\n')
    sh.write(f'echo -e "\\033[1;36m[GoJIRA] OpenCode 세션 연결 — Ctrl+C 로 안전하게 종료\\033[0m\\n"\n')
    sh.write(f'{cmd}\n')
    sh.write(f'echo -e "\\n\\033[1;33m세션 연결 종료. 창을 닫아도 안전합니다.\\033[0m"\n')
    sh.write(f'sleep 2; exit\n')
    sh.close()
    os.chmod(sh.name, 0o755)
    subprocess.Popen(["open", "-a", "Terminal", sh.name])
    return {"ok": True, "command": cmd, "openCodeSessionId": oc_sid}


@router.delete("/session/{session_id}")
async def delete_session(session_id: str, x_user_id: str = Header(default="batman", alias="X-User-Id")):
    agent_sessions().delete_one({"_id": session_id})
    eb.clear(session_id)
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
    # 첫 user 메시지 content 만 title 용도로 꺼내온다 (전체 messages 를 로드하면 페이로드 비대).
    docs = list(agent_sessions().find(
        {"userId": x_user_id},
        {"title": 1, "createdAt": 1, "updatedAt": 1, "messages": {"$slice": 5}},
    ).sort("updatedAt", -1).limit(50))

    def _pick_title(d):
        if d.get("title"):
            return d["title"]
        for m in d.get("messages") or []:
            if m.get("role") == "user" and m.get("content"):
                return m["content"][:40]
        return "New session"

    return [
        {
            "sessionId": d["_id"],
            "title": _pick_title(d),
            "createdAt": d.get("createdAt"),
            "updatedAt": d.get("updatedAt"),
        }
        for d in docs
    ]


@router.get("/session/{session_id}")
async def get_session(session_id: str):
    doc = agent_sessions().find_one({"_id": session_id})
    if not doc:
        return {"sessionId": session_id, "messages": [], "active": False}
    oc_sid = doc.get("openCodeSessionId")
    active = False
    if oc_sid:
        active = await oc.session_exists(oc_sid)
    return {
        "sessionId": doc["_id"],
        "messages": doc.get("messages", []),
        "active": active,
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
