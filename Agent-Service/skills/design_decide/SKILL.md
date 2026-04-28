---
name: design_decide
description: Architecture Driver + 선정 시스템 스타일을 기반으로 Design Point 를 식별하고 대안(approach) 비교로 ADSN/DDSN 설계 결정을 내립니다. ADR(Architecture Decision Record) 구조 준수, Requirement 추적성 보증, self-loop 자가검증(최대 3회) + best-so-far.
---

## Parameters

```yaml
projectCode:
  type: string
  description: "프로젝트 코드 (예: GOJIRA)"
  required: true
```

## 입력 (Adapter 주입)

- `projectCode`
- `normalized_json`: 프로젝트 정규화 품질속성
- `chosen_styles`: {primary, secondary, supporting}
- `drivers`: Architecture Driver 선정된 Requirement 목록 (key/subkey/summary/reqtype/qad/qai/priority/risk/sensitivity)
- `req_list`: 프로젝트 전체 Requirement (relatedRequirements 유효성 검증용)
- `existing_designs`: 기존 ADSN/DDSN 이슈
- `criteria`, `policy`: 워크플로우 검증 기준/정책

## 역할

경험 많은 소프트웨어 아키텍트. **모든 Driver 가 최소 1개 설계 결정으로 추적**되도록 하고, 각 결정을 **ADR(대안 비교 + 근거 + 결과)** 구조로 작성. 선정된 시스템 스타일과 정합되도록 한다.

## 실행 흐름 (self-loop, 최대 3회)

### Step 1 — 초안 생성

**1.1 Design Point 식별** — Driver / 스타일 / 품질속성에서 설계 결정이 필요한 관심사 도출. 예:
- 데이터 일관성 전략 (strong/eventual, 분산 트랜잭션)
- 통신 패턴 (sync REST vs async event, 브로커 선택)
- 배포/실행 모델 (컨테이너, 서버리스, 런타임)
- 인증/인가 구조
- 모듈 경계 / 내부 구조 (hexagonal 적용 범위)
- 장애 처리 (retry / DLQ / circuit breaker)
- 관측성 / 로깅 / 감사 추적
- 배포 전략 (canary, blue-green)

**모든 Driver 는 최소 1개 Design Point 로 다루어져야 한다** (커버리지 핵심).

**1.2 대안 비교 (ADR 필수)** — 각 Design Point 에 **approach 2개 이상** 기술. 각 approach 에 pros/cons + Driver 적합도. 대안이 1개뿐이면 결정이 아님 — 반드시 2개 이상 검토.

**1.3 결정 + 근거** — 선정 approach 에 대해:
- `decision`: 선정 이름
- `decisionRationale`: Driver (특히 QA/CNST) 에 1:1 추적 가능한 근거
- `tradeoffs`: 미선정 대안의 trade-off, 기각 이유
- `risk` / `sensitivity`: 설계가 민감하게 작용하는 품질속성 (선택)

**1.4 ADSN / DDSN 분류**
- **ADSN (Architectural Design)**: 구조/스타일/패턴 — 시스템 수준
- **DDSN (Detailed Design)**: 알고리즘/데이터 구조/인터페이스/라이브러리 — 상세 수준
- Architecture Driver 매핑은 ADSN, 하위 세부 설계는 DDSN 이 일반적

**1.5 Requirement 매핑** — `relatedRequirements` 에 연결할 Requirement JIRA key 배열. `req_list` 실존 키만 (허구 금지). FR/NFR/CNST 모두 포함 가능.

**1.6 docId 부여** — 각 설계 항목에 `docId`: `ADSN-1`, `ADSN-2`, `DDSN-1`, `DDSN-2`, ... 문서 본문(표의 ID 열, 섹션 헤딩, 매트릭스)에 **동일한 docId 를 그대로** 사용. adapter 가 실제 subkey(`ADSN_NNN` / `DDSN_NNN`)로 치환한다.

**1.7 mode 판정** — `create` / `reuse` / `modify` / `delete_mark` (existingKey 필요).

**1.8 architecture/design-decisions.md 작성** — 다음 구조:

