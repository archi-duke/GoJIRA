"""세션별 SSE 이벤트 버퍼 + 백그라운드 구독자.

OpenCode /event SSE를 상시 구독하여 세션별로 이벤트를 버퍼링.
브라우저 연결/끊김과 무관하게 동작.
"""

import asyncio
import logging
import time
import threading
from collections import defaultdict

log = logging.getLogger("agent.event_buffer")

_MAX_EVENTS = 500
_TTL = 600  # 10분 후 세션 버퍼 자동 삭제

_lock = threading.Lock()
_buffers: dict[str, list[dict]] = defaultdict(list)
_counters: dict[str, int] = defaultdict(int)
_last_access: dict[str, float] = {}
# 세션별 asyncio.Event — 새 이벤트 도착 시 대기 중인 reader를 깨움
_notifiers: dict[str, asyncio.Event] = {}


def push(session_id: str, event_type: str, data_json: str) -> int:
    """이벤트 추가, 할당된 event_id 반환. 대기 중인 reader에 알림."""
    with _lock:
        _counters[session_id] += 1
        eid = _counters[session_id]
        _buffers[session_id].append({
            "id": eid,
            "type": event_type,
            "data": data_json,
        })
        if len(_buffers[session_id]) > _MAX_EVENTS:
            _buffers[session_id] = _buffers[session_id][-_MAX_EVENTS:]
        _last_access[session_id] = time.time()
    # notifier는 lock 밖에서 set (asyncio thread-safe)
    notifier = _notifiers.get(session_id)
    if notifier:
        notifier.set()
    return eid


def get_latest_id(session_id: str) -> int:
    """현재 세션의 마지막 이벤트 ID."""
    with _lock:
        return _counters.get(session_id, 0)


def read_since(session_id: str, after_id: int) -> list[dict]:
    """after_id 이후 이벤트 목록 반환."""
    with _lock:
        _last_access[session_id] = time.time()
        buf = _buffers.get(session_id, [])
        return [e for e in buf if e["id"] > after_id]


def get_notifier(session_id: str) -> asyncio.Event:
    """세션의 notifier를 가져오거나 생성."""
    if session_id not in _notifiers:
        _notifiers[session_id] = asyncio.Event()
    return _notifiers[session_id]


def clear(session_id: str):
    with _lock:
        _buffers.pop(session_id, None)
        _counters.pop(session_id, None)
        _last_access.pop(session_id, None)
    _notifiers.pop(session_id, None)


def cleanup_expired():
    now = time.time()
    with _lock:
        expired = [sid for sid, t in _last_access.items() if now - t > _TTL]
        for sid in expired:
            _buffers.pop(sid, None)
            _counters.pop(sid, None)
            _last_access.pop(sid, None)
    for sid in expired:
        _notifiers.pop(sid, None)


# ── 백그라운드 구독자 ──

_subscriber_tasks: dict[str, asyncio.Task] = {}


async def ensure_subscriber(oc_session_id: str, gojira_session_id: str, user_id: str = ""):
    """해당 세션의 백그라운드 구독자가 없으면 시작.

    user_id 는 OpenCode 인스턴스 라우팅용. 빈 문자열이면 client._resolve_url 이 fallback URL 사용.
    """
    key = gojira_session_id
    task = _subscriber_tasks.get(key)
    if task and not task.done():
        return
    task = asyncio.create_task(_subscribe_loop(oc_session_id, gojira_session_id, user_id))
    _subscriber_tasks[key] = task
    log.info(f"백그라운드 구독 시작: {gojira_session_id} → {oc_session_id[:16]} user={user_id or '(fallback)'}")


