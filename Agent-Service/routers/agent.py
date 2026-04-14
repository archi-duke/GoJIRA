"""Agent API 라우트 — Go handlers/agent.go 라우트 대응."""

import os
import uuid
import logging
from datetime import datetime, timezone
from typing import List, Dict

from fastapi import APIRouter, Header, Request
from fastapi.responses import StreamingResponse

from core.models import ChatRequest, ChatResponse, AssistRequest, AssistResponse
from core.database import agent_sessions
from skills.registry import get_registry
from agents.orchestrator import agent_loop, staged_orchestrate, classify_intent
from agents.streaming import agent_loop_stream

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


@router.post("/chat/stream")
async def chat_stream(req: ChatRequest, x_user_id: str = Header(default="anonymous")):
    """스트리밍 채팅 — SSE로 진행 과정 실시간 전송."""
    session_id = req.session_id or str(uuid.uuid4())
    now = datetime.now(timezone.utc)

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

    user_msg = {"role": "user", "content": req.message, "timestamp": now}
    agent_sessions().update_one(
        {"_id": session_id},
        {"$push": {"messages": user_msg}, "$set": {"updatedAt": now}},
    )
    session["messages"].append(user_msg)
    messages = _build_messages(session)

    import json as _json

    async def event_generator():
        # 세션 ID 전송
        yield f"event: session\ndata: {_json.dumps({'sessionId': session_id})}\n\n"

        full_reply = ""
        all_actions = []

        async for chunk in agent_loop_stream(messages, x_user_id):
            yield chunk
            # done 이벤트에서 최종 결과 캡처
            if chunk.startswith("event: done"):
                data_line = chunk.split("data: ", 1)[1].split("\n")[0]
                done_data = _json.loads(data_line)
                full_reply = done_data.get("reply", "")
                all_actions = done_data.get("actions", [])

        # assistant 메시지 DB 저장
        if full_reply:
            agent_sessions().update_one(
                {"_id": session_id},
                {
                    "$push": {"messages": {
                        "role": "assistant",
                        "content": full_reply,
                        "timestamp": datetime.now(timezone.utc),
                    }},
                    "$set": {"updatedAt": datetime.now(timezone.utc)},
                },
            )

    return StreamingResponse(
        event_generator(),
        media_type="text/event-stream",
        headers={
            "Cache-Control": "no-cache",
            "X-Accel-Buffering": "no",
        },
    )


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


@router.get("/llm")
async def get_llm_config():
    """현재 활성 LLM 프로바이더 + 등록된 프로바이더 목록 반환."""
    import json
    from config import llm_config
    from pathlib import Path

    # llm.json 직접 읽어서 등록된 프로바이더 목록 구성 (registry 형식 전제)
    from config import settings as _settings
    llm_path = Path(__file__).parent.parent / "llm.json"
    providers_info: List[Dict] = []

    def _api_key_present(env_name: str) -> bool:
        if not env_name:
            return True
        # settings 필드 우선 확인 (.env 파일 로드분 반영), fallback 으로 os.environ
        field = env_name.lower()  # OPENAI_API_KEY → openai_api_key
        val = getattr(_settings, field, None)
        if val:
            return True
        return bool(os.environ.get(env_name))

    try:
        with open(llm_path) as f:
            raw = json.load(f)
        if "providers" in raw:
            for name, p in raw["providers"].items():
                env_name = p.get("apiKeyEnv", "")
                providers_info.append({
                    "name": name,
                    "model": p.get("model", ""),
                    "endpoint": p.get("endpoint", ""),
                    "apiKeyEnv": env_name,
                    "apiKeyPresent": _api_key_present(env_name),
                })
    except Exception as e:
        log.warning("llm.json 읽기 실패: %s", e)

    return {
        "active": llm_config.provider,
        "activeModel": llm_config.model,
        "providers": providers_info,
    }


@router.post("/llm")
async def switch_llm(request: Request):
    """활성 LLM 프로바이더 전환 — llm.json 의 active 를 수정하고 클라이언트 리로드."""
    import json
    from pathlib import Path
    from config import _build_cfg_from_dict
    import config as _config_mod
    import core.llm as _llm_mod

    body = await request.json()
    target = (body.get("active") or "").strip()
    if not target:
        return {"ok": False, "error": "active 필드 필수"}

    llm_path = Path(__file__).parent.parent / "llm.json"
    try:
        with open(llm_path) as f:
            raw = json.load(f)
    except Exception as e:
        return {"ok": False, "error": f"llm.json 읽기 실패: {e}"}

    providers = raw.get("providers") or {}
    if target not in providers:
        return {"ok": False, "error": f"'{target}' 프로바이더 없음. 사용 가능: {list(providers.keys())}"}

    # 파일에 active 반영
    raw["active"] = target
    try:
        with open(llm_path, "w") as f:
            json.dump(raw, f, ensure_ascii=False, indent=2)
    except Exception as e:
        return {"ok": False, "error": f"llm.json 쓰기 실패: {e}"}

    # 런타임 싱글턴 갱신 — config.llm_config 재로드 + core.llm._client 리셋
    chosen = providers[target]
    new_cfg = _build_cfg_from_dict(chosen, provider_label=target)
    _config_mod.llm_config = new_cfg
    _llm_mod.llm_config = new_cfg       # 이미 임포트된 모듈의 참조도 갱신
    _llm_mod._client = None              # 다음 호출 시 재생성

    log.info("LLM 프로바이더 전환: %s (model=%s)", target, new_cfg.model)
    return {
        "ok": True,
        "active": target,
        "activeModel": new_cfg.model,
    }


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