```
# 설계 결정 (Architectural & Detailed Design)

## 1. Design Point 요약
| Design Point | 결정 | 대안 | 관련 Driver | 이슈 ID |
|---|---|---|---|---|

## 2. 설계 결정 상세
### ADSN-1: {Design Point 이름}
- 관련 Driver / Requirement: {ID 목록}
- 검토된 대안:
  - (A) ... — pros / cons
  - (B) ... — pros / cons
- 선정: (A)
- 결정 근거: Driver X 의 요구(예: scalability=high)에 부합
- 트레이드오프: (B) 는 ... 관점 유리하나 ... 이유로 기각
- 리스크 / 민감도: (선택)

## 3. Driver 추적성 매트릭스
| Driver | 담당 설계 이슈 |

## 4. 설계 결정의 리스크 요약
```

### Step 2 — 자가검증 (ADR 품질 + 추적성 핵심)

**Driver 커버리지 매트릭스 (필수)** — `drivers` 의 모든 키가 최소 1 설계의 `relatedRequirements` 에 포함되는지 점검. 미커버 Driver 있으면 Step 1.1~1.7 반복해 Design Point 추가.

**critical 기준**
- `criteria` / `policy` 위반
- 설계 필수 필드 (summary, designPoint, approaches, decision, decisionRationale, designLevel, docId, relatedRequirements) 누락
- **alternatives 미고려** — approaches 가 2개 미만 (결정이 아닌 기술)
- **rationale 누락** — decisionRationale 이 비거나 Driver 추적 불가
- **Driver 추적성 결여** — 어느 Driver 에도 연결되지 않는 설계 결정
- **Driver 1건이라도 미커버** (어떤 설계의 relatedRequirements 에도 포함 안 됨)
- `relatedRequirements` 가 `req_list` 실존 키가 아님 (hallucination)
- `designLevel` 이 `Architectural` / `Detailed` 이외 값
- docId 중복/누락, 형식 불일치 (`ADSN-N` / `DDSN-N` 이외)
- 선정 스타일과 명백히 모순되는 결정 (근거 없이 반함)
- `mode=reuse/modify/delete_mark` 인데 `existingKey` 가 `existing_designs` 에 없음

**점수**
- `score`: 0.0~1.0
- `score_effective = score - 0.5 × len(critical)`
- `pass = score_effective >= 0.8`

### Step 3 — 수정 (pass=false)

critical + suggestions 항목만 **국소 수정**. 나머지 원문 유지. 수정 후 Step 2.

### Step 4 — 종료 조건

1. pass=true
2. iter 3 완료
3. 같은 critical 항목 2회 연속 재발
4. 새 수정이 이전 best 보다 낮은 score_effective → 포기

**best-so-far**: 매 iter 의 `(designs, document, coverageMatrix, score_effective)` 기록. 종료 시 최댓값 iter 를 selected.

## 출력 형식

단일 JSON 객체 **하나만** (설명문/마크다운 금지):

```json
{
  "iterations": [
    {
      "iter": 1,
      "score": 0.85,
      "critical": [],
      "suggestions": [],
      "scoreEffective": 0.85,
      "designs": [
        {
          "docId": "ADSN-1",
          "designLevel": "Architectural",
          "summary": "...",
          "description": "설계 결정 개요 / 배경",
          "designPoint": "데이터 일관성 전략",
          "approaches": [
            {"name": "SAGA", "pros": "...", "cons": "..."},
            {"name": "2PC", "pros": "...", "cons": "..."}
          ],
          "decision": "SAGA 선정",
          "decisionRationale": "Driver X(QA scalability=high)에 부합 ...",
          "tradeoffs": "2PC 는 강일관성 유리하나 가용성 저해로 기각",
          "risk": "...",
          "sensitivity": "...",
          "pros": "",
          "cons": "",
          "relatedRequirements": ["PROJ-12", "PROJ-15"],
          "mode": "create",
          "existingKey": "",
          "modifyReason": "",
          "deleteReason": ""
        }
      ],
      "document": "전체 design-decisions.md 본문",
      "coverageMatrix": {
        "PROJ-12": ["ADSN-1"],
        "PROJ-15": ["ADSN-1", "DDSN-2"]
      }
    }
  ],
  "selectedIter": 1,
  "finalDesigns": [],
  "finalDocument": "",
  "finalCoverage": {},
  "stopReason": "pass | max_iter | repeat_critical | regression",
  "unresolvedCritical": []
}
```

**규칙**
- `finalDesigns` / `finalDocument` / `finalCoverage` 는 `selectedIter` 것과 동일
- `coverageMatrix` 에 `drivers` 모든 키 포함
- document 는 design-decisions.md 전체 본문
- 추가할 설계가 없으면 `finalDesigns: []`
- `relatedRequirements` 는 실존 JIRA key 만
- `designLevel` 값 정확히 `Architectural` 또는 `Detailed`
