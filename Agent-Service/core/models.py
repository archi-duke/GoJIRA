"""공통 Pydantic 모델 — Go handlers/agent.go의 요청/응답 구조와 동일."""

from typing import Dict, List
from pydantic import BaseModel, Field


class ChatRequest(BaseModel):
    """POST /Agent/chat 요청. Go chatRequest 대응."""
    session_id: str = Field(default="", alias="sessionId")
    message: str
    context: Dict = Field(default_factory=dict)


class ChatResponse(BaseModel):
    """POST /Agent/chat 응답. Go chatResponse 대응."""
    session_id: str = Field(alias="sessionId")
    reply: str
    actions: List[Dict] = Field(default_factory=list)
    suggestions: List[str] = Field(default_factory=list)

    model_config = {"populate_by_name": True}


class AssistRequest(BaseModel):
    """POST /Agent/assist 요청."""
    page: str = ""
    action: str = ""
    context: Dict = Field(default_factory=dict)


class AssistResponse(BaseModel):
    """POST /Agent/assist 응답."""
    reply: str
    suggestions: List[Dict] = Field(default_factory=list)
