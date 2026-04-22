---
name: satam-analyze2
description: ADSN(Architectural Design) 이슈의 risk/sensitivity/tradeoffs 원문을 구조화 YAML 로 변환해 각 ADSN 의 risk/sensitivity/mitigation 필드에 저장합니다. QA(NFR) 매핑 자가검증 + self-loop(최대 3회). 문서 파일 생성 없음. Claude Code 네이티브(curl/jq).
allowed-tools: Bash, Read
---

## 한 generation 당 제약 (GLM 계열 스톨 방지 — 필독)

- **한 번의 LLM generation 에서 Step 두 개 이상 처리 금지**. 각 Step 종료는 **반드시 Bash tool call 로 마무리** 해 generation 경계 형성
- reasoning 은 각 Step 당 3~5 줄 이내. 원문 해석은 Step 1 1회에만 집중
- Step 1 매핑 초안 종료는 **반드시 `draft-io.sh save`** — 전체 매핑 JSON 을 파일에 flush 후 그 generation 종료
- Step 2 자가검증은 **반드시 `validate-satam.py` 호출** — 에이전트가 직접 qa hallucination / id 중복 / counterRisk 형식 계산 금지
- critical 수정 후 다시 `draft-io.sh save` → 이어 `validate-satam.py` 로 재검증. 한 generation 당 1 iter
- assistant 출력에는 전체 매핑 JSON 뱉지 말 것. "/tmp/... 저장" / "critical N건" 한두 줄만

## 진행 상태 가시화 — `todowrite` (선택, OpenCode)

OpenCode 환경이면 각 Step 진입 시 내장 `todowrite` 로 현재/남은 단계를 알려주면 UX 가 좋다. 강제 아님, Claude Code 에서는 무시. 자세한 규칙은 `.claude/skills/_shared/todowrite-guide.md`.

이 스킬 Step 체크리스트:
```
# Todos
[ ] Step 0 — ADSN + NFR(QA) 데이터 수집
[ ] Step 1 — risk/sensitivity/mitigation 매핑 초안
[ ] Step 2 — 자가검증 (validate-satam.py)
[ ] Step 3 — critical 수정 (반복)
[ ] Step 5 — ADSN PUT + satamDone 플래그 영속화
[ ] Step 6 — 최종 보고
```

## 환경
- `PLATFORM_API` 기본값: `http://localhost:9000/api/v2`
- `GOJIRA_API` 기본값: `http://localhost:9001/api/v2`
- 임시 파일: `/tmp/${CODE}-satam-{context|draft|validation}.json`

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

### Step 0 종료 — 수집 데이터 flush (필수)

수집한 ADSN 목록과 QA 참조 키를 context 파일로 저장. Step 1 이후 모든 generation 이 이 파일을 load 해서 참조.

```bash
ADSN_JSON=$(curl -s "${GOJIRA_API}/Issue" \
  -H "XBody: {\"prjID\":\"${CODE}\",\"infoType\":\"\"}" -H "X-User-Id: ${USER}" \
  | jq '[.[] | select(.issueType=="Architectural Design")
         | {key, summary, risk, sensitivity, tradeoffs, mitigation,
            linkedQAs: [(.issueLinks // [])[]
                        | select(.type.name | test("Contains"))
                        | (.inwardIssue.key // .outwardIssue.key)] | unique}]
        | reduce .[] as $i ({}; . + {($i.key): {risk:$i.risk, sensitivity:$i.sensitivity, tradeoffs:$i.tradeoffs, mitigation:$i.mitigation, linkedQAs:$i.linkedQAs}})')

QA_KEYS=$(curl -s "${GOJIRA_API}/Issue" \
  -H "XBody: {\"prjID\":\"${CODE}\",\"infoType\":\"\"}" -H "X-User-Id: ${USER}" \
  | jq '[.[] | select(.issueType=="Requirement" and .reqtype=="NFR") | .key]')

jq -n --argjson a "$ADSN_JSON" --argjson q "$QA_KEYS" '{adsns:$a, qaKeys:$q}' \
  | bash "${PROJECT_ROOT}/.claude/skills/_scripts/draft-io.sh" save "${CODE}" "satam" "context"
```

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

