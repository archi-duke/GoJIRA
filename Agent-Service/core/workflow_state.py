"""워크플로우 Belief State — LLM 이 매 턴 프로젝트 진행 상태를 인지하도록 돕는다.

design-agent.md §10, design-workflow.md §10 참조.
"""

import asyncio
import logging
import re
from typing import Optional, Dict, Any

from core.database import issues, projects

log = logging.getLogger("agent.workflow_state")

_PROJECT_CODE_RE = re.compile(r"\b([A-Z][A-Z0-9]{1,19})\b")

# 워크플로우 단계별 산출 문서 경로
_STEP_DOCS = {
    "projdesc": "business/project-description.md",
    "usecase_analyze": "business/use-cases.md",
    "requirement_refine": "business/requirements.md",
    "driver_select": "architecture/drivers.md",
    "style_define": "architecture/architecture-style.md",
    "design_decide": "architecture/design-decisions.md",
    "design_integrate": "architecture/architecture-integrated.md",
}


def detect_project_code(message: str) -> Optional[str]:
    """사용자 메시지에서 프로젝트 코드를 추정."""
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


async def get_workflow_state(project_code: str) -> Dict[str, Any]:
    """프로젝트의 워크플로우 진행 상태를 구조화해 반환.

    비동기인 이유: DocRepo 에 문서 존재 여부를 확인하기 위해 platform_api 를 호출.
    DB 조회는 sync (pymongo) 이므로 즉시 실행.
    """
    key_prefix = {"$regex": f"^{re.escape(project_code)}-"}

    vos_count = issues().count_documents({"key": key_prefix, "issueType": "VOS"})
    feature_count = issues().count_documents({"key": key_prefix, "issueType": "Feature"})
    requirement_count = issues().count_documents({"key": key_prefix, "issueType": "Requirement"})
    fr_count = issues().count_documents({"key": key_prefix, "issueType": "Requirement", "reqtype": "FR"})
    nfr_count = issues().count_documents({"key": key_prefix, "issueType": "Requirement", "reqtype": "NFR"})
    cnst_count = issues().count_documents({"key": key_prefix, "issueType": "Requirement", "reqtype": "CNST"})
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
    integrated_done = bool(arch.get("designIntegrated"))

    primary_styles = chosen.get("primary") or []
    secondary_styles = chosen.get("secondary") or []
    supporting_styles = chosen.get("supporting") or []

    # 문서 존재 여부 (DocRepo 조회)
    documents_present = await _check_documents(project_code)

    # 완료 플래그 — DB/파일 상태에서 파생
    completed_steps = {
        "projdesc": documents_present.get("business/project-description.md", False),
        "vos_collect": vos_count > 0,
        "usecase_analyze": feature_count > 0,
        "requirement_refine": requirement_count > 0,
        "driver_select": bool(normalized) and driver_count > 0,
        "style_define": bool(primary_styles),
        "design_decide": (adsn_count + ddsn_count) > 0,
        "design_integrate": integrated_done,
    }

    state: Dict[str, Any] = {
        "projectCode": project_code,
        "vosCount": vos_count,
        "featureCount": feature_count,
        "requirementCount": requirement_count,
        "requirementBreakdown": {
            "FR": fr_count,
            "NFR": nfr_count,
            "CNST": cnst_count,
        },
        "driverCount": driver_count,
        "adsnCount": adsn_count,
        "ddsnCount": ddsn_count,
        "hasNormalizedAttributes": bool(normalized),
        "chosenStyles": chosen or None,
        "chosenStylesCount": {
            "primary": len(primary_styles),
            "secondary": len(secondary_styles),
            "supporting": len(supporting_styles),
        },
        "designIntegrated": integrated_done,
        "documentsPresent": documents_present,
        "completedSteps": completed_steps,
    }
    state["nextRecommendedStep"] = next_recommended_step(state)
    return state


