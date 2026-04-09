"""환경 설정 — Go GoJIRA-API와 동일한 환경변수명 사용."""

import json
import os
from pathlib import Path
from typing import Optional, Dict, List

from pydantic_settings import BaseSettings
from pydantic import BaseModel, Field


class LLMConfig(BaseModel):
    endpoint: str = "https://api.openai.com/v1/chat/completions"
    model: str = "gpt-4o-mini"
    max_tokens: int = 500
    temperature: float = 0.3
    headers: Dict[str, str] = Field(default_factory=lambda: {"Content-Type": "application/json"})
    auth_type: str = "bearer"        # "bearer" | "header" | "none"
    auth_header: str = ""            # authType이 "header"일 때 사용할 헤더 이름


class Settings(BaseSettings):
    # 서버
    agent_service_port: int = Field(default=9002, validation_alias="AGENT_SERVICE_PORT")

    # MongoDB
    mongo_uri: str = Field(default="mongodb://localhost:27017", validation_alias="GOJIRA_MONGO_URI")
    mongo_db: str = Field(default="gojira", validation_alias="GOJIRA_MONGO_DB")

    # API 엔드포인트
    gojira_api_url: str = Field(default="http://localhost:9001/api/v2", validation_alias="GOJIRA_API_URL")
    platform_api_url: str = Field(default="http://localhost:9000/api/v2", validation_alias="GOJIRA_PLATFORM_API")

    # LLM
    openai_api_key: str = Field(default="", validation_alias="OPENAI_API_KEY")
    llm_config_path: str = Field(default="", validation_alias="GOJIRA_LLM_CONFIG")

    # 스킬
    skills_dir: str = Field(default="", validation_alias="GOJIRA_SKILLS_DIR")

    model_config = {
        "env_file": ".env",
        "env_file_encoding": "utf-8",
        "extra": "ignore",
        "populate_by_name": True,
    }


def _load_llm_config(s: Settings) -> LLMConfig:
    """llm.json 파일 로드 — Go LoadLLMConfig()와 동일한 탐색 순서."""
    search_paths: List[str] = []

    if s.llm_config_path:
        search_paths.append(s.llm_config_path)

    here = Path(__file__).parent
    search_paths.append(str(here / "llm.json"))
    search_paths.append(str(here.parent / "GoJIRA-API" / "config" / "llm.json"))

    for p in search_paths:
        if os.path.isfile(p):
            try:
                with open(p) as f:
                    raw = json.load(f)
                cfg = LLMConfig(
                    endpoint=raw.get("endpoint", LLMConfig().endpoint),
                    model=raw.get("model", LLMConfig().model),
                    max_tokens=raw.get("maxTokens", LLMConfig().max_tokens),
                    temperature=raw.get("temperature", LLMConfig().temperature),
                    headers=raw.get("headers", {"Content-Type": "application/json"}),
                    auth_type=raw.get("authType", LLMConfig().auth_type),
                    auth_header=raw.get("authHeader", LLMConfig().auth_header),
                )
                print(f"[config] LLM 설정 로드: {p} (model={cfg.model})")
                return cfg
            except Exception as e:
                print(f"[config] llm.json 파싱 실패 ({p}): {e}")

    print("[config] llm.json 없음 — 기본값 사용")
    return LLMConfig()


# 싱글턴
settings = Settings()
llm_config = _load_llm_config(settings)
