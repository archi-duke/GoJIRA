"""OpenCode 이벤트 → UI SSE 이벤트 번역기.

UI 가 기대하는 이벤트: session, thinking, tool_call, tool_result, content, done, error.
OpenCode /event 포맷:
  {type: "message.part.updated", properties: {part: {...}}}
  part.type ∈ {step-start, text, tool, step-finish}
  tool part: {tool, state:{status, input, output}}
"""

import json
from typing import AsyncIterator


def _sse(event: str, data: dict) -> str:
    return f"event: {event}\ndata: {json.dumps(data, ensure_ascii=False, default=str)}\n\n"


async def translate(opencode_events: AsyncIterator[dict], gojira_session_id: str):
    """OpenCode 이벤트 스트림 → UI SSE 이벤트 문자열 yield.

    assistant message 완료(step-finish) 감지 시 done 을 yield 하고 종료.
    """
    final_text_by_part: dict[str, str] = {}
    text_part_order: list[str] = []
    actions: list[dict] = []
    # session 이벤트 먼저
    yield _sse("session", {"sessionId": gojira_session_id})

    turn = 0
    seen_tool_ids: set[str] = set()
    # session.idle 또는 finish=="stop" 일 때만 종료
    TERMINAL_FINISHES = {"stop", "end_turn", "length", "content-filter"}

    async for evt in opencode_events:
        t = evt.get("type")
        if t != "message.part.updated":
            if t == "message.updated":
                info = evt.get("properties", {}).get("info", {})
                fin = info.get("finish")
                if info.get("role") == "assistant" and fin in TERMINAL_FINISHES:
                    reply = "".join(final_text_by_part[pid] for pid in text_part_order).strip()
                    yield _sse("done", {"reply": reply, "actions": actions})
                    return
            elif t == "session.status":
                st = evt.get("properties", {}).get("status", {}).get("type")
                # 보조 종료 신호: idle 이고 최근 text 가 있으면 종료
                if st == "idle" and final_text_by_part:
                    reply = "".join(final_text_by_part[pid] for pid in text_part_order).strip()
                    if reply:
                        yield _sse("done", {"reply": reply, "actions": actions})
                        return
            continue

        part = evt.get("properties", {}).get("part", {})
        ptype = part.get("type")

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
            if txt:
                yield _sse("reasoning", {"turn": turn, "step": "thought", "content": txt[-200:]})

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
