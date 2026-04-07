---
name: issue_query
description: 프로젝트의 이슈 목록을 조회합니다. issueType으로 필터링할 수 있습니다.
parameters:
  projectCode:
    type: string
    description: "프로젝트 코드 (예: GOJIRA)"
    required: true
  issueType:
    type: string
    description: "이슈 타입 필터 (VOS, Feature, Requirement, Architectural Design, Detailed Design). 생략하면 전체 조회"
    enum: [VOS, Feature, Requirement, Architectural Design, Detailed Design]
api:
  method: GET
  path: /Issue
  headers:
    XBody: '{"jiraCode":"{{.projectCode}}"}'
  response_filter:
    issueType: "{{.issueType}}"
  response_fields: [key, issueType, summary, status, priority, assignee]
---

## 이슈 조회 규칙

- projectCode는 필수입니다
- issueType이 지정되면 해당 타입만 필터링합니다
- 결과를 마크다운 표로 정리하되, key/타입/요약/상태를 포함합니다
- 이슈 수를 먼저 알려줍니다
- 타입별 통계가 있으면 함께 제공합니다
