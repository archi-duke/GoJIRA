"""스킬 디스패처 — Go executor.go executeSkill switch/case 대응."""

import logging
import asyncio
from typing import Any

log = logging.getLogger("agent.skills.executor")

# 동기 스킬
from skills.executors.project import exec_project_query
from skills.executors.issue import (
    exec_issue_query, exec_issue_create, exec_issue_update,
    exec_issue_delete, exec_issue_link,
)

# 비동기 스킬 (LLM 호출 또는 Platform-API 호출 포함)
from skills.executors.analysis import (
    exec_satam_analyze, exec_issue_analyze, exec_schedule_analyze,
)
from skills.executors.doc import exec_doc_list, exec_doc_summarize, exec_doc_diff
from skills.executors.pr import exec_pr_list, exec_pr_create

# 스킬 → 실행 함수 매핑
_SYNC_EXECUTORS = {
    "project_query": exec_project_query,
    "issue_query": exec_issue_query,
    "issue_create": exec_issue_create,
    "issue_update": exec_issue_update,
    "issue_delete": exec_issue_delete,
    "issue_link": exec_issue_link,
    "issue_analyze": exec_issue_analyze,
}

_ASYNC_EXECUTORS = {
    "satam_analyze": exec_satam_analyze,
    "schedule_analyze": exec_schedule_analyze,
    "doc_list": exec_doc_list,
    "doc_summarize": exec_doc_summarize,
    "doc_diff": exec_doc_diff,
    "pr_list": exec_pr_list,
    "pr_create": exec_pr_create,
}


async def execute_skill(name: str, params: dict, user: str) -> Any:
    """스킬 실행 — Go SkillRegistry.Execute() 대응."""
    log.info("skill=%s params=%s user=%s", name, params, user)

    if name in _SYNC_EXECUTORS:
        return _SYNC_EXECUTORS[name](params, user)
    elif name in _ASYNC_EXECUTORS:
        return await _ASYNC_EXECUTORS[name](params, user)
    else:
        raise ValueError(f"executor not implemented for skill: {name}")
