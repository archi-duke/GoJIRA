"""생성 → 비판적 평가 → 수정 루프 헬퍼.

각 워크플로우 스킬(vos_collect, usecase_analyze, requirement_refine)에서
생성물 품질을 비판적 평가자 관점에서 검증하고 실패 시 수정 반복한다.
"""

import re
import json
import logging
from typing import Callable, Optional, Tuple, List, Dict, Any

from core.llm import call_llm

log = logging.getLogger("agent.critique")

MAX_ITERATIONS = 2  # 초기 생성 후 최대 수정 횟수 (총 LLM 호출 최대 1 + 2×(critique+revise) = 5)


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


async def iterate_generate_critique(
    generate_prompt: str,
    critique_prompt: Callable[[str], str],
    revise_prompt: Callable[[str, Dict[str, Any]], str],
    max_iter: int = MAX_ITERATIONS,
) -> Tuple[str, List[Dict[str, Any]]]:
    """생성 → 비판적 평가 → 수정 루프.

    Args:
        generate_prompt: 초기 생성 프롬프트
        critique_prompt: (draft) -> 평가 프롬프트 — LLM이 아래 JSON을 반환해야 함
            {"pass": bool, "issues": ["..."], "reasoning": "..."}
        revise_prompt: (draft, critique) -> 수정 프롬프트
        max_iter: 최대 수정 횟수

    Returns:
        (final_draft, trail) — trail은 반복마다의 평가 결과 기록
    """
    draft = await call_llm(generate_prompt)
    trail: List[Dict[str, Any]] = []

    for i in range(max_iter + 1):  # +1 for final pass without revise
        # 평가
        try:
            critique_raw = await call_llm(critique_prompt(draft))
            critique = _extract_json_obj(critique_raw)
        except Exception as e:
            log.error("critique LLM error: %s", e)
            critique = None

        if not isinstance(critique, dict):
            # 평가 파싱 실패 — 통과로 간주하고 종료 (무한 루프 방지)
            trail.append({
                "iteration": i + 1,
                "pass": True,
                "issues": [],
                "note": "평가 파싱 실패, 현재 결과로 종료",
            })
            break

        passed = bool(critique.get("pass", False))
        trail.append({
            "iteration": i + 1,
            "pass": passed,
            "issues": critique.get("issues", []) if not passed else [],
            "reasoning": critique.get("reasoning", ""),
        })

        if passed:
            break

        if i >= max_iter:
            # 최대 반복 초과 — 마지막 draft로 종료
            trail[-1]["note"] = f"최대 반복({max_iter}회) 초과, 마지막 생성물로 종료"
            break

        # 수정
        try:
            draft = await call_llm(revise_prompt(draft, critique))
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
    if last.get("pass"):
        if n == 1:
            return "검증 통과 (1회차)"
        return f"검증 통과 ({n}회차, {n-1}회 수정)"
    return f"검증 미통과 ({n}회 시도, 마지막 결과 사용)"
