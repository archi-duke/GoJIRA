"""프로젝트 설명 문서 생성/보강 스킬."""

import json
import base64
import logging

from core.llm import call_llm
from clients.platform_api import PlatformClient

log = logging.getLogger("agent.skills.doc_generate")


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
        raise ValueError("answers 또는 content 필수")


async def _generate(code: str, answers: dict, user: str) -> dict:
    """인터뷰 답변으로 문서 생성."""
    prompt = f"""다음 인터뷰 답변을 기반으로 프로젝트 설명 문서(project-description.md)를 마크다운으로 작성하세요.

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
(배경과 동기를 구체적으로 서술)

## 과제 목적
(핵심 목표를 명확하게 서술)

## 주요 이해관계자
(이해관계자와 역할을 정리)

## 주요 사항
(핵심 요구사항과 제약사항을 정리)

> VOS 자동생성 시 이 문서의 내용을 참조합니다.

마크다운 원문만 출력하세요. 코드블록으로 감싸지 마세요."""

    doc = await call_llm(prompt)
    await _save_doc(code, doc, user)

    return {
        "projectCode": code,
        "mode": "generate",
        "message": "project-description.md가 생성되었습니다.",
        "filePath": "business/project-description.md",
        "content": doc,
    }


async def _enrich(code: str, content: str, user: str) -> dict:
    """기존 내용 보강."""
    truncated = content[:6000] if len(content) > 6000 else content

    prompt = f"""다음은 프로젝트 {code}의 과제 설명 문서입니다.

{truncated}

이 문서를 다음 관점에서 보강하세요:
1. 내용이 간략한 섹션은 더 구체적으로 확장
2. 누락된 섹션(과제 배경, 과제 목적, 주요 사항, 이해관계자)이 있으면 추가
3. 문장을 명확하고 전문적으로 다듬기
4. 기존 내용의 의미와 사실은 변경하지 않기

보강된 마크다운 전체를 출력하세요. 코드블록으로 감싸지 마세요."""

    doc = await call_llm(prompt)
    await _save_doc(code, doc, user)

    return {
        "projectCode": code,
        "mode": "enrich",
        "message": "project-description.md가 보강되었습니다.",
        "filePath": "business/project-description.md",
        "content": doc,
    }


async def _save_doc(code: str, content: str, user: str):
    """사용자 브랜치에 문서 저장."""
    client = PlatformClient()
    await client.save_file(
        code,
        path="business/project-description.md",
        content=content,
        message="project-description.md AI 생성/보강",
        user=user,
    )
    log.info("doc_generate: saved business/project-description.md for %s (user=%s)", code, user)
