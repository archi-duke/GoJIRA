"""VOS 이슈 생성 스킬."""

from skills.helpers import create_issue


async def execute(params: dict, user: str) -> dict:
    extras = {}
    if params.get("client"):
        extras["client"] = params["client"]
    return await create_issue("VOS", params, user, extras=extras or None)
