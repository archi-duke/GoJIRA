"""세션별 SSE 이벤트 버퍼 + 백그라운드 구독자.

OpenCode /event SSE를 상시 구독하여 세션별로 이벤트를 버퍼링.
브라우저 연결/끊김과 무관하게 동작.
"""

import asyncio
import json
import logging
import time
import threading
from collections import defaultdict
from typing import Optional

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
_watchdog_tasks: dict[str, asyncio.Task] = {}


def _start_watchdog(oc_session_id: str, gojira_session_id: str):
    key = gojira_session_id
    wt = _watchdog_tasks.get(key)
    if wt and not wt.done():
        return
    wt = asyncio.create_task(_watchdog_loop(oc_session_id, gojira_session_id))
    _watchdog_tasks[key] = wt


async def _watchdog_loop(oc_session_id: str, gojira_session_id: str):
    """주기적으로 OpenCode 세션 상태를 확인하여
    - finish 이 설정됐으면 synthetic done 발행 후 구독자 종료
    - finish=None 인데 이벤트가 IDLE_REFRESH_SEC 이상 stale 이면 /event 스트림 hung 으로 간주, 구독자 강제 리셋
    """
    from . import client as oc
    CHECK_INTERVAL = 1.0
    IDLE_REFRESH_SEC = 10.0  # 이벤트 없이 이 시간 지나면 subscriber 리셋
    last_id = get_latest_id(gojira_session_id)
    idle_since = time.time()
    try:
        while True:
            await asyncio.sleep(CHECK_INTERVAL)
            sub = _subscriber_tasks.get(gojira_session_id)
            if not sub or sub.done():
                return
            current_id = get_latest_id(gojira_session_id)
            if current_id != last_id:
                last_id = current_id
                idle_since = time.time()
                continue
            idle_elapsed = time.time() - idle_since
            # idle — OpenCode 세션 상태 확인
            try:
                msgs = await oc.get_session_messages(oc_session_id)
            except Exception as e:
                log.warning(f"watchdog oc 조회 실패 {gojira_session_id}: {e}")
                continue
            finished = False
            for m in reversed(msgs):
                info = m.get("info", {}) if isinstance(m, dict) else {}
                if info.get("role") == "assistant":
                    fin = info.get("finish")
                    if fin and str(fin).strip():
                        finished = True
                    break
            if finished:
                log.info(f"watchdog: OpenCode finish 감지 — synthetic done 발행 {gojira_session_id}")
                push(gojira_session_id, "done", '{"reply":"","actions":[]}')
                if sub and not sub.done():
                    sub.cancel()
                return
            # finish=None 인데 이벤트 IDLE_REFRESH_SEC 이상 멎어있으면 /event 스트림이 hung 이라고 보고 subscriber 강제 리셋
            if idle_elapsed >= IDLE_REFRESH_SEC:
                log.warning(f"watchdog: subscriber idle {idle_elapsed:.1f}s (finish=None) — 강제 리셋 {gojira_session_id}")
                # 현재 태스크가 구독자 task 라서 cancel 만 걸고 탈출. reset 은 다음 /events 재접속 시 ensure_subscriber 가 처리
                # 대신 여기서 직접 새 구독자 task 생성
                try:
                    sub.cancel()
                except Exception:
                    pass
                _subscriber_tasks.pop(gojira_session_id, None)
                new_task = asyncio.create_task(_subscribe_loop(oc_session_id, gojira_session_id))
                _subscriber_tasks[gojira_session_id] = new_task
                # 새 구독자에 watchdog 도 다시 붙여야 하므로 — 이 watchdog 은 여기서 종료하고 새로 시작
                _start_watchdog(oc_session_id, gojira_session_id)
                return
    except asyncio.CancelledError:
        return


async def ensure_subscriber(oc_session_id: str, gojira_session_id: str):
    """해당 세션의 백그라운드 구독자가 없으면 시작."""
    key = gojira_session_id
    task = _subscriber_tasks.get(key)
    if task and not task.done():
        _start_watchdog(oc_session_id, gojira_session_id)
        return
    task = asyncio.create_task(_subscribe_loop(oc_session_id, gojira_session_id))
    _subscriber_tasks[key] = task
    _start_watchdog(oc_session_id, gojira_session_id)
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
    wt = _watchdog_tasks.get(key)
    if wt and not wt.done():
        wt.cancel()
    _subscriber_tasks.pop(key, None)
    _watchdog_tasks.pop(key, None)
    task = asyncio.create_task(_subscribe_loop(oc_session_id, gojira_session_id))
    _subscriber_tasks[key] = task
    _start_watchdog(oc_session_id, gojira_session_id)
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
            if evt_type == "done":
                log.info(f"백그라운드 구독 완료 (done): {gojira_session_id}")
                # Mongo에 이번 턴의 process + assistant만 저장
                await _save_to_mongo(gojira_session_id, evt_data, turn_start_id)
                break
    except asyncio.CancelledError:
        log.info(f"백그라운드 구독 취소: {gojira_session_id}")
    except Exception as e:
        log.warning(f"백그라운드 구독 오류: {gojira_session_id} — {e}")
    finally:
        _subscriber_tasks.pop(gojira_session_id, None)


async def _save_to_mongo(gojira_session_id: str, done_data_json: str, after_id: int = 0):
    """done 시점에 이번 턴의 process steps + assistant reply를 Mongo에 저장."""
    from core.database import agent_sessions
    from datetime import datetime, timezone

    now = datetime.now(timezone.utc)

    # 버퍼에서 이번 턴의 이벤트만 수집 (연속 content는 병합)
    events = read_since(gojira_session_id, after_id)
    process_steps = []
    for e in events:
        if e["type"] in ("reasoning", "tool_call", "tool_result", "content"):
            try:
                step = {"type": e["type"], "data": json.loads(e["data"])}
                if e["type"] == "content" and process_steps and process_steps[-1]["type"] == "content":
                    prev_text = process_steps[-1]["data"].get("text", "")
                    step["data"]["text"] = prev_text + step["data"].get("text", "")
                    process_steps[-1] = step
                else:
                    process_steps.append(step)
            except Exception:
                pass

    # done에서 reply 추출
    reply = ""
    try:
        done_payload = json.loads(done_data_json)
        reply = done_payload.get("reply", "")
    except Exception:
        pass

    msgs_to_push = []
    if process_steps:
        msgs_to_push.append({"role": "process", "steps": process_steps, "timestamp": now})
    if reply:
        msgs_to_push.append({"role": "assistant", "content": reply, "timestamp": now})
    if msgs_to_push:
        agent_sessions().update_one(
            {"_id": gojira_session_id},
            {"$push": {"messages": {"$each": msgs_to_push}}, "$set": {"updatedAt": now}},
        )
