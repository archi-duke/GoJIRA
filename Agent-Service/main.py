"""GoJIRA Agent Service — FastAPI 엔트리포인트.

OpenCode 브리지 단일 모드.
LLM 호출은 OpenCode 서버(:4096) 가 직접 수행하고, 본 서비스는 SSE 번역 + 세션 라우팅만 담당한다.
"""

import logging

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from config import settings
from core.database import ensure_indexes
from opencode_bridge.router import router as agent_router

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(name)s] %(levelname)s: %(message)s",
)
log = logging.getLogger("agent")

app = FastAPI(
    title="GoJIRA Agent Service",
    version="0.2.0",
    description="GoJIRA AI Agent — OpenCode 브리지",
)

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

app.include_router(agent_router)


@app.on_event("startup")
async def startup():
    log.info("─── Agent-Service ──────────────────────")
    log.info("LISTEN            : %d (mode=opencode_bridge)", settings.agent_service_port)
    log.info("MongoDB           : %s/%s", settings.mongo_uri, settings.mongo_db)
    log.info("OpenCode peer     : %s", settings.opencode_url)
    log.info("GoJIRA-API peer   : %s", settings.gojira_api_url)
    log.info("Platform-API peer : %s", settings.platform_api_url)
    log.info("Ports overview    : docs/ports.md")
    log.info("─────────────────────────────────────────")
    ensure_indexes()


@app.get("/health")
async def health():
    return {"status": "ok", "service": "agent-service"}


if __name__ == "__main__":
    import uvicorn
    uvicorn.run("main:app", host="0.0.0.0", port=settings.agent_service_port, reload=True)
