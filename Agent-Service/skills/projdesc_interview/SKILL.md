---
name: projdesc_interview
description: project-description.md 작성을 위한 인터뷰를 진행합니다. 문서가 없거나 내용이 부족하면 질문 목록을 반환하고, 답변을 수집하여 projdesc_generate 스킬로 문서를 생성합니다.
parameters:
  projectCode:
    type: string
    description: "프로젝트 코드 (예: GOJIRA)"
    required: true
  answers:
    type: string
    description: "이전 질문에 대한 사용자 답변 (JSON 형식). 최초 호출 시 비워두면 질문 목록을 반환합니다."
---

## 인터뷰 흐름

1. 최초 호출: 기존 project-description.md를 확인하고 부족한 항목의 질문을 반환
2. 답변 수집: 사용자가 답변하면 answers에 담아 재호출
3. 문서 생성: 충분한 답변이 모이면 projdesc_generate를 호출하여 문서 자동 생성

## 질문 항목

- 과제 배경: 이 과제를 시작하게 된 배경이나 동기
- 과제 목적: 이 과제가 달성하려는 핵심 목표
- 주요 이해관계자: 이 과제의 주요 사용자/관계자
- 핵심 요구사항: 반드시 충족해야 할 주요 요구사항
- 제약사항/특이사항: 기술적 또는 비즈니스적 제약

## 규칙

- 사용자에게 한 번에 모든 질문을 던지지 않고, 자연스러운 대화로 진행
- 답변이 모이면 자동으로 projdesc_generate 스킬을 호출
