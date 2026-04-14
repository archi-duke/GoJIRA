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
    provider: str = "openai"         # "openai" | "gemini" | 기타 라벨 (정보용)
    api_key_env: str = ""            # 읽어올 환경변수명. 비어있으면 OPENAI_API_KEY 사용


class Settings(BaseSettings):
    # 서버
    agent_service_port: int = Field(default=9002, validation_alias="AGENT_SERVICE_PORT")

    # MongoDB
    mongo_uri: str = Field(default="mongodb://localhost:27017", validation_alias="GOJIRA_MONGO_URI")
    mongo_db: str = Field(default="gojira", validation_alias="GOJIRA_MONGO_DB")

    # API 엔드포인트
    gojira_api_url: str = Field(default="http://localhost:9001/api/v2", validation_alias="GOJIRA_API_URL")
    platform_api_url: str = Field(default="http://localhost:9000/api/v2", validation_alias="GOJIRA_PLATFORM_API")

    # LLM — API 키는 llm.json 의 apiKeyEnv 로 지정한 환경변수에서 읽음 (기본: OPENAI_API_KEY)
    openai_api_key: str = Field(default="", validation_alias="OPENAI_API_KEY")
    gemini_api_key: str = Field(default="", validation_alias="GEMINI_API_KEY")
    llm_config_path: str = Field(default="", validation_alias="GOJIRA_LLM_CONFIG")
    # llm.json registry 에서 활성 프로바이더 선택 (비어있으면 registry 의 active 값 사용)
    llm_active: str = Field(default="", validation_alias="GOJIRA_LLM_ACTIVE")

    def get_llm_api_key(self, env_name: str = "") -> str:
        """llm.json 의 api_key_env 에 지정된 환경변수 값을 반환.
        지정이 없으면 OPENAI_API_KEY, 그것도 없고 env_name 이 'GEMINI_API_KEY'면 GEMINI_API_KEY 반환."""
        if env_name:
            return os.environ.get(env_name, "")
        return self.openai_api_key

    # 스킬
    skills_dir: str = Field(default="", validation_alias="GOJIRA_SKILLS_DIR")

    model_config = {
        "env_file": ".env",
        "env_file_encoding": "utf-8",
        "extra": "ignore",
        "populate_by_name": True,
    }


def _build_cfg_from_dict(raw: dict, provider_label: str = "") -> LLMConfig:
    """dict → LLMConfig 변환."""
    return LLMConfig(
        endpoint=raw.get("endpoint", LLMConfig().endpoint),
        model=raw.get("model", LLMConfig().model),
        max_tokens=raw.get("maxTokens", LLMConfig().max_tokens),
        temperature=raw.get("temperature", LLMConfig().temperature),
        headers=raw.get("headers", {"Content-Type": "application/json"}),
        auth_type=raw.get("authType", LLMConfig().auth_type),
        auth_header=raw.get("authHeader", LLMConfig().auth_header),
        provider=raw.get("provider", provider_label or LLMConfig().provider),
        api_key_env=raw.get("apiKeyEnv", LLMConfig().api_key_env),
    )


def _load_llm_config(s: Settings) -> LLMConfig:
    """llm.json 파일 로드.

    두 포맷 지원:
    1) registry 형식 — 여러 프로바이더 중 active 선택 (권장):
       { "active": "openai", "providers": { "openai": {...}, "gemini": {...} } }
       GOJIRA_LLM_ACTIVE 환경변수로 active override 가능
    2) flat 형식 — 단일 프로바이더 (하위 호환):
       { "endpoint": "...", "model": "...", ... }
    """
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

                # registry 형식 감지
                if isinstance(raw, dict) and "providers" in raw and isinstance(raw["providers"], dict):
                    providers = raw["providers"]
                    # 활성 프로바이더: 환경변수 > 파일의 active > providers 의 첫 번째
                    active = s.llm_active or raw.get("active") or next(iter(providers.keys()), "")
                    if active not in providers:
                        print(f"[config] active='{active}' 가 providers 에 없음 — 사용 가능: {list(providers.keys())}")
                        # fallback: 첫 번째 프로바이더
                        active = next(iter(providers.keys()), "")
                    chosen = providers.get(active, {})
                    cfg = _build_cfg_from_dict(chosen, provider_label=active)
                    print(f"[config] LLM registry 로드: {p} (active={active}, model={cfg.model})")
                    return cfg

                # flat 형식 (하위 호환)
                cfg = _build_cfg_from_dict(raw)
                print(f"[config] LLM 설정 로드: {p} (provider={cfg.provider}, model={cfg.model})")
                return cfg
            except Exception as e:
                print(f"[config] llm.json 파싱 실패 ({p}): {e}")

    print("[config] llm.json 없음 — 기본값 사용")
    return LLMConfig()


# 싱글턴
settings = Settings()
llm_config = _load_llm_config(settings)
