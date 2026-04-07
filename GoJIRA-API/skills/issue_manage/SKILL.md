---
name: issue_manage
description: 이슈를 생성, 수정, 삭제하거나 이슈 간 관계를 설정합니다.
parameters:
  action:
    type: string
    description: 수행할 작업
    enum: [create, update, delete, link]
    required: true
  projectCode:
    type: string
    description: "프로젝트 코드 (create 시 필수)"
  key:
    type: string
    description: "이슈 키 (update/delete 시 필수, 예: GOJIRA-1)"
  issueType:
    type: string
    description: "이슈 타입 (create 시 필수)"
    enum: [VOS, Feature, Requirement, Architectural Design, Detailed Design]
  summary:
    type: string
    description: 이슈 제목
  description:
    type: string
    description: 이슈 설명
  status:
    type: string
    description: "상태 (update 시)"
    enum: [To Do, In Progress, Done]
  priority:
    type: string
    description: 우선순위
    enum: [Highest, High, Medium, Low, Lowest]
  assignee:
    type: string
    description: 담당자 ID
  fromKey:
    type: string
    description: "연결 출발 이슈 (link 시)"
  toKey:
    type: string
    description: "연결 도착 이슈 (link 시)"
  linkType:
    type: string
    description: "관계 유형 (link 시)"
    enum: [contain, relate, duplicate, block]
actions:
  create:
    method: POST
    path: /Issue
    auto_key: true
    defaults:
      status: To Do
      priority: Medium
      _source: gojira-agent
  update:
    method: PUT
    path: /Issue
  delete:
    method: DELETE
    path: /Issue
  link:
    method: POST
    path: /Issue/link
    bidirectional: true
---

## 이슈 관리 규칙

### create
- projectCode, issueType, summary는 필수
- 이슈 키는 자동 할당: {projectCode}-{다음번호}
- 생성된 이슈에 `_source: "gojira-agent"` 기록
- priority 기본값은 "Medium"

### update
- key는 필수
- 변경할 필드만 전달하면 해당 필드만 수정됨
- updated 타임스탬프 자동 갱신

### delete
- key는 필수
- 삭제 전 이슈 존재 여부 확인

### link
- fromKey, toKey, linkType 필수
- 양방향 관계 설정 (outward/inward)
- contain: 상위→하위 포함 관계 (VOS→Feature→Requirement→ADSN)
- relate: 관련 관계
- block: 차단 관계
