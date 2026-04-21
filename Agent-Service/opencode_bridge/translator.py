"""OpenCode 이벤트 → UI SSE 이벤트 번역기.

UI 가 기대하는 이벤트: session, thinking, tool_call, tool_result, content, done, error.
OpenCode /event 포맷:
  {type: "message.part.updated", properties: {part: {...}}}
  part.type ∈ {step-start, text, tool, step-finish}
  tool part: {tool, state:{status, input, output}}
"""

import json
import time
from typing import AsyncIterator, Optional


def _sse(event: str, data: dict) -> str:
    return f"event: {event}\ndata: {json.dumps(data, ensure_ascii=False, default=str)}\n\n"

# SSE keepalive 주석 — UI 에게 무시되지만 watchdog 타이머를 리셋시킨다.
_KEEPALIVE = ": keepalive\n\n"

# 권한 대기 감지 시 abort 대신 에러 이벤트를 yield 한다.
_PERMISSION_TIMEOUT = 60.0


async def translate(opencode_events: AsyncIterator[dict], gojira_session_id: str, initial_assistant_msg_id: Optional[str] = None):
    """OpenCode 이벤트 스트림 → UI SSE 이벤트 문자열 yield.

    assistant message 완료(step-finish) 감지 시 done 을 yield 하고 종료.
    재접속 시 message.created 를 놓친 경우 initial_assistant_msg_id 로 현재 진행 중 메시지 추적.
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
    # session.idle 또는 finish=="stop" 일 때만 종료
    TERMINAL_FINISHES = {"stop", "end_turn", "length", "content-filter"}
    permission_asked_at: float = 0.0  # 권한 요청 시각 (0이면 미발생)
    last_keepalive: float = 0.0

    async for evt in opencode_events:
        t = evt.get("type")

        # 권한 대기 감지
        if t == "permission.asked":
            if permission_asked_at == 0.0:
                permission_asked_at = time.monotonic()
        elif t in ("permission.resolved", "permission.granted"):
            permission_asked_at = 0.0

        # 권한 대기가 _PERMISSION_TIMEOUT 초과 시 → abort 요청 (router 에서 처리)
        if permission_asked_at > 0 and (time.monotonic() - permission_asked_at) > _PERMISSION_TIMEOUT:
            yield _sse("error", {"message": f"OpenCode 권한 요청 대기 {_PERMISSION_TIMEOUT}초 초과 — 작업을 중단합니다."})
            reply = "".join(final_text_by_part.get(pid, "") for pid in text_part_order).strip()
            yield _sse("done", {"reply": reply, "actions": actions, "_abort": True})
            return

        if t != "message.part.updated":
            if t == "message.updated":
                info = evt.get("properties", {}).get("info", {})
                fin = info.get("finish")
                msg_id = info.get("id")
                if info.get("role") == "assistant" and fin in TERMINAL_FINISHES:
                    # 현재 턴의 assistant가 완료된 경우만 done 처리
                    if msg_id and msg_id == assistant_msg_id:
                        reply = "".join(final_text_by_part[pid] for pid in text_part_order).strip()
                        yield _sse("done", {"reply": reply, "actions": actions})
                        return
                    # 과거 턴의 완료 이벤트는 무시 목록에 추가
                    if msg_id:
                        finished_msg_ids.add(msg_id)
            elif t == "message.created":
                info = evt.get("properties", {}).get("info", {})
                role = info.get("role")
                if role == "assistant":
                    assistant_msg_id = info.get("id") or evt.get("properties", {}).get("id")
                    yield _sse("thinking", {"turn": turn, "message": "응답 생성 중..."})
            # raw 이벤트가 계속 오고 있으면 keepalive 전송 (3초 간격)
            now = time.monotonic()
            if now - last_keepalive > 3.0:
                last_keepalive = now
                yield _KEEPALIVE
            continue

        part = evt.get("properties", {}).get("part", {})
        ptype = part.get("type")
        msg_id = evt.get("properties", {}).get("messageID")

        # 과거 완료된 메시지의 part 는 무시 (OpenCode 재전송)
        if msg_id and msg_id in finished_msg_ids:
            continue
        # user message의 part는 무시 (assistant message만 처리)
        if msg_id and assistant_msg_id and msg_id != assistant_msg_id:
            continue

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
