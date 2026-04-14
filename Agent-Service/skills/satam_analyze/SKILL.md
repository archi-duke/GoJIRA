---
name: satam_analyze
description: ADSN 의 risk/sensitivity/tradeoffs 원문을 구조화 YAML 로 변환해 각 ADSN 이슈의 risk/sensitivity/mitigation 필드에 직접 반영합니다. 문서 파일을 생성하지 않으며, 프론트엔드 SATAM 컴포넌트가 이 필드를 파싱해 렌더링합니다.
---

## Parameters

```yaml
projectCode:
  type: string
  description: 프로젝트 코드
  required: true
```

## 동작

1. ADSN(Architectural Design) 이슈를 조회해 `risk` / `sensitivity` / `tradeoffs` 원문 수집
2. QA(Requirement 중 reqtype=NFR) 이슈 목록 레퍼런스 준비
3. LLM 에 각 ADSN 의 원문을 **구조화 항목 리스트**(Risks/Sensitivities/Trade-offs) 로 변환 + QA 매핑 요청
4. 반환 결과를 YAML 텍스트로 직렬화해 **각 ADSN 이슈의 `risk` / `sensitivity` / `mitigation` 필드에 직접 저장**
5. `archSettings.satamDone = true` 영속화
6. `nextAction` 으로 `ui_navigate(page=satam)` 를 반환 — 호출 측 에이전트가 이어서 실행

## 저장 YAML 포맷 (ADSN 이슈 필드별)

```yaml
- ID : R1
  QA : [PROJECTCODE-123, PROJECTCODE-124]
  Description : ...
```

Mitigation 필드만 `Counter Risk` 라인 추가:

```yaml
- ID : M1
  QA : [PROJECTCODE-123]
  Counter Risk : ["R1:PROJECTCODE-123"]
  Description : ...
```

## 규칙

- ID 는 ADSN 내부에서만 유일하면 됨 (R1/S1/M1 ADSN 마다 재시작)
- QA 는 이슈 key 형식 (subkey 금지)
- Counter Risk 는 같은 ADSN 내 Risk ID 참조
- 문서 파일 생성 절대 금지 — 이슈 필드 반영만
- 완료 후 호출자는 ui_navigate(page=satam) 로 SATAM 화면 전환
