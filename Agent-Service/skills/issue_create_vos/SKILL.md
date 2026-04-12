---
name: issue_create_vos
description: VOS(이해관계자 요구) 이슈를 생성합니다. 이해관계자의 원시 요구사항(Voice of Stakeholder)을 기록할 때 사용합니다.
---

## Parameters

```yaml
projectCode:
  type: string
  description: "프로젝트 코드 (예: GOJIRA)"
  required: true
summary:
  type: string
  description: "VOS 제목 (이해관계자 요구를 한 줄로)"
  required: true
description:
  type: string
  description: "이해관계자 요구 상세. 누가, 무엇을, 왜 원하는지 원문에 가깝게 기록."
client:
  type: string
  description: "VOS를 요청한 사람 (knoxID). customfield_15235에 매핑됨."
priority:
  type: string
  description: "우선순위"
  enum: [Highest, High, Medium, Low, Lowest]
assignee:
  type: string
  description: "담당자 ID"
```

## VOS 생성 규칙

- **호출 전 필수**: `issue_query`로 동일 프로젝트의 기존 VOS를 조회하여 유사 이슈가 없는지 확인. 유사 이슈가 있으면 생성 대신 재사용/수정/삭제마킹으로 처리 (system.txt "이슈 생성 공통 규칙" 참조)
- VOS는 가공되지 않은 이해관계자의 목소리를 그대로 담는 이슈입니다
- `client`에는 요구를 제기한 이해관계자의 knoxID를 기록합니다
- 이후 Requirement(FR/NFR/CNST)로 정제되어 contain 관계로 연결됩니다
- 쓰기 작업이므로 사용자 확인이 필요합니다
- 생성 후 반환된 이슈 키를 사용자에게 알려줍니다
