---
name: design-decide2
description: Architecture Driver + 선정 시스템 스타일을 기반으로 Design Point 를 식별하고 대안 비교로 ADSN/DDSN 설계 결정(ADR)을 생성합니다. Driver 커버리지 자가검증 + self-loop(최대 3회) + Requirement-Design Contains 링크 + architecture/design-decisions.md 저장. Claude Code 네이티브(curl/jq).
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
경험 많은 소프트웨어 아키텍트. **모든 Driver 가 최소 1 설계 결정으로 추적**되도록 하고, 각 결정을 **ADR(대안 비교 + 선정 + 근거 + 트레이드오프)** 구조로 작성.

---

## Step 0 — 데이터 수집

```bash
PLATFORM_API="${PLATFORM_API:-http://localhost:9000/api/v2}"
GOJIRA_API="${GOJIRA_API:-http://localhost:9001/api/v2}"

# 프로젝트 archSettings (chosenStyles, normalizedAttributes)
echo "=== archSettings ==="
curl -s "${PLATFORM_API}/Project/${CODE}" -H "X-User-Id: ${USER}" | jq '.archSettings // {}'

# Architecture Drivers (driverSelected=true)
echo
echo "=== Drivers ==="
curl -s "${GOJIRA_API}/Issue" -H "XBody: {\"prjID\":\"${CODE}\",\"infoType\":\"\"}" -H "X-User-Id: ${USER}" \
  | jq '[.[] | select(.issueType=="Requirement" and .driverSelected==true) | {key,subkey,summary,reqtype,qad,qai,priority,risk,sensitivity}]'

# Requirement 전체 (relatedRequirements 유효 키 풀)
echo
echo "=== Requirement 전체 ==="
curl -s "${GOJIRA_API}/Issue" -H "XBody: {\"prjID\":\"${CODE}\",\"infoType\":\"\"}" -H "X-User-Id: ${USER}" \
  | jq '[.[] | select(.issueType=="Requirement") | {key,subkey,summary,reqtype}]'

# 기존 ADSN/DDSN
echo
echo "=== 기존 설계 이슈 ==="
curl -s "${GOJIRA_API}/Issue" -H "XBody: {\"prjID\":\"${CODE}\",\"infoType\":\"\"}" -H "X-User-Id: ${USER}" \
  | jq '[.[] | select(.issueType=="Architectural Design" or .issueType=="Detailed Design") | {key,subkey,issueType,summary}]'
```

선행 부족 (drivers 0건 / chosenStyles 비어있음 / normalizedAttributes 없음) → **중단** 하고 "style_define 을 먼저 실행하세요" 보고.

---

## Step 1 — 초안 생성 (iter 1)

### 1.1 Design Point 식별
Driver / 스타일 / 품질속성에서 설계 결정이 필요한 관심사 도출. 예:
- 데이터 일관성 전략 (strong/eventual, 분산 트랜잭션)
- 통신 패턴 (sync REST vs async event)
- 배포/실행 모델, 인증/인가, 모듈 경계, 장애 처리, 관측성, 배포 전략

**모든 Driver 는 최소 1 Design Point 로 다루어져야 한다.**

### 1.2 대안 비교 (ADR 필수)
각 Design Point 에 **approach 2개 이상** + pros/cons + Driver 적합도.
대안 1개뿐이면 결정이 아님 — 반드시 2개 이상 검토.

### 1.3 결정 + 근거
- `decision`: 선정 approach 이름
- `decisionRationale`: Driver (QA/CNST) 에 1:1 추적 가능한 근거
- `tradeoffs`: 미선정 대안 trade-off, 기각 이유
- `risk` / `sensitivity`: (선택)

### 1.4 ADSN / DDSN 분류
- **ADSN (Architectural)**: 구조/스타일/패턴
- **DDSN (Detailed)**: 알고리즘/데이터 구조/인터페이스/라이브러리

### 1.5 Requirement 매핑
`relatedRequirements` 에 Requirement 실존 JIRA key 배열. FR/NFR/CNST 모두 가능.

### 1.6 docId 부여
각 설계에 `docId: "ADSN-N"` 또는 `"DDSN-N"` (1부터 순차). 문서 본문(표 ID 열, 섹션 헤딩, 매트릭스) 에 동일 docId 사용. Step 5 에서 실제 subkey(`ADSN_NNN`/`DDSN_NNN`) 로 치환.

### 1.7 mode
`create` / `reuse` / `modify` / `delete_mark` (existingKey 필요).

### 1.8 architecture/design-decisions.md 작성

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
- 결정 근거: Driver X(scalability=high) 에 부합 ...
- 트레이드오프: (B) 는 ... 유리하나 ... 이유로 기각
- 리스크 / 민감도: ...

## 3. Driver 추적성 매트릭스
| Driver | 담당 설계 이슈 |

