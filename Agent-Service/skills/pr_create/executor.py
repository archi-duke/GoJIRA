"""PR 생성 스킬."""

from clients.platform_api import PlatformClient


async def execute(params: dict, user: str) -> dict:
    code = params.get("projectCode", "")
    if not code:
        raise ValueError("projectCode 필수")

    title = params.get("title", f"[{code}] 설계문서 배포 요청")
    body = params.get("body", "")

    client = PlatformClient()
    result = await client.create_pr(code, title=title, body=body, head=f"docs/{user}", base="main", user=user)

    return {
        "projectCode": code,
        "prNumber": result.get("number"),
        "prURL": result.get("html_url"),
        "title": title,
    }
