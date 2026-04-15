"""스트리밍 에이전트 루프 — SSE로 진행 과정을 실시간 전송."""

import asyncio
import json
import logging
import time
from typing import List, Dict, AsyncGenerator

from core.llm import call_llm_with_tools
from core.progress import bind_queue, reset_queue
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
    executed_skills: set = set()  # 환각 가드용 — 이 세션에서 실제로 호출된 스킬 이름
    # 완료형 문장 패턴 (LLM 이 호출하지도 않고 '저장했다/완료했다' 고 주장할 때 탐지)
    _COMPLETION_PATTERNS = (
        "저장했습니다", "저장되었", "작성했습니다", "작성되었", "작성이 완료",
        "생성했습니다", "생성되었", "완료되었", "완료했습니다",
    )
    _GEN_SKILLS = {
        "vos_collect", "usecase_analyze", "requirement_refine",
        "driver_select", "style_define", "design_decide", "design_integrate",
        "satam_analyze",
    }
    # Phase B — 워크플로우 stage 에 매핑되는 스킬. 이 스킬들은 jobId 자동 주입 + advance 전이 평가 대상.
    _STAGE_SKILLS = {
        "projdesc_interview", "projdesc_generate",
        "vos_collect", "usecase_analyze", "requirement_refine",
        "driver_select", "style_define", "design_decide", "design_integrate",
        "satam_analyze",
    }

    # 사용자 메시지 추출 (마지막 user 메시지)
    user_message = ""
    for m in reversed(messages):
        if m.get("role") == "user":
            user_message = m.get("content", "")
            break

    # 워크플로우 상태 주입 (Belief State) + Phase B 엔진 job 결합
    detected_code = None
    state = None
    workflow_job = None
    try:
        from core.workflow_state import detect_project_code, get_workflow_state, format_state_for_llm
        for m in reversed(messages):
            role = m.get("role")
            if role not in ("user", "assistant"):
                continue
            content = m.get("content") or ""
            if not isinstance(content, str):
                continue
            detected_code = detect_project_code(content)
            if detected_code:
                break
        if detected_code:
            # Phase B — workflow job 먼저 load/start (belief state 에 currentStage 주입 위해)
            try:
                from core import workflow_engine
                workflow_job = workflow_engine.load_or_start_job(detected_code, created_by=user_id)
                log.info("workflow job: %s stage=%s", workflow_job.get("jobId"), workflow_job.get("currentStage"))
            except Exception as e:
                log.warning("workflow job 로드 실패: %s", e)
            state = await get_workflow_state(detected_code, workflow_job=workflow_job, user=user_id)
            state_text = format_state_for_llm(state)
            log.info("워크플로우 상태 주입: code=%s stage=%s skill=%s completed=%s",
                     detected_code, state.get("currentStage"), state.get("currentStageSkill"),
                     [k for k, v in (state.get("completedSteps") or {}).items() if v])
            injected = False
            for i, m in enumerate(messages):
                if m.get("role") == "system":
                    messages[i] = {"role": "system", "content": m.get("content", "") + "\n\n" + state_text}
                    injected = True
                    break
            if not injected:
                messages.insert(0, {"role": "system", "content": state_text})
        else:
            log.info("워크플로우 상태 주입 스킵: 프로젝트 코드 미감지")
    except Exception as e:
        log.warning("워크플로우 상태 주입 실패: %s", e)

    # Phase B — 워크플로우 모드에서 tools 강제 제한: 현재 stage 주 skill + 보조 skill 만 노출
    required_skills = []
    workflow_mode = False
    workflow_allowed: set = set()
    if workflow_job and workflow_job.get("status") in ("in_progress", "pending", "paused"):
        cur_stage = workflow_job.get("currentStage")
        try:
            from core.workflows import get_stage
            stage_def = get_stage(workflow_job["workflowId"], cur_stage)
            required_skills.append(stage_def["skill"])
            workflow_allowed.add(stage_def["skill"])
            # projdesc stage 는 interview/generate 함께 노출 (답변 처리 위해)
            if cur_stage == "projdesc":
                workflow_allowed.update({"projdesc_interview", "projdesc_generate"})
            workflow_mode = True
        except Exception as e:
            log.warning("stage tools 제한 실패: %s", e)
    elif state and state.get("nextRecommendedStep"):
        required_skills.append(state["nextRecommendedStep"])

    tools = registry.select_tools(
        user_message,
        required=required_skills,
        workflow_mode=workflow_mode,
        workflow_allowed=workflow_allowed,
    )

    # thinking 계열 모델은 내부 추론을 수행하므로 별도 Pass 1 (명시적 ReACT 외재화) 불필요.
    # Pass 2 (tool_choice="auto") 단일 호출로 충분.
    from core.llm import is_thinking_model
    thinking_model = is_thinking_model()

    for turn in range(max_turns):
        yield _sse("thinking", {"turn": turn, "message": "추론 중..."})

        thought = ""
        if not thinking_model:
            # ── Pass 1: 추론 (tool_choice="none") ── (비-thinking 모델 전용)
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
                tools = registry.select_tools(
                    user_message, extra_context=thought,
                    required=required_skills,
                    workflow_mode=workflow_mode,
                    workflow_allowed=workflow_allowed,
                )

                # 추론이 사용자에게 질문하는 형태라면 Pass 2 건너뛰고 텍스트 답변으로 종료.
                # 단, belief state 기준 워크플로우 진행 중이면 이를 무시하고 Pass 2 진입
                # (LLM 이 인터뷰 질문을 인라인 작성 → projdesc_interview 호출 기회 상실 방지)
                workflow_pending = bool(required_skills)  # nextRecommendedStep 이 있으면 워크플로우 진행 중
                if _is_user_question(thought) and not workflow_pending:
                    final_content = _strip_reasoning(thought)
                    for chunk in _split_chunks(final_content, 80):
                        yield _sse("content", {"text": chunk})
                    yield _sse("done", {"reply": final_content, "actions": all_actions})
                    return

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
            # ── 환각 가드 ──
            # LLM 이 실제 스킬을 호출하지 않고 '저장했다/완료했다' 고만 주장한 경우 탐지.
            # belief state 의 nextRecommendedStep 이 여전히 생성 스킬이고 이번 세션에서
            # 호출된 적 없으면 해당 스킬을 tool_choice 로 강제해 1 회 재시도.
            guard_text = (thought or "") + " " + (content or "")
            if detected_code and any(p in guard_text for p in _COMPLETION_PATTERNS):
                try:
                    from core.workflow_state import get_workflow_state
                    fresh = await get_workflow_state(detected_code)
                    pending = fresh.get("nextRecommendedStep")
                    if pending in _GEN_SKILLS and pending not in executed_skills:
                        log.warning("환각 탐지: 완료 주장이나 %s 미실행. 강제 호출 재시도.", pending)
                        yield _sse("thinking", {"turn": turn, "message": f"완료 주장 감지 — {pending} 강제 호출"})
                        messages.append({
                            "role": "system",
                            "content": (
                                f"[가드] 방금 응답은 '{pending}' 을 호출하지 않은 채 완료됐다고 주장했다. "
                                f"belief state 체크리스트 기준 '{pending}' 은 아직 ⬜ 미완료. "
                                "반드시 해당 도구를 호출하라. 다른 설명 없이 tool call 만."
                            ),
                        })
                        try:
                            forced = await call_llm_with_tools(
                                messages, tools,
                                tool_choice={"type": "function", "function": {"name": pending}},
                            )
                            f_msg = forced["message"]
                            f_tool_calls = f_msg.tool_calls or []
                            if f_tool_calls:
                                # 아래 '각 도구 실행' 블록으로 자연 진입 — outer append 가 처리
                                tool_calls = f_tool_calls
                                content = f_msg.content or ""
                            else:
                                log.warning("강제 호출에도 tool_calls 없음 — 포기")
                        except Exception as e:
                            log.warning("강제 호출 실패: %s", e)
                except Exception as e:
                    log.warning("환각 가드 실패: %s", e)

            if not tool_calls:
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
            executed_skills.add(fn_name)
            try:
                fn_args = json.loads(tc.function.arguments)
            except json.JSONDecodeError:
                fn_args = {}

            # Phase B — 워크플로우 stage 스킬에는 jobId 자동 주입
            if workflow_job and fn_name in _STAGE_SKILLS:
                fn_args.setdefault("jobId", workflow_job["jobId"])

            # 도구 호출 시작 알림
            yield _sse("tool_call", {"tool": fn_name, "params": fn_args, "turn": turn})

            start_ms = int(time.time() * 1000)
            try:
                # 진행 알림 큐 바인딩 — 스킬 내부 emit_progress 호출이 SSE로 흘러나오게
                progress_queue: asyncio.Queue = asyncio.Queue()
                token = bind_queue(progress_queue)
                skill_task = asyncio.create_task(execute_skill(fn_name, fn_args, user_id))
                try:
                    # 스킬이 끝날 때까지 큐를 폴링하며 즉시 SSE로 yield
                    while not skill_task.done():
                        get_task = asyncio.create_task(progress_queue.get())
                        done, _ = await asyncio.wait(
                            {skill_task, get_task},
                            return_when=asyncio.FIRST_COMPLETED,
                        )
                        if get_task in done:
                            yield _sse("progress", get_task.result())
                        else:
                            get_task.cancel()
                    # 큐에 남은 메시지 마저 즉시 발행
                    while not progress_queue.empty():
                        yield _sse("progress", progress_queue.get_nowait())
                finally:
                    reset_queue(token)
                skill_result = skill_task.result()
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

                # Phase B — stage 스킬 결과로 engine 전이 평가 (9단계 전면, belief-state fallback)
                # fn_name 이 현재 stage 주 skill 과 다르더라도 advance 를 호출해 belief state 에서
                # 자동 전이 조건을 확인하도록 한다 (예: projdesc_generate → projdesc 완료).
                if workflow_job and fn_name in _STAGE_SKILLS and isinstance(skill_result, dict):
                    try:
                        from core import workflow_engine
                        from core.workflows import get_stage
                        cur = workflow_job.get("currentStage")
                        adv = await workflow_engine.advance(workflow_job, skill_result, fn_name=fn_name)
                        workflow_job = adv["job"]
                        if adv.get("transitioned"):
                            next_stage = workflow_job.get("currentStage")
                            try:
                                next_skill = get_stage(workflow_job["workflowId"], next_stage)["skill"] if not adv.get("completed") else None
                            except Exception:
                                next_skill = None
                            yield _sse("progress", {
                                "skill": "workflow_engine",
                                "message": f"단계 전이: {cur} → {next_stage if not adv.get('completed') else '완료'}",
                                "currentStage": next_stage,
                            })
                            # R4 — LLM 에 imperative 지시 주입 (다음 stage 의 skill 호출 강제)
                            if next_skill:
                                messages.append({
                                    "role": "system",
                                    "content": (
                                        f"[워크플로우 엔진] {cur} 단계 완료. "
                                        f"다음 stage: {next_stage}. "
                                        f"반드시 `{next_skill}(projectCode=\"{workflow_job['projectCode']}\", jobId=\"{workflow_job['jobId']}\")` 를 호출하라. "
                                        f"다른 도구 호출 금지."
                                    ),
                                })
                                # 다음 stage 의 tools 로 재선별 (같은 턴 내 연속 진행 — R5)
                                # 후속 turn 의 Pass 1 재선별이 이전 stage 로 되돌아가지 않도록
                                # workflow_allowed / required_skills 상위 변수도 갱신한다.
                                new_allowed = {next_skill}
                                if next_stage == "projdesc":
                                    new_allowed.update({"projdesc_interview", "projdesc_generate"})
                                workflow_allowed = new_allowed
                                required_skills = [next_skill]
                                tools = registry.select_tools(
                                    user_message,
                                    required=[next_skill],
                                    workflow_mode=True,
                                    workflow_allowed=new_allowed,
                                )
                    except Exception as e:
                        log.warning("engine.advance 실패: %s", e)

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


_USER_QUESTION_MARKERS = (
    "답변을 주시면", "답변을 부탁", "답변 부탁", "알려주세요", "여쭤볼게요", "여쭤볼게",
    "여쭤보겠습니다", "진행할까요", "어떻게 할까요", "확인해 주세요", "선택해 주세요",
    "답해 주세요", "답해주세요", "다음 질문",
)


def _is_user_question(text: str) -> bool:
    """추론 내용이 사용자에게 질문을 던지고 답을 기다리는 형태인지 판정.

    질문 직후 같은 턴에 도구를 호출하여 사용자 답변을 무시하는 문제를 막기 위함.
    조건: '?'를 포함하면서 명시적인 질문/요청 마커가 있어야 함 (단순 self-question 제외)
    """
    if not text or "?" not in text:
        return False
    return any(marker in text for marker in _USER_QUESTION_MARKERS)


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
    "reset_project": "[개발/테스트] {projectCode} 프로젝트 초기화",
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
