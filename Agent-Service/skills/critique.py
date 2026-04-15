"""생성 → 비판적 평가 → 수정 루프 헬퍼.

각 워크플로우 스킬(vos_collect, usecase_analyze, requirement_refine)에서
생성물 품질을 비판적 평가자 관점에서 검증하고 실패 시 수정 반복한다.

검증 기준과 정책은 워크플로우 문서(prompts/workflow-*.md)의 각 단계 섹션에서
"검증 기준" / "검증 정책" 헤딩을 찾아 추출한다 (단일 진실 출처).
정책이 명시되지 않으면 system.txt의 글로벌 기본 정책을 따른다.

상세는 docs/design-workflow.md 참조.
"""

import os
import re
import json
import logging
from typing import Callable, Optional, Tuple, List, Dict, Any

from core.llm import call_llm

log = logging.getLogger("agent.critique")

DEFAULT_THRESHOLD = 0.8
DEFAULT_MAX_ITER = 2
HARD_MAX_ITER = 5  # 안전장치: 정책이 아무리 크게 잡혀 있어도 이 수치를 넘지 않음

_PROMPTS_DIR = os.path.join(os.path.dirname(__file__), "..", "prompts")


def _extract_json_obj(raw: str) -> Optional[dict]:
    try:
        data = json.loads(raw)
        return data if isinstance(data, dict) else None
    except json.JSONDecodeError:
        m = re.search(r'\{.*\}', raw, re.DOTALL)
        if m:
            try:
                data = json.loads(m.group())
                return data if isinstance(data, dict) else None
            except json.JSONDecodeError:
                return None
    return None


def load_workflow_section(workflow_filename: str, step_heading: str) -> Optional[str]:
    """워크플로우 마크다운에서 특정 단계 섹션 본문을 추출.

    Args:
        workflow_filename: prompts/ 하위 파일명 (예: "workflow-requirement-analysis.md")
        step_heading: 단계 헤딩 텍스트 (예: "① VOS 도출")

    Returns:
        해당 단계의 본문 (다음 ## 또는 ### 헤딩 직전까지). 없으면 None.
    """
    path = os.path.join(_PROMPTS_DIR, workflow_filename)
    try:
        with open(path, "r", encoding="utf-8") as f:
            content = f.read()
    except FileNotFoundError:
        log.warning("workflow doc not found: %s", path)
        return None

    # ## 또는 ### 헤딩에서 step_heading을 포함하는 섹션 찾기
    pattern = rf'^(#{{2,3}})\s+.*{re.escape(step_heading)}.*$'
    lines = content.split("\n")
    start = None
    start_level = None
    for i, line in enumerate(lines):
        m = re.match(pattern, line)
        if m:
            start = i + 1
            start_level = len(m.group(1))
            break
    if start is None:
        return None

    # 같은 레벨(또는 상위 레벨)의 다음 헤딩까지가 한 섹션
    end = len(lines)
    for j in range(start, len(lines)):
        m2 = re.match(r'^(#{1,6})\s+', lines[j])
        if m2 and len(m2.group(1)) <= start_level:
            end = j
            break

    return "\n".join(lines[start:end]).strip()


def extract_field(section: str, field_label: str) -> Optional[str]:
    """섹션 본문에서 `**field_label**:` 다음의 내용을 추출 (다음 `**...**:` 직전까지)."""
    if not section:
        return None
    pattern = rf'\*\*{re.escape(field_label)}\*\*:\s*(.*?)(?=\n\*\*[^*]+\*\*:|\Z)'
    m = re.search(pattern, section, re.DOTALL)
    if not m:
        return None
    return m.group(1).strip()


def parse_policy(policy_text: Optional[str]) -> Dict[str, Any]:
    """자연어 정책 텍스트에서 threshold/max_iter/on_fail 추출.

    파싱 실패 항목은 기본값으로 fallback. 자연어 풀 텍스트는 critique LLM에 그대로 전달되므로
    여기서는 executor 제어용 수치만 뽑아낸다.
    """
    threshold = DEFAULT_THRESHOLD
    max_iter = DEFAULT_MAX_ITER
    on_fail = "continue"  # "abort" or "continue"

    if not policy_text:
        return {"threshold": threshold, "max_iter": max_iter, "on_fail": on_fail}

    text = policy_text

    m = re.search(r'(?:점수|score|임계값)[^0-9]*([0-9]*\.?[0-9]+)', text)
    if m:
        try:
            v = float(m.group(1))
            if 0.0 <= v <= 1.0:
                threshold = v
        except ValueError:
            pass

    m = re.search(r'최대\s*(\d+)\s*회', text)
    if m:
        try:
            max_iter = min(int(m.group(1)), HARD_MAX_ITER)
        except ValueError:
            pass

    if "중단" in text or "abort" in text.lower():
        on_fail = "abort"

    return {"threshold": threshold, "max_iter": max_iter, "on_fail": on_fail}


def load_step_policy(workflow_filename: str, step_heading: str) -> Dict[str, Any]:
    """워크플로우 단계의 검증 기준/정책을 로드.

    Returns:
        {
            "criteria": str,  # 검증 기준 텍스트 (자연어, critique 프롬프트에 주입)
            "policy_text": str,  # 검증 정책 텍스트 (자연어, critique 프롬프트에 주입)
            "threshold": float,
            "max_iter": int,
            "on_fail": str,
        }
    """
    section = load_workflow_section(workflow_filename, step_heading)
    criteria = extract_field(section, "검증 기준") if section else None
    policy_text = extract_field(section, "검증 정책") if section else None

    parsed = parse_policy(policy_text)
    return {
        "criteria": criteria or "",
        "policy_text": policy_text or "기본 정책 적용",
        **parsed,
    }


