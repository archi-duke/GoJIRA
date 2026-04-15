"""이슈 링크 스킬."""

from skills.helpers import link_issues


async def execute(params: dict, user: str) -> dict:
    from_key = params.get("fromKey", "")
    to_key = params.get("toKey", "")
    link_type = params.get("linkType", "contains")
    return await link_issues(from_key, to_key, link_type)
