---
name: usecase_analyze
description: VOS와 project-description.md를 기반으로 Business Domain 컨텍스트와 Use Case를 분석하여 business/use-cases.md를 작성하고, Actor와 직접 관계를 가지는 Use Case를 Feature 이슈로 생성하여 VOS와 contains 관계로 연결합니다.
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

1. `business/project-description.md` 조회 (main → docs/{user})
2. 기존 VOS 이슈 목록 조회 — VOS가 없으면 `vos_collect` 선행 안내
3. Business Domain Context와 Actor/외부 시스템 식별
4. 각 Actor 관점에서 Use Case 도출 (VOS와 매핑)
5. 각 Use Case 시나리오 정의 (Basic/Alternative/Exception/Pre/Post)
6. **Actor와 직접 관계를 가지는 Use Case → Feature 이슈로 생성** (보조 Use Case 제외)
7. 각 Feature ↔ 관련 VOS를 `contains` 관계로 연결 (VOS contains Feature)
8. Mermaid 다이어그램(Context / Use Case / Sequence)을 포함한 `business/use-cases.md` 작성·저장 (사용자 브랜치 `docs/{user}`)

## 출력 문서 양식

`prompts/use-cases.md`의 양식을 따릅니다.

## Feature 이슈 필드

- `issueType`: Feature
- `summary`: Use Case 이름
- `description`: Use Case 개요
- Use Case Story 필드: `actor`, `precondition`, `basicflow`, `alternativeflow`, `exceptionflow`, `postcondition`

## 규칙

- VOS가 1건도 없으면 `vos_collect`를 먼저 실행하도록 안내
- 동일 summary의 Feature가 이미 있으면 생성 스킵
- 쓰기 작업이므로 결과 요약을 사용자에게 보고
- 다이어그램은 반드시 Mermaid 코드블록으로 작성
