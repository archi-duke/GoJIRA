---
name: pr_list
description: 프로젝트의 Pull Request 목록을 조회합니다
parameters:
  projectCode:
    type: string
    description: "프로젝트 코드 (예: GOJIRA)"
    required: true
  state:
    type: string
    description: "PR 상태 필터 (생략하면 open)"
    enum: [open, closed, all]
api:
  method: GET
  path: /PullRequest
---

## PR 목록 조회 규칙

- projectCode는 필수입니다
- state 생략 시 기본값은 open입니다
- PR 번호, 제목, 상태, 작성자를 포함하여 결과를 정리합니다
- 결과를 마크다운 표로 정리합니다
- PR 수를 먼저 알려줍니다
