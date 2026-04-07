---
name: pr_manage
description: Pull Request를 생성하거나 목록을 조회합니다.
parameters:
  action:
    type: string
    description: 수행할 작업
    enum: [create, list]
    required: true
  projectCode:
    type: string
    description: 프로젝트 코드
    required: true
  title:
    type: string
    description: "PR 제목 (create 시, 생략하면 자동 생성)"
  body:
    type: string
    description: "PR 본문 (create 시)"
  state:
    type: string
    description: "PR 상태 필터 (list 시)"
    enum: [open, closed, all]
actions:
  create:
    method: POST
    path: "/DocRepo/{{.projectCode}}/pr"
    platform_api: true
    body_template:
      title: "{{.title | default '[{projectCode}] 설계문서 배포 요청'}}"
      body: "{{.body}}"
      head: "docs/{{.user}}"
      base: main
    headers:
      X-User-Id: "{{.user}}"
  list:
    method: GET
    path: "/DocRepo/{{.projectCode}}/prs?state={{.state | default 'open'}}"
    platform_api: true
    response_fields: [number, title, state, user.login]
---

## PR 관리 규칙

### create
- projectCode는 필수
- title 생략 시 "[프로젝트코드] 설계문서 배포 요청" 자동 생성
- head 브랜치는 docs/{user}
- base 브랜치는 main

### list
- state 기본값은 "open"
- 번호, 제목, 상태, 작성자를 포함하여 결과 반환
