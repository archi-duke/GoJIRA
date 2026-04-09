"""에이전트 오케스트레이터 — Go handlers/agent.go agentLoop 대응.

Phase 1: ReACT 단일 루프 (Go와 동일 동작)
Phase 3에서 Stage Machine으로 업그레이드 예정.
"""

import json
import logging
from typing import List, Dict, Tuple, Any

from core.llm import call_llm_with_tools
from skills.executor import execute_skill
from skills.registry import get_registry

log = logging.getLogger("agent.orchestrator")

MAX_TURNS = 10


async def agent_loop(
    messages: List[Dict],
    user_id: str,
    max_turns: int = MAX_TURNS,
) -> Tuple[str, List[Dict]]:
    """ReACT 에이전트 루프 — Go agentLoop() 직역.

    Args:
        messages: LLM에 전달할 메시지 배열 (system + history + user)
        user_id: 현재 사용자 ID
        max_turns: 최대 턴 수

    Returns:
        (reply_text, actions_list)
    """
    registry = get_registry()
    tools = registry.list_openai_tools()
    all_actions: List[Dict] = []

    for turn in range(max_turns):
        # LLM 호출
        try:
            result = await call_llm_with_tools(messages, tools)
        except Exception as e:
            log.error("LLM error at turn %d: %s", turn, e)
            return f"LLM 호출 중 오류가 발생했습니다: {e}", all_actions

        msg = result["message"]
        finish_reason = result["finish_reason"]
        content = msg.content or ""
        tool_calls = msg.tool_calls or []

        # tool_calls가 없으면 루프 종료
        if not tool_calls or finish_reason == "stop":
            if not content and all_actions:
                content = "작업이 완료되었습니다."
            return content, all_actions

        # assistant message를 대화 기록에 추가
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

        # 각 tool_call 실행
        for tc in tool_calls:
            fn_name = tc.function.name
            try:
                fn_args = json.loads(tc.function.arguments)
            except json.JSONDecodeError:
                fn_args = {}

            log.info("turn=%d tool=%s args=%s", turn, fn_name, fn_args)

            try:
                skill_result = await execute_skill(fn_name, fn_args, user_id)
                result_json = json.dumps(skill_result, ensure_ascii=False, default=str)

                messages.append({
                    "role": "tool",
                    "tool_call_id": tc.id,
                    "content": result_json,
                })
                all_actions.append({
                    "skill": fn_name, "params": fn_args, "result": skill_result,
                })

            except Exception as e:
                log.error("tool error: %s — %s", fn_name, e)
                messages.append({
                    "role": "tool",
                    "tool_call_id": tc.id,
                    "content": str(e),
                })
                all_actions.append({
                    "skill": fn_name, "params": fn_args, "error": str(e),
                })

    return "최대 처리 단계를 초과했습니다. 질문을 좁혀서 다시 시도해주세요.", all_actions
