"""OpenCode 서버 HTTP/SSE 클라이언트.

라우팅: user_id 별로 Mongo opencodeInstances 에서 활성 인스턴스 host:port 를 조회한다.
미등록 사용자는 settings.opencode_url 로 자동 fallback (로그) — 단계 A → 단계 B 전환의 안전한
점진적 마이그레이션을 위해. fallback 빈도가 높다면 운영자가 gojira-user-add.sh 로 등록을
보완해야 한다.
"""

import json
import base64
import logging
from typing import AsyncIterator, Optional

import httpx

from config import settings
from core.database import find_active_opencode_instance

log = logging.getLogger("agent.opencode_bridge.client")


def _creds() -> tuple[str, str]:
    return settings.opencode_username, settings.opencode_password


# user_id → resolved URL 캐시 — Mongo round-trip 비용 절감.
# 운영자가 사용자 등록/제거 후에는 Agent-Service 재시작 또는 캐시 invalidate 필요.
# (단계 B 의 헬스체크 7.6 이 status=down 표시할 때 함께 invalidate 검토)
_url_cache: dict[str, str] = {}


def _resolve_url(user_id: str) -> str:
    """user_id 의 활성 OpenCode 인스턴스 URL. 미등록 시 settings.opencode_url 로 fallback.

    빈 user_id 는 fallback 만 반환 (시스템 호출 호환).
    """
    if not user_id:
        return settings.opencode_url

    cached = _url_cache.get(user_id)
    if cached:
        return cached

    inst = find_active_opencode_instance(user_id)
    if inst:
        host = inst.get("host") or "localhost"
        port = inst.get("port")
        if port:
            url = f"http://{host}:{port}"
            _url_cache[user_id] = url
            return url
        log.warning("opencodeInstances 의 user=%s 항목에 port 누락 — fallback 사용", user_id)
    else:
        log.warning("opencodeInstances 미등록 user=%s — settings.opencode_url 로 fallback", user_id)

    return settings.opencode_url


def invalidate_url_cache(user_id: Optional[str] = None) -> None:
    """헬스체크가 status=down 으로 마킹했거나 사용자 등록이 갱신됐을 때 호출."""
    if user_id is None:
        _url_cache.clear()
    else:
        _url_cache.pop(user_id, None)


def _base_url() -> str:
    """레거시 호출 경로용 — user_id 가 없는 시스템 호출에서만 사용."""
    return settings.opencode_url


def _auth_header() -> dict[str, str]:
    u, p = _creds()
    if not p:
        return {}
    token = base64.b64encode(f"{u}:{p}".encode()).decode()
    return {"Authorization": f"Basic {token}"}


async def session_exists(session_id: str, user_id: str = "") -> bool:
    """OpenCode 에 해당 세션이 존재하는지 확인."""
    try:
        async with httpx.AsyncClient(timeout=5.0) as c:
            r = await c.get(
                f"{_resolve_url(user_id)}/session/{session_id}",
                headers=_auth_header(),
            )
            return r.status_code == 200
    except Exception:
        return False


async def create_session(user_id: str, title: Optional[str] = None) -> str:
    async with httpx.AsyncClient(timeout=30.0) as c:
        r = await c.post(
            f"{_resolve_url(user_id)}/session",
            json={"title": title or "GoJIRA session"},
            headers={**_auth_header(), "Content-Type": "application/json"},
        )
        r.raise_for_status()
        return r.json()["id"]


async def send_message_async(session_id: str, text: str, user_id: str = "", agent: str = "gojira") -> None:
    """메시지 전송 (비동기 시작, 이벤트는 /event 에서 수신)."""
    async with httpx.AsyncClient(timeout=60.0) as c:
        r = await c.post(
            f"{_resolve_url(user_id)}/session/{session_id}/prompt_async",
            json={"parts": [{"type": "text", "text": text}], **({"agent": agent} if agent else {})},
            headers={**_auth_header(), "Content-Type": "application/json"},
        )
        r.raise_for_status()


async def inject_context(session_id: str, text: str, user_id: str = "", agent: str = "gojira") -> None:
    """noReply 로 컨텍스트만 주입 (워크플로우 stage 등)."""
    async with httpx.AsyncClient(timeout=30.0) as c:
        r = await c.post(
            f"{_resolve_url(user_id)}/session/{session_id}/prompt_async",
            json={"noReply": True, "parts": [{"type": "text", "text": text}], **({"agent": agent} if agent else {})},
            headers={**_auth_header(), "Content-Type": "application/json"},
        )
        r.raise_for_status()


async def abort_session(session_id: str, user_id: str = "") -> bool:
    """진행 중인 OpenCode 세션을 강제 중단."""
    async with httpx.AsyncClient(timeout=10.0) as c:
        r = await c.post(
            f"{_resolve_url(user_id)}/session/{session_id}/abort",
            headers=_auth_header(),
        )
        return r.status_code < 400


async def get_session_messages(session_id: str, user_id: str = "") -> list:
    """세션의 현재 메시지 목록 조회 (폴링용)."""
    try:
        async with httpx.AsyncClient(timeout=5.0) as c:
            r = await c.get(
                f"{_resolve_url(user_id)}/session/{session_id}/message",
                headers=_auth_header(),
            )
            if r.status_code == 200:
                return r.json()
    except Exception:
        pass
    return []


async def stream_events(session_id: str, user_id: str = "") -> AsyncIterator[dict]:
    """OpenCode /event SSE 를 파싱해 이 세션 의 이벤트만 yield."""
    async with httpx.AsyncClient(timeout=None) as c:
        async with c.stream("GET", f"{_resolve_url(user_id)}/event", headers=_auth_header()) as r:
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
