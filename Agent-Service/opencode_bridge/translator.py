"""OpenCode 이벤트 → UI SSE 이벤트 번역기.

UI 가 기대하는 이벤트: session, thinking, tool_call, tool_result, content, done, error.
OpenCode /event 포맷:
  {type: "message.part.updated", properties: {part: {...}}}
  part.type ∈ {step-start, text, tool, step-finish}
  tool part: {tool, state:{status, input, output}}
"""

import json
import logging
import time
from typing import AsyncIterator, Optional

log = logging.getLogger("agent.translator")


def _sse(event: str, data: dict) -> str:
    return f"event: {event}\ndata: {json.dumps(data, ensure_ascii=False, default=str)}\n\n"

# SSE keepalive 주석 — UI 가 무시. 프록시/네트워크가 idle SSE 를 끊는 것을 방지.
_KEEPALIVE = ": keepalive\n\n"


async def translate(
    opencode_events: AsyncIterator[dict],
    gojira_session_id: str,
    initial_assistant_msg_id: Optional[str] = None,
    initial_part_types: Optional[dict] = None,
):
    """OpenCode 이벤트 스트림 → UI SSE 이벤트 문자열 yield.

    assistant message 완료(step-finish) 감지 시 done 을 yield 하고 종료.
    재접속 시 message.created 를 놓친 경우 initial_assistant_msg_id 로 현재 진행 중 메시지 추적.
    initial_part_types 는 subscriber 재시작 시 누락된 message.part.updated 를 보완하기 위해
    기존 part 의 partID→type 매핑을 선공급한다. 없으면 delta 가 reasoning 인지 text 인지
    구분할 수 없어 모두 response 로 출력되는 버그가 난다.
    """
    final_text_by_part: dict[str, str] = {}
    final_reasoning_by_part: dict[str, str] = {}
    text_part_order: list[str] = []
    actions: list[dict] = []
    # session 이벤트 먼저
    yield _sse("session", {"sessionId": gojira_session_id})

    turn = 0
    seen_tool_ids: set[str] = set()
    assistant_msg_id: Optional[str] = initial_assistant_msg_id  # assistant message ID 추적
    finished_msg_ids: set[str] = set()  # 이미 완료된 assistant message ID (재전송 차단)
    user_msg_ids: set[str] = set()      # role=user 로 생성된 메시지 ID — part/delta 필터용
    emitted_user_parts: set[str] = set()  # 이미 user_message 로 yield 한 user message text part id
    part_types: dict[str, str] = dict(initial_part_types or {})  # partID → part.type (reasoning/text 등) — delta 이벤트 분류용
    # session.idle 또는 finish=="stop" 일 때만 종료
    TERMINAL_FINISHES = {"stop", "end_turn", "length", "content-filter"}
    last_keepalive: float = 0.0

    # 권한 대기 자동 abort 는 제거됨. UI 가 /session/{id}/state 폴링으로 hasPermissionPending 을 확인하고
    # 사용자에게 표시 + 수동 취소 옵션을 제공한다.

    async for evt in opencode_events:
        t = evt.get("type")
        # 진단: message.updated 의 role+id+finish 자세히 기록 (정확한 이벤트 흐름 추적용)
        if t == "message.updated":
            _info = evt.get("properties", {}).get("info", {})
            log.info(f"[trans/{gojira_session_id[:8]}] message.updated role={_info.get('role')} mid={_info.get('id')} finish={_info.get('finish')}")
        elif t == "message.part.updated":
            _props = evt.get("properties", {})
            _part = _props.get("part", {})
            log.info(f"[trans/{gojira_session_id[:8]}] message.part.updated props_keys={list(_props.keys())} part_keys={list(_part.keys())} ptype={_part.get('type')}")
        elif t and t not in ("message.part.delta",):
            log.info(f"[trans/{gojira_session_id[:8]}] evt type={t}")

        if t == "message.part.delta":
            # OpenCode 가 part 내용을 char/토큰 단위로 흘려보내는 이벤트.
            # delta.field 는 어느 필드의 변경인지만 나타내므로, part 의 실제 타입은 part_types 에서 조회.
            props = evt.get("properties", {})
            # OpenCode 의 part.delta 도 part 객체 안에 messageID/partID 가 있음 (part.updated 와 동일).
            _delta_part = props.get("part") or {}
            msg_id = props.get("messageID") or _delta_part.get("messageID")
            part_id = props.get("partID") or _delta_part.get("id")
            delta = props.get("delta", "")
            if msg_id and msg_id in finished_msg_ids:
                continue
            if msg_id and msg_id in user_msg_ids:
                continue
            if msg_id and assistant_msg_id and msg_id != assistant_msg_id:
                continue
            if not delta or not part_id:
                continue
            # part 타입 판단: 기억한 매핑 우선, 없으면 delta.field 로 추정
            ptype = part_types.get(part_id) or props.get("field", "text")
            if ptype == "reasoning":
                prev = final_reasoning_by_part.get(part_id, "")
                final_reasoning_by_part[part_id] = prev + delta
                yield _sse("reasoning", {"turn": turn, "step": "thought", "content": delta})
            elif ptype == "text":
                prev = final_text_by_part.get(part_id, "")
                final_text_by_part[part_id] = prev + delta
                if part_id not in text_part_order:
                    text_part_order.append(part_id)
                yield _sse("content", {"text": delta})
            continue

        if t != "message.part.updated":
            if t == "message.updated":
                info = evt.get("properties", {}).get("info", {})
                fin = info.get("finish")
                msg_id = info.get("id")
                role = info.get("role")
                # OpenCode 는 message.created 를 emit 하지 않고 message.updated 로 통합 신호.
                # 따라서 여기서 user/assistant 등록을 함께 처리한다.
                if msg_id and role == "user":
                    if msg_id not in user_msg_ids:
                        user_msg_ids.add(msg_id)
                elif msg_id and role == "assistant":
                    if msg_id != assistant_msg_id and msg_id not in finished_msg_ids:
                        # 새 assistant 시작 — assistant_msg_id 갱신 + thinking 1회
                        assistant_msg_id = msg_id
                        yield _sse("thinking", {"turn": turn, "message": "응답 생성 중..."})

                if role == "assistant" and fin in TERMINAL_FINISHES:
                    # 현재 turn 의 assistant 완료
                    if msg_id and msg_id == assistant_msg_id:
                        reply = "".join(final_text_by_part[pid] for pid in text_part_order).strip()
                        log.info(f"[trans/{gojira_session_id[:8]}] done yield: mid={msg_id} fin={fin}")
                        yield _sse("done", {"reply": reply, "actions": actions})
                        return
                    if msg_id and msg_id not in finished_msg_ids:
                        # 위 등록 분기에서 assistant_msg_id 가 이미 갱신됐어야 정상.
                        # 그래도 안전망: 알려지지 않은 mid 의 terminal finish 면 done 으로 수용.
                        finished_msg_ids.add(msg_id)
                        log.info(f"[trans/{gojira_session_id[:8]}] 미등록 assistant 의 finish={fin} — done 으로 수용 (mid={msg_id})")
                        yield _sse("done", {"reply": "", "actions": actions})
                        return
            # raw 이벤트가 계속 오고 있으면 keepalive 전송 (3초 간격)
            now = time.monotonic()
            if now - last_keepalive > 3.0:
                last_keepalive = now
                yield _KEEPALIVE
            continue

        part = evt.get("properties", {}).get("part", {})
        ptype = part.get("type")
        # messageID 는 part 객체 안에 있다 (properties.messageID 아님)
        msg_id = part.get("messageID") or evt.get("properties", {}).get("messageID")

        # 과거 완료된 메시지의 part 는 무시 (OpenCode 재전송)
        if msg_id and msg_id in finished_msg_ids:
            continue
        # user message의 part — text 이면 user_message 로 한 번 yield (TUI 입력 반영). 이후 assistant 흐름은 안 탐
        if msg_id and msg_id in user_msg_ids:
            if ptype == "text":
                pid = part.get("id")
                full_text = part.get("text", "")
                if pid and full_text and pid not in emitted_user_parts:
                    emitted_user_parts.add(pid)
                    yield _sse("user_message", {"messageId": msg_id, "partId": pid, "text": full_text})
            continue
        # 다른 assistant 메시지의 part 는 무시 (assistant_msg_id 가 세팅되어 있을 때만)
        if msg_id and assistant_msg_id and msg_id != assistant_msg_id:
            continue

        # partID → type 매핑 기록 (delta 이벤트 분류에 사용)
        _pid_for_type = part.get("id")
        if _pid_for_type and ptype:
            part_types[_pid_for_type] = ptype

        if ptype == "text":
            txt = part.get("text", "")
            pid = part.get("id")
            prev = final_text_by_part.get(pid, "")
            if len(txt) > len(prev):
                delta = txt[len(prev):]
                yield _sse("content", {"text": delta})
            if pid not in final_text_by_part:
                text_part_order.append(pid)
            final_text_by_part[pid] = txt

        elif ptype == "reasoning":
            txt = part.get("text") or part.get("content") or ""
            pid = part.get("id") or part.get("callID") or "reasoning"
            prev = final_reasoning_by_part.get(pid, "")
            if len(txt) > len(prev):
                delta = txt[len(prev):]
                yield _sse("reasoning", {"turn": turn, "step": "thought", "content": delta})
            final_reasoning_by_part[pid] = txt

        elif ptype == "tool":
            state = part.get("state", {})
            status = state.get("status")
            tool = part.get("tool", "?")
            call_id = part.get("callID") or part.get("id")

            if status == "running" and call_id not in seen_tool_ids:
                seen_tool_ids.add(call_id)
                turn += 1
                yield _sse("tool_call", {
                    "tool": tool,
                    "params": state.get("input") or {},
                    "turn": turn,
                })
            elif status == "completed":
                out = state.get("output")
                summary = _summarize_output(out)
                detail = out if isinstance(out, str) else json.dumps(out, ensure_ascii=False)[:500] if out else ""
                yield _sse("tool_result", {
                    "tool": tool,
                    "status": "success",
                    "summary": summary,
                    "detail": detail,
                })
                actions.append({"tool": tool, "params": state.get("input") or {}, "summary": summary})
            elif status == "error":
                err = state.get("error") or "unknown"
                yield _sse("tool_result", {
                    "tool": tool,
                    "status": "error",
                    "summary": str(err)[:200],
                    "detail": str(err),
                })

        elif ptype == "step-start":
            turn += 1
            yield _sse("thinking", {"turn": turn, "message": "추론 중..."})


def _add_or_update_text(buf: list[str], pid: str, txt: str) -> None:
    # 간단하게 한 part 만 최종 응답으로 취급 (보통 마지막 assistant text part)
    # 여러 text part 가 있으면 dict 로 교체하는 게 맞지만 간이 구현
    if not buf:
        buf.append(txt)
    else:
        buf[-1] = txt


def _summarize_output(out) -> str:
    if not out:
        return ""
    s = out if isinstance(out, str) else json.dumps(out, ensure_ascii=False)
    if len(s) <= 200:
        return s
    return s[:200] + "..."
