---
name: design-decide2
description: Architecture Driver + 선정 시스템 스타일을 기반으로 Design Point 를 식별하고 대안 비교로 ADSN/DDSN 설계 결정(ADR)을 생성합니다. Driver 커버리지 자가검증 + self-loop(최대 3회) + Requirement-Design Contains 링크 + architecture/design-decisions.md 저장. Claude Code 네이티브(curl/jq).
allowed-tools: Bash, Read
---

## 한 generation 당 제약 (GLM 계열 스톨 방지 — 필독)

- **한 번의 LLM generation 에서 Step 두 개 이상 처리 금지**. 각 Step 종료는 **반드시 Bash tool call 로 마무리** 해 generation 경계 형성
- reasoning 은 각 Step 당 3~5 줄 이내. 장기 추론 불필요. 대안 비교는 표 형식으로 간결하게
- Step 1 완료 시 **반드시 `draft-io.sh save`** — designs / document 전체 JSON 을 파일에 flush 후 generation 종료
- Step 2 자가검증은 **반드시 `validate-design-decisions.py` 호출** — 에이전트가 Driver 커버리지 / approach 개수 / hallucination 직접 계산 금지
- critical 수정 후 다시 `draft-io.sh save` → 이어 `validate-design-decisions.py` 로 재검증. 한 generation 당 1 iter
- assistant 출력에는 긴 document JSON 뱉지 말 것. "저장 완료 / critical N건" 한두 줄만

## 환경
- `PLATFORM_API` 기본값: `http://localhost:9000/api/v2`
- `GOJIRA_API` 기본값: `http://localhost:9001/api/v2`
- 임시 파일: `/tmp/${CODE}-dd-{draft|validation}.json`

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

**반드시** 아래 템플릿 파일의 구조를 따른다. 섹션 1~4 와 ADR 필수 필드를 준수:

```bash
cat "${PROJECT_ROOT}/.claude/skills/design-decide2/templates/design-decisions.md"
```

- 섹션 1 Design Point 요약 / 2 설계 결정 상세 (ADR 형식) / 3 Driver 추적성 매트릭스 / 4 리스크 요약 전부 필수
- ADR 필수 필드: 대안 2개 이상, pros/cons, 선정 + 근거, 트레이드오프, 리스크 + 민감도

---

### Step 1 종료 — draft flush (필수)

```bash
cat > /tmp/${CODE}-dd-draft-input.json <<'JSON'
{
  "driverKeys": [ ...Driver 키 (driverSelected=true Requirement)... ],
  "requirementKeys": [ ...Step 0 Requirement 전체 키 목록... ],
  "designs": [ ...Step 1.1~1.7 산출 설계 목록 (approaches 배열 + relatedRequirements 등 포함)... ],
  "document": "...Step 1.8 작성 마크다운 전체..."
}
JSON
bash "${PROJECT_ROOT}/.claude/skills/_scripts/draft-io.sh" save "${CODE}" "dd" "draft" \
  < /tmp/${CODE}-dd-draft-input.json
```

---

## Step 2 — 자가검증 (스크립트 기반)

**반드시 `validate-design-decisions.py` 호출**. 에이전트가 직접 검증 금지.

```bash
bash "${PROJECT_ROOT}/.claude/skills/_scripts/draft-io.sh" load "${CODE}" "dd" "draft" \
  | python3 "${PROJECT_ROOT}/.claude/skills/_scripts/validate-design-decisions.py" \
  > /tmp/${CODE}-dd-validation.json
cat /tmp/${CODE}-dd-validation.json
```

스크립트가 잡는 critical:
- `missing_field` — 필수 필드 누락 (summary/designPoint/approaches/decision/decisionRationale/designLevel/docId/relatedRequirements)
- `insufficient_approaches` — approaches < 2 (ADR 아님)
- `empty_rationale` — decisionRationale 비어있음
- `no_requirement_link` / `relatedRequirements_hallucination`
- `driver_uncovered` — Driver 1건이라도 미매핑
- `bad_designLevel` — Architectural/Detailed 이외
- `docId_level_mismatch` — ADSN-N 은 Architectural, DDSN-N 은 Detailed
- `bad_docId_format` / `duplicate_docId`
- `missing_section` — 섹션 1~4 누락

### 통과 기준
- `pass == true` → Step 5 진입
- `pass == false` → Step 3 수정

---

## Step 3 — 수정 (pass=false)

critical 만 **국소 수정** 후 draft 재저장:

```bash
bash "${PROJECT_ROOT}/.claude/skills/_scripts/draft-io.sh" save "${CODE}" "dd" "draft" \
  < /tmp/${CODE}-dd-draft-input.json
```

