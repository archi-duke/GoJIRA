"""스킬 실행 중 진행 상황 알림 — contextvar 기반 큐.

스트리밍 라우터가 큐를 바인딩한 뒤 execute_skill을 호출하면, 스킬 내부에서
`await emit_progress(...)`로 보낸 메시지를 동시 drain하여 SSE로 전달한다.
큐가 바인딩되지 않은 호출(REST/배치)에서는 no-op.
"""

import asyncio
from contextvars import ContextVar
from typing import Optional

_queue_var: ContextVar[Optional[asyncio.Queue]] = ContextVar("progress_queue", default=None)


def bind_queue(queue: asyncio.Queue) -> "object":
    """현재 contextvar에 큐를 바인딩하고 token 반환 (reset_queue로 원복)."""
    return _queue_var.set(queue)


def reset_queue(token) -> None:
    _queue_var.reset(token)


async def emit_progress(stage: str, message: str, **extra) -> None:
    """진행 상황을 큐에 넣는다. 큐가 없으면 무시.

    Args:
        stage: 단계 식별자 (예: "vos_collect", "usecase_analyze")
        message: 사용자에게 보일 한 줄 요약
        **extra: 추가 페이로드 (created_count 등)
    """
    q = _queue_var.get()
    if q is None:
        return
    payload = {"stage": stage, "message": message}
    if extra:
        payload.update(extra)
    try:
        await q.put(payload)
    except Exception:
        pass
