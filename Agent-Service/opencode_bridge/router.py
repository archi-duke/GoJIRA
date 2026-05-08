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

    # persistent subscriber: 이미 살아있으면 그대로, 없으면 시작.
    # 버퍼는 비우지 않음 — 프론트의 lastEventId 기반 재연결 호환성 + turn 사이 이벤트 보존.
    await eb.ensure_subscriber(oc_sid, gojira_sid)

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

    # EventSource API 가 custom header 설정을 지원 안 해서 클라이언트가 수동 재연결할 때
    # 쿼리 파라미터로도 last-event-id 를 받는다 (header 우선).
    _hdr = request.headers.get("Last-Event-ID", "") or request.query_params.get("lastEventId", "") or "0"
    try:
        last_event_id = int(_hdr)
    except ValueError:
        last_event_id = 0

    # 재연결 시 백그라운드 구독자가 없으면 시작
    await eb.ensure_subscriber(oc_sid, session_id)

    async def sse_stream():
        # 서버 측 IDLE 타임아웃 없음 — 에이전트가 얼마나 오래 걸려도 대기.
        # done 이벤트로 stream 끝나지 않음 — turn 사이 TUI 입력이나 후속 chat-send 이벤트를 끊김 없이 전달.
        # 클라이언트가 브라우저 탭 닫으면 EventSource 가 자연 종료되어 이 루프도 cancel 된다.
        cursor = last_event_id
        last_keepalive = asyncio.get_event_loop().time()

        while True:
            events = eb.read_since(session_id, cursor)
            if events:
                last_keepalive = asyncio.get_event_loop().time()
                for evt in events:
                    cursor = evt["id"]
                    yield f"id: {evt['id']}\nevent: {evt['type']}\ndata: {evt['data']}\n\n"
            else:
                now = asyncio.get_event_loop().time()
                if now - last_keepalive > 3.0:
                    last_keepalive = now
                    yield ": keepalive\n\n"

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
        last_keepalive = asyncio.get_event_loop().time()

        while True:
            events = eb.read_since(gojira_sid, cursor)
            if events:
                last_keepalive = asyncio.get_event_loop().time()
                for evt in events:
                    cursor = evt["id"]
                    yield f"id: {evt['id']}\nevent: {evt['type']}\ndata: {evt['data']}\n\n"
                    if evt["type"] == "done":
                        return
            else:
                now = asyncio.get_event_loop().time()
                if now - last_keepalive > 3.0:
                    last_keepalive = now
                    yield ": keepalive\n\n"

            await asyncio.sleep(0.1)

    return StreamingResponse(sse_stream(), media_type="text/event-stream")


# ── 세션 관리 ──