## 4. 설계 결정의 리스크 요약
```

---

## Step 2 — 자가검증 (ADR 품질 + Driver 커버리지)

### Driver 커버리지 매트릭스 (필수)
Driver 목록의 **모든 키**가 최소 1 설계의 `relatedRequirements` 에 포함되는지 점검. 미커버 시 Design Point 추가 (Step 1.1~1.7 반복).

### critical 기준
- 설계 필수 필드 (summary, designPoint, approaches, decision, decisionRationale, designLevel, docId, relatedRequirements) 누락
- **approaches 가 2개 미만** (alternatives 미고려 — ADR 아님)
- **decisionRationale 비어있음** 또는 Driver 추적 불가
- **어느 Driver 에도 연결되지 않는 설계 결정** (rationale 추적성 결여)
- **Driver 1건이라도 미커버**
- `relatedRequirements` 가 Requirement 전체 목록에 없는 키 참조 (hallucination)
- `designLevel` 이 `Architectural`/`Detailed` 이외 값
- docId 중복/누락 또는 형식 위반 (`ADSN-N`/`DDSN-N` 아님)
- 선정 스타일과 근거 없이 모순되는 결정

### 점수
- `score`: 0.0~1.0
- `score_effective = score - 0.5 × |critical|`
- `pass`: `score_effective >= 0.8`

---

## Step 3 — 수정 (pass=false)

critical + suggestions 항목만 **국소 수정**. 나머지 원문 유지. 수정 후 Step 2.

---

## Step 4 — 종료 조건
1. pass=true
2. iter 3 완료
3. 같은 critical 2회 연속 재발
4. 새 수정이 이전 best 보다 낮은 score_effective → 포기

**best-so-far**: `(designs, document, coverageMatrix, score_effective)` 기록. 종료 시 최댓값 iter 를 selected.

---

## Step 5 — 이슈 생성 + 링크

### 5.1 설계 이슈 생성 (create 모드)
```bash
RESP=$(curl -s -X POST "${GOJIRA_API}/Issue" \
  -H "Content-Type: application/json" -H "X-User-Id: ${USER}" \
  -d @- <<EOF
{
  "prjID": "${CODE}",
  "issueType": "Architectural Design",
  "summary": "...",
  "desc": "...",
  "user": "${USER}"
}
EOF
)
KEY=$(echo "$RESP" | jq -r '.key')
```

`issueType` 은 `designLevel=Architectural` → `"Architectural Design"`, `Detailed` → `"Detailed Design"`.

이후 도메인 필드(designPoint/approaches/decision/decisionRationale/tradeoffs/risk/sensitivity/designLevel) 는 PUT /Issue 로 업데이트:
```bash
curl -s -X PUT "${GOJIRA_API}/Issue" \
  -H "Content-Type: application/json" -H "X-User-Id: ${USER}" \
  -d "$(jq -n --arg p "$CODE" --arg k "$KEY" --arg u "$USER" \
    --arg dp "$DESIGN_POINT" --argjson ap "$APPROACHES_JSON" \
    --arg dc "$DECISION" --arg dr "$RATIONALE" --arg tr "$TRADEOFFS" \
    --arg rk "$RISK" --arg se "$SENS" --arg dl "$LEVEL" \
    '{prjID:$p, key:$k, user:$u, data:{designPoint:$dp, approaches:($ap|tostring), decision:$dc, decisionRationale:$dr, tradeoffs:$tr, risk:$rk, sensitivity:$se, designLevel:$dl}}')"
```

### 5.2 Requirement → Design Contains 링크
각 설계의 `relatedRequirements` 의 각 req 키에 대해:
```bash
curl -s -X POST "${GOJIRA_API}/Issue/link" \
  -H "Content-Type: application/json" -H "X-User-Id: ${USER}" \
  -d "$(jq -n --arg p "$CODE" --arg iw "$REQ_KEY" --arg ow "$DESIGN_KEY" --arg lt "Contains(WBSGantt)" --arg u "$USER" \
    '{prjID:$p, inwardKey:$iw, outwardKey:$ow, linkType:$lt, user:$u}')"
```

링크 실패 시 **silent drop 금지** — 사용자 보고.

### 5.3 docId → subkey 치환
각 생성 설계의 subkey (ADSN_001, DDSN_001...) 를 docId 와 매핑.
```bash
DOCUMENT=$(echo "$DOCUMENT" | sed "s/ADSN-1\b/ADSN_001/g; s/DDSN-1\b/DDSN_001/g")
```

### 5.4 design-decisions.md 저장
```bash
curl -s -X PUT "${PLATFORM_API}/DocRepo/${CODE}/file" \
  -H "Content-Type: application/json" -H "X-User-Id: ${USER}" \
  -d "$(jq -n --arg c "$DOCUMENT" '{path:"architecture/design-decisions.md", content:$c, message:"설계 결정 (design-decide2)"}')"
```

---

## Step 6 — 최종 보고

```
## 설계 결정 완료 (v2 Claude Code)

- 설계 이슈 생성: ADSN N건 / DDSN M건 (키 리스트)
- Requirement-Design 링크: L건
- 문서 저장: architecture/design-decisions.md

### Driver 커버리지 매트릭스
| Driver | 담당 설계 |
|---|---|
| PROJ-N (QA_001) | ADSN-1, DDSN-2 |
| PROJ-M (CNST_001) | ADSN-3 |

### Self-loop
- iter N회 / selected=iter X (score_eff Y) / stop=pass|max_iter|...
- unresolvedCritical: [...]
```

---

## 주의
- 모든 curl 에 `X-User-Id: ${USER}` 필수 (403 방지)
- 링크 linkType 은 정확히 `Contains(WBSGantt)`
- `issueType` 은 `Architectural Design` / `Detailed Design` (공백 포함, 정확히)
- `relatedRequirements` 는 Step 0 에서 수집한 Requirement 실존 key 만 사용 (hallucination 금지)
- `approaches` 는 2개 이상 — 대안이 1개면 ADR 이 아니므로 critical
- design-decisions.md 는 UTF-8 plain markdown (Platform-API 가 서버측 base64 인코딩)
