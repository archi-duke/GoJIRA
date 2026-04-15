"""이슈 수정 스킬."""

from core import issuestore


async def execute(params: dict, user: str) -> dict:
    key = params.get("key", "")
    if not key:
        raise ValueError("key 필수")

    doc = issuestore.get_issue(key)
    if not doc:
        raise ValueError(f"이슈 없음: {key}")

    changed = []
    req_type = params.get("reqType", "")
    if req_type:
        doc.reqtype = req_type
        changed.append("reqType")

    for f in ("summary", "description", "status", "priority", "assignee"):
        v = params.get(f, "")
        if v:
            setattr(doc, f, v)
            changed.append(f)

    if not changed:
        return {"key": key, "message": "변경 없음"}

    await issuestore.save_issue(doc, user, commit_message=f"[update] {key} ({', '.join(changed)})")
    return {"key": key, "changed": changed}
