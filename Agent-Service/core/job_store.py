"""Workflow Job Store — MongoDB workflow_jobs CRUD.

Phase B State Machine 전용. 기존 state/job_store.py (Plan 모드 스텁) 와 분리 운영.

문서 스키마 (docs/agent-workflow-evolution.md §3.2):
  jobId, projectCode, workflowId, status, currentStage, stageState, history, ...
"""

import logging
import uuid
from datetime import datetime
from typing import Optional, Dict, Any, List

from core.database import workflow_jobs

log = logging.getLogger("agent.job_store")


JOB_STATUSES = {"pending", "in_progress", "completed", "failed", "paused"}


def _now() -> datetime:
    return datetime.utcnow()


def create_job(
    project_code: str,
    workflow_id: str,
    starting_stage: str,
    created_by: str = "anonymous",
) -> Dict[str, Any]:
    job = {
        "_id": uuid.uuid4().hex,
        "jobId": None,
        "projectCode": project_code,
        "workflowId": workflow_id,
        "status": "in_progress",
        "currentStage": starting_stage,
        "stageState": {},
        "history": [],
        "createdBy": created_by,
        "createdAt": _now(),
        "updatedAt": _now(),
    }
    job["jobId"] = job["_id"]
    workflow_jobs().insert_one(job)
    log.info("workflow job 생성: %s project=%s wf=%s start=%s",
             job["jobId"], project_code, workflow_id, starting_stage)
    return job


def get_job(job_id: str) -> Optional[Dict[str, Any]]:
    return workflow_jobs().find_one({"_id": job_id})


def get_active_job(project_code: str, workflow_id: str) -> Optional[Dict[str, Any]]:
    """프로젝트/워크플로우의 진행 중 Job 1건 조회 (pending/in_progress/paused)."""
    return workflow_jobs().find_one({
        "projectCode": project_code,
        "workflowId": workflow_id,
        "status": {"$in": ["pending", "in_progress", "paused"]},
    })


def list_jobs(project_code: str, limit: int = 20) -> List[Dict[str, Any]]:
    cursor = workflow_jobs().find({"projectCode": project_code}).sort("updatedAt", -1).limit(limit)
    return list(cursor)


def set_current_stage(job_id: str, stage_id: str):
    workflow_jobs().update_one(
        {"_id": job_id},
        {"$set": {"currentStage": stage_id, "updatedAt": _now()}},
    )


def update_stage_state(job_id: str, stage_id: str, patch: Dict[str, Any]):
    """stageState.<stage_id> 하위 필드를 부분 업데이트.

    patch 는 { "answers.background": "...", "status": "collecting" } 처럼 하위 경로 허용.
    """
    set_doc = {"updatedAt": _now()}
    for k, v in patch.items():
        set_doc[f"stageState.{stage_id}.{k}"] = v
    workflow_jobs().update_one({"_id": job_id}, {"$set": set_doc})


def replace_stage_state(job_id: str, stage_id: str, state: Dict[str, Any]):
    workflow_jobs().update_one(
        {"_id": job_id},
        {"$set": {f"stageState.{stage_id}": state, "updatedAt": _now()}},
    )


def append_history(job_id: str, entry: Dict[str, Any]):
    entry = {**entry, "at": _now()}
    workflow_jobs().update_one(
        {"_id": job_id},
        {"$push": {"history": entry}, "$set": {"updatedAt": _now()}},
    )


def set_status(job_id: str, status: str, reason: str = None):
    if status not in JOB_STATUSES:
        raise ValueError(f"invalid status: {status}")
    upd = {"status": status, "updatedAt": _now()}
    if reason is not None:
        upd["statusReason"] = reason
    workflow_jobs().update_one({"_id": job_id}, {"$set": upd})


def mark_completed(job_id: str):
    set_status(job_id, "completed")
    log.info("workflow job 완료: %s", job_id)


def mark_failed(job_id: str, reason: str):
    set_status(job_id, "failed", reason)
    log.info("workflow job 실패: %s — %s", job_id, reason)
