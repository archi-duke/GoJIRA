"""프로젝트 스킬 실행기 — Go executor.go execProjectQuery 대응."""

from core.database import projects


def exec_project_query(params: dict, user: str) -> dict:
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
