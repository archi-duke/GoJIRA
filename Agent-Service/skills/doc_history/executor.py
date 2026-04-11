"""문서 이력 조회 스킬."""

from clients.platform_api import PlatformClient


async def execute(params: dict, user: str) -> dict:
    code = params.get("projectCode", "")
    if not code:
        raise ValueError("projectCode 필수")

    branch = params.get("branch", "main")
    limit = int(params.get("limit", "20"))

    client = PlatformClient()
    commits = await client.list_commits(code, branch=branch, per_page=limit)

    results = []
    if isinstance(commits, list):
        for c in commits:
            commit_info = c.get("commit", {})
            author_info = commit_info.get("author", {})
            results.append({
                "sha": (c.get("sha", "") or "")[:7],
                "message": commit_info.get("message", "").strip().split("\n")[0],
                "author": author_info.get("name", ""),
                "date": author_info.get("date", ""),
            })

    return {"projectCode": code, "branch": branch, "count": len(results), "commits": results}
