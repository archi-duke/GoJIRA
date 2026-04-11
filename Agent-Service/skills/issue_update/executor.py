"""이슈 수정 스킬."""

from datetime import datetime

from core.database import issues


def execute(params: dict, user: str) -> dict:
    key = params.get("key", "")
    if not key:
        raise ValueError("key 필수")

    existing = issues().find_one({"key": key})
    if not existing:
        raise ValueError(f"이슈 없음: {key}")

    update = {"updated": datetime.now().strftime("%Y-%m-%dT%H:%M:%S.000+0900")}
    changed = []

    req_type = params.get("reqType", "")
    if req_type:
        update["reqtype"] = req_type
        changed.append("reqType")

    for f in ("summary", "description", "status", "priority", "assignee"):
        v = params.get(f, "")
        if v:
            update[f] = v
            changed.append(f)

    if not changed:
        return {"key": key, "message": "변경 없음"}

    issues().update_one({"key": key}, {"$set": update})
    return {"key": key, "changed": changed}
