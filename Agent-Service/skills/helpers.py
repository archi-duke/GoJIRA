"""스킬 공용 헬퍼 함수."""

import re

from core.database import issues, projects


def get_next_issue_num(code: str) -> int:
    """프로젝트 이슈 키에서 최대 번호 + 1 반환."""
    cursor = issues().find(
        {"key": {"$regex": f"^{re.escape(code)}-"}},
        {"key": 1},
    )
    prefix = f"{code}-"
    mx = 0
    for doc in cursor:
        k = doc.get("key", "")
        if len(k) > len(prefix):
            try:
                n = int(k[len(prefix):])
                if n > mx:
                    mx = n
            except ValueError:
                pass
    return mx + 1


def add_issue_to_project(code: str, issue_key: str):
    """프로젝트의 issues 배열에 이슈 키 추가."""
    projects().update_one(
        {"jiraCodes.main.jiraCode": code},
        {"$addToSet": {"issues": issue_key}},
    )
