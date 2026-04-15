---
name: satam-analyze2
description: ADSN(Architectural Design) 이슈의 risk/sensitivity/tradeoffs 원문을 구조화 YAML 로 변환해 각 ADSN 의 risk/sensitivity/mitigation 필드에 저장합니다. QA(NFR) 매핑 자가검증 + self-loop(최대 3회). 문서 파일 생성 없음. Claude Code 네이티브(curl/jq).
allowed-tools: Bash, Read
---

## 환경
- `PLATFORM_API` 기본값: `http://localhost:9000/api/v2`
- `GOJIRA_API` 기본값: `http://localhost:9001/api/v2`

## 인자 추출

사용자 메시지에서 다음 값들을 추출해 **먼저** 환경변수로 export 한 뒤 아래 bash 블록들을 실행하라. 누락 시 사용자에게 되물어 확정하라.

- `CODE` — projectCode (예: GOJIRA3)
- `USER` — user (예: batman)

예:
```bash
export CODE="GOJIRA3"
export USER="batman"
```

## 역할
소프트웨어 아키텍트. ATAM 변형(SATAM)으로 **각 ADSN 의 아키텍처 결정이 품질속성(QA)에 미치는 영향**을 구조화한다. 자유 서술 원문을 Risk / Sensitivity Point / Trade-off 세 축으로 분해하고, 영향 받는 QA(NFR) 를 명시한다.

- Risk : 결정으로 인해 QA 가 저하될 수 있는 지점
- Sensitivity Point : 결정이 특정 QA 에 크게 영향을 주는 지점
- Trade-off : 한 QA 를 얻기 위해 다른 QA 를 포기하는 지점 — Counter Risk 명시 필수

원문에 없는 내용 창작 금지.

---

## Step 0 — 데이터 수집

```bash
PLATFORM_API="${PLATFORM_API:-http://localhost:9000/api/v2}"
GOJIRA_API="${GOJIRA_API:-http://localhost:9001/api/v2}"

# ADSN 목록 (issueLinks 포함)
echo "=== ADSN 목록 ==="
curl -s "${GOJIRA_API}/Issue" \
  -H "XBody: {\"prjID\":\"${CODE}\",\"infoType\":\"\"}" \
  -H "X-User-Id: ${USER}" \
  | jq '[.[] | select(.issueType=="Architectural Design")
         | {key, subkey, summary, designPoint, decision,
            risk, sensitivity, tradeoffs, mitigation, issueLinks}]'

# QA 참조 (Requirement 중 reqtype=NFR)
echo
echo "=== QA(NFR) 참조 ==="
curl -s "${GOJIRA_API}/Issue" \
  -H "XBody: {\"prjID\":\"${CODE}\",\"infoType\":\"\"}" \
  -H "X-User-Id: ${USER}" \
  | jq '[.[] | select(.issueType=="Requirement" and .reqtype=="NFR")
         | {key, subkey, summary}]'
```

ADSN 이 0건이면 **중단** 하고 "design_decide 를 먼저 실행하세요" 보고.

각 ADSN 에 대해 `issueLinks` 중 타입명이 "Contains" 를 포함하는 링크에서 `inwardIssue.key` / `outwardIssue.key` 를 추출 → `linkedQAs` 로 사용.

---

## Step 1 — 초안 생성 (iter 1)

각 ADSN 별로 아래를 수행해 `mapping[<ADSN_KEY>]` 엔트리 생성:

1. `risk` 원문을 의미 단위 분해 → Risk 리스트 (id: R1, R2, ...)
2. `sensitivity` 원문을 분해 → Sensitivity 리스트 (id: S1, S2, ...)
3. `tradeoffs`/`mitigation` 원문을 분해 → Trade-off 리스트 (id: M1, M2, ...)
4. 각 항목에 `qa` 배열 (영향 받는 QA 이슈 key) 부여
   - `linkedQAs` 우선
   - 부족 시 QA 참조 목록에서 summary 의미 매칭
   - 억지 매핑 금지 — 불명확하면 `qa: []`
5. Trade-off 항목에 `counterRisk: ["R<id>:<QA_KEY>", ...]` 필수 (같은 ADSN 내 Risk ID)

**ID 규칙**: ADSN 내부에서만 유일 — 다른 ADSN 과 번호 공유 금지.
**키 규칙**: 모든 키(ADSN key, qa[] 원소)는 **이슈 key** (예: `GOJIRA3-23`). subkey 금지.
**빈 원문**: 해당 리스트는 `[]`.

