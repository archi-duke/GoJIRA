"""이슈 생성 스킬."""

from datetime import datetime

from core.database import issues
from skills.helpers import get_next_issue_num, add_issue_to_project


def execute(params: dict, user: str) -> dict:
    code = params.get("projectCode", "")
    issue_type = params.get("issueType", "")
    summary = params.get("summary", "")
    if not code or not issue_type or not summary:
        raise ValueError("projectCode, issueType, summary 필수")

    key = f"{code}-{get_next_issue_num(code)}"
    now = datetime.now().strftime("%Y-%m-%dT%H:%M:%S.000+0900")

    doc = {
        "key": key,
        "issueType": issue_type,
        "summary": summary,
        "description": params.get("description", ""),
        "status": "To Do",
        "priority": params.get("priority", "Medium"),
        "assignee": params.get("assignee", ""),
        "reporter": user,
        "created": now,
        "updated": now,
        "labels": [],
        "issueLinks": [],
        "_source": "gojira-agent",
    }
    req_type = params.get("reqType", "")
    if req_type:
        doc["reqtype"] = req_type

    issues().update_one({"key": key}, {"$set": doc}, upsert=True)
    add_issue_to_project(code, key)
    return {"key": key, "issueType": issue_type, "summary": summary, "status": "To Do"}
