"""이슈 조회 스킬."""

import re

from core.database import issues


def execute(params: dict, user: str) -> dict:
    code = params.get("projectCode", "")
    if not code:
        raise ValueError("projectCode는 필수입니다")

    filt = {"key": {"$regex": f"^{re.escape(code)}-"}}
    issue_type = params.get("issueType", "")
    if issue_type:
        filt["issueType"] = issue_type

    req_type = params.get("reqType", "")
    if req_type:
        filt["issueType"] = "Requirement"
        filt["reqtype"] = req_type

    cursor = issues().find(filt)
    results = []
    for issue in cursor:
        results.append({
            "key": issue.get("key", ""),
            "issueType": issue.get("issueType", ""),
            "reqType": issue.get("reqtype", ""),
            "summary": issue.get("summary", ""),
            "status": issue.get("status", ""),
            "assignee": issue.get("assignee", ""),
        })
    return {"projectCode": code, "count": len(results), "issues": results}
