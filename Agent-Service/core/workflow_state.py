"""워크플로우 Belief State — LLM 이 매 턴 프로젝트 진행 상태를 인지하도록 돕는다.

design-agent.md §10, design-workflow.md §10 참조.
"""

import re
from typing import Optional, Dict, Any

from core.database import issues, projects


_PROJECT_CODE_RE = re.compile(r"\b([A-Z][A-Z0-9]{1,19})\b")


def detect_project_code(message: str) -> Optional[str]:
    """사용자 메시지에서 프로젝트 코드를 추정.

    간단한 규칙: 대문자로 시작하는 영대문자+숫자 토큰. 2자 이상.
    다수 매치 시 첫 번째를 사용. 실제 존재 여부를 projects 컬렉션에서 확인한다.
    """
    if not message:
        return None
    for m in _PROJECT_CODE_RE.findall(message):
        exists = projects().find_one(
            {"jiraCodes.main.jiraCode": m},
            {"_id": 1},
        )
        if exists:
            return m
    return None


def get_workflow_state(project_code: str) -> Dict[str, Any]:
    """프로젝트의 워크플로우 진행 상태를 구조화해 반환."""
    key_prefix = {"$regex": f"^{re.escape(project_code)}-"}

    vos_count = issues().count_documents({"key": key_prefix, "issueType": "VOS"})
    feature_count = issues().count_documents({"key": key_prefix, "issueType": "Feature"})
    requirement_count = issues().count_documents({"key": key_prefix, "issueType": "Requirement"})
    driver_count = issues().count_documents(
        {"key": key_prefix, "issueType": "Requirement", "driverSelected": True}
    )
    adsn_count = issues().count_documents({"key": key_prefix, "issueType": "Architectural Design"})
    ddsn_count = issues().count_documents({"key": key_prefix, "issueType": "Detailed Design"})

    proj = projects().find_one(
        {"jiraCodes.main.jiraCode": project_code},
        {"archSettings": 1, "_id": 0},
    ) or {}
    arch = proj.get("archSettings") or {}
    chosen = arch.get("chosenStyles") or {}
    normalized = arch.get("normalizedAttributes") or {}

    state: Dict[str, Any] = {
        "projectCode": project_code,
        "vosCount": vos_count,
        "featureCount": feature_count,
        "requirementCount": requirement_count,
        "driverCount": driver_count,
        "adsnCount": adsn_count,
        "ddsnCount": ddsn_count,
        "hasNormalizedAttributes": bool(normalized),
        "chosenStyles": chosen or None,
    }
    state["nextRecommendedStep"] = next_recommended_step(state)
    return state


def next_recommended_step(state: Dict[str, Any]) -> Optional[str]:
    """현재 상태에서 권장되는 다음 워크플로우 스킬.

    단순 선형 규칙 — LLM 이 반드시 따를 필요는 없으나 모호한 사용자 입력
    ('계속', '진행해줘') 에서 판단 근거로 활용.
    """
    if state["vosCount"] == 0:
        return "vos_collect"          # (vos_collect 내부에서 projdesc 부족을 감지해 인터뷰 제안)
    if state["featureCount"] == 0:
        return "usecase_analyze"
    if state["requirementCount"] == 0:
        return "requirement_refine"
    if state["driverCount"] == 0:
        return "driver_select"
    if not state["chosenStyles"]:
        return "style_define"
    if state["adsnCount"] == 0 and state["ddsnCount"] == 0:
        return "design_decide"
    # design_integrate / satam 은 사용자 명시 요청 시에만
    return None


def format_state_for_llm(state: Dict[str, Any]) -> str:
    """LLM system 메시지에 주입할 한국어 요약."""
    code = state["projectCode"]
    lines = [f"[워크플로우 현재 상태 — {code}]"]

    def _status(count: int, label: str) -> str:
        return f"{label}: {count}건" if count > 0 else f"{label}: 0건"

    lines.append(f"- VOS: {state['vosCount']}건")
    lines.append(f"- Feature: {state['featureCount']}건")
    lines.append(f"- Requirement: {state['requirementCount']}건 (Driver 선정: {state['driverCount']}건)")
    if state["chosenStyles"]:
        primary = ", ".join(state["chosenStyles"].get("primary") or [])
        lines.append(f"- 선정 스타일 (Primary): {primary or '미정'}")
    else:
        lines.append("- 선정 스타일: 미정")
    lines.append(f"- ADSN: {state['adsnCount']}건 / DDSN: {state['ddsnCount']}건")

    nxt = state["nextRecommendedStep"]
    if nxt:
        lines.append(f"\n다음 권장 스킬: {nxt}")
    else:
        lines.append("\n선형 워크플로우 단계 전부 완료. 사용자 요청에 따라 design_integrate / satam_analyze 등 호출.")
    return "\n".join(lines)
