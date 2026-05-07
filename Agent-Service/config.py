"""환경 설정 — OpenCode 브리지 모드 전용.

LLM 호출은 OpenCode 서버(:4096)가 자체 정책으로 수행하므로 본 서비스는 LLM 키/설정을
보유하지 않는다. legacy ReACT 모드의 LLM/registry 코드는 모두 제거됨.
"""

from pydantic_settings import BaseSettings
from pydantic import Field


class Settings(BaseSettings):
    # 서버
    agent_service_port: int = Field(default=9002, validation_alias="AGENT_SERVICE_PORT")

    # MongoDB
    mongo_uri: str = Field(default="mongodb://localhost:27017", validation_alias="GOJIRA_MONGO_URI")
    mongo_db: str = Field(default="gojira", validation_alias="GOJIRA_MONGO_DB")

    # 백엔드 API 엔드포인트
    gojira_api_url: str = Field(default="http://localhost:9001/api/v2", validation_alias="GOJIRA_API_URL")
    platform_api_url: str = Field(default="http://localhost:9000/api/v2", validation_alias="GOJIRA_PLATFORM_API")

    model_config = {
        "env_file": ".env",
        "env_file_encoding": "utf-8",
        "extra": "ignore",
        "populate_by_name": True,
    }


# 싱글턴
settings = Settings()
