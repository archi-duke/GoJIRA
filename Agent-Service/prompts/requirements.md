# Requirements

> 과제의 요구 사양을 기술하는 문서입니다. Feature와 Use Case를 기반으로 도출된 FR / NFR / CNST를 정리합니다.

## Functional Requirements (FR)

> **User Story 형식 필수**: "{역할}은(는) {목적}을 위해 {기능}을 할 수 있다"

| 이슈 키 | 관련 Feature | User Story |
|--------|-------------|-----------|
| (FR 이슈 키) | (Feature 키) | (User Story 본문) |

## Quality Attributes / Non-Functional Requirements (NFR)

> 각 NFR은 **QAS(Quality Attribute Scenario)** 형식으로 작성됩니다.
> Measure가 원본(VOS/과제 소개)에 명시되지 않은 항목은 LLM이 일반적 수준으로 자동 결정합니다 — **설계자 검토/조정 필요**.
> 자동 결정된 항목은 표의 비고에 `[자동결정]`으로 표시합니다.

| 이슈 키 | QA | Source | Stimulus | Environment | Artifact | Response | Measure | 비고 |
|--------|----|----|---------|------------|---------|---------|---------|------|
| (NFR 이슈 키) | (품질속성 이름) | (자극 발생 주체) | (자극) | (환경) | (자극 받는 요소) | (반응) | (측정값/범위) | (자동결정 여부) |

## Constraints (CNST)

| 이슈 키 | 제약 | 출처 |
|--------|------|------|
| (CNST 이슈 키) | (제약 내용) | (project-description / use-case 등) |

## QAW(Quality Attributes Workshop) 결과

> QAW가 진행된 경우 QA별 QAD/QAI/QAS 결과를 **QAS 기준 내림차순**으로 정렬한 테이블을 작성합니다.
> QAW가 진행되지 않은 경우 아래 코멘트만 남깁니다.

_QAW 평가 진행 이후 작성 예정._
