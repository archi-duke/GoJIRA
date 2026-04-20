"""세션별 SSE 이벤트 버퍼 — 재연결 시 놓친 이벤트 재전송 지원."""

import threading
import time
from collections import defaultdict

_MAX_EVENTS = 500
_TTL = 600  # 10분 후 세션 버퍼 자동 삭제

_lock = threading.Lock()
_buffers: dict[str, list[dict]] = defaultdict(list)
_counters: dict[str, int] = defaultdict(int)
_last_access: dict[str, float] = {}


def push(session_id: str, event_type: str, sse_str: str) -> int:
    """이벤트 추가, 할당된 event_id 반환."""
    with _lock:
        _counters[session_id] += 1
        eid = _counters[session_id]
        _buffers[session_id].append({
            "id": eid,
            "type": event_type,
            "sse": sse_str,
        })
        if len(_buffers[session_id]) > _MAX_EVENTS:
            _buffers[session_id] = _buffers[session_id][-_MAX_EVENTS:]
        _last_access[session_id] = time.time()
        return eid


def replay(session_id: str, last_event_id: int) -> list[dict]:
    """last_event_id 이후 이벤트 목록 반환."""
    with _lock:
        _last_access[session_id] = time.time()
        buf = _buffers.get(session_id, [])
        return [e for e in buf if e["id"] > last_event_id]


def clear(session_id: str):
    """세션 버퍼 삭제."""
    with _lock:
        _buffers.pop(session_id, None)
        _counters.pop(session_id, None)
        _last_access.pop(session_id, None)


def cleanup_expired():
    """만료된 세션 버퍼 정리."""
    now = time.time()
    with _lock:
        expired = [sid for sid, t in _last_access.items() if now - t > _TTL]
        for sid in expired:
            _buffers.pop(sid, None)
            _counters.pop(sid, None)
            _last_access.pop(sid, None)


def sse_with_id(event_id: int, event_type: str, data_json: str) -> str:
    """이벤트 ID가 포함된 SSE 문자열 생성."""
    return f"id: {event_id}\nevent: {event_type}\ndata: {data_json}\n\n"
