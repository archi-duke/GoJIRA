---
name: issue_link
description: 두 이슈 간의 관계를 설정합니다 (contain, relate, block 등)
parameters:
  fromKey:
    type: string
    description: "출발 이슈 키 (예: GOJIRA-1)"
    required: true
  toKey:
    type: string
    description: "대상 이슈 키 (예: GOJIRA-2)"
    required: true
  linkType:
    type: string
    description: "관계 유형"
    required: true
    enum: [contain, relate, duplicate, block]
api:
  method: POST
  path: /IssueLink
---

## 이슈 링크 규칙

- fromKey, toKey, linkType 모두 필수입니다
- linkType은 contain(포함), relate(관련), duplicate(중복), block(차단) 중 하나입니다
- 링크 생성 후 결과를 사용자에게 알려줍니다
- 쓰기 작업이므로 사용자 확인이 필요합니다