### Step 1 종료 — draft flush (필수)

context 파일(`/tmp/${CODE}-satam-context.json`) 에 mapping 을 추가해 draft 로 저장:

```bash
CTX=$(bash "${PROJECT_ROOT}/.claude/skills/_scripts/draft-io.sh" load "${CODE}" "satam" "context")

# MAPPING 은 위에서 생성한 mapping 전체 JSON (에이전트가 이 generation 에서 만든 유일한 산출물)
echo "$CTX" | jq --argjson m "$MAPPING" '. + {mapping:$m}' \
  | bash "${PROJECT_ROOT}/.claude/skills/_scripts/draft-io.sh" save "${CODE}" "satam" "draft"
```

이 tool call 로 generation 종료. 다음 generation 에서 validate.

---

## Step 2 — 자가검증 (스크립트 기반)

**반드시 `validate-satam.py` 호출**. 에이전트가 직접 검증 금지.

```bash
bash "${PROJECT_ROOT}/.claude/skills/_scripts/draft-io.sh" load "${CODE}" "satam" "draft" \
  | python3 "${PROJECT_ROOT}/.claude/skills/_scripts/validate-satam.py" \
  > /tmp/${CODE}-satam-validation.json
cat /tmp/${CODE}-satam-validation.json
```

출력 스키마:
```json
{
  "pass": bool,
  "score": 0.0~1.0,
  "score_effective": number,
  "critical": [{"type":"...","adsn":"...","id":"...","detail":"..."}],
  "suggestions": [...],
  "stats": {"adsns":N,"risks":R,"sensitivities":S,"tradeoffs":T,"qaCoveredADSNs":Q}
}
```

스크립트가 잡는 critical:
- `unknown_adsn` — Step 0 목록에 없는 키
- `category_empty` — 원문 있는데 리스트 비어있음
- `bad_id_format` — R1/S1/M1 형식 위반
- `duplicate_id` — ADSN 내부 id 중복
- `bad_qa_type` / `qa_hallucination` — qa 배열 오류 또는 미존재 키
- `missing_description` / `missing_counter_risk` / `bad_counter_risk_format`
- `counter_risk_external_adsn` — 외부 ADSN 의 Risk 참조
- `no_qa_referenced` — 전 항목 qa=[]

### 통과 기준
- `pass == true` → Step 5 진입
- `pass == false` → Step 3 수정

---

## Step 3 — 수정 (pass=false)

critical 항목만 **국소 수정** 후 draft 재저장:

```bash
CTX=$(bash "${PROJECT_ROOT}/.claude/skills/_scripts/draft-io.sh" load "${CODE}" "satam" "context")
echo "$CTX" | jq --argjson m "$MAPPING_FIXED" '. + {mapping:$m}' \
  | bash "${PROJECT_ROOT}/.claude/skills/_scripts/draft-io.sh" save "${CODE}" "satam" "draft"
```

이 tool call 로 generation 종료. 다음 generation 에서 validate 재실행.

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

### 5.3 완료 플래그 영속화 — `archSettings.satamDone=true`

ADSN 전체 업데이트가 성공하면 (5.2 모든 PUT 이 2xx) 프로젝트 플래그를 세움. 프런트엔드 SATAM 화면이 이 값을 기준으로 완료 여부 렌더.

```bash
curl -s -X PATCH "${GOJIRA_API}/Project/${CODE}/archSettings" \
  -H "Content-Type: application/json" -H "X-User-Id: ${USER}" \
  -d '{"satamDone": true}' \
  | jq '{updated, archSettings}'
```

5.2 에서 한 건이라도 PUT 실패했으면 이 호출은 **스킵** 하고 보고에 ⚠ 로 표시 (부분 성공 상태를 완료로 표기하지 않음).

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
- Step 0 flush / Step 1 draft save / Step 2 validate / Step 3 재-save / Step 5 PUT 루프 **모두 Bash tool call** 로 종료. 한 generation 에 몰아서 처리 금지
