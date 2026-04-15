"""프로젝트 설명 문서 생성/보강 스킬."""

import json
import logging

from core.llm import call_llm
from clients.platform_api import PlatformClient

log = logging.getLogger("agent.skills.doc_generate")


async def _save_to_branch(code: str, content: str, user: str, message: str) -> dict:
    """생성한 문서를 docs/{user} 브랜치에 저장. 성공/실패 정보 반환."""
    try:
        client = PlatformClient()
        await client.save_file(code, "business/project-description.md", content, message, user)
        return {"saved": True, "path": "business/project-description.md", "branch": f"docs/{user}"}
    except Exception as e:
        log.warning("projdesc save failed: %s", e)
        return {"saved": False, "error": str(e)}


async def execute(params: dict, user: str) -> dict:
    code = params.get("projectCode", "")
    if not code:
        raise ValueError("projectCode 필수")

    mode = params.get("mode", "generate")
    answers_raw = params.get("answers", "")
    content = params.get("content", "")

    if mode == "enrich" and content:
        return await _enrich(code, content, user)
    elif answers_raw:
        answers = json.loads(answers_raw) if isinstance(answers_raw, str) else answers_raw
        return await _generate(code, answers, user)
    else:
        # answers/content 없이 호출됨 — 인터뷰가 선행되어야 함
        return {
            "projectCode": code,
            "status": "needs_interview",
            "suggestSkill": "projdesc_interview",
            "message": (
                "프로젝트 설명을 생성하려면 먼저 인터뷰로 답변을 수집해야 합니다. "
                "projdesc_interview(projectCode=...) 를 호출해 질문을 받고, "
                "사용자 답변을 모은 뒤 다시 projdesc_generate(answers=...) 로 호출하세요."
            ),
        }


async def _generate(code: str, answers: dict, user: str) -> dict:
    """인터뷰 답변으로 문서 생성."""
    prompt = f"""# 역할
당신은 노련한 시니어 기획자입니다. 주어진 정보 외에도 다양한 유사 사례와 경험을 바탕으로,
제안자가 미처 검토하지 못한 사항들까지 고려하여 과제를 기획합니다.
- 답변에 누락된 이해관계자, 간접 영향을 받는 그룹, 암묵적 제약 등을 능동적으로 식별
- 업계 일반적인 관행과 유사 과제의 함정(pitfall)을 반영
- 답변이 모호할 때는 가장 합리적인 해석을 적용하되, 해석을 문서에 명시

# 작업
아래 인터뷰 답변을 기반으로 과제 설명 문서(project-description.md)를 마크다운으로 작성하세요.

프로젝트 코드: {code}

인터뷰 답변:
- 과제 배경: {answers.get('background', '(미입력)')}
- 과제 목적: {answers.get('purpose', '(미입력)')}
- 주요 이해관계자: {answers.get('stakeholders', '(미입력)')}
- 핵심 요구사항: {answers.get('requirements', '(미입력)')}
- 제약사항: {answers.get('constraints', '(미입력)')}

문서 구조:
# Project Description

## 과제 배경
(배경과 동기를 구체적으로 서술. 답변에 없는 맥락도 시니어 관점에서 보강)

## 과제 목적
(핵심 목표를 명확하게 서술. 달성 지표 또는 성공 기준이 있으면 함께 기술)

## 주요 이해관계자
(직접/간접 이해관계자와 역할을 정리. 답변에 누락된 관련자가 합리적으로 예상되면 포함)

## 주요 사항
(핵심 요구사항, 제약사항, 유사 사례에서 검토되어야 할 잠재 이슈 정리)

> VOS 자동생성 시 이 문서의 내용을 참조합니다.

마크다운 원문만 출력하세요. 코드블록으로 감싸지 마세요."""

    doc = await call_llm(prompt)
    save_info = await _save_to_branch(code, doc, user, "[projdesc_generate] 인터뷰 답변으로 생성 (mode=generate)")

    return {
        "projectCode": code,
        "mode": "generate",
        "applyToEditor": True,
        "message": "과제 설명 문서가 생성되었습니다." + (
            f" (docs/{user} 브랜치 저장 완료)" if save_info.get("saved") else f" (저장 실패: {save_info.get('error')})"
        ),
        "filePath": "business/project-description.md",
        "content": doc,
        "documentSaved": save_info.get("path") if save_info.get("saved") else None,
        "saveError": save_info.get("error"),
    }


async def _enrich(code: str, content: str, user: str) -> dict:
    """기존 내용 보강."""
    truncated = content[:6000] if len(content) > 6000 else content

    prompt = f"""# 역할
당신은 노련한 시니어 기획자입니다. 주어진 정보 외에도 다양한 유사 사례와 경험을 바탕으로,
제안자가 미처 검토하지 못한 사항들까지 고려하여 과제를 기획합니다.
- 기존 내용의 의미와 사실은 보존하되, 경험에 비추어 보강이 필요한 부분을 능동적으로 식별
- 누락된 이해관계자, 간접 영향 그룹, 암묵적 제약 등을 추가

# 작업
다음은 프로젝트 {code}의 과제 설명 문서입니다.

{truncated}

이 문서를 다음 관점에서 보강하세요:
1. 내용이 간략한 섹션은 유사 사례 경험을 반영해 더 구체적으로 확장
2. 누락된 섹션(과제 배경, 과제 목적, 주요 사항, 이해관계자)이 있으면 추가
3. 문장을 명확하고 전문적으로 다듬기
4. **기존 내용의 의미와 사실은 변경하지 않기** (보강만 허용, 삭제/왜곡 금지)

보강된 마크다운 전체를 출력하세요. 코드블록으로 감싸지 마세요."""

    doc = await call_llm(prompt)

    return {
        "projectCode": code,
        "mode": "enrich",
        "applyToEditor": True,
        "message": "과제 설명 문서가 보강되었습니다.",
        "filePath": "business/project-description.md",
        "content": doc,
    }
