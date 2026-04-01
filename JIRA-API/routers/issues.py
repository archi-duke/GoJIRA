import time

from fastapi import APIRouter, HTTPException, Response

from services.issue_service import (
    get_issue,
    list_issues,
    search_issues,
    create_issue,
    delete_issue,
    update_issue,
    create_issue_link,
    delete_issue_link,
)

router = APIRouter()

# 실제 JIRA 환경의 응답 지연을 시뮬레이션 (초)
SEARCH_DELAY_SEC = 5


# --- Issue CRUD ---

@router.post("/rest/api/2/issue", status_code=201)
def api_create_issue(payload: dict):
    if not payload.get("fields", {}).get("summary"):
        raise HTTPException(status_code=400, detail="summary is required")
    result = create_issue(payload)
    return result


@router.get("/rest/api/2/issue/{issue_id_or_key}")
def api_get_issue(issue_id_or_key: str):
    issue = get_issue(issue_id_or_key)
    if issue is None:
        raise HTTPException(status_code=404, detail=f"Issue Does Not Exist: {issue_id_or_key}")
    return issue


@router.put("/rest/api/2/issue/{issue_id_or_key}", status_code=204)
def api_update_issue(issue_id_or_key: str, payload: dict):
    result = update_issue(issue_id_or_key, payload)
    if result is None:
        raise HTTPException(status_code=404, detail=f"Issue Does Not Exist: {issue_id_or_key}")
    return Response(status_code=204)


@router.delete("/rest/api/2/issue/{issue_id_or_key}", status_code=204)
def api_delete_issue(issue_id_or_key: str):
    deleted = delete_issue(issue_id_or_key)
    if not deleted:
        raise HTTPException(status_code=404, detail=f"Issue Does Not Exist: {issue_id_or_key}")
    return Response(status_code=204)


# --- List & Search ---

@router.get("/rest/api/2/issue")
def api_list_issues(startAt: int = 0, maxResults: int = 50):
    time.sleep(SEARCH_DELAY_SEC)
    return list_issues(start_at=startAt, max_results=maxResults)


@router.get("/rest/api/2/search")
def api_search_issues(jql: str = "", startAt: int = 0, maxResults: int = 50):
    time.sleep(SEARCH_DELAY_SEC)
    return search_issues(jql=jql, start_at=startAt, max_results=maxResults)


# --- Issue Links ---

@router.post("/rest/api/2/issueLink", status_code=201)
def api_create_issue_link(payload: dict):
    if not payload.get("outwardIssue") or not payload.get("inwardIssue"):
        raise HTTPException(status_code=400, detail="inwardIssue and outwardIssue are required")
    success = create_issue_link(payload)
    if not success:
        raise HTTPException(status_code=404, detail="One or both issues not found")
    return Response(status_code=201)


@router.delete("/rest/api/2/issueLink/{link_id}", status_code=204)
def api_delete_issue_link(link_id: str):
    deleted = delete_issue_link(link_id)
    if not deleted:
        raise HTTPException(status_code=404, detail=f"Issue Link Does Not Exist: {link_id}")
    return Response(status_code=204)
