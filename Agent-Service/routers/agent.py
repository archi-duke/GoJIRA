"""Agent API 라우트 — Go handlers/agent.go 라우트 대응."""

import os
import uuid
import logging
from datetime import datetime, timezone
from typing import List, Dict

from fastapi import APIRouter, Header

from core.models import ChatRequest, ChatResponse, AssistRequest, AssistResponse
from core.database import agent_sessions
from skills.registry import get_registry
from agents.orchestrator import agent_loop, staged_orchestrate, classify_intent

log = logging.getLogger("agent.router")

router = APIRouter(prefix="/api/v2/Agent", tags=["Agent"])

# 시스템 프롬프트 로드
_system_prompt = ""


def _load_system_prompt() -> str:
    global _system_prompt
    if _system_prompt:
        return _system_prompt
    paths = [
        os.path.join(os.path.dirname(__file__), "..", "prompts", "system.txt"),
        os.path.join(os.path.dirname(__file__), "..", "..", "GoJIRA-API", "prompts", "agent-system.txt"),
    ]
    for p in paths:
        if os.path.isfile(p):
            with open(p, "r", encoding="utf-8") as f:
                _system_prompt = f.read().strip()
                return _system_prompt
    _system_prompt = "당신은 GoJIRA 프로젝트 관리 시스템의 AI 에이전트입니다."
    return _system_prompt


def _build_messages(session: dict) -> List[Dict]:
    """세션 히스토리에서 LLM messages 구성 — Go buildMessages() 대응."""
    msgs = [{"role": "system", "content": _load_system_prompt()}]

    history = session.get("messages", [])
    # 최근 20개만 사용 (컨텍스트 윈도우 관리)
    if len(history) > 20:
        history = history[-20:]

    for m in history:
        role = m.get("role", "")
        if role in ("user", "assistant"):
            msgs.append({"role": role, "content": m.get("content", "")})

    return msgs


@router.post("/chat", response_model=ChatResponse)
async def chat(req: ChatRequest, x_user_id: str = Header(default="anonymous")):
    """메인 채팅 — Go AgentChatHandler 대응."""
    session_id = req.session_id or str(uuid.uuid4())
    now = datetime.now(timezone.utc)

    # 세션 로드 또는 생성
    session = agent_sessions().find_one({"_id": session_id})
    if not session:
        session = {
            "_id": session_id,
            "userId": x_user_id,
            "messages": [],
            "context": req.context,
            "createdAt": now,
            "updatedAt": now,
        }
        agent_sessions().insert_one(session)

    # 사용자 메시지 저장
    user_msg = {"role": "user", "content": req.message, "timestamp": now}
    agent_sessions().update_one(
        {"_id": session_id},
        {"$push": {"messages": user_msg}, "$set": {"updatedAt": now}},
    )
    session["messages"].append(user_msg)

    # LLM messages 구성
    messages = _build_messages(session)

    # 의도 분류 → 하이브리드 실행
    intent = classify_intent(req.message)
    log.info("intent=%s message=%.50s", intent, req.message)

    if intent == "complex":
        reply, actions = await staged_orchestrate(req.message, messages, x_user_id, session_id)
    else:
        reply, actions = await agent_loop(messages, x_user_id)

    # assistant 메시지 저장
    assistant_msg = {"role": "assistant", "content": reply, "timestamp": datetime.now(timezone.utc)}
    agent_sessions().update_one(
        {"_id": session_id},
        {"$push": {"messages": assistant_msg}, "$set": {"updatedAt": datetime.now(timezone.utc)}},
    )

    return ChatResponse(sessionId=session_id, reply=reply, actions=actions, suggestions=[])


@router.post("/assist", response_model=AssistResponse)
async def assist(req: AssistRequest, x_user_id: str = Header(default="anonymous")):
    """페이지별 보조 — Go AgentAssistHandler 대응. Phase 1: 기본 구현."""
    return AssistResponse(
        reply=f"[Agent Service] assist: page={req.page}, action={req.action}",
        suggestions=[],
    )


@router.get("/tools")
async def list_tools():
    """사용 가능한 도구 목록 — Go AgentToolsHandler 대응."""
    registry = get_registry()
    return registry.list_tools_info()


@router.get("/sessions")
async def list_sessions(x_user_id: str = Header(default="anonymous")):
    """세션 목록 — Go AgentSessionsHandler 대응."""
    cursor = agent_sessions().find(
        {"userId": x_user_id},
        {"messages": {"$slice": 1}},
    ).sort("updatedAt", -1).limit(50)

    result = []
    for doc in cursor:
        title = ""
        if doc.get("messages"):
            content = doc["messages"][0].get("content", "")
            title = content[:50] if len(content) > 50 else content
        result.append({
            "sessionId": doc["_id"],
            "title": title,
            "createdAt": str(doc.get("createdAt", "")),
            "updatedAt": str(doc.get("updatedAt", "")),
        })
    return result


@router.get("/session/{session_id}")
async def get_session(session_id: str):
    """세션 상세 — Go AgentSessionDetailHandler 대응."""
    doc = agent_sessions().find_one({"_id": session_id})
    if not doc:
        return {"error": "session not found"}
    return {
        "sessionId": doc["_id"],
        "userId": doc.get("userId"),
        "messages": doc.get("messages", []),
        "context": doc.get("context", {}),
        "createdAt": str(doc.get("createdAt", "")),
        "updatedAt": str(doc.get("updatedAt", "")),
    }