async def reset_subscriber(oc_session_id: str, gojira_session_id: str, user_id: str = ""):
    """기존 구독자 취소 후 새로 시작. 턴 경계에서 깨끗한 재구독 보장."""
    key = gojira_session_id
    task = _subscriber_tasks.get(key)
    if task and not task.done():
        task.cancel()
        try:
            await task
        except (asyncio.CancelledError, Exception):
            pass
    _subscriber_tasks.pop(key, None)
    task = asyncio.create_task(_subscribe_loop(oc_session_id, gojira_session_id, user_id))
    _subscriber_tasks[key] = task
    log.info(f"백그라운드 구독 리셋: {gojira_session_id} → {oc_session_id[:16]} user={user_id or '(fallback)'}")


async def _subscribe_loop(oc_session_id: str, gojira_session_id: str, user_id: str = ""):
    """OpenCode /event SSE를 구독하고 event_buffer 에 누적.

    persistent: turn (done) 끝나도 종료하지 않고 다음 turn 진입을 대기.
    OpenCode TUI 입력이나 후속 chat-send 의 이벤트를 끊김 없이 받아내기 위함.
    cancel/exception 시에만 종료.
    """
    from . import client as oc
    from .translator import translate

    log.info(f"백그라운드 구독 시작: {gojira_session_id} → {oc_session_id[:16]}")

    try:
        # /event SSE 연결은 세션 전체 수명 동안 유지 (translate 호출 사이 stream 상태 보존)
        events_iter = oc.stream_events(oc_session_id, user_id)

        while True:  # turn loop — done 마다 새 translate() 진입
            turn_start_id = get_latest_id(gojira_session_id)

            # 진행 중 assistant 가 있으면 그 id 로 초기화 (재시작 시 part 분류 보장)
            initial_msg_id = None
            initial_part_types: dict = {}
            try:
                msgs = await oc.get_session_messages(oc_session_id, user_id)
                for m in reversed(msgs):
                    info = m.get("info", {}) if isinstance(m, dict) else {}
                    if info.get("role") == "assistant" and info.get("finish") in (None, ""):
                        initial_msg_id = info.get("id")
                        for p in (m.get("parts") or []):
                            pid = p.get("id")
                            ptype = p.get("type")
                            if pid and ptype:
                                initial_part_types[pid] = ptype
                        break
            except Exception:
                pass

            log.info(f"턴 시작: {gojira_session_id} initial_msg={initial_msg_id} initial_parts={len(initial_part_types)}")
            saved = False
            evt_counts: dict[str, int] = {}
            started_at = time.time()

            async for sse_str in translate(events_iter, gojira_session_id, initial_msg_id, initial_part_types):
                if sse_str.startswith(":"):
                    continue
                evt_type = None
                evt_data = "{}"
                for line in sse_str.split("\n"):
                    if line.startswith("event: "):
                        evt_type = line[7:]
                    elif line.startswith("data: "):
                        evt_data = line[6:]
                if not evt_type:
                    continue
                # done 은 _save_to_mongo 를 먼저 끝낸 뒤에 push — 프론트의 loadSession 이 stale Mongo 를 읽는 race 방지
                if evt_type == "done":
                    log.info(f"턴 완료 (done): {gojira_session_id} elapsed={time.time()-started_at:.1f}s counts={evt_counts}")
                    await _save_to_mongo(gojira_session_id, evt_data, turn_start_id, user_id)
                    push(gojira_session_id, evt_type, evt_data)
                    evt_counts[evt_type] = evt_counts.get(evt_type, 0) + 1
                    saved = True
                    break
                push(gojira_session_id, evt_type, evt_data)
                evt_counts[evt_type] = evt_counts.get(evt_type, 0) + 1

            # done 못 받고 translate() 가 빠져나온 케이스 (이론상 거의 없음)
            if not saved:
                log.warning(f"턴 비정상 종료 (done 미수신): {gojira_session_id} elapsed={time.time()-started_at:.1f}s counts={evt_counts}")
                try:
                    await _save_to_mongo(gojira_session_id, "{}", turn_start_id, user_id)
                except Exception as se:
                    log.warning(f"부분 저장 실패: {gojira_session_id} — {se}")

            # 다음 turn 대기 (events_iter 는 그대로 유지 — TUI / chat-send 이벤트가 들어오면 translate 가 다시 진입)
            log.info(f"다음 턴 대기: {gojira_session_id}")

    except asyncio.CancelledError:
        log.info(f"백그라운드 구독 취소: {gojira_session_id}")
    except Exception as e:
        log.warning(f"백그라운드 구독 오류: {gojira_session_id} — {e}")
    finally:
        _subscriber_tasks.pop(gojira_session_id, None)


