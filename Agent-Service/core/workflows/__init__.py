"""Phase B — 명시적 워크플로우 정의 모음.

각 워크플로우는 선형 stage 리스트 (또는 분기) 로 구성되고, engine 이 실행한다.
"""

from typing import Optional

from core.workflows.business_design import BUSINESS_DESIGN


WORKFLOWS = {
    BUSINESS_DESIGN["id"]: BUSINESS_DESIGN,
}


def get_workflow(workflow_id: str) -> dict:
    wf = WORKFLOWS.get(workflow_id)
    if not wf:
        raise KeyError(f"Unknown workflow: {workflow_id}")
    return wf


def get_stage(workflow_id: str, stage_id: str) -> dict:
    wf = get_workflow(workflow_id)
    for st in wf["stages"]:
        if st["id"] == stage_id:
            return st
    raise KeyError(f"Unknown stage {stage_id} in workflow {workflow_id}")


def next_stage_id(workflow_id: str, current_stage_id: str) -> Optional[str]:
    wf = get_workflow(workflow_id)
    ids = [s["id"] for s in wf["stages"]]
    try:
        idx = ids.index(current_stage_id)
    except ValueError:
        return None
    return ids[idx + 1] if idx + 1 < len(ids) else None