이 tool call 로 generation 종료. 다음 generation 에서 validate 재실행.

---

## Step 4 — 종료 조건
1. pass=true
2. iter 3 완료
3. 같은 critical 2회 연속 재발
4. 새 수정이 이전 best 보다 낮은 score_effective → 포기

**best-so-far**: `(designs, document, coverageMatrix, score_effective)` 기록. 종료 시 최댓값 iter 를 selected.

---

## Step 5 — 이슈 생성 + 링크

### 5.1 설계 이슈 일괄 생성 (create 모드)

create 모드 항목을 JSON 배열로 모아 batch 스크립트 **1회** 실행.
`issueType` 은 `designLevel=Architectural` → `"Architectural Design"`, `Detailed` → `"Detailed Design"`.

```bash
cat > /tmp/design_items.json <<'JSON'
[
  {"issueType":"Architectural Design","summary":"...","desc":"..."},
  {"issueType":"Detailed Design","summary":"...","desc":"..."}
]
JSON

bash "${PROJECT_ROOT}/.claude/skills/_scripts/issue-batch-create.sh" "${CODE}" "${USER}" < /tmp/design_items.json \
  > /tmp/design_result.json
cat /tmp/design_result.json
```

결과의 `keys` 배열에서 생성된 이슈 key 확인 후, 도메인 필드를 PUT /Issue 로 업데이트:
```bash
curl -s -X PUT "${GOJIRA_API}/Issue" \
  -H "Content-Type: application/json" -H "X-User-Id: ${USER}" \
  -d "$(jq -n --arg p "$CODE" --arg k "$KEY" --arg u "$USER" \
    --arg dp "$DESIGN_POINT" --argjson ap "$APPROACHES_JSON" \
    --arg dc "$DECISION" --arg dr "$RATIONALE" --arg tr "$TRADEOFFS" \
    --arg rk "$RISK" --arg se "$SENS" --arg dl "$LEVEL" \
    --arg obj "$OBJECTIVES" --arg pr "$PROS" --arg cn "$CONS" \
    --arg sc "$SCORE" --arg ev "$EVAL" --arg dm "$DMODEL" \
    '{prjID:$p, key:$k, user:$u, data:{designPoint:$dp, approaches:($ap|tostring), decision:$dc, decisionRationale:$dr, tradeoffs:$tr, risk:$rk, sensitivity:$se, designLevel:$dl, objectives:$obj, pros:$pr, cons:$cn, score:$sc, eval:$ev, dmodel:$dm}}')"
```

**ADSN/DDSN 필드 매핑**:
| 필드 | 내용 |
|---|---|
| `designPoint` | Design Point 이름 |
| `approaches` | 검토된 대안 목록 (JSON 문자열) |
| `decision` | 선정된 대안명 |
| `decisionRationale` | 선정 근거 (Driver 역추적 포함) |
| `objectives` | 이 설계의 주요 목표 |
| `pros` | 선정 대안의 장점 |
| `cons` | 선정 대안의 단점 |
| `score` | 평가 점수 |
| `eval` | 평가 내용 요약 |
| `dmodel` | 적용 도메인 모델/시스템 영역 |
| `tradeoffs` | 트레이드오프 원문 |
| `risk` | 위험 사항 |
| `sensitivity` | 민감도 |
| `designLevel` | `Architectural` 또는 `Detailed` |

### 5.2 Requirement → Design Contains 링크 일괄 생성
각 설계의 `relatedRequirements` 에서 링크 JSON 배열을 구성해 batch 스크립트 **1회** 실행:
```bash
cat > /tmp/design_links.json <<'JSON'
[
  {"inwardKey":"GOJIRA3-10","outwardKey":"GOJIRA3-30","linkType":"Contains(WBSGantt)"},
  ...
]
JSON

bash "${PROJECT_ROOT}/.claude/skills/_scripts/issue-batch-link.sh" "${CODE}" "${USER}" < /tmp/design_links.json \
  > /tmp/design_link_result.json
cat /tmp/design_link_result.json
```

링크 실패 시 **silent drop 금지** — 사용자 보고.

### 5.3 docId → subkey 치환
각 생성 설계의 subkey (ADSN_001, DDSN_001...) 를 docId 와 매핑.
```bash
DOCUMENT=$(echo "$DOCUMENT" | sed "s/ADSN-1\b/ADSN_001/g; s/DDSN-1\b/DDSN_001/g")
```

### 5.4 design-decisions.md 저장
```bash
echo "$DOCUMENT" | bash "${PROJECT_ROOT}/.claude/skills/_scripts/doc-save.sh" "${CODE}" "${USER}" "architecture/design-decisions.md"
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
