"""이슈 삭제 스킬 — YAML 제거 + Mongo 캐시 제거."""

from core import issuestore


async def execute(params: dict, user: str) -> dict:
    key = params.get("key", "")
    if not key:
        raise ValueError("key 필수")

    res = await issuestore.delete_issue(key, user)
    if not res.get("cacheDeleted") and not res.get("yamlRemoved"):
        raise ValueError(f"이슈 없음: {key}")
    return {"key": key, "deleted": True, **res}
