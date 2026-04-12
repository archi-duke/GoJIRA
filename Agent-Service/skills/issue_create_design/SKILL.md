---
name: issue_create_design
description: 설계 이슈를 생성합니다. designLevel로 Architectural(아키텍처/ADSN) 또는 Detailed(상세/DDSN)를 구분합니다. ADSN은 평가/SATAM 필드를 함께 입력합니다.
---

## Parameters

```yaml
projectCode:
  type: string
  description: "프로젝트 코드 (예: GOJIRA)"
  required: true
designLevel:
  type: string
  description: "설계 수준. Architectural(ADSN) / Detailed(DDSN)"
  required: true
  enum: [Architectural, Detailed]
summary:
  type: string
  description: "설계 이슈 제목"
  required: true
description:
  type: string
  description: "설계 내용. 결정 사항, 트레이드오프 등."
priority:
  type: string
  description: "우선순위"
  enum: [Highest, High, Medium, Low, Lowest]
assignee:
  type: string
  description: "담당자 ID"
adiagram:
  type: string
  description: "[ADSN+DDSN] 정적 구조 다이어그램. customfield_18204."
bdiagram:
  type: string
  description: "[ADSN+DDSN] 동적 동작 다이어그램. customfield_18205."
dmodel:
  type: string
  description: "[ADSN] 디자인이 적용된 도메인 모델/시스템 영역. customfield_18218."
archdiagram:
  type: string
  description: "[ADSN] 디자인의 통합 구조. customfield_18215."
objectives:
  type: string
  description: "[ADSN] 디자인의 주요 목표. customfield_18206."
pros:
  type: string
  description: "[ADSN] 디자인의 장점. customfield_18207."
cons:
  type: string
  description: "[ADSN] 디자인의 단점. customfield_18208."
score:
  type: string
  description: "[ADSN] 디자인 평가 점수 (보통 5점/10점). customfield_18209."
eval:
  type: string
  description: "[ADSN] 디자인 평가 내용. customfield_18210."
sensitivity:
  type: string
  description: "[ADSN] Sensitivity 양식(YAML-like). 조건 변화 시 Risk가 될 수 있는 사항. customfield_18212."
mitigation:
  type: string
  description: "[ADSN] Mitigation 양식(YAML-like). Risk를 감소시키는 대응. customfield_18213."
risk:
  type: string
  description: "[ADSN] Risk 양식(YAML-like). 요구사항 달성에 장애가 확정적인 사항. customfield_18216."
decision:
  type: string
  description: "[ADSN] 채택 여부. 비어있지 않으면 최종 선택안. customfield_18211."
```

## 설계 이슈 생성 규칙

- **호출 전 필수**: `issue_query`로 동일 프로젝트의 기존 설계 이슈를 조회하여 유사 이슈가 없는지 확인. 유사 이슈가 있으면 생성 대신 재사용/수정/삭제마킹으로 처리 (system.txt "이슈 생성 공통 규칙" 참조)
- designLevel에 따라 issueType이 결정됩니다:
  - Architectural → "Architectural Design" (ADSN)
  - Detailed → "Detailed Design" (DDSN)
- DDSN은 adiagram, bdiagram만 의미 있습니다. 다른 ADSN 전용 필드를 입력해도 무시됩니다
- ADSN은 일반적으로 Requirement에 contain 관계로 연결됩니다
- DDSN은 일반적으로 ADSN 하위에 위치합니다
- sensitivity / risk / mitigation는 specs/issue_schema.md의 YAML-like 양식을 따릅니다
- 쓰기 작업이므로 사용자 확인이 필요합니다
- 생성 후 반환된 이슈 키를 사용자에게 알려줍니다
