---
name: issue_create
description: 프로젝트에 새 이슈를 생성합니다. Requirement 이슈 생성 시 reqType(NFR/FR/CNST)을 지정해야 합니다.
---

## Parameters

```yaml
projectCode:
  type: string
  description: "프로젝트 코드 (예: GOJIRA)"
  required: true
issueType:
  type: string
  description: "이슈 타입"
  required: true
  enum: [VOS, Feature, Requirement, Architectural Design, Detailed Design]
reqType:
  type: string
  description: "Requirement 세부 타입. issueType이 Requirement일 때 필수. NFR(QA/품질속성), FR(기능), CNST(제약)"
  enum: [NFR, FR, CNST]
summary:
  type: string
  description: "이슈 제목"
  required: true
description:
  type: string
  description: "이슈 상세 설명"
priority:
  type: string
  description: "우선순위"
  enum: [Highest, High, Medium, Low, Lowest]
assignee:
  type: string
  description: "담당자 ID"
```

## 이슈 생성 규칙

- projectCode, issueType, summary는 필수입니다
- issueType이 Requirement인 경우 reqType도 지정해야 합니다:
  - QA/품질속성 요구사항 → reqType=NFR
  - 기능 요구사항 → reqType=FR
  - 제약사항 → reqType=CNST
- priority 생략 시 기본값은 Medium입니다
- 생성 후 반환된 이슈 키를 사용자에게 알려줍니다
- 쓰기 작업이므로 사용자 확인이 필요합니다
