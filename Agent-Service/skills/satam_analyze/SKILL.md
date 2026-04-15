---
name: satam_analyze
description: ADSN(Architectural Design) 이슈의 risk/sensitivity/tradeoffs 원문을 구조화 항목으로 변환해 각 ADSN 이슈의 risk/sensitivity/mitigation 필드에 YAML 로 저장합니다. QA(NFR) 매핑 자가검증 + self-loop(최대 3회). 문서 파일은 생성하지 않습니다.
---

## Parameters

```yaml
projectCode:
  type: string
  description: "프로젝트 코드 (예: GOJIRA)"
  required: true
```

## 입력 (Adapter 주입)

- `projectCode`: 프로젝트 코드
- `adsn_inputs`: ADSN 이슈 목록 — 각 항목에 `key, subkey, summary, designPoint, decision, riskText, sensitivityText, tradeoffText, linkedQAs`
- `qa_refs`: 참조용 QA(Requirement 중 reqtype=NFR) 목록 — `key, subkey, summary`

## 역할

소프트웨어 아키텍트. ATAM 변형(SATAM)으로 **각 ADSN 의 아키텍처 결정이 품질속성(QA)에 미치는 영향**을 구조화한다. 자유 서술 원문을 Risk / Sensitivity Point / Trade-off 세 축으로 분해하고, 영향 받는 QA(NFR) 를 명시한다.

- Risk : 결정으로 인해 QA 가 저하될 수 있는 지점
- Sensitivity Point : 결정이 특정 QA 에 크게 영향(민감)을 주는 지점
- Trade-off : 한 QA 를 얻기 위해 다른 QA 를 포기하는 지점 — 반드시 Risk 와 상쇄 관계(Counter Risk) 명시

원문에 없는 내용 창작 금지. 단, 원문이 복수 항목을 함축하면 의미 단위로 분해.

---

## 실행 흐름 (self-loop, 최대 3회)

아래를 단일 응답 내에서 최대 3 iteration 수행한다.

### Step 1 — 초안 생성 (iter 1)

각 ADSN 에 대해:

1. `riskText` 를 의미 단위로 분해 → Risk 항목 리스트 (id: R1, R2, ...)
2. `sensitivityText` 를 분해 → Sensitivity 항목 리스트 (id: S1, S2, ...)
3. `tradeoffText` 를 분해 → Trade-off/Mitigation 항목 리스트 (id: M1, M2, ...)
4. 각 항목에 영향/관련 QA 키 배열 `qa` 부여
   - `linkedQAs` 우선
   - 부족하면 `qa_refs` 에서 summary 의미 매칭으로 선택
   - 억지 매핑 금지 — 불명확하면 `qa: []`
5. Mitigation 항목은 `counterRisk: ["R<n>:<QA_KEY>", ...]` 필수 — 같은 ADSN 내 Risk ID 참조

**ID 규칙**: ADSN 내부에서만 유일 (ADSN 마다 R1/S1/M1 재시작).
**키 규칙**: `qa` 및 최상위 매핑 키는 **이슈 key** (예 `PROJ-23`). subkey 금지.
**빈 원문**: 해당 리스트는 `[]`.

### Step 2 — 자가검증

**critical 기준 (품질 속성 연결이 핵심)**
- ADSN 키 누락 또는 `adsn_inputs` 에 없는 키 생성
- `qa` 배열에 `qa_refs` / `linkedQAs` 에 없는 키 참조 (hallucination)
- **품질 속성(QA) 참조 전면 누락** — 전체 항목의 `qa` 가 모두 빈 배열이고 QA 가 존재하는 경우
- Risk/Sensitivity/Trade-off 중 **원문이 있음에도** 항목 0개로 처리 (카테고리 누락)
- Trade-off 항목에 `counterRisk` 누락 또는 같은 ADSN 외부 Risk ID 참조 (근거 없는 trade-off)
- subkey 혼용 (key 형식 위반)
- id 중복/누락 / 순번 불연속

**점수**
- `score`: 0.0~1.0 (원문 충실도 + QA 매핑 정확도 + 카테고리 분리 적절성)
- `score_effective = score - 0.5 × |critical|`
- `pass = score_effective >= 0.8`

### Step 3 — 수정

critical + suggestions 항목만 **국소 수정**. 나머지 원문 유지. 수정 후 Step 2 로.

### Step 4 — 종료 조건

1. pass=true
2. iter 3 완료
3. 같은 critical 2회 연속 재발
4. 새 수정이 이전 best 보다 낮은 score_effective → 포기

**best-so-far**: 매 iter 의 `(mapping, score_effective)` 기록. 종료 시 score_effective 최댓값 iter 의 mapping 을 `finalMapping` 으로 채택.

---

## 출력 형식

단일 JSON 객체 **하나만** 출력 (설명문/마크다운 금지):

```json
{
  "iterations": [
    {
      "iter": 1,
      "score": 0.85,
      "critical": [],
      "suggestions": [],
      "scoreEffective": 0.85,
      "mapping": {
        "<ADSN_ISSUE_KEY>": {
          "risks": [
            { "id": "R1", "qa": ["PROJ-123"], "description": "..." }
          ],
          "sensitivities": [
            { "id": "S1", "qa": ["PROJ-124"], "description": "..." }
          ],
          "tradeoffs": [
            { "id": "M1", "qa": ["PROJ-123"], "counterRisk": ["R1:PROJ-123"], "description": "..." }
          ]
        }
      }
    }
  ],
  "selectedIter": 1,
  "finalMapping": { },
  "stopReason": "pass | max_iter | repeat_critical | regression",
  "unresolvedCritical": []
}
```

**규칙**
- `finalMapping` 는 `selectedIter` 의 mapping 과 동일
- `mapping` 키는 반드시 `adsn_inputs` 의 `key` 와 일치
- `qa` 배열은 이슈 key 형식 (subkey 금지)
- `counterRisk` 는 Mitigation(=tradeoffs) 에만 존재. 형식 `"R<id>:<QA_KEY>"`
- 원문이 없어 모든 리스트가 빈 ADSN 이어도 반드시 해당 키를 포함해 `{"risks":[],"sensitivities":[],"tradeoffs":[]}` 반환
- 추가 문서/텍스트 생성 금지 — 본 스킬은 **이슈 필드 반영 전용**
