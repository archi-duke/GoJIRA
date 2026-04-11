"""프로젝트 조회 스킬."""

from core.database import projects


def execute(params: dict, user: str) -> dict:
    filter_code = params.get("projectCode", "")
    cursor = projects().find({})
    results = []
    for prj in cursor:
        jira_codes = prj.get("jiraCodes", {})
        main = jira_codes.get("main", {})
        code = main.get("jiraCode", "")
        if not code:
            continue
        if filter_code and code != filter_code:
            continue
        status = prj.get("status", "")
        if status == "Deleted":
            continue
        results.append({
            "code": code,
            "name": prj.get("name", ""),
            "status": status,
        })
    return {"count": len(results), "projects": results}
