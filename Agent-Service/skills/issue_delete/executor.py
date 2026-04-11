"""이슈 삭제 스킬."""

from core.database import issues


def execute(params: dict, user: str) -> dict:
    key = params.get("key", "")
    if not key:
        raise ValueError("key 필수")

    result = issues().delete_one({"key": key})
    if result.deleted_count == 0:
        raise ValueError(f"이슈 없음: {key}")
    return {"key": key, "deleted": True}
