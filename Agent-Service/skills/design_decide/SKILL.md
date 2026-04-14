---
name: design_decide
description: Architecture Driver 와 선정된 시스템 스타일을 기반으로 주요 Design Point 를 식별하고, 대안(approach)을 비교해 ADSN/DDSN 설계 결정을 내립니다. ADSN/DDSN 이슈를 생성하고 Requirement 와 contains 관계로 연결하며 architecture/design-decisions.md 를 작성합니다.
---

# design_decide

Phase 3 ⑦ 설계안 결정.

## Parameters

```yaml
projectCode:
  type: string
  required: true
```

## 선행 의존

- Architecture Driver 선정 완료 (Requirement 이슈에 `driverSelected: true`)
- 시스템 스타일 결정 완료 (projects.archSettings.chosenStyles)
- 부족 시 `style_define` 자동 위임 (style_define 은 내부적으로 driver_select 위임)

## 동작

1. 선행 확인 — drivers + chosenStyles (없으면 `style_define` 자동 위임)
2. LLM 으로 Design Point 식별 + 대안 비교 + 결정
3. ADSN/DDSN 이슈 생성 (`dispatch_by_mode`, subkey 자동 부여: ADSN_NNN / DDSN_NNN)
4. Requirement —(contains)→ ADSN/DDSN 링크 설정
5. docId → subkey 치환 후 `architecture/design-decisions.md` 저장

## 산출

- ADSN/DDSN 이슈 목록 + contains 링크
- `architecture/design-decisions.md`
- 결과: `{designs, linked, documentSaved, prerequisitesRan}`
