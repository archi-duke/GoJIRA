"""Job State Pydantic 모델 — req-agent-gojira.md 3.3절 기반."""

from typing import Optional, List, Dict, Any
from datetime import datetime
from enum import Enum

from pydantic import BaseModel, Field


class JobStatus(str, Enum):
    PLANNING = "planning"
    RESEARCHING = "researching"
    EXECUTING = "executing"
    VALIDATING = "validating"
    AWAITING_APPROVAL = "awaiting_approval"
    COMPLETED = "completed"
    FAILED = "failed"


class StageStatus(str, Enum):
    PENDING = "pending"
    RUNNING = "running"
    COMPLETED = "completed"
    FAILED = "failed"
    SKIPPED = "skipped"


class ToolResult(BaseModel):
    tool: str
    params: Dict[str, Any] = Field(default_factory=dict)
    status: str = "success"  # success | failed | retrying
    result: Optional[Any] = None
    error: Optional[str] = None
    retry_count: int = 0
    duration_ms: int = 0
    timestamp: datetime = Field(default_factory=datetime.utcnow)


class StageState(BaseModel):
    id: str
    role: str = ""             # planner | researcher | executor | reviewer
    status: StageStatus = StageStatus.PENDING
    result: Optional[Any] = None
    error: Optional[str] = None
    retries: int = 0
    max_retries: int = 2


class JobState(BaseModel):
    job_id: str
    session_id: str = ""
    user_id: str = ""
    status: JobStatus = JobStatus.PLANNING
    goal: str = ""
    plan: List[StageState] = Field(default_factory=list)
    current_stage_idx: int = 0
    context: Dict[str, Any] = Field(default_factory=dict)
    artifacts: List[Dict[str, Any]] = Field(default_factory=list)
    tool_log: List[ToolResult] = Field(default_factory=list)
    errors: List[str] = Field(default_factory=list)
    created_at: datetime = Field(default_factory=datetime.utcnow)
    updated_at: datetime = Field(default_factory=datetime.utcnow)
