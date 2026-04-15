"""Phase B — 워크플로우 State Machine 실행기 (골격).

책임 범위 (Step 1):
- Job load / start / resume
- 현재 stage 조회
- 스킬 실행 결과의 status 를 기반으로 완료/실패/대기 판정
- 다음 stage 로 전이

본격 stage 별 핸들러(인터뷰 누적 등)는 Step 2 이후에서 각 stage 에 맞게 확장한다.
엔진은 스킬을 **직접 호출하지 않는다** — agents/streaming.py 가 스킬 호출 결과를
engine.advance(job, stage_id, skill_result) 로 전달하는 형태로 결합한다.
"""

import logging
from typing import Optional, Dict, Any

from core import job_store
from core.workflows import get_workflow, get_stage, next_stage_id
from core.workflow_state import get_workflow_state

log = logging.getLogger("agent.workflow_engine")


# ---------- Job 진입/재개 ----------

def load_or_start_job(
    project_code: str,
    workflow_id: str = "business_design",
    created_by: str = "anonymous",
) -> Dict[str, Any]:
    """진행 중 Job 이 있으면 재개, 없으면 시작 stage 를 결정해 새 Job 생성."""
    existing = job_store.get_active_job(project_code, workflow_id)
    if existing:
        log.info("workflow job 재개: %s stage=%s", existing["jobId"], existing.get("currentStage"))
        return existing
    # 시작 stage: 워크플로우 정의의 첫 stage 고정 (Step 1 간단화)
    wf = get_workflow(workflow_id)
    first_stage = wf["stages"][0]["id"]
    return job_store.create_job(project_code, workflow_id, first_stage, created_by)


async def resolve_starting_stage(project_code: str, workflow_id: str = "business_design") -> str:
    """belief state 기준으로 아직 완료되지 않은 첫 stage 를 반환. 전부 완료면 마지막 stage id.

    새 Job 생성 직후, 이미 일부 단계가 DB/파일에 존재할 경우 엔진이 그 지점부터 시작하도록.
    """
    wf = get_workflow(workflow_id)
    state = await get_workflow_state(project_code)
    done = state.get("completedSteps") or {}
    stage_to_flag = {
        "projdesc": "projdesc",
        "vos": "vos_collect",
        "usecase": "usecase_analyze",
        "requirement": "requirement_refine",
        "driver": "driver_select",
        "style": "style_define",
        "design_decide": "design_decide",
        "design_integrate": "design_integrate",
        "satam": "satam_analyze",
    }
    for st in wf["stages"]:
        flag = stage_to_flag.get(st["id"])
        if flag and not done.get(flag):
            return st["id"]
    # 전부 완료
    return wf["stages"][-1]["id"]


# ---------- stage 조회 ----------

def current_stage(job: Dict[str, Any]) -> Dict[str, Any]:
    return get_stage(job["workflowId"], job["currentStage"])


def stage_state(job: Dict[str, Any], stage_id: str = None) -> Dict[str, Any]:
    sid = stage_id or job["currentStage"]
    return (job.get("stageState") or {}).get(sid) or {}


# ---------- 전이 ----------

async def advance(job: Dict[str, Any], skill_result: Dict[str, Any]) -> Dict[str, Any]:
    """스킬 실행 결과를 받아 현재 stage 의 상태를 갱신하고 필요 시 다음 stage 로 전이.

    반환: { job, transitioned: bool, completed: bool, status: ..., nextStage: ... }
    """
    job_id = job["jobId"]
    stage_id = job["currentStage"]
    status = (skill_result or {}).get("status", "completed")

    job_store.append_history(job_id, {
        "stage": stage_id,
        "action": "skill_result",
        "status": status,
        "blockedAt": skill_result.get("blockedAt"),
    })

    if status == "failed":
        job_store.update_stage_state(job_id, stage_id, {"status": "failed"})
        job_store.mark_failed(job_id, f"stage {stage_id} failed")
        return {"job": job_store.get_job(job_id), "transitioned": False, "completed": False, "status": "failed"}

    if status == "blocked":
        # 진행 불가 — Job 은 paused 로 두고 사용자/엔진 재시도 대기
        job_store.update_stage_state(job_id, stage_id, {"status": "blocked", "blockedAt": skill_result.get("blockedAt")})
        job_store.set_status(job_id, "paused", reason=f"blocked at {stage_id}")
        return {"job": job_store.get_job(job_id), "transitioned": False, "completed": False, "status": "blocked"}

    if status == "partial":
        # 같은 stage 유지. stageState 만 업데이트하고 대기
        job_store.update_stage_state(job_id, stage_id, {"status": "partial"})
        return {"job": job_store.get_job(job_id), "transitioned": False, "completed": False, "status": "partial"}

    # completed — belief state 로 다시 한번 확인 후 전이
    verified = await _verify_stage_completion(job, stage_id)
    if not verified:
        job_store.update_stage_state(job_id, stage_id, {"status": "claimed_completed_unverified"})
        return {"job": job_store.get_job(job_id), "transitioned": False, "completed": False, "status": "partial"}

    job_store.update_stage_state(job_id, stage_id, {"status": "completed"})
    nxt = next_stage_id(job["workflowId"], stage_id)
    if nxt is None:
        job_store.mark_completed(job_id)
        return {"job": job_store.get_job(job_id), "transitioned": True, "completed": True, "status": "completed", "nextStage": None}

    job_store.set_current_stage(job_id, nxt)
    job_store.set_status(job_id, "in_progress")
    job_store.append_history(job_id, {"stage": nxt, "action": "entered"})
    return {"job": job_store.get_job(job_id), "transitioned": True, "completed": False, "status": "completed", "nextStage": nxt}


async def _verify_stage_completion(job: Dict[str, Any], stage_id: str) -> bool:
    """belief state 로 해당 stage 의 completion 플래그를 재확인.

    workflow_state 의 completedSteps 와 stage.id 매핑. Step 1 은 단순 매핑만 구현.
    """
    mapping = {
        "projdesc": "projdesc",
        "vos": "vos_collect",
        "usecase": "usecase_analyze",
        "requirement": "requirement_refine",
        "driver": "driver_select",
        "style": "style_define",
        "design_decide": "design_decide",
        "design_integrate": "design_integrate",
        "satam": "satam_analyze",
    }
    flag = mapping.get(stage_id)
    if not flag:
        return True
    state = await get_workflow_state(job["projectCode"])
    return bool((state.get("completedSteps") or {}).get(flag))


# ---------- 편의 ----------

def describe(job: Dict[str, Any]) -> Dict[str, Any]:
    """LLM/프론트 노출용 요약."""
    st = current_stage(job)
    return {
        "jobId": job["jobId"],
        "workflowId": job["workflowId"],
        "projectCode": job["projectCode"],
        "status": job["status"],
        "currentStage": job["currentStage"],
        "currentSkill": st["skill"],
        "stageState": stage_state(job),
    }
