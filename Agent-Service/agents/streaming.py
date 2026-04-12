"""스트리밍 에이전트 루프 — SSE로 진행 과정을 실시간 전송."""

import json
import logging
import time
from typing import List, Dict, AsyncGenerator

from core.llm import call_llm_with_tools
from skills.executor import execute_skill
from skills.registry import get_registry
from agents.utils import strip_reasoning as _strip_reasoning

log = logging.getLogger("agent.streaming")

MAX_TURNS = 10


async def agent_loop_stream(
    messages: List[Dict],
    user_id: str,
    max_turns: int = MAX_TURNS,
) -> AsyncGenerator[str, None]:
    """스트리밍 에이전트 루프 — SSE 이벤트 생성기.

    이벤트 타입:
    - thinking: LLM 호출 중
    - tool_call: 도구 호출 시작
    - tool_result: 도구 실행 완료
    - content: 최종 텍스트 (청크 단위)
    - done: 완료
    - error: 에러 발생
    """
    registry = get_registry()
    all_actions = []

    # 사용자 메시지 추출 (마지막 user 메시지)
    user_message = ""
    for m in reversed(messages):
        if m.get("role") == "user":
            user_message = m.get("content", "")
            break

    # 1차 스킬 선별 (키워드 + description 매칭)
    tools = registry.select_tools(user_message)

    for turn in range(max_turns):
        yield _sse("thinking", {"turn": turn, "message": "추론 중..."})

        # ── Pass 1: 추론 (tool_choice="none") ──
        # LLM에게 도구 호출 없이 사고 과정만 출력하도록 요청
        try:
            think_result = await call_llm_with_tools(messages, tools, tool_choice="none")
            thought = think_result["message"].content or ""
        except Exception:
            thought = ""

        if thought:
            # ReACT 마커별로 분리 전송 (Thought / Action / Answer)
            for evt in _parse_reasoning_steps(thought, turn):
                yield evt

            # Pass 1 추론 결과로 스킬 재선별 (추론에서 언급된 스킬 반영)
            tools = registry.select_tools(user_message, extra_context=thought)

            # 추론 내용을 대화에 추가 (도구 호출 판단에 활용)
            messages.append({"role": "assistant", "content": thought})
            messages.append({"role": "user", "content": "위 분석에 따라 필요한 도구가 있으면 호출하세요. 더 이상 도구가 필요 없다면 최종 답변을 제공하세요."})

        # ── Pass 2: 도구 호출 (tool_choice="auto") ──
        try:
            result = await call_llm_with_tools(messages, tools)
        except Exception as e:
            log.error("LLM error at turn %d: %s", turn, e)
            yield _sse("error", {"message": f"LLM 호출 오류: {e}"})
            yield _sse("done", {"reply": f"LLM 호출 중 오류가 발생했습니다: {e}", "actions": all_actions})
            return

        msg = result["message"]
        finish_reason = result["finish_reason"]
        content = msg.content or ""
        tool_calls = msg.tool_calls or []

        # Pass 2의 추론 내용도 분리 전송
        if content and not tool_calls:
            for evt in _parse_reasoning_steps(content, turn):
                yield evt

        # 도구 호출 없으면 최종 응답
        if not tool_calls or finish_reason == "stop":
            final_content = thought or content
            if not final_content and all_actions:
                final_content = "작업이 완료되었습니다."
            final_content = _strip_reasoning(final_content)
            for chunk in _split_chunks(final_content, 80):
                yield _sse("content", {"text": chunk})
            yield _sse("done", {"reply": final_content, "actions": all_actions})
            return

        # assistant message 기록
        assistant_msg = {"role": "assistant", "content": content}
        if tool_calls:
            assistant_msg["tool_calls"] = [
                {
                    "id": tc.id,
                    "type": "function",
                    "function": {
                        "name": tc.function.name,
                        "arguments": tc.function.arguments,
                    },
                }
                for tc in tool_calls
            ]
        messages.append(assistant_msg)

        # 각 도구 실행
        for tc in tool_calls:
            fn_name = tc.function.name
            try:
                fn_args = json.loads(tc.function.arguments)
            except json.JSONDecodeError:
                fn_args = {}

            # 도구 호출 시작 알림
            yield _sse("tool_call", {"tool": fn_name, "params": fn_args, "turn": turn})

            start_ms = int(time.time() * 1000)
            try:
                skill_result = await execute_skill(fn_name, fn_args, user_id)
                duration = int(time.time() * 1000) - start_ms
                result_json = json.dumps(skill_result, ensure_ascii=False, default=str)

                messages.append({
                    "role": "tool",
                    "tool_call_id": tc.id,
                    "content": result_json,
                })

                action = {"skill": fn_name, "params": fn_args, "result": skill_result, "duration_ms": duration}
                all_actions.append(action)

                # 도구 결과 알림
                tool_result_data = {
                    "tool": fn_name,
                    "status": "success",
                    "duration_ms": duration,
                    "summary": _summarize_result(skill_result),
                    "detail": _truncate_detail(skill_result),
                }
                # _action이 있으면 프론트엔드에 전달 (ui_navigate 등)
                if isinstance(skill_result, dict) and "_action" in skill_result:
                    tool_result_data["_action"] = skill_result["_action"]
                    tool_result_data["_payload"] = {k: v for k, v in skill_result.items() if k != "_action"}
                yield _sse("tool_result", tool_result_data)

            except Exception as e:
                duration = int(time.time() * 1000) - start_ms
                log.error("tool error: %s — %s", fn_name, e)
                messages.append({
                    "role": "tool",
                    "tool_call_id": tc.id,
                    "content": str(e),
                })
                all_actions.append({"skill": fn_name, "params": fn_args, "error": str(e), "duration_ms": duration})

                yield _sse("tool_result", {
                    "tool": fn_name,
                    "status": "error",
                    "duration_ms": duration,
                    "summary": str(e),
                })

    yield _sse("done", {"reply": "최대 처리 단계를 초과했습니다.", "actions": all_actions})


