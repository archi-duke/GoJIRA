"""VOS 수집 스킬 — project-description.md 분석 → VOS 이슈 자동 생성."""

import re
import json
from datetime import datetime

from core.database import issues
from core.llm import call_llm
from clients.platform_api import PlatformClient
from skills.helpers import get_next_issue_num, add_issue_to_project


async def execute(params: dict, user: str) -> dict:
    code = params.get("projectCode", "")
    if not code:
        raise ValueError("projectCode 필수")

    req_user = params.get("user", user)

    # 1. project-description.md 조회 (main → 사용자 브랜치, 템플릿이면 사용자 브랜치 재시도)
    client = PlatformClient()
    doc_content = await client.get_file_content(code, "business/project-description.md", ref="main")
    if not doc_content or _is_template_only(doc_content):
        user_content = await client.get_file_content(code, "business/project-description.md", ref=f"docs/{req_user}")
        if user_content and not _is_template_only(user_content):
            doc_content = user_content
    if not doc_content or _is_template_only(doc_content):
        return {
            "projectCode": code,
            "message": "business/project-description.md 문서가 없거나 내용이 불충분합니다. "
                       "VOS를 추출하려면 문서를 먼저 작성해야 합니다. "
                       "자동 생성을 도와드릴 수 있습니다. 간단한 인터뷰를 진행할까요?",
            "action": "문서 보강 필요",
            "suggestSkill": "projdesc_interview",
            "created": [],
            "skipped": 0,
        }

    # 2. 기존 VOS 이슈 조회
    prefix = {"key": {"$regex": f"^{re.escape(code)}-"}, "issueType": "VOS"}
    existing_vos = list(issues().find(prefix))
    existing_summaries = [v.get("summary", "") for v in existing_vos]

    existing_info = ""
    if existing_vos:
        vos_list = [{"key": v.get("key", ""), "summary": v.get("summary", ""), "client": v.get("client", "")} for v in existing_vos]
        existing_info = f"\n\n## 기존 VOS ({len(existing_vos)}건)\n{json.dumps(vos_list, ensure_ascii=False, indent=2)}\n\n위 VOS와 중복되는 항목은 생성하지 마세요."

    # 3. LLM으로 VOS 추출
    truncated = doc_content[:6000] if len(doc_content) > 6000 else doc_content

    prompt = f"""다음은 프로젝트 {code}의 과제 설명 문서입니다.

{truncated}
{existing_info}

위 문서를 분석하여 이해관계자(Stakeholder)의 요구사항(VOS: Voice of Stakeholder)을 추출하세요.

분석 관점:
- 과제 배경에서 도출되는 비즈니스 요구
- 과제 목적에서 도출되는 핵심 목표
- 주요 사항에서 도출되는 구체적 요구사항
- 이해관계자(사용자, 관리자, 운영자, 외부 시스템 등)별 관점

반드시 아래 JSON 배열 형식으로만 응답하세요. 다른 텍스트는 포함하지 마세요:
[
  {{
    "summary": "요구사항 제목 (간결하게)",
    "description": "요구사항 상세 내용 (배경, 근거 포함)",
    "client": "이해관계자 역할명 (예: 사용자, 관리자, 운영자, 개발자 등)"
  }}
]

추가할 VOS가 없으면 빈 배열 []을 반환하세요."""

    raw = await call_llm(prompt)

    # JSON 파싱
    try:
        vos_items = json.loads(raw)
    except json.JSONDecodeError:
        match = re.search(r'\[.*\]', raw, re.DOTALL)
        if match:
            try:
                vos_items = json.loads(match.group())
            except json.JSONDecodeError:
                return {
                    "projectCode": code,
                    "message": "LLM 응답을 파싱할 수 없습니다.",
                    "raw": raw[:500],
                    "created": [],
                    "skipped": 0,
                }
        else:
            return {
                "projectCode": code,
                "message": "LLM 응답에서 VOS 목록을 찾을 수 없습니다.",
                "raw": raw[:500],
                "created": [],
                "skipped": 0,
            }

    if not isinstance(vos_items, list):
        vos_items = []

    # 4. 중복 필터링 및 이슈 생성
    created = []
    skipped = 0
    now = datetime.now().strftime("%Y-%m-%dT%H:%M:%S.000+0900")

    for item in vos_items:
        summary = item.get("summary", "").strip()
        if not summary:
            continue

        if _is_duplicate(summary, existing_summaries):
            skipped += 1
            continue

        key = f"{code}-{get_next_issue_num(code)}"
        doc = {
            "key": key,
            "issueType": "VOS",
            "summary": summary,
            "description": item.get("description", ""),
            "client": item.get("client", ""),
            "status": "To Do",
            "priority": "Medium",
            "reporter": user,
            "created": now,
            "updated": now,
            "labels": [],
            "issueLinks": [],
            "_source": "gojira-agent",
        }
        issues().update_one({"key": key}, {"$set": doc}, upsert=True)
        add_issue_to_project(code, key)
        created.append({"key": key, "summary": summary, "client": item.get("client", "")})
        existing_summaries.append(summary)

    return {
        "projectCode": code,
        "existingCount": len(existing_vos),
        "created": created,
        "createdCount": len(created),
        "skipped": skipped,
        "message": f"{len(created)}건 VOS 생성, {skipped}건 중복 스킵" if created else "추가 VOS 없음",
    }


def _is_template_only(content: str) -> bool:
    """템플릿만 있고 실제 내용이 없는지 확인."""
    stripped = re.sub(r'#[^\n]*', '', content)          # 제목 제거
    stripped = re.sub(r'>.*', '', stripped)              # 인용 제거
    stripped = re.sub(r'\(작성 필요\)', '', stripped)
    # 템플릿 안내 문구 제거
    for phrase in ['기록하는 문서입니다', '기술하는 문서입니다', '반드시 작성해', '참조합니다']:
        stripped = stripped.replace(phrase, '')
    stripped = stripped.strip()
    return len(stripped) < 30


def _is_duplicate(new_summary: str, existing: list) -> bool:
    """기존 VOS summary와 유사한지 확인."""
    norm_new = re.sub(r'\s+', ' ', new_summary.lower().strip())
    for ex in existing:
        norm_ex = re.sub(r'\s+', ' ', ex.lower().strip())
        if norm_new == norm_ex:
            return True
        if len(norm_new) > 5 and len(norm_ex) > 5:
            if norm_new in norm_ex or norm_ex in norm_new:
                return True
    return False
