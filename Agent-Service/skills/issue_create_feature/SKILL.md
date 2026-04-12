---
name: issue_create_feature
description: Feature(Use Case) 이슈를 생성합니다. 사용자 가치 단위의 기능 묶음과 Use Case Story를 표현할 때 사용합니다.
---

## Parameters

```yaml
projectCode:
  type: string
  description: "프로젝트 코드 (예: GOJIRA)"
  required: true
summary:
  type: string
  description: "Feature 제목"
  required: true
description:
  type: string
  description: "Feature 상세 설명. 사용자 가치와 범위를 기술."
actor:
  type: string
  description: "Use Case Story의 행위 주체 (customfield_15215)"
precondition:
  type: string
  description: "Use Case Story의 진입시점 상태 (customfield_15216)"
basicflow:
  type: string
  description: "Use Case Story의 기본 작업 흐름 (customfield_15217)"
alternativeflow:
  type: string
  description: "Use Case Story의 대체/조건부 흐름 (customfield_15218)"
exceptionflow:
  type: string
  description: "Use Case Story의 예외 처리 흐름 (customfield_15219)"
postcondition:
  type: string
  description: "Use Case Story 종료 상태 (customfield_15220)"
priority:
  type: string
  description: "우선순위"
  enum: [Highest, High, Medium, Low, Lowest]
assignee:
  type: string
  description: "담당자 ID"
```

## Feature 생성 규칙

- **호출 전 필수**: `issue_query`로 동일 프로젝트의 기존 Feature를 조회하여 유사 이슈가 없는지 확인. 유사 이슈가 있으면 생성 대신 재사용/수정/삭제마킹으로 처리 (system.txt "이슈 생성 공통 규칙" 참조)
- Feature는 사용자에게 전달되는 가치 단위(Use Case)입니다
- Use Case Story는 actor / precondition / basicflow / alternativeflow / exceptionflow / postcondition로 구성됩니다
- 일반적으로 여러 Requirement를 contain 관계로 묶습니다
- 쓰기 작업이므로 사용자 확인이 필요합니다
- 생성 후 반환된 이슈 키를 사용자에게 알려줍니다
