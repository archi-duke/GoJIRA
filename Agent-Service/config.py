"""환경 설정 — OpenCode 브리지 모드 전용.

설정은 모두 .env 파일 (또는 환경변수) 에서 읽는다. 환경별로 달라야 하는 항목
(MongoDB / 백엔드 URL / OpenCode URL) 은 default 가 없어 미설정 시 startup
시점에 ValidationError 로 즉시 실패한다 (fail-fast). 임의의 localhost fallback
이 silent 하게 동작하는 것을 방지한다.
"""

from pydantic_settings import BaseSettings
from pydantic import Field


class Settings(BaseSettings):
    # ── 필수 (default 없음 — 미설정 시 startup 실패) ─────────────────────────
    mongo_uri: str = Field(validation_alias="MONGO_URI")
    gojira_api_url: str = Field(validation_alias="GOJIRA_API_URL")
    platform_api_url: str = Field(validation_alias="GOJIRA_PLATFORM_API")
    opencode_url: str = Field(validation_alias="OPENCODE_SERVER_URL")

    # ── 선택 (default 허용) ────────────────────────────────────────────────
    agent_service_port: int = Field(default=9002, validation_alias="AGENT_SERVICE_PORT")
    mongo_db: str = Field(default="gojira", validation_alias="GOJIRA_MONGO_DB")
    opencode_username: str = Field(default="opencode", validation_alias="OPENCODE_SERVER_USERNAME")
    opencode_password: str = Field(default="", validation_alias="OPENCODE_SERVER_PASSWORD")

    model_config = {
        "env_file": ".env",
        "env_file_encoding": "utf-8",
        "extra": "ignore",
        "populate_by_name": True,
    }


# 싱글턴 — import 시점에 .env 검증, 필수 항목 누락 시 ValidationError
settings = Settings()
