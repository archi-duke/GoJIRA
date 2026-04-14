---
name: style_define
description: Architecture Driver 와 정규화된 시스템 속성을 기반으로 적합한 시스템 스타일(Primary/Secondary/Supporting)을 추천합니다. 점수 엔진(카탈로그 기반)으로 후보를 산정하고 LLM 으로 근거를 서술해 architecture/architecture-style.md 를 작성합니다.
---

# style_define

Phase 2 ⑥ 시스템 스타일 정의 — 10개 스타일 카탈로그(`knowledge/arch-styles/`) 에서 정규화 속성에 적합한 스타일을 추천한다.

## Parameters

```yaml
projectCode:
  type: string
  required: true
normalizedAttributes:
  type: object
  description: "{attr: level} 형식. 생략 시 프로젝트의 archSettings.normalizedAttributes 사용, 그것도 없으면 driver_select 자동 위임"
```

## 동작

1. `normalizedAttributes` 확보 (파라미터 → projects.archSettings → driver_select 자동 위임)
2. 카탈로그 로드 + 스키마 검증
3. 점수 엔진 실행 → Primary/Secondary/Supporting + breakdown
4. LLM 이 점수 결과를 근거 서술로 정리 (architecture/architecture-style.md 본문)
5. 검증 루프 (workflow-architecture-design.md ⑥ 기준 SSOT)
6. 문서 저장

## 산출

- `architecture/architecture-style.md` 저장
- 결과에 `recommendation`(엔진 출력)과 `document` 포함

## 선행 의존

- `driver_select` 결과의 `normalizedAttributes` — 없으면 자동 위임