@router.post("/session/{session_id}/terminal")
async def open_terminal(session_id: str):
    """OS 별 터미널에서 해당 세션의 OpenCode attach 실행.

    - macOS  : Terminal.app + bash 스크립트
    - Windows: PowerShell 새 콘솔 창
    - 기타   : 미지원 (사용자가 직접 명령 실행 — 응답에 command 포함)
    """
    import sys
    import subprocess

    doc = agent_sessions().find_one({"_id": session_id}) or {}
    oc_sid = doc.get("openCodeSessionId")
    if not oc_sid:
        return {"ok": False, "reason": "openCodeSessionId 없음"}
    base_url = oc._base_url()
    cmd = f"opencode attach {base_url} -s {oc_sid}"

    if sys.platform == "darwin":
        import tempfile, os
        sh = tempfile.NamedTemporaryFile(mode="w", suffix=".sh", delete=False, prefix="gojira-term-")
        sh.write("#!/bin/bash\n")
        sh.write('echo -e "\\033[1;36m[GoJIRA] OpenCode 세션 연결 — Ctrl+C 로 안전하게 종료\\033[0m\\n"\n')
        sh.write(f"{cmd}\n")
        sh.write('echo -e "\\n\\033[1;33m세션 연결 종료. 창을 닫아도 안전합니다.\\033[0m"\n')
        sh.write("sleep 2; exit\n")
        sh.close()
        os.chmod(sh.name, 0o755)
        subprocess.Popen(["open", "-a", "Terminal", sh.name])
        return {"ok": True, "command": cmd, "openCodeSessionId": oc_sid, "platform": "darwin"}

    if sys.platform == "win32":
        # PowerShell 새 콘솔 창 (-NoExit 로 명령 종료 후에도 창 유지)
        ps_cmd = (
            'Write-Host "[GoJIRA] OpenCode 세션 연결 - Ctrl+C 로 안전하게 종료" -ForegroundColor Cyan; '
            'Write-Host ""; '
            f"{cmd}; "
            'Write-Host ""; '
            'Write-Host "세션 연결 종료. 창을 닫아도 안전합니다." -ForegroundColor Yellow'
        )
        subprocess.Popen(
            ["powershell", "-NoExit", "-Command", ps_cmd],
            creationflags=getattr(subprocess, "CREATE_NEW_CONSOLE", 0),
        )
        return {"ok": True, "command": cmd, "openCodeSessionId": oc_sid, "platform": "win32"}

    # Linux 등 — 자동 실행 미지원, 사용자가 직접 실행할 수 있도록 명령만 반환
    return {
        "ok": False,
        "reason": f"자동 터미널 실행 미지원 OS: {sys.platform} — 아래 command 를 직접 실행해주세요",
        "command": cmd,
        "openCodeSessionId": oc_sid,
        "platform": sys.platform,
    }


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


@router.get("/session/{session_id}/state")
async def get_session_state(session_id: str):
    """경량 상태 조회 — 클라이언트가 이벤트 침묵 후 폴링할 때 사용.

    returns:
      - finish: 마지막 assistant 메시지의 finish 값 (stop/end_turn/... / None=진행중)
      - hasPermissionPending: permission.asked 가 아직 resolve 안 된 상태 여부
      - lastActivityAt: 마지막 assistant part 생성 시간 (unix ms) — 정체 판단용
      - assistantMsgId: 현재(혹은 마지막) assistant message id
    """
    doc = agent_sessions().find_one({"_id": session_id}, {"openCodeSessionId": 1}) or {}
    oc_sid = doc.get("openCodeSessionId")
    if not oc_sid:
        return {"finish": None, "hasPermissionPending": False, "lastActivityAt": None, "assistantMsgId": None, "active": False}

    try:
        msgs = await oc.get_session_messages(oc_sid)
    except Exception:
        return {"finish": None, "hasPermissionPending": False, "lastActivityAt": None, "assistantMsgId": None, "active": False}

    # 마지막 assistant 메시지
    last_assistant = None
    for m in reversed(msgs or []):
        info = m.get("info") if isinstance(m, dict) else None
        if info and info.get("role") == "assistant":
            last_assistant = m
            break

    if not last_assistant:
        return {"finish": None, "hasPermissionPending": False, "lastActivityAt": None, "assistantMsgId": None, "active": True}

    info = last_assistant.get("info") or {}
    parts = last_assistant.get("parts") or []

    # 가장 최근 part 의 time.end (또는 start)
    last_activity = 0
    for p in parts:
        t = p.get("time") or {}
        for k in ("end", "start"):
            v = t.get(k)
            if isinstance(v, (int, float)) and v > last_activity:
                last_activity = v
        state = p.get("state") or {}
        st = state.get("time") or {}
        for k in ("end", "start"):
            v = st.get(k)
            if isinstance(v, (int, float)) and v > last_activity:
                last_activity = v

    # permission 대기: 가장 최근 tool part 중 status=pending/running 이며 permission 을 요구하는 케이스
    has_permission = False
    for p in parts:
        if p.get("type") == "tool":
            state = p.get("state") or {}
            if state.get("status") == "pending" or state.get("permission") == "asked":
                has_permission = True
                break

    return {
        "finish": info.get("finish"),
        "hasPermissionPending": has_permission,
        "lastActivityAt": last_activity or None,
        "assistantMsgId": info.get("id"),
        "active": True,
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
