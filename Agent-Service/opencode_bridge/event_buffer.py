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


async def ensure_subscriber(oc_session_id: str, gojira_session_id: str):
    """해당 세션의 백그라운드 구독자가 없으면 시작."""
    key = gojira_session_id
    task = _subscriber_tasks.get(key)
    if task and not task.done():
        return
    task = asyncio.create_task(_subscribe_loop(oc_session_id, gojira_session_id))
    _subscriber_tasks[key] = task
    log.info(f"백그라운드 구독 시작: {gojira_session_id} → {oc_session_id[:16]}")


async def reset_subscriber(oc_session_id: str, gojira_session_id: str):
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
    task = asyncio.create_task(_subscribe_loop(oc_session_id, gojira_session_id))
    _subscriber_tasks[key] = task
    log.info(f"백그라운드 구독 리셋: {gojira_session_id} → {oc_session_id[:16]}")


async def _subscribe_loop(oc_session_id: str, gojira_session_id: str):
    """OpenCode /event SSE를 구독하고 event_buffer에 누적."""
    from . import client as oc
    from .translator import translate

    # 이 턴의 시작 event ID 기록 (이전 턴 이벤트 제외용)
    turn_start_id = get_latest_id(gojira_session_id)

    # 재접속 대응: 현재 OpenCode 세션에 진행 중(finish=None)인 assistant 메시지가 있으면 그 id 로 초기화.
    # 동시에 해당 메시지의 기존 parts 에서 partID→type 매핑을 수집해 translator 에 전달
    # (재시작 subscriber 가 message.part.updated 를 놓쳐 delta 를 text 로 오분류하는 문제 방지).
    initial_msg_id = None
    initial_part_types: dict = {}
    try:
        msgs = await oc.get_session_messages(oc_session_id)
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

    saved = False
    log.info(f"백그라운드 구독 시작-loop: {gojira_session_id} initial_msg={initial_msg_id} initial_parts={len(initial_part_types)}")
    evt_counts: dict[str, int] = {}
    started_at = time.time()
    try:
        events_iter = oc.stream_events(oc_session_id)
        async for sse_str in translate(events_iter, gojira_session_id, initial_msg_id, initial_part_types):
            if sse_str.startswith(":"):
                continue
            # SSE 문자열에서 event type과 data 추출
            evt_type = None
            evt_data = "{}"
            for line in sse_str.split("\n"):
                if line.startswith("event: "):
                    evt_type = line[7:]
                elif line.startswith("data: "):
                    evt_data = line[6:]
            if evt_type:
                push(gojira_session_id, evt_type, evt_data)
                evt_counts[evt_type] = evt_counts.get(evt_type, 0) + 1
            if evt_type == "done":
                log.info(f"백그라운드 구독 완료 (done): {gojira_session_id} elapsed={time.time()-started_at:.1f}s counts={evt_counts}")
                # Mongo에 이번 턴의 process + assistant만 저장
                await _save_to_mongo(gojira_session_id, evt_data, turn_start_id)
                saved = True
                break
    except asyncio.CancelledError:
        log.info(f"백그라운드 구독 취소: {gojira_session_id} elapsed={time.time()-started_at:.1f}s counts={evt_counts}")
    except Exception as e:
        log.warning(f"백그라운드 구독 오류: {gojira_session_id} — {e} counts={evt_counts}")
    finally:
        # done 미수신으로 종료된 경우에도 버퍼에 쌓인 process 이벤트를 Mongo 에 반영.
        # 그렇지 않으면 세션 이력에 user 질문만 남아 세션 제목이 '질문 내용' 만 표시되는 문제 발생.
        if not saved:
            log.warning(f"백그라운드 구독 종료 (done 미수신): {gojira_session_id} elapsed={time.time()-started_at:.1f}s counts={evt_counts}")
            try:
                await _save_to_mongo(gojira_session_id, "{}", turn_start_id)
            except Exception as se:
                log.warning(f"구독 종료 시점 부분 저장 실패: {gojira_session_id} — {se}")
        _subscriber_tasks.pop(gojira_session_id, None)


async def _save_to_mongo(gojira_session_id: str, done_data_json: str = "{}", after_id: int = 0):
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

    try:
        oc_messages = await oc_client.get_session_messages(oc_sid)
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
