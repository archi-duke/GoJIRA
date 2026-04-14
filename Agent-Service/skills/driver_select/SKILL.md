---
name: driver_select
description: Requirement(FR/NFR/CNST) 중 아키텍처 결정에 영향이 큰 항목을 Architecture Driver로 선정하고 QAD/QAI/Priority/risk/sensitivity를 평가합니다. Requirement 이슈를 업데이트하고 ASR 라벨을 부여하며 architecture/drivers.md 문서를 작성합니다. 부산물로 정규화된 시스템 속성(style_define 입력)을 산출합니다.
---

# driver_select

Phase 2 ⑤ Architecture Driver 선정 — 요구사항(FR/NFR/CNST)에서 아키텍처 결정에 영향이 큰 항목을 선별하고 QAD/QAI/Priority 를 부여한다. 부산물로 **정규화된 요구사항 속성**(style_define 입력)을 함께 산출한다.

## 입력

| 필드 | 필수 | 설명 |
|------|------|------|
| projectCode | ✓ | 프로젝트 코드 |
| priorityWeight | 선택 | `{qad: 0.5, qai: 0.5}` 기본. 사용자가 지정하면 프로젝트의 `archSettings.priorityWeight` 에 영속화되어 다음 호출부터 재사용됨 |

## 전제

- Requirement 이슈가 존재해야 함 → 없으면 `requirement_refine` 자동 위임

## 산출

- Requirement 이슈 업데이트: `qad`, `qai`, `priority`, `risk`, `sensitivity`, `driverSelected`
- NFR 및 driver 선정 FR/CNST 에 `ASR` 라벨 부여
- `architecture/drivers.md` 문서 저장 (Driver 목록 + 정규화 속성 요약)
- 결과에 `normalizedAttributes` 포함 (style_define 의 입력)

## 출력 스키마

```json
{
  "projectCode": "...",
  "drivers": [
    {"key": "GOJIRA3-27", "subkey": "NFR_001", "qad": 4, "qai": 5, "priority": 4.5,
     "risk": "...", "sensitivity": "...", "rationale": "..."}
  ],
  "normalizedAttributes": {
    "scalability": "high", "team_size": "small", "ops_maturity": "medium", ...
  },
  "updated": [...],
  "documentSaved": "architecture/drivers.md",
  "critiqueTrail": [...],
  "prerequisitesRan": [...]
}
```

## 검증

`workflow-architecture-design.md ⑤ Architecture Driver 선정` 섹션의 검증 기준/정책 SSOT 적용 (기본 정책).
