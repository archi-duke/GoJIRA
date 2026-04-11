"""프로젝트 설명 문서 인터뷰 스킬 — 문서 상태 점검 + 질문 생성."""

import json
import re

from clients.platform_api import PlatformClient


INTERVIEW_QUESTIONS = [
    {"id": "background", "label": "과제 배경", "question": "이 과제를 시작하게 된 배경이나 동기는 무엇인가요?"},
    {"id": "purpose", "label": "과제 목적", "question": "이 과제가 달성하려는 핵심 목표는 무엇인가요?"},
    {"id": "stakeholders", "label": "주요 이해관계자", "question": "이 과제의 주요 사용자나 이해관계자는 누구인가요?"},
    {"id": "requirements", "label": "핵심 요구사항", "question": "반드시 충족해야 할 주요 요구사항이 있다면 알려주세요."},
    {"id": "constraints", "label": "제약사항", "question": "기술적 또는 비즈니스적 제약사항이 있나요? (없으면 '없음')"},
]


async def execute(params: dict, user: str) -> dict:
    code = params.get("projectCode", "")
    if not code:
        raise ValueError("projectCode 필수")

    answers_raw = params.get("answers", "")

    # 기존 문서 확인
    client = PlatformClient()
    doc_content = await client.get_file_content(code, "business/project-description.md", ref="main")
    if not doc_content or _is_template_only(doc_content):
        user_content = await client.get_file_content(code, "business/project-description.md", ref=f"docs/{user}")
        if user_content and not _is_template_only(user_content):
            doc_content = user_content

    has_content = doc_content and not _is_template_only(doc_content)

    # 답변이 있으면 파싱
    answers = {}
    if answers_raw:
        try:
            answers = json.loads(answers_raw) if isinstance(answers_raw, str) else answers_raw
        except json.JSONDecodeError:
            pass

    # 답변이 충분하면 projdesc_generate 호출 안내
    if answers:
        missing = [q for q in INTERVIEW_QUESTIONS if q["id"] not in answers or not answers[q["id"]].strip()]
        if not missing:
            return {
                "projectCode": code,
                "status": "ready",
                "message": "인터뷰 완료! projdesc_generate 스킬을 호출하여 문서를 생성하세요.",
                "answers": answers,
                "nextAction": "projdesc_generate",
            }
        else:
            return {
                "projectCode": code,
                "status": "incomplete",
                "message": f"아직 {len(missing)}개 항목이 부족합니다.",
                "answered": list(answers.keys()),
                "remaining": [{"id": q["id"], "question": q["question"]} for q in missing],
            }

    # 최초 호출 — 문서 상태에 따라 질문 생성
    if has_content:
        return {
            "projectCode": code,
            "status": "sufficient",
            "message": "project-description.md에 내용이 있습니다. 보강이 필요하면 projdesc_generate 스킬을 사용하세요.",
            "existingContent": doc_content[:500],
        }

    return {
        "projectCode": code,
        "status": "needs_interview",
        "message": "project-description.md 문서가 비어 있거나 부족합니다. "
                   "자동 생성을 위해 몇 가지 질문에 답해주세요.",
        "questions": [{"id": q["id"], "label": q["label"], "question": q["question"]} for q in INTERVIEW_QUESTIONS],
    }


def _is_template_only(content: str) -> bool:
    """템플릿만 있고 실제 내용이 없는지 확인."""
    stripped = re.sub(r'#[^\n]*', '', content)
    stripped = re.sub(r'>.*', '', stripped)
    stripped = re.sub(r'\(작성 필요\)', '', stripped)
    for phrase in ['기록하는 문서입니다', '기술하는 문서입니다', '반드시 작성해', '참조합니다']:
        stripped = stripped.replace(phrase, '')
    stripped = stripped.strip()
    return len(stripped) < 30
