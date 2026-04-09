"""이슈 스킬 실행기 — Go executor.go issue 관련 함수 대응."""

import re
import logging
from datetime import datetime

from core.database import issues

log = logging.getLogger("agent.skills.issue")


def exec_issue_query(params: dict, user: str) -> dict:
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


def exec_issue_create(params: dict, user: str) -> dict:
    code = params.get("projectCode", "")
    issue_type = params.get("issueType", "")
    summary = params.get("summary", "")
    if not code or not issue_type or not summary:
        raise ValueError("projectCode, issueType, summary 필수")

    key = f"{code}-{_get_next_issue_num(code)}"
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
    log.info("issue_create %s (%s)", key, issue_type)
    return {"key": key, "issueType": issue_type, "summary": summary, "status": "To Do"}


def exec_issue_update(params: dict, user: str) -> dict:
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
    log.info("issue_update %s %s", key, changed)
    return {"key": key, "changed": changed}


def exec_issue_delete(params: dict, user: str) -> dict:
    key = params.get("key", "")
    if not key:
        raise ValueError("key 필수")

    result = issues().delete_one({"key": key})
    if result.deleted_count == 0:
        raise ValueError(f"이슈 없음: {key}")
    log.info("issue_delete %s", key)
    return {"key": key, "deleted": True}


def exec_issue_link(params: dict, user: str) -> dict:
    from_key = params.get("fromKey", "")
    to_key = params.get("toKey", "")
    link_type = params.get("linkType", "")
    if not from_key or not to_key or not link_type:
        raise ValueError("fromKey, toKey, linkType 필수")

    link_id = f"{from_key}_{link_type}_{to_key}"
    issues().update_one(
        {"key": from_key},
        {"$addToSet": {"issueLinks": {"id": link_id, "type": link_type, "outward": to_key}}},
    )
    issues().update_one(
        {"key": to_key},
        {"$addToSet": {"issueLinks": {"id": link_id, "type": link_type, "inward": from_key}}},
    )
    log.info("issue_link %s → %s (%s)", from_key, to_key, link_type)
    return {"linkId": link_id, "fromKey": from_key, "toKey": to_key, "linkType": link_type}


def _get_next_issue_num(code: str) -> int:
    """프로젝트 이슈 키에서 최대 번호 + 1 반환."""
    cursor = issues().find(
        {"key": {"$regex": f"^{re.escape(code)}-"}},
        {"key": 1},
    )
    prefix = f"{code}-"
    mx = 0
    for doc in cursor:
        k = doc.get("key", "")
        if len(k) > len(prefix):
            try:
                n = int(k[len(prefix):])
                if n > mx:
                    mx = n
            except ValueError:
                pass
    return mx + 1
