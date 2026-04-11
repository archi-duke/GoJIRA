---
name: issue_query
description: 프로젝트의 이슈 목록을 조회합니다. issueType과 reqType으로 필터링할 수 있습니다. Requirement 이슈는 reqType(NFR/FR/CNST)으로 세부 분류가 가능합니다.
parameters:
  projectCode:
    type: string
    description: "프로젝트 코드 (예: GOJIRA)"
    required: true
  issueType:
    type: string
    description: "이슈 타입 필터 (VOS, Feature, Requirement, Architectural Design, Detailed Design). 생략하면 전체 조회"
    enum: [VOS, Feature, Requirement, Architectural Design, Detailed Design]
  reqType:
    type: string
    description: "Requirement 세부 타입 필터. QA/NFR(품질속성)은 NFR, 기능요구사항은 FR, 제약사항은 CNST로 검색"
    enum: [NFR, FR, CNST]
api:
  method: GET
  path: /Issue
  headers:
    XBody: '{"jiraCode":"{{.projectCode}}"}'
  response_filter:
    issueType: "{{.issueType}}"
    reqtype: "{{.reqType}}"
  response_fields: [key, issueType, reqType, summary, status, priority, assignee]
---

## 이슈 조회 규칙

- projectCode는 필수입니다
- issueType이 지정되면 해당 타입만 필터링합니다
- reqType은 Requirement 이슈의 세부 분류입니다:
  - QA 또는 NFR (Quality Attribute, 품질속성 요구사항) → reqType=NFR
  - FR (Functional Requirement, 기능 요구사항) → reqType=FR
  - CNST (Constraint, 제약사항) → reqType=CNST
- reqType을 지정하면 issueType은 자동으로 Requirement로 설정됩니다
- 결과를 마크다운 표로 정리하되, key/타입/reqType/요약/상태를 포함합니다
- 이슈 수를 먼저 알려줍니다
