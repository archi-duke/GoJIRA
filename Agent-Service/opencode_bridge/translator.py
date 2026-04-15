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
    final_text_parts: list[str] = []
    actions: list[dict] = []
    # session 이벤트 먼저
    yield _sse("session", {"sessionId": gojira_session_id})

    turn = 0
    seen_tool_ids: set[str] = set()

    async for evt in opencode_events:
        t = evt.get("type")
        if t != "message.part.updated":
            # assistant 완료 감지 (message.updated + finish)
            if t == "message.updated":
                info = evt.get("properties", {}).get("info", {})
                if info.get("role") == "assistant" and info.get("finish"):
                    yield _sse("done", {
                        "reply": "".join(final_text_parts).strip(),
                        "actions": actions,
                    })
                    return
            continue

        part = evt.get("properties", {}).get("part", {})
        ptype = part.get("type")

        if ptype == "text":
            txt = part.get("text", "")
            # OpenCode 는 delta 가 아니라 누적 텍스트 — 변경분을 UI 에 내려보내기 위해
            # part id 별 마지막 길이를 추적
            pid = part.get("id")
            prev_len = translate._text_cache.get(pid, 0) if hasattr(translate, "_text_cache") else 0
            if not hasattr(translate, "_text_cache"):
                translate._text_cache = {}
            if len(txt) > prev_len:
                delta = txt[prev_len:]
                translate._text_cache[pid] = len(txt)
                yield _sse("content", {"text": delta})
                # final 누적(복원용)
                if pid in [p.get("id") for p in []]:
                    pass
            # final 저장 (마지막 값이 전체)
            _add_or_update_text(final_text_parts, pid, txt)

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