async def _check_documents(project_code: str) -> Dict[str, bool]:
    """산출 문서 존재 여부를 DocRepo 트리에서 확인. 실패 시 빈 dict."""
    try:
        from clients.platform_api import PlatformClient
        client = PlatformClient()
        tree = await client.get_file_tree(project_code, ref="main")
        # Gitea/Gogs 스타일: {tree: [{path, type: blob|tree, ...}]}
        entries = tree.get("tree") if isinstance(tree, dict) else tree
        paths = {e.get("path") for e in (entries or []) if isinstance(e, dict) and e.get("type") == "blob"}
        return {doc: (doc in paths) for doc in _STEP_DOCS.values()}
    except Exception as e:
        log.warning("문서 존재 확인 실패: %s", e)
        return {doc: False for doc in _STEP_DOCS.values()}


def next_recommended_step(state: Dict[str, Any]) -> Optional[str]:
    """현재 상태에서 권장되는 다음 워크플로우 스킬."""
    done = state.get("completedSteps") or {}
    if not done.get("vos_collect"):
        return "vos_collect"
    if not done.get("usecase_analyze"):
        return "usecase_analyze"
    if not done.get("requirement_refine"):
        return "requirement_refine"
    if not done.get("driver_select"):
        return "driver_select"
    if not done.get("style_define"):
        return "style_define"
    if not done.get("design_decide"):
        return "design_decide"
    if not done.get("design_integrate"):
        return "design_integrate"
    return None


def format_state_for_llm(state: Dict[str, Any]) -> str:
    """LLM system 메시지에 주입할 한국어 요약."""
    code = state["projectCode"]
    lines = [f"[워크플로우 현재 상태 — {code}]"]

    done = state.get("completedSteps") or {}
    docs = state.get("documentsPresent") or {}
    rb = state.get("requirementBreakdown") or {}
    csc = state.get("chosenStylesCount") or {}

    def _mark(b: bool) -> str:
        return "✅" if b else "⬜"

    lines.append("\n■ 단계 완료 체크리스트")
    lines.append(f"  {_mark(done.get('projdesc'))} project-description (문서 {_mark(docs.get('business/project-description.md'))})")
    lines.append(f"  {_mark(done.get('vos_collect'))} vos_collect (VOS {state['vosCount']}건)")
    lines.append(f"  {_mark(done.get('usecase_analyze'))} usecase_analyze (Feature {state['featureCount']}건, 문서 {_mark(docs.get('business/use-cases.md'))})")
    lines.append(f"  {_mark(done.get('requirement_refine'))} requirement_refine (FR {rb.get('FR',0)}/NFR {rb.get('NFR',0)}/CNST {rb.get('CNST',0)}, 문서 {_mark(docs.get('business/requirements.md'))})")
    lines.append(f"  {_mark(done.get('driver_select'))} driver_select (Driver {state['driverCount']}건, 문서 {_mark(docs.get('architecture/drivers.md'))})")
    lines.append(f"  {_mark(done.get('style_define'))} style_define (Primary {csc.get('primary',0)}/Sec {csc.get('secondary',0)}/Sup {csc.get('supporting',0)}, 문서 {_mark(docs.get('architecture/architecture-style.md'))})")
    lines.append(f"  {_mark(done.get('design_decide'))} design_decide (ADSN {state['adsnCount']}/DDSN {state['ddsnCount']}, 문서 {_mark(docs.get('architecture/design-decisions.md'))})")
    lines.append(f"  {_mark(done.get('design_integrate'))} design_integrate (문서 {_mark(docs.get('architecture/architecture-integrated.md'))})")

    nxt = state["nextRecommendedStep"]
    if nxt:
        lines.append(f"\n다음 권장 스킬: {nxt}")
        lines.append("위 체크리스트에서 ⬜ 인 단계는 **아직 실행되지 않았다**. '완료했다'는 직전 응답/문장은 무시하고 반드시 해당 스킬을 호출하라.")
    else:
        lines.append("\n선형 워크플로우 단계 전부 완료.")
    return "\n".join(lines)
