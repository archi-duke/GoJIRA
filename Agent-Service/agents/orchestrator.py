"""에이전트 오케스트레이터 — 하이브리드 방식.

- 단순 Q&A: ReACT 단일 루프 (Go agentLoop 동일)
- 멀티스텝: Stage Machine (PLANNING→RESEARCHING→EXECUTING→VALIDATING→COMPLETED)
"""

import json
import logging
import time
import uuid
from datetime import datetime
from typing import List, Dict, Tuple, Any, Optional

from core.llm import call_llm, call_llm_with_tools
from skills.executor import execute_skill
from skills.registry import get_registry
from agents.utils import strip_reasoning as _strip_reasoning
from state.models import JobState, JobStatus, StageState, StageStatus, ToolResult
from state import job_store

log = logging.getLogger("agent.orchestrator")

MAX_TURNS = 10

# ─── 의도 분류 ───

_COMPLEX_KEYWORDS = [
    "분석하고", "분석해서", "작성해줘", "생성하고", "만들고", "설계서",
    "리포트", "보고서", "검토하고", "평가하고", "종합", "전체",
]


def classify_intent(message: str) -> str:
    """사용자 메시지에서 단순/복잡 분류. 키워드 기반."""
    msg_lower = message.lower()
    # 복합 조건: "~하고 ~해줘" 패턴 또는 복잡 키워드
    connectors = msg_lower.count("하고") + msg_lower.count("해서") + msg_lower.count("한 다음")
    if connectors >= 1:
        for kw in _COMPLEX_KEYWORDS:
            if kw in msg_lower:
                return "complex"
    # 단순 키워드
    if any(kw in msg_lower for kw in ["작성해줘", "설계서", "리포트", "보고서"]):
        return "complex"
    return "simple"


# ─── ReACT 단일 루프 (기본) ───

async def agent_loop(
    messages: List[Dict],
    user_id: str,
    max_turns: int = MAX_TURNS,
) -> Tuple[str, List[Dict]]:
    """ReACT 에이전트 루프 — Go agentLoop() 동일."""
    registry = get_registry()

    # 사용자 메시지 기반 스킬 선별
    user_message = ""
    for m in reversed(messages):
        if m.get("role") == "user":
            user_message = m.get("content", "")
            break
    tools = registry.select_tools(user_message)
    all_actions: List[Dict] = []

    for turn in range(max_turns):
        try:
            result = await call_llm_with_tools(messages, tools)
        except Exception as e:
            log.error("LLM error at turn %d: %s", turn, e)
            return f"LLM 호출 중 오류가 발생했습니다: {e}", all_actions

        msg = result["message"]
        finish_reason = result["finish_reason"]
        content = msg.content or ""
        tool_calls = msg.tool_calls or []

        if not tool_calls or finish_reason == "stop":
            if not content and all_actions:
                content = "작업이 완료되었습니다."
            return _strip_reasoning(content), all_actions

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

        for tc in tool_calls:
            fn_name = tc.function.name
            try:
                fn_args = json.loads(tc.function.arguments)
            except json.JSONDecodeError:
                fn_args = {}

            log.info("turn=%d tool=%s args=%s", turn, fn_name, fn_args)
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
                all_actions.append({
                    "skill": fn_name, "params": fn_args, "result": skill_result,
                    "duration_ms": duration,
                })

            except Exception as e:
                duration = int(time.time() * 1000) - start_ms
                log.error("tool error: %s — %s", fn_name, e)
                messages.append({
                    "role": "tool",
                    "tool_call_id": tc.id,
                    "content": str(e),
                })
                all_actions.append({
                    "skill": fn_name, "params": fn_args, "error": str(e),
                    "duration_ms": duration,
                })

    return "최대 처리 단계를 초과했습니다. 질문을 좁혀서 다시 시도해주세요.", all_actions


# ─── Stage Machine 오케스트레이터 (멀티스텝) ───

