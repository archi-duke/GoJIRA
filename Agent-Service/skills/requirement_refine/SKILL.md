---
name: requirement_refine
description: Feature와 use-cases.md를 분석하여 FR(User Story)/NFR(QAS)/CNST(제약) Requirement를 도출, 이슈로 등록하고 Feature와 contains 관계를 연결하며 business/requirements.md를 작성합니다.
---

## Parameters

```yaml
projectCode:
  type: string
  description: "프로젝트 코드 (예: GOJIRA)"
  required: true
user:
  type: string
  description: "요청 사용자 ID (문서 브랜치/저장용)"
```

## 절차

1. `business/project-description.md`, `business/use-cases.md` 조회 (main → docs/{user})
2. 기존 Feature, VOS, Requirement 이슈 조회
3. Feature가 없으면 `usecase_analyze` 선행 안내
4. LLM으로 각 Feature/Use Case에서 FR/NFR/CNST 도출:
   - **FR**: User Story 형식 ("{역할}은(는) {목적}을 위해 {기능}을 할 수 있다") — 필수
   - **NFR**: QAS(Quality Attribute Scenario) — Source/Stimulus/Environment/Artifact/Response/Measure 필수. Measure가 원본에 없으면 LLM이 일반적 수준에서 결정하고 `[자동결정]` 표시
   - **CNST**: project-description / use-case에서 반드시 지켜야 하는 사양/제약
5. 각 Requirement를 이슈로 생성 (issue_create_requirement 헬퍼 사용)
6. 각 Requirement ↔ 관련 Feature를 `contains` 관계로 연결 (Feature contains Requirement)
7. `business/requirements.md` 작성·저장 (사용자 브랜치)
8. **자동 결정된 NFR Measure 항목을 사용자에게 명시하고 검토 요청**

## 출력 문서 양식

`prompts/requirements.md`의 양식을 따릅니다.

## 규칙

- Feature가 1건도 없으면 `usecase_analyze`를 먼저 실행하도록 안내
- 동일 summary의 Requirement가 이미 있으면 생성 스킵
- NFR Measure가 자동 결정된 항목은 결과에 별도 목록으로 반환하여 설계자 검토 유도
- QAW가 진행되지 않은 경우 문서 하단에 "QAW 평가 진행 이후 작성 예정" 코멘트만