import re as _re

_REACT_MARKER = _re.compile(r'\*\*(Thought|Action|Observation|Answer)\*\*\s*:\s*')


def _parse_reasoning_steps(text: str, turn: int) -> List[str]:
    """LLM 응답에서 **Thought**:, **Action**:, **Answer**: 마커를 찾아
    각각 별도 SSE 이벤트로 분리한다.

    반환: SSE 문자열 리스트
    - reasoning 이벤트: step 필드로 thought/action/observation/answer 구분
    """
    markers = list(_REACT_MARKER.finditer(text))

    if not markers:
        # 마커가 없으면 전체를 하나의 thought로
        return [_sse("reasoning", {"turn": turn, "step": "thought", "content": text.strip()})]

    events = []
    for i, m in enumerate(markers):
        step_type = m.group(1).lower()  # thought, action, observation, answer
        start = m.end()
        end = markers[i + 1].start() if i + 1 < len(markers) else len(text)
        content = text[start:end].strip()
        if content:
            events.append(_sse("reasoning", {"turn": turn, "step": step_type, "content": content}))

    return events if events else [_sse("reasoning", {"turn": turn, "step": "thought", "content": text.strip()})]


def _sse(event: str, data: dict) -> str:
    """SSE 형식 문자열 생성."""
    return f"event: {event}\ndata: {json.dumps(data, ensure_ascii=False, default=str)}\n\n"


def _split_chunks(text: str, size: int) -> List[str]:
    """텍스트를 줄 단위 또는 고정 크기로 분할."""
    if not text:
        return []
    lines = text.split("\n")
    chunks = []
    buf = ""
    for line in lines:
        if len(buf) + len(line) + 1 > size and buf:
            chunks.append(buf)
            buf = line
        else:
            buf = buf + "\n" + line if buf else line
    if buf:
        chunks.append(buf)
    return chunks


_TOOL_DESCRIPTIONS = {
    "project_query": "프로젝트 목록 확인",
    "issue_query": "{projectCode} 이슈 조회",
    "issue_create_vos": "{projectCode}에 VOS 이슈 생성",
    "issue_create_feature": "{projectCode}에 Feature 이슈 생성",
    "issue_create_requirement": "{projectCode}에 Requirement({reqType}) 이슈 생성",
    "issue_create_design": "{projectCode}에 {designLevel} Design 이슈 생성",
    "usecase_analyze": "{projectCode} Use Case 분석 → Feature 도출",
    "requirement_refine": "{projectCode} Requirement 정제 (FR/NFR/CNST)",
    "issue_update": "{key} 이슈 수정",
    "issue_delete": "{key} 이슈 삭제",
    "issue_link": "{fromKey}와 {toKey} 관계 설정",
    "issue_analyze": "{projectCode} 이슈 관계 분석",
    "satam_analyze": "{projectCode} SATAM 분석 수행",
    "schedule_analyze": "{projectCode} 일정 분석",
    "doc_list": "{projectCode} 문서 목록 조회",
    "doc_summarize": "{projectCode}/{filePath} 문서 요약",
    "doc_diff": "{projectCode}/{filePath} 변경 비교",
    "doc_history": "{projectCode} 커밋 이력 조회",
    "pr_list": "{projectCode} PR 목록 조회",
    "pr_create": "{projectCode} PR 생성",
}


def _describe_tool_call(name: str, args: dict) -> str:
    """도구 호출을 자연스러운 한국어 설명으로 변환."""
    template = _TOOL_DESCRIPTIONS.get(name, f"{name} 실행")
    try:
        return template.format(**{k: v for k, v in args.items() if isinstance(v, str)})
    except KeyError:
        return template.split("{")[0].strip() or f"{name} 실행"


def _truncate_detail(result, max_len=500) -> str:
    """도구 결과를 디버그용으로 요약 (마스터 사용자에게 표시)."""
    try:
        text = json.dumps(result, ensure_ascii=False, default=str)
        if len(text) > max_len:
            return text[:max_len] + "..."
        return text
    except Exception:
        return str(result)[:max_len]


def _summarize_result(result) -> str:
    """도구 결과를 한 줄 요약."""
    if isinstance(result, dict):
        if "count" in result:
            return f"{result.get('count', 0)}건 조회"
        if "key" in result:
            return f"{result['key']} 처리"
        if "analysis" in result:
            return "분석 완료"
        if "commits" in result:
            return f"커밋 {result.get('count', 0)}건"
    return "완료"
