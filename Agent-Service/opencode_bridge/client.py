"""OpenCode 서버 HTTP/SSE 클라이언트."""

import os
import json
import base64
from typing import AsyncIterator, Optional

import httpx


def _creds() -> tuple[str, str]:
    user = os.environ.get("OPENCODE_SERVER_USERNAME", "opencode")
    pw = os.environ.get("OPENCODE_SERVER_PASSWORD", "")
    return user, pw


def _base_url() -> str:
    return os.environ.get("OPENCODE_BASE_URL", "http://127.0.0.1:4096")


def _auth_header() -> dict[str, str]:
    u, p = _creds()
    if not p:
        return {}
    token = base64.b64encode(f"{u}:{p}".encode()).decode()
    return {"Authorization": f"Basic {token}"}


async def create_session(title: Optional[str] = None) -> str:
    async with httpx.AsyncClient(timeout=30.0) as c:
        r = await c.post(
            f"{_base_url()}/session",
            json={"title": title or "GoJIRA session"},
            headers={**_auth_header(), "Content-Type": "application/json"},
        )
        r.raise_for_status()
        return r.json()["id"]


async def send_message_async(session_id: str, text: str, agent: str = "gojira") -> None:
    """메시지 전송 (비동기 시작, 이벤트는 /event 에서 수신)."""
    async with httpx.AsyncClient(timeout=60.0) as c:
        r = await c.post(
            f"{_base_url()}/session/{session_id}/prompt_async",
            json={"parts": [{"type": "text", "text": text}], "agent": agent},
            headers={**_auth_header(), "Content-Type": "application/json"},
        )
        r.raise_for_status()


async def inject_context(session_id: str, text: str, agent: str = "gojira") -> None:
    """noReply 로 컨텍스트만 주입 (워크플로우 stage 등)."""
    async with httpx.AsyncClient(timeout=30.0) as c:
        r = await c.post(
            f"{_base_url()}/session/{session_id}/prompt_async",
            json={"noReply": True, "parts": [{"type": "text", "text": text}], "agent": agent},
            headers={**_auth_header(), "Content-Type": "application/json"},
        )
        r.raise_for_status()


async def abort_session(session_id: str) -> bool:
    """진행 중인 OpenCode 세션을 강제 중단."""
    async with httpx.AsyncClient(timeout=10.0) as c:
        r = await c.post(
            f"{_base_url()}/session/{session_id}/abort",
            headers=_auth_header(),
        )
        return r.status_code < 400


async def stream_events(session_id: str) -> AsyncIterator[dict]:
    """OpenCode /event SSE 를 파싱해 이 세션 의 이벤트만 yield."""
    async with httpx.AsyncClient(timeout=None) as c:
        async with c.stream("GET", f"{_base_url()}/event", headers=_auth_header()) as r:
            r.raise_for_status()
            async for line in r.aiter_lines():
                if not line or not line.startswith("data: "):
                    continue
                try:
                    evt = json.loads(line[6:])
                except Exception:
                    continue
                sid = evt.get("properties", {}).get("sessionID")
                if sid and sid != session_id:
                    continue
                yield evt
