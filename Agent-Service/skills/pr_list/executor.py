"""PR 목록 조회 스킬."""

from clients.platform_api import PlatformClient


async def execute(params: dict, user: str) -> dict:
    code = params.get("projectCode", "")
    if not code:
        raise ValueError("projectCode 필수")

    state = params.get("state", "all")
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
