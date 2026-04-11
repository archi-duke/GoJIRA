"""스킬 디스패처 — SkillDef.executor를 동적으로 호출."""

import asyncio
import logging
from typing import Any

log = logging.getLogger("agent.skills.dispatcher")


async def execute_skill(name: str, params: dict, user: str) -> Any:
    """스킬 실행 — 레지스트리에서 executor를 찾아 호출."""
    from skills.registry import get_registry

    registry = get_registry()
    skill = registry.get(name)

    if not skill:
        raise ValueError(f"스킬 없음: {name}")
    if not skill.executor:
        raise ValueError(f"executor 미구현: {name}")

    log.info("skill=%s params=%s user=%s", name, params, user)

    result = skill.executor(params, user)
    if asyncio.iscoroutine(result):
        result = await result

    return result