---

## Step 2 — 자가검증

### critical 기준 (품질 속성 연결이 핵심)
- ADSN 키 누락 또는 Step 0 목록에 없는 키 생성
- `qa` 가 `linkedQAs` / QA 참조 목록에 없는 키 참조 (hallucination)
- **품질 속성 참조 전면 누락** — 모든 항목의 `qa` 가 빈 배열인데 QA 참조가 존재
- 원문이 있음에도 Risk/Sensitivity/Trade-off 중 한 카테고리가 0 (카테고리 누락)
- Trade-off 에 `counterRisk` 누락 또는 외부 ADSN 의 Risk ID 참조 (근거 없는 trade-off)
- subkey 혼용
- id 중복/누락 / 순번 불연속

### 점수
- `score`: 0.0~1.0
- `score_effective = score - 0.5 × |critical|`
- `pass = score_effective >= 0.8`

---

## Step 3 — 수정 (pass=false)

critical + suggestions 항목만 **국소 수정**. 나머지 원문 유지. 수정 후 Step 2 로.

---

## Step 4 — 종료 조건
1. pass=true
2. iter 3 완료
3. 같은 critical 2회 연속 재발
4. 새 수정이 이전 best 보다 낮은 score_effective → 포기

**best-so-far**: 매 iter 의 `(mapping, score_effective)` 기록. 종료 시 최댓값 iter 의 mapping 을 selected.

---

## Step 5 — 이슈 필드 저장 (문서 생성 금지)

각 ADSN 에 대해 YAML 직렬화 후 PUT /Issue 로 `risk` / `sensitivity` / `mitigation` 3개 필드 동시 업데이트.

### 5.1 YAML 직렬화 포맷 (프론트엔드 parseRST 규격)

Risk / Sensitivity:
```
- ID : R1
  QA : [GOJIRA3-123, GOJIRA3-124]
  Description : ...
```

Mitigation (Counter Risk 라인 추가):
```
- ID : M1
  QA : [GOJIRA3-123]
  Counter Risk : ["R1:GOJIRA3-123"]
  Description : ...
```

- 항목 없는 리스트는 해당 필드를 빈 문자열 ""
- Description 은 한 줄로 (개행 공백 치환)

### 5.2 PUT /Issue

```bash
curl -s -X PUT "${GOJIRA_API}/Issue" \
  -H "Content-Type: application/json" -H "X-User-Id: ${USER}" \
  -d "$(jq -n \
    --arg p "$CODE" --arg k "$ADSN_KEY" --arg u "$USER" \
    --arg r "$RISK_YAML" --arg s "$SENS_YAML" --arg m "$MITI_YAML" \
    '{prjID:$p, key:$k, user:$u, data:{risk:$r, sensitivity:$s, mitigation:$m}}')"
```

### 5.3 완료 플래그

프로젝트 `archSettings.satamDone=true` 는 Platform-API 경로로 업데이트하거나 Agent-Service 와 합의된 경로 사용. (로컬 네이티브 실행 시 생략 가능 — 사용자에게 수동 플래그 필요 보고)

### 5.4 문서 생성 금지
본 스킬은 business/*.md 파일을 **생성하지 않는다**. 프론트엔드 SATAM 컴포넌트가 세 필드를 직접 파싱해 렌더링한다.

---

## Step 6 — 최종 보고

```
## SATAM 분석 완료 (v2 Claude Code)

- ADSN 업데이트: N건 (키 리스트)
- Risks 총합: X / Sensitivities: Y / Trade-offs: Z
- 실패: F건 (키 + 사유)

### QA 매핑 요약
| ADSN | Risks | Sensitivities | Trade-offs | 참조 QA 수 |

### Self-loop
- iter N회 / selected=iter X (score_eff Y) / stop=pass|max_iter|...
- unresolvedCritical: [...]
```

이어서 `ui_navigate(page=satam)` 로 SATAM 화면 전환 안내.

---

## 주의
- 모든 curl 에 `X-User-Id: ${USER}` 헤더 필수 (403 방지)
- `qa` 배열 및 ADSN 매핑 키는 반드시 **이슈 key** (subkey 금지)
- Trade-off 의 `counterRisk` 는 **같은 ADSN 내** Risk ID 만 참조 (`R<id>:<QA_KEY>` 형식)
- 문서 파일 생성 절대 금지 — 이슈 필드 3종 반영만
- Step 0 / Step 5 만 실제 Bash 호출. Step 1~4 는 에이전트 내부 추론.
