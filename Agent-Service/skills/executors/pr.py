"""PR 스킬 실행기 — Go executor.go PR 관련 함수 대응."""

import logging

from clients.platform_api import PlatformClient

log = logging.getLogger("agent.skills.pr")


async def exec_pr_list(params: dict, user: str) -> dict:
    code = params.get("projectCode", "")
    if not code:
        raise ValueError("projectCode 필수")

    state = params.get("state", "open")
    client = PlatformClient()
    prs = await client.list_prs(code, state=state)

    results = []
    for p in prs:
        u = ""
        if isinstance(p.get("user"), dict):
            u = p["user"].get("login", "")
        results.append({
            "number": p.get("number", 0),
            "title": p.get("title", ""),
            "state": p.get("state", ""),
            "user": u,
        })
    return {"projectCode": code, "state": state, "count": len(results), "prs": results}


async def exec_pr_create(params: dict, user: str) -> dict:
    code = params.get("projectCode", "")
    if not code:
        raise ValueError("projectCode 필수")

    title = params.get("title", f"[{code}] 설계문서 배포 요청")
    body = params.get("body", "")

    client = PlatformClient()
    result = await client.create_pr(code, title=title, body=body, head=f"docs/{user}", base="main", user=user)

    log.info("pr_create %s #%s", code, result.get("number"))
    return {
        "projectCode": code,
        "prNumber": result.get("number"),
        "prURL": result.get("html_url"),
        "title": title,
    }