async def iterate_generate_critique(
    generate_prompt: str,
    critique_prompt: Callable[[str], str],
    revise_prompt: Callable[[str, Dict[str, Any]], str],
    max_iter: int = DEFAULT_MAX_ITER,
    threshold: float = DEFAULT_THRESHOLD,
    on_fail: str = "continue",
) -> Tuple[str, List[Dict[str, Any]]]:
    """생성 → 비판적 평가 → 수정 루프.

    Args:
        generate_prompt: 초기 생성 프롬프트
        critique_prompt: (draft) -> 평가 프롬프트. LLM은 다음 JSON을 반환:
            {"score": 0.0~1.0, "pass": bool, "critical": [...], "suggestions": [...], "reasoning": "..."}
        revise_prompt: (draft, critique) -> 수정 프롬프트
        max_iter: 최대 수정 횟수 (HARD_MAX_ITER로 클램프됨)
        threshold: pass 판정 임계값 (score >= threshold 이고 critical 비어있어야 pass)
        on_fail: 임계값 미달 시 동작 — "continue" (현재 결과 유지) | "abort" (워크플로우 중단)

    Returns:
        (final_draft, trail) — trail은 반복마다의 평가 결과 기록
    """
    max_iter = min(max(max_iter, 0), HARD_MAX_ITER)

    # 개발/디버깅용: AGENT_SKIP_CRITIQUE=1 이면 평가 루프 건너뛰고 초안만 반환
    import os
    if os.environ.get("AGENT_SKIP_CRITIQUE", "").strip() in ("1", "true", "TRUE", "yes"):
        draft = await call_llm(generate_prompt)
        return draft, [{"iteration": 0, "skipped": True, "reason": "AGENT_SKIP_CRITIQUE env"}]

    draft = await call_llm(generate_prompt)
    trail: List[Dict[str, Any]] = []

    for i in range(max_iter + 1):
        try:
            critique_raw = await call_llm(critique_prompt(draft))
            critique = _extract_json_obj(critique_raw)
        except Exception as e:
            log.error("critique LLM error: %s", e)
            critique = None

        if not isinstance(critique, dict):
            trail.append({
                "iteration": i + 1,
                "pass": True,
                "score": None,
                "critical": [],
                "suggestions": [],
                "note": "평가 파싱 실패, 현재 결과로 종료",
            })
            break

        critical = critique.get("critical")
        suggestions = critique.get("suggestions", [])
        if critical is None:
            critical = critique.get("issues", [])

        # score 처리 (legacy: score 없으면 None)
        score = critique.get("score")
        if isinstance(score, (int, float)):
            score = float(score)
        else:
            score = None

        # pass 판정: critical 우선, 그 다음 score
        if critical:
            passed = False
        elif score is not None:
            passed = score >= threshold
        else:
            # legacy: score 없으면 critical 비어있는 것만으로 통과
            passed = True

        trail.append({
            "iteration": i + 1,
            "pass": passed,
            "score": score,
            "threshold": threshold,
            "critical": critical,
            "suggestions": suggestions,
            "reasoning": critique.get("reasoning", ""),
        })

        if passed:
            break

        if i >= max_iter:
            note = f"최대 반복({max_iter}회) 초과, 마지막 생성물로 종료"
            if score is not None:
                note += f" (최종 score {score:.2f}, 임계값 {threshold:.2f} 미달)"
            trail[-1]["note"] = note
            trail[-1]["on_fail"] = on_fail
            break

        try:
            critique_for_revise = {**critique, "issues": critical}
            draft = await call_llm(revise_prompt(draft, critique_for_revise))
        except Exception as e:
            log.error("revise LLM error: %s", e)
            trail[-1]["note"] = f"수정 단계 실패: {e}"
            break

    return draft, trail


def trail_summary(trail: List[Dict[str, Any]]) -> str:
    """사용자용 메시지로 trail 요약."""
    if not trail:
        return ""
    last = trail[-1]
    n = len(trail)
    sug_count = len(last.get("suggestions", []))
    sug_note = f", 개선 제안 {sug_count}건" if sug_count else ""
    score = last.get("score")
    score_note = f", score {score:.2f}" if isinstance(score, float) else ""

    if last.get("pass"):
        if n == 1:
            return f"검증 통과 (1회차{score_note}{sug_note})"
        return f"검증 통과 ({n}회차, {n-1}회 수정{score_note}{sug_note})"

    threshold = last.get("threshold", DEFAULT_THRESHOLD)
    on_fail = last.get("on_fail", "continue")
    base = f"검증 미통과 ({n}회 시도{score_note}, 임계값 {threshold:.2f} 미달"
    if on_fail == "abort":
        return base + ", 워크플로우 중단)"
    return base + ", 마지막 결과 사용)"


class WorkflowAborted(Exception):
    """검증 정책상 워크플로우를 중단해야 할 때 raise."""

    def __init__(self, message: str, trail: List[Dict[str, Any]]):
        super().__init__(message)
        self.trail = trail


def check_abort(trail: List[Dict[str, Any]]) -> None:
    """trail 끝 항목이 abort 정책 + 미통과면 WorkflowAborted를 raise."""
    if not trail:
        return
    last = trail[-1]
    if not last.get("pass") and last.get("on_fail") == "abort":
        raise WorkflowAborted(trail_summary(trail), trail)