async def staged_orchestrate(
    user_message: str,
    messages: List[Dict],
    user_id: str,
    session_id: str = "",
) -> Tuple[str, List[Dict]]:
    """Stage Machine 기반 멀티스텝 오케스트레이션.

    흐름: PLANNING → RESEARCHING → EXECUTING → VALIDATING → COMPLETED
    """
    all_actions: List[Dict] = []

    # 1. Planning — LLM에게 작업 계획 수립 요청
    plan_prompt = _load_role_prompt("planner", {
        "user_request": user_message,
    })
    messages_for_plan = messages.copy()
    messages_for_plan.append({"role": "user", "content": plan_prompt})

    plan_text = await call_llm(plan_prompt)
    plan_json = _parse_json(plan_text)

    if not plan_json or "steps" not in plan_json:
        # 계획 수립 실패 → ReACT 루프로 fallback
        log.warning("계획 수립 실패, ReACT 루프로 전환")
        return await agent_loop(messages, user_id)

    # 2. Job State 생성
    job_id = f"JOB-{datetime.utcnow().strftime('%Y%m%d-%H%M%S')}-{uuid.uuid4().hex[:4]}"
    stages = []
    for step in plan_json["steps"]:
        stages.append(StageState(
            id=f"step_{step.get('id', 0)}",
            role=step.get("role", "executor"),
        ))

    job = JobState(
        job_id=job_id,
        session_id=session_id,
        user_id=user_id,
        status=JobStatus.RESEARCHING,
        goal=plan_json.get("goal", user_message),
        plan=stages,
        context={
            "projectCode": plan_json.get("projectCode", ""),
            "original_plan": plan_json,
        },
    )

    try:
        job_store.create_job(job)
    except Exception as e:
        log.warning("Job 저장 실패: %s", e)

    # 3. 각 단계 순차 실행 (ReACT 루프로)
    progress_parts = [f"**작업 계획** ({job.goal}):"]
    for i, step in enumerate(plan_json["steps"]):
        step_desc = step.get("task", f"단계 {i+1}")
        progress_parts.append(f"\n### 단계 {i+1}: {step_desc}")

        # 역할별 프롬프트 + ReACT 루프 실행
        role_prompt = f"현재 단계: {step_desc}\n이전 결과를 참고하여 이 단계를 수행하세요."
        step_messages = messages.copy()
        step_messages.append({"role": "user", "content": role_prompt})

        try:
            job_store.update_stage(job_id, i, StageStatus.RUNNING)
        except Exception:
            pass

        reply, actions = await agent_loop(step_messages, user_id, max_turns=5)
        all_actions.extend(actions)
        progress_parts.append(reply)

        # messages에 결과 추가 (다음 단계에서 참조)
        messages.append({"role": "assistant", "content": reply})

        try:
            job_store.update_stage(job_id, i, StageStatus.COMPLETED, result=reply)
        except Exception:
            pass

    # 4. 완료
    try:
        job_store.mark_completed(job_id)
    except Exception:
        pass

    final_reply = "\n".join(progress_parts)
    return final_reply, all_actions


def _load_role_prompt(role: str, variables: Dict[str, str]) -> str:
    """Jinja2 역할별 프롬프트 로드."""
    import os
    from jinja2 import Template

    prompts_dir = os.path.join(os.path.dirname(__file__), "..", "prompts", "roles")
    template_path = os.path.join(prompts_dir, f"{role}.j2")

    if os.path.isfile(template_path):
        with open(template_path, "r", encoding="utf-8") as f:
            tmpl = Template(f.read())
        return tmpl.render(**variables)

    return f"당신은 {role} 역할입니다. 요청: {variables.get('user_request', '')}"


def _parse_json(text: str) -> Optional[Dict]:
    """텍스트에서 JSON 추출."""
    try:
        # 코드블록 안에 있을 수 있음
        if "```" in text:
            start = text.find("{")
            end = text.rfind("}") + 1
            if start >= 0 and end > start:
                text = text[start:end]
        return json.loads(text)
    except (json.JSONDecodeError, ValueError):
        return None
