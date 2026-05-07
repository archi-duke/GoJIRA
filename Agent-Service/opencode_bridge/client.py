"""OpenCode 서버 HTTP/SSE 클라이언트."""

import json
import base64
from typing import AsyncIterator, Optional

import httpx

from config import settings


def _creds() -> tuple[str, str]:
    return settings.opencode_username, settings.opencode_password


def _base_url() -> str:
    return settings.opencode_url


def _auth_header() -> dict[str, str]:
    u, p = _creds()
    if not p:
        return {}
    token = base64.b64encode(f"{u}:{p}".encode()).decode()
    return {"Authorization": f"Basic {token}"}


async def session_exists(session_id: str) -> bool:
    """OpenCode 에 해당 세션이 존재하는지 확인."""
    try:
        async with httpx.AsyncClient(timeout=5.0) as c:
            r = await c.get(
                f"{_base_url()}/session/{session_id}",
                headers=_auth_header(),
            )
            return r.status_code == 200
    except Exception:
        return False


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
            json={"parts": [{"type": "text", "text": text}], **({"agent": agent} if agent else {})},
            headers={**_auth_header(), "Content-Type": "application/json"},
        )
        r.raise_for_status()


async def inject_context(session_id: str, text: str, agent: str = "gojira") -> None:
    """noReply 로 컨텍스트만 주입 (워크플로우 stage 등)."""
    async with httpx.AsyncClient(timeout=30.0) as c:
        r = await c.post(
            f"{_base_url()}/session/{session_id}/prompt_async",
            json={"noReply": True, "parts": [{"type": "text", "text": text}], **({"agent": agent} if agent else {})},
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


async def get_session_messages(session_id: str) -> list:
    """세션의 현재 메시지 목록 조회 (폴링용)."""
    try:
        async with httpx.AsyncClient(timeout=5.0) as c:
            r = await c.get(
                f"{_base_url()}/session/{session_id}/message",
                headers=_auth_header(),
            )
            if r.status_code == 200:
                return r.json()
    except Exception:
        pass
    return []


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
