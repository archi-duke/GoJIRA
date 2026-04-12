---
name: issue_create_requirement
description: Requirement 이슈를 생성합니다. reqType으로 FR(기능)/NFR(품질속성/QA)/CNST(제약)을 구분하며, NFR일 경우 품질속성 시나리오 필드를 추가로 입력합니다.
---

## Parameters

```yaml
projectCode:
  type: string
  description: "프로젝트 코드 (예: GOJIRA)"
  required: true
reqType:
  type: string
  description: "요구사항 종류. FR(기능), NFR(품질속성/QA), CNST(제약). customfield_12555에 저장됨."
  required: true
  enum: [FR, NFR, CNST]
summary:
  type: string
  description: "요구사항 제목"
  required: true
description:
  type: string
  description: "요구사항 상세. 검증 가능한 형태로 기술."
priority:
  type: string
  description: "우선순위"
  enum: [Highest, High, Medium, Low, Lowest]
assignee:
  type: string
  description: "담당자 ID"
qa:
  type: string
  description: "[NFR 전용] 품질속성 이름 (예: Performance, Security). customfield_18217."
source:
  type: string
  description: "[NFR 전용] 품질 속성 시나리오의 Source (자극을 발생시키는 주체). customfield_15210."
stimulus:
  type: string
  description: "[NFR 전용] 품질 속성 시나리오의 Stimulus (자극). customfield_15211."
environment:
  type: string
  description: "[NFR 전용] 측정 조건이 되는 환경."
artifact:
  type: string
  description: "[NFR 전용] 자극을 받아 반응을 생성하는 요소. customfield_15208."
response:
  type: string
  description: "[NFR 전용] Artifact의 반응. customfield_15212."
measure:
  type: string
  description: "[NFR 전용] 반응의 측정값/범위 (반드시 측정 가능한 수치). customfield_15213."
responseScenario:
  type: string
  description: "[NFR 전용] 품질 속성 시나리오 요약. customfield_15207."
qai:
  type: string
  description: "[NFR 전용] 품질 속성 평가 — 중요도 점수. customfield_15204."
qad:
  type: string
  description: "[NFR 전용] 품질 속성 평가 — 난이도 점수. customfield_15205."
qas:
  type: string
  description: "[NFR 전용] 품질 속성 종합 점수 (보통 QAI/QAD 평균 또는 가중평균). customfield_15206."
```

## Requirement 생성 규칙

- **호출 전 필수**: `issue_query`로 동일 프로젝트의 기존 Requirement를 조회하여 유사 이슈가 없는지 확인. 유사 이슈가 있으면 생성 대신 재사용/수정/삭제마킹으로 처리 (system.txt "이슈 생성 공통 규칙" 참조)
- reqType은 필수입니다:
  - 기능 요구사항 → FR
  - 품질속성/QA 요구사항(성능, 보안, 가용성 등) → NFR
  - 제약사항(기술/규정/환경 제약) → CNST
- NFR일 경우 품질 속성 시나리오 필드(qa, source, stimulus, environment, artifact, response, measure, responseScenario)를 가능한 한 채웁니다
- NFR 평가 점수(qai, qad, qas)는 알고 있을 때만 입력합니다 — 보통 별도 평가 단계에서 채워집니다
- FR/CNST는 시나리오 필드 없이도 무방합니다
- Requirement는 일반적으로 VOS로부터 contain 관계로 도출됩니다
- 쓰기 작업이므로 사용자 확인이 필요합니다
- 생성 후 반환된 이슈 키를 사용자에게 알려줍니다
