"""임베딩 모듈 — 스킬 description 과 사용자 메시지를 벡터화해 코사인 유사도 계산.

설정: Agent-Service/embeddings.json (llm.json 과 동일 패턴)
- type=local: sentence-transformers 로컬 모델
- type=openai: OpenAI 호환 endpoint (/v1/embeddings)

런타임 override: 환경변수 GOJIRA_EMBEDDING_ACTIVE
"""

from __future__ import annotations

import json
import logging
import math
import os
from pathlib import Path
from typing import List, Optional

log = logging.getLogger("agent.embeddings")

_CONFIG_PATH = Path(__file__).resolve().parent.parent / "embeddings.json"

_config: dict = {}
_provider_name: str = ""
_provider_cfg: dict = {}
_local_model = None
_openai_client = None


def _load_config() -> dict:
    global _config, _provider_name, _provider_cfg
    if not _CONFIG_PATH.exists():
        log.warning("embeddings.json 없음 → 임베딩 비활성화")
        _config = {"active": None, "providers": {}}
        return _config
    _config = json.loads(_CONFIG_PATH.read_text(encoding="utf-8"))
    active = os.environ.get("GOJIRA_EMBEDDING_ACTIVE") or _config.get("active")
    providers = _config.get("providers") or {}
    if not active or active not in providers:
        log.warning("active embedding provider %r 미정의 → 비활성화", active)
        _provider_name = ""
        _provider_cfg = {}
    else:
        _provider_name = active
        _provider_cfg = providers[active]
        log.info("embedding provider 로드: %s (type=%s, model=%s)",
                 active, _provider_cfg.get("type"), _provider_cfg.get("model"))
    return _config


_load_config()


def get_top_n() -> int:
    return int(_config.get("topN", 5))


def get_min_score() -> float:
    return float(_config.get("minScore", 0.35))


def is_enabled() -> bool:
    return bool(_provider_name and _provider_cfg)


def reload():
    """런타임 재로드 — provider 전환 후 호출."""
    global _local_model, _openai_client
    _local_model = None
    _openai_client = None
    _load_config()


# ──────────────────────────────────────────────────────────────────────────
# Provider impls
# ──────────────────────────────────────────────────────────────────────────


def _get_local_model():
    global _local_model
    if _local_model is None:
        from sentence_transformers import SentenceTransformer
        model_id = _provider_cfg.get("model")
        device = _provider_cfg.get("device", "cpu")
        log.info("sentence-transformers 로드 중: %s (device=%s)", model_id, device)
        _local_model = SentenceTransformer(model_id, device=device)
    return _local_model


def _get_openai_client():
    global _openai_client
    if _openai_client is None:
        from openai import OpenAI
        endpoint = _provider_cfg.get("endpoint", "")
        base_url = endpoint
        if base_url.endswith("/embeddings"):
            base_url = base_url[: -len("/embeddings")]
        env_key = _provider_cfg.get("apiKeyEnv", "")
        api_key = os.environ.get(env_key) if env_key else None
        # config.settings 에 있는 키 fallback
        if not api_key and env_key:
            try:
                from config import settings
                api_key = settings.get_llm_api_key(env_key)
            except Exception:
                pass
        _openai_client = OpenAI(api_key=api_key or "dummy", base_url=base_url)
    return _openai_client


# ──────────────────────────────────────────────────────────────────────────
# Public API
# ──────────────────────────────────────────────────────────────────────────


def embed_batch(texts: List[str]) -> List[List[float]]:
    """문자열 리스트 → 벡터 리스트 (정규화)."""
    if not is_enabled():
        return [[] for _ in texts]
    if not texts:
        return []

    ptype = _provider_cfg.get("type", "local")
    if ptype == "local":
        model = _get_local_model()
        vecs = model.encode(texts, normalize_embeddings=True, show_progress_bar=False).tolist()
        return vecs
    elif ptype == "openai":
        client = _get_openai_client()
        model_id = _provider_cfg.get("model")
        resp = client.embeddings.create(model=model_id, input=texts)
        vecs = [_normalize(d.embedding) for d in resp.data]
        return vecs
    else:
        raise ValueError(f"알 수 없는 embedding provider type: {ptype}")


def embed(text: str) -> List[float]:
    return embed_batch([text])[0] if text else []


def cosine(a: List[float], b: List[float]) -> float:
    if not a or not b or len(a) != len(b):
        return 0.0
    # 정규화 가정 → dot product
    return sum(x * y for x, y in zip(a, b))


def _normalize(v):
    norm = math.sqrt(sum(x * x for x in v))
    if norm <= 0:
        return list(v)
    return [x / norm for x in v]


__all__ = [
    "is_enabled",
    "get_top_n",
    "get_min_score",
    "reload",
    "embed",
    "embed_batch",
    "cosine",
]
