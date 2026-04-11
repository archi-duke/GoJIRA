"""일정 분석 스킬."""

import re
from datetime import datetime

from core.database import issues, projects
from core.llm import call_llm


async def execute(params: dict, user: str) -> dict:
    code = params.get("projectCode", "")
    if not code:
        raise ValueError("projectCode 필수")

    prj = projects().find_one({"jiraCodes.main.jiraCode": code})
    if not prj:
        raise ValueError(f"프로젝트 없음: {code}")

    name = prj.get("name", "")
    status = prj.get("status", "")

    all_issues = list(issues().find({"key": {"$regex": f"^{re.escape(code)}-"}}))
    total = len(all_issues)
    done = sum(1 for i in all_issues if i.get("status") == "Done")
    prog = sum(1 for i in all_issues if i.get("status") == "In Progress")

    info = (
        f"프로젝트: {name} ({code}), 상태: {status}, "
        f"이슈: 총{total} 완료{done} 진행{prog} 미착수{total - done - prog}, "
        f"오늘: {datetime.now().strftime('%Y-%m-%d')}"
    )

    analysis = await call_llm(f"프로젝트 일정 분석:\n{info}")

    return {
        "projectCode": code,
        "projectName": name,
        "issueStats": {"total": total, "done": done, "inProgress": prog},
        "analysis": analysis,
    }
