---
name: satam_analyze
description: ADSN의 risk/sensitivity/tradeoffs 를 읽어 SATAM(Software Architecture Trade-off Analysis Method) 문서를 `architecture/satam.md` 에 YAML 포맷으로 생성합니다. Risks/Sensitivities/Trade-offs 3개 섹션으로 구성.
---

## Parameters

```yaml
projectCode:
  type: string
  description: 프로젝트 코드
  required: true
```

## 동작

1. ADSN(Architectural Design) 이슈를 조회해 `risk` / `sensitivity` / `tradeoffs` 필드를 수집
2. QA(Requirement 중 reqtype=NFR) 이슈 목록을 레퍼런스로 준비
3. LLM 에 YAML 양식을 **엄격히 강제**해 `Risks` / `Sensitivities` / `Trade-offs` 3개 섹션 생성
4. `architecture/satam.md` 에 YAML 코드블록 형태로 저장

## 출력 YAML 포맷 (절대 규칙)

```yaml
Risks:
  - ID: R1
    QA: [PROJECTCODE-XXX, ...]
    Description: ...

Sensitivities:
  - ID: S1
    QA: [PROJECTCODE-XXX, ...]
    Description: ...

Trade-offs:
  - ID: M1
    QA: [PROJECTCODE-XXX, ...]
    Counter Risk: ["R1:PROJECTCODE-XXX", "R2:PROJECTCODE-YYY"]
    Description: ...
```

- ID 순차 부여 규칙: Risk→R1,R2... / Sensitivity→S1,S2... / Trade-off→M1,M2...
- QA 필드는 **이슈 key** 형식 사용 (subkey 금지)
- Trade-off 의 `Counter Risk` 는 해당 Mitigation 이 완화하는 Risk ID 와 원래 영향받던 QA key 조합 (`"R1:PROJECTCODE-XXX"` 포맷)
- ADSN 에 원천 내용이 비어있는 항목은 해당 섹션에서 제외
- 임의 창작 금지 — ADSN 필드 내용을 그대로 Description 에 반영
