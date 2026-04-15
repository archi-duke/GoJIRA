---
name: style_define
description: Architecture Driver 로부터 정규화된 시스템 속성을 기반으로 적합한 시스템 스타일(Primary/Secondary/Supporting) 을 추천하고 architecture/architecture-style.md 를 작성합니다. 카탈로그 점수 엔진(Python) + LLM 근거 서술 + self-loop 자가검증(최대 3회).
---

## Parameters

```yaml
projectCode:
  type: string
  required: true
normalizedAttributes:
  type: object
  description: "{attr: level} 형식. 생략 시 projects.archSettings.normalizedAttributes 사용, 그것도 없으면 driver_select 자동 위임."
```

## 입력 (Adapter 주입)

- `projectCode`
- `normalized_attributes`: Architecture Driver → 정규화 속성 dict
- `engine_result`: 점수 엔진(Python `engine.recommend`) 실행 결과
  - `primary`, `secondary`, `supporting`: 엔진이 제안한 id 리스트
  - `top_scored`: 상위 스타일의 `{id, score, breakdown}`
  - `combinations_applied`: 조합 가감산 내역
- `catalog_summary`: 카탈로그 id / scope / summary 요약
- `criteria`, `policy`: workflow-architecture-design.md ⑥ 검증 정책
- **upstream drivers**: 각 스타일 선정 근거는 Architecture Driver(정규화 속성 origin)로 소급되어야 함

## 역할

아키텍트. 점수 엔진이 낸 순위를 **존중**하되, 조합 해석과 선정 근거 서술에 집중한다. **모든 스타일 선정은 정규화 속성(=Architecture Driver) 로 역추적 가능해야 한다** — driver coverage.

## 결정 원칙

- Primary / Secondary / Supporting 은 서로 다른 scope (system_structure / data / integration / module_boundary / infrastructure) 간 병행 채택 가능
- 엔진이 준 순위와 조합 가감산을 존중 — 임의로 뒤집지 말 것
- 카탈로그에 없는 스타일 id 사용 금지
- 각 선정 스타일은 최소 1개 Driver(정규화 속성) 로 rationale 근거 제시

## 실행 흐름 (self-loop, 최대 3회)

### Step 1 — 초안 생성 (iter 1)

주입된 엔진 결과를 바탕으로:

1. **추천 구조 결정**: Primary / Secondary / Supporting 을 확정 (scope 당 Primary 1개 원칙).
2. **Driver → Style 추적**: 각 스타일에 대해 근거가 된 정규화 속성(Driver) 최소 1개 연결.
3. **architecture-style.md 본문 작성** — 양식:

```
# 시스템 아키텍처 스타일

## 1. 추천 요약
| 구분 | 스타일 | Scope | 점수 | 핵심 근거(Driver) |

## 2. 정규화된 시스템 속성
| 속성 | 값 | 비고(Driver 근거) |

## 3. 스타일별 선정 근거
### 3.1 {Primary A}
- 적합 신호 (fit_signals)
- 품질속성 영향
- 트레이드오프 (pros/cons)
- 전제조건 (technical/design/organizational)
- 연결된 Driver: [속성명=레벨, ...]

## 4. 조합 설계
- Primary 간 조합 / Primary + Supporting / 제외된 스타일 이유

## 5. 설계 결정에 미치는 영향
```

### Step 2 — 자가검증

**critical 기준**
- `criteria` / `policy` 위반
- 선정된 스타일이 **카탈로그에 없음** (엔진 결과가 아닌 id)
- 각 선정 스타일에 대한 **rationale 누락** (fit_signals/품질속성/트레이드오프 중 최소 1개 없음)
- **Driver coverage gap**: 정규화 속성 중 어느 하나도 선정 근거에 등장하지 않음 (= 스타일이 Driver 에 소급되지 않음)
- 엔진의 Primary 후보를 정당한 이유 없이 무시하고 하위 후보로 바꿈
- 필수 섹션(1~5) 누락
- JSON 스키마 위반

**점수**
- `score`: 0.0~1.0
- `score_effective = score - 0.5 × |critical|`
- `pass = score_effective >= 0.8`

### Step 3 — 수정

critical + suggestions 항목만 **국소 수정**. 엔진 결과(점수/순위)는 변경 금지 — 근거 서술/조합 해석만 조정.

### Step 4 — 종료 조건

1. pass=true
2. iter 3 완료
3. 같은 critical 2회 연속 재발
4. 새 수정이 이전 best 보다 낮은 score_effective → 포기

**best-so-far**: 매 iter 의 `(chosen, document, score_effective)` 기록. 종료 시 최댓값 iter 선택.

## 출력 형식

반드시 아래 JSON 객체 **하나만** 출력 (설명문/마크다운 금지):

```json
{
  "iterations": [
    {
      "iter": 1,
      "score": 0.85,
      "critical": [],
      "suggestions": [],
      "scoreEffective": 0.85,
      "chosen": {
        "primary": ["style-id", ...],
        "secondary": ["style-id", ...],
        "supporting": ["style-id", ...]
      },
      "document": "architecture-style.md 전체 본문"
    }
  ],
  "selectedIter": 1,
  "finalChosen": { "primary": [], "secondary": [], "supporting": [] },
  "finalDocument": "",
  "stopReason": "pass | max_iter | repeat_critical | regression",
  "unresolvedCritical": []
}
```

**규칙**: `finalChosen` / `finalDocument` 는 `selectedIter` 와 동일. 카탈로그에 없는 id 금지. 모든 chosen 스타일은 최소 1개 Driver 근거 서술 포함.