async def _save_to_mongo(gojira_session_id: str, done_data_json: str = "{}", after_id: int = 0, user_id: str = ""):
    """OpenCode 가 관리하는 메시지(info + parts) 를 그대로 Mongo 에 저장.

    과거 방식(translator 가 조립한 process.steps)은 저장 시점 translator 버그가 영구 동결되는 문제가 있었다.
    이제 OpenCode 원본 parts 를 저장하고, 렌더/복구는 클라이언트 partsToSegments 에서 수행.
    이미 저장된 assistant 메시지 (ocMessageId) 는 중복 push 하지 않는다.
    """
    from core.database import agent_sessions
    from datetime import datetime, timezone
    from . import client as oc_client

    doc = agent_sessions().find_one({"_id": gojira_session_id}, {"messages": 1, "openCodeSessionId": 1}) or {}
    oc_sid = doc.get("openCodeSessionId")
    if not oc_sid:
        return

    saved_ids = set()
    # 채팅창에서 직접 push 한 user 메시지(content 기반 dedup 용) 누적 카운트
    pending_user_contents: dict[str, int] = {}
    for m in doc.get("messages") or []:
        mid = m.get("ocMessageId")
        if mid:
            saved_ids.add(mid)
        elif m.get("role") == "user" and m.get("content"):
            c = m["content"]
            pending_user_contents[c] = pending_user_contents.get(c, 0) + 1

    # user_id 누락 시 agent_sessions 에서 복구 — _save_to_mongo 가 단독 호출되는 경로 대응
    if not user_id:
        user_id = doc.get("userId", "") or ""

    try:
        oc_messages = await oc_client.get_session_messages(oc_sid, user_id)
    except Exception as e:
        log.warning(f"save: OC 메시지 조회 실패 {gojira_session_id}: {e}")
        return

    def _ts(info):
        t = (info or {}).get("time") or {}
        created = t.get("created")
        if created:
            try:
                return datetime.fromtimestamp(created / 1000, tz=timezone.utc)
            except Exception:
                pass
        return datetime.now(timezone.utc)

    def _user_text(m: dict) -> str:
        return "".join(
            (p.get("text") or "")
            for p in (m.get("parts") or [])
            if p.get("type") == "text"
        )

    now = datetime.now(timezone.utc)
    msgs_to_push = []
    for m in oc_messages:
        if not isinstance(m, dict):
            continue
        info = m.get("info") or {}
        role = info.get("role")
        mid = info.get("id")
        if not mid or mid in saved_ids:
            continue

        if role == "user":
            text = _user_text(m)
            # 이미 채팅창에서 push 된 동일 content 가 있으면 OC 측 저장은 건너뜀 (중복 방지)
            if text and pending_user_contents.get(text, 0) > 0:
                pending_user_contents[text] -= 1
                continue
            msgs_to_push.append({
                "role": "user",
                "ocMessageId": mid,
                "content": text,
                "timestamp": _ts(info),
            })
        elif role == "assistant":
            msgs_to_push.append({
                "role": "assistant",
                "ocMessageId": mid,
                "info": info,
                "parts": m.get("parts") or [],
                "timestamp": _ts(info),
            })

    if msgs_to_push:
        agent_sessions().update_one(
            {"_id": gojira_session_id},
            {"$push": {"messages": {"$each": msgs_to_push}}, "$set": {"updatedAt": now}},
        )
