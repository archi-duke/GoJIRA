---
name: issue_update
description: 기존 이슈의 필드를 수정합니다. Requirement 이슈의 경우 reqType도 변경할 수 있습니다.
---

## Parameters

```yaml
key:
  type: string
  description: "이슈 키 (예: GOJIRA-1)"
  required: true
summary:
  type: string
  description: "변경할 이슈 제목"
description:
  type: string
  description: "변경할 이슈 설명"
status:
  type: string
  description: "변경할 상태"
  enum: [To Do, In Progress, Done]
priority:
  type: string
  description: "변경할 우선순위"
  enum: [Highest, High, Medium, Low, Lowest]
assignee:
  type: string
  description: "변경할 담당자 ID"
reqType:
  type: string
  description: "Requirement 세부 타입 변경. NFR(QA/품질속성), FR(기능), CNST(제약)"
  enum: [NFR, FR, CNST]
```

## 이슈 수정 규칙

- key는 필수입니다
- 변경할 필드만 전달합니다 (부분 업데이트)
- Requirement 이슈의 reqType 변경: NFR(QA), FR(기능), CNST(제약)
- status 변경 시 허용된 enum 값만 사용합니다
- 쓰기 작업이므로 사용자 확인이 필요합니다
