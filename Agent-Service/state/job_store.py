"""Job State Store — MongoDB agent_jobs CRUD."""

import logging
from typing import Optional, Dict, Any
from datetime import datetime

from core.database import agent_jobs
from state.models import JobState, JobStatus, StageStatus, ToolResult

log = logging.getLogger("agent.state")


def create_job(job: JobState) -> str:
    doc = job.dict()
    doc["_id"] = job.job_id
    agent_jobs().insert_one(doc)
    log.info("Job 생성: %s (goal=%s)", job.job_id, job.goal)
    return job.job_id


def get_job(job_id: str) -> Optional[JobState]:
    doc = agent_jobs().find_one({"_id": job_id})
    if not doc:
        return None
    doc.pop("_id", None)
    return JobState(**doc)


def update_job_status(job_id: str, status: JobStatus):
    agent_jobs().update_one(
        {"_id": job_id},
        {"$set": {"status": status.value, "updated_at": datetime.utcnow()}},
    )


def update_stage(job_id: str, stage_idx: int, status: StageStatus, result: Any = None, error: str = None):
    update = {
        f"plan.{stage_idx}.status": status.value,
        "updated_at": datetime.utcnow(),
    }
    if result is not None:
        update[f"plan.{stage_idx}.result"] = result
    if error is not None:
        update[f"plan.{stage_idx}.error"] = error
    agent_jobs().update_one({"_id": job_id}, {"$set": update})


def advance_stage(job_id: str, new_idx: int):
    agent_jobs().update_one(
        {"_id": job_id},
        {"$set": {"current_stage_idx": new_idx, "updated_at": datetime.utcnow()}},
    )


def add_artifact(job_id: str, artifact: Dict[str, Any]):
    agent_jobs().update_one(
        {"_id": job_id},
        {"$push": {"artifacts": artifact}, "$set": {"updated_at": datetime.utcnow()}},
    )


def add_tool_result(job_id: str, tool_result: ToolResult):
    agent_jobs().update_one(
        {"_id": job_id},
        {"$push": {"tool_log": tool_result.dict()}, "$set": {"updated_at": datetime.utcnow()}},
    )


def add_error(job_id: str, error: str):
    agent_jobs().update_one(
        {"_id": job_id},
        {"$push": {"errors": error}, "$set": {"updated_at": datetime.utcnow()}},
    )


def mark_completed(job_id: str):
    update_job_status(job_id, JobStatus.COMPLETED)
    log.info("Job 완료: %s", job_id)


def mark_failed(job_id: str, reason: str):
    update_job_status(job_id, JobStatus.FAILED)
    add_error(job_id, reason)
    log.info("Job 실패: %s — %s", job_id, reason)
