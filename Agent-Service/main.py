"""GoJIRA Agent Service — FastAPI 엔트리포인트."""

import logging

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from config import settings
from core.database import ensure_indexes
from skills.registry import get_registry
from routers.agent import router as agent_router

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(name)s] %(levelname)s: %(message)s",
)
log = logging.getLogger("agent")

app = FastAPI(
    title="GoJIRA Agent Service",
    version="0.1.0",
    description="GoJIRA AI Agent — Python 기반 오케스트레이터",
)

# CORS — 프론트엔드 개발 환경 대응
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
    log.info("Agent Service 시작 (port=%d)", settings.agent_service_port)
    ensure_indexes()
    registry = get_registry()
    log.info("스킬 %d개 로드 완료", len(registry._order))


@app.get("/health")
async def health():
    return {"status": "ok", "service": "agent-service"}


if __name__ == "__main__":
    import uvicorn
    uvicorn.run("main:app", host="0.0.0.0", port=settings.agent_service_port, reload=True)