# 파일 경로 → 워크플로우 스킬 매핑 (워크플로우 스킬은 자체적으로 선행 검증 + 파일 저장 수행)
_DOC_WORKFLOW_SKILLS = {
    "business/use-cases.md": "usecase_analyze",
    "business/requirements.md": "requirement_refine",
}

# 워크플로우 스킬별 선행(prerequisite) 체인 — runPrerequisites=true일 때 순서대로 실행
_PREREQUISITE_CHAIN = {
    "usecase_analyze": ["vos_collect"],
    "requirement_refine": ["vos_collect", "usecase_analyze"],
}


async def _run_chain(skill_name: str, params: dict, user: str, prereqs: list):
    """선행 스킬을 순서대로 실행하고 마지막에 target 스킬 실행.

    중간 스킬이 suggestSkill을 반환하면 (= 추가 선행 필요) 체인 중단하고 그 결과 반환.
    """
    from skills.dispatcher import execute_skill
    ran = []
    for p in prereqs:
        r = await execute_skill(p, params, user)
        ran.append({
            "skill": p,
            "createdCount": r.get("createdCount") or len(r.get("created") or []),
            "message": r.get("message", ""),
        })
        if r.get("suggestSkill"):
            return {**r, "prerequisitesRan": ran, "blockedAt": p}

    target = await execute_skill(skill_name, params, user)
    return {**target, "prerequisitesRan": ran}


@router.post("/doc/enrich")
async def doc_enrich(request: Request, x_user_id: str = Header(default="anonymous")):
    """문서 AI 보강 — DocEditor의 AI생성 버튼용.

    파일별 분기:
    - business/project-description.md: 충분성 판단 → projdesc_generate(enrich) (편집기에 content 반영)
    - business/use-cases.md: usecase_analyze (Feature 이슈 생성 + 파일 저장 → refreshFile)
    - business/requirements.md: requirement_refine (FR/NFR/CNST 이슈 + 파일 저장 → refreshFile)
    """
    import re
    body = await request.json()
    project_code = body.get("projectCode", "")
    content = body.get("content", "")
    file_path = body.get("filePath", "")

    if not project_code or not file_path:
        return {"error": "projectCode, filePath 필수"}

    run_prereq = bool(body.get("runPrerequisites", False))
    log.info("doc_enrich: filePath=%r project=%s user=%s prereq=%s",
             file_path, project_code, x_user_id, run_prereq)

    from skills.dispatcher import execute_skill

    # 1. 워크플로우 스킬 분기 (use-cases / requirements)
    if file_path in _DOC_WORKFLOW_SKILLS:
        skill_name = _DOC_WORKFLOW_SKILLS[file_path]
        skill_params = {"projectCode": project_code, "user": x_user_id}
        try:
            if run_prereq:
                prereqs = _PREREQUISITE_CHAIN.get(skill_name, [])
                result = await _run_chain(skill_name, skill_params, x_user_id, prereqs)
            else:
                result = await execute_skill(skill_name, skill_params, x_user_id)
            # 워크플로우 스킬은 자체적으로 파일을 저장하므로 편집기는 파일을 재로드해야 함
            return {
                **result,
                "filePath": file_path,
                "refreshFile": True,
                "skill": skill_name,
            }
        except Exception as e:
            log.error("doc_enrich workflow error (%s): %s", skill_name, e)
            return {"error": str(e)}

    # 2. 기존 흐름: project-description.md 충분성 판단 → projdesc_generate
    stripped = re.sub(r'#[^\n]*', '', content)
    stripped = re.sub(r'>.*', '', stripped)
    stripped = re.sub(r'\(작성 필요\)', '', stripped)
    for phrase in ['기록하는 문서입니다', '기술하는 문서입니다', '반드시 작성해', '참조합니다']:
        stripped = stripped.replace(phrase, '')
    stripped = stripped.strip()

    if len(stripped) < 10:
        return {"sufficient": False, "reason": "문서 내용이 거의 없습니다.", "needsInterview": True, "message": "문서 내용이 부족합니다. 인터뷰를 통해 자동 생성할 수 있습니다."}

    from core.llm import call_llm
    judge_prompt = f"""다음 문서 내용을 읽고, 과제의 배경, 목적, 주요 이해관계자, 요구사항을 파악할 수 있는 수준인지 판단하세요.

문서 내용:
{content[:3000]}

판단 기준:
- 과제 배경이나 동기가 서술되어 있는가
- 과제 목적이 명확한가
- 주요 이해관계자나 요구사항이 언급되어 있는가

위 항목 중 2개 이상 충분히 서술되어 있으면 "예", 아니면 "아니오"로만 답하세요."""

    try:
        answer = (await call_llm(judge_prompt)).strip()
        is_sufficient = answer.startswith("예")
    except Exception as e:
        log.error("LLM judge error: %s", e)
        is_sufficient = len(stripped) >= 100

    if not is_sufficient:
        return {"sufficient": False, "reason": "과제 배경/목적/요구사항이 부족합니다.", "needsInterview": True, "message": "문서 내용이 부족합니다. 인터뷰를 통해 자동 생성할 수 있습니다."}

    try:
        result = await execute_skill("projdesc_generate", {
            "projectCode": project_code,
            "content": content,
            "mode": "enrich",
        }, x_user_id)
        return result
    except Exception as e:
        log.error("doc_enrich error: %s", e)
        return {"error": str(e)}


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
