---
name: issue_create
description: 프로젝트에 새 이슈를 생성합니다
parameters:
  projectCode:
    type: string
    description: "프로젝트 코드 (예: GOJIRA)"
    required: true
  issueType:
    type: string
    description: "이슈 타입"
    required: true
    enum: [VOS, Feature, Requirement, Architectural Design, Detailed Design]
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
api:
  method: POST
  path: /Issue
---

## 이슈 생성 규칙

- projectCode, issueType, summary는 필수입니다
- issueType은 반드시 허용된 enum 값 중 하나여야 합니다
- priority 생략 시 기본값은 Medium입니다
- 생성 후 반환된 이슈 키를 사용자에게 알려줍니다
- 쓰기 작업이므로 사용자 확인이 필요합니다
