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

### 기존 전역 ID 풀 파악 (ID 재사용·중복 방지)

v2 규칙상 R/S/M 은 프로젝트 전역 유일. 기존 ADSN 의 risk/sensitivity/mitigation 필드에서 이미 쓰인 ID 를 파악해 둔다. Step 1 에서 같은 내용이면 재사용, 새 항목이면 최대 번호 + 1:

```bash
# risk/sensitivity/mitigation 원문에서 "- ID : R{N}" 패턴 추출
python3 <<'PY'
import json, re, os, subprocess
ctx = subprocess.run(
    ["bash", os.environ["PROJECT_ROOT"] + "/.claude/skills/_scripts/draft-io.sh",
     "load", os.environ["CODE"], "satam", "context"],
    capture_output=True, text=True).stdout
d = json.loads(ctx) if ctx.strip() else {}
ids = {"R": set(), "S": set(), "M": set()}
for adsn in d.get("adsns", {}).values():
    for field, prefix in [("risk","R"), ("sensitivity","S"), ("mitigation","M")]:
        for m in re.findall(r"^-\s*ID\s*:\s*([RSM]\d+)", adsn.get(field,"") or "", re.M):
            if m[0] == prefix:
                ids[prefix].add(m)
print("기존 Risk IDs:", sorted(ids["R"]))
print("기존 Sensitivity IDs:", sorted(ids["S"]))
print("기존 Mitigation IDs:", sorted(ids["M"]))
def max_num(s): return max([int(x[1:]) for x in s], default=0)
print(f"신규 할당 시작: R{max_num(ids['R'])+1}, S{max_num(ids['S'])+1}, M{max_num(ids['M'])+1}")
PY
```

이 결과는 Step 1 초안 작성 시 참조. 기존 풀에 없는 새 항목에 대해서만 새 번호 발급.

---

## Step 1 — 초안 생성 (iter 1)

### 1.1 3-way 분류 기준 (v2, 현실적 엄격화)

이전 버전은 "원문에 risk 라는 단어가 있으면 전부 risks 로" 분류해 거의 모든 ADSN 에 risk·sensitivity 가 채워지는 문제가 있었습니다. v2 는 **현실 설계 맥락 기반 엄격 분류** 를 적용합니다.

| 카테고리 | 기준 | 예시 |
|---|---|---|
| **Risk (R)** | 현 수준 설계에서 **명확히 문제가 발생할 수밖에 없는** 사항. 구체적 impact·재현 조건이 이미 드러남. 정량화 가능하면 가치 높음 | "단일 DB 병목으로 초당 100 TPS 이상 처리 불가", "외부 SSO 장애 시 전 서비스 중단" |
| **Sensitivity (S)** | 현재는 허용 수준이지만 **조건이 바뀌면 Risk 로 악화** 될 사항. "~ 초과 시 / ~ 증가 시 / ~ 변화 시" 표현이 자연스러운 것 | "사용자 10만 명 초과 시 캐시 병목", "지역 확장 시 지연 증가" |
| **Concerns (C, 기타 우려)** | 관찰·검토 필요. 가능성 수준, 정량화 어렵거나 추상적, 완화 여부 불명확 | "팀의 Saga 패턴 숙련도 부족", "추후 문서화 필요", "도입한 기술의 커뮤니티 성숙도" |
| **Mitigation (M)** | tradeoffs/mitigation 원문에서 **실제 Risk 에 대한 대응** 으로 제시된 것. Counter Risk 필수 |

**분류 지침**:
- "일반적으로 ~할 수 있음", "~할 가능성" 같이 확률·추측 수준 → **Risk 아님**. Concerns 나 제외
- 원문에 풀어써진 "단점" 중 구체적 impact 없는 것은 Concerns 로
- Risk / Sensitivity 가 **0건이어도 정상** (엄격 기준 충족 못하면 빈 배열)
- Concerns 는 선택 사항 — 적을 필요 없으면 빈 배열

**완화된 Risk 제외 규칙 (중요)**:
같은 셀(동일 QA) 에서 Risk 와 해당 Risk 를 대응하는 Mitigation 이 **동시에 존재** 하면, 그 Risk 는 **이미 완화** 된 상태입니다. Risk 도 Mitigation 도 **둘 다 미기재**.
- 구체 판정: Risk `Ri` 의 `qa` 집합이 `{Qa, Qb, ...}` 일 때, 원문 tradeoffs/mitigation 에 `Ri:Qa`, `Ri:Qb`... 를 **모두** 커버하는 Mitigation 이 있으면 → `Ri` 와 그 Mitigation **둘 다 버림**
- 부분 커버 (일부 QA 만 대응) → Risk 는 커버되지 않은 QA 에 한해 남김, Mitigation 은 남김
- 여러 Risk 를 대응하는 Mitigation 은 모든 대응 Risk 가 전부 다른 Mitigation 들로 이미 제거될 때만 함께 제거. 하나라도 남아있으면 Mitigation 도 유지
- 제거된 쌍은 Concerns 로도 옮기지 않음 — 분석 노이즈 최소화가 목적

### 1.2 매핑 엔트리 생성 절차

각 ADSN 별로 `mapping[<ADSN_KEY>]` 엔트리 생성:

1. `risk` 원문 의미 단위 분해 → 위 기준에 따라 **Risk 아니면 Concerns 로 재분류**. 엄격하게 통과한 것만 Risk 리스트 (id: R1, R2, ...)
2. `sensitivity` 원문 분해 → Sensitivity 기준 부합만 S 리스트 (id: S1, S2, ...), 애매하면 Concerns 로
3. `tradeoffs`/`mitigation` 원문 분해 → Mitigation 리스트 (id: M1, M2, ...)
4. 위에서 분류 탈락한 관찰 사항들 → Concerns 리스트 (id: C1, C2, ...)
5. 각 항목에 `qa` 배열 (영향 받는 QA 이슈 key) — Concerns 는 빈 배열 허용
   - `linkedQAs` 우선, 부족 시 QA 참조 목록 summary 매칭. 억지 매핑 금지
6. Mitigation 항목에 `counterRisk: ["R<id>:<QA_KEY>", ...]` 필수 (같은 ADSN 내 Risk ID)

**ID 규칙 (v2 변경)**:
- **Risk (`R`) / Sensitivity (`S`) / Mitigation (`M`) — 프로젝트 전체 Design 기준 전역 유일**
  - 의미상 동일한 항목이 다른 ADSN 에 이미 있으면 → **같은 ID 재사용** (번호 중복 부여 금지)
  - 새 항목은 프로젝트 전역 기존 최대 번호 + 1 로 할당. 예: 기존 R1~R5 있으면 신규는 R6
  - Step 0 데이터 수집에서 기존 ADSN 의 원문으로부터 이미 쓰인 ID 풀 확보 필수
- **Concerns (`C`)** — ADSN 국소 식별자. 같은 ADSN 내부 유일이면 충분. 다른 ADSN 과 번호 공유 무방

**키 규칙**: 모든 키(ADSN key, qa[] 원소)는 **이슈 key** (예: `GOJIRA3-23`). subkey 금지.
**빈 원문**: 해당 리스트는 `[]`.

동일 ID 재사용 시 description 은 기존 것과 **의미상 같아야** 함. 문구가 완전히 동일할 필요는 없지만 같은 사실·상황을 가리켜야 함 — 상이하면 별도 신규 ID.

### 1.3 JSON 스키마

**JSON 필드명은 스키마 그대로 사용**. validator 는 **복수형 + concerns** 키만 인식. 각 항목은 `{id, description, qa}` 필수, Mitigation 은 + `counterRisk`.

```json
{
  "GOJIRA3-23": {
    "risks": [
      {"id":"R1","description":"현 단일 DB 구성으로 초당 500 TPS 초과 처리 불가","qa":["GOJIRA3-15"]}
    ],
    "sensitivities": [
      {"id":"S1","description":"사용자 10만 돌파 시 캐시 병목 가능","qa":["GOJIRA3-15"]}
    ],
    "tradeoffs": [
      {"id":"M1","description":"읽기 전용 레플리카 추가로 부하 분산","qa":["GOJIRA3-15"],"counterRisk":["R1:GOJIRA3-15"]}
    ],
    "concerns": [
      {"id":"C1","description":"Saga 패턴 팀 숙련도 — 사전 교육 필요","qa":[]},
      {"id":"C2","description":"선정 메시지 브로커의 커뮤니티 버전 장기 지원 여부 모니터링","qa":[]}
    ]
  }
}
```

- `concerns` 는 optional. 적을 게 없으면 `"concerns": []` 또는 필드 생략 가능
- `concerns[].qa` 는 빈 배열 허용 (특정 QA 에 묶이지 않는 관찰)

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

Concerns (QA 선택, Counter Risk 없음):
```
- ID : C1
  Description : 팀의 Saga 패턴 숙련도 부족 — 사전 교육 필요
- ID : C2
  QA : [GOJIRA3-123]
  Description : 특정 QA 에 묶이는 관찰이면 QA 라인 추가 가능
```

**⚠ 포맷 주의 (이전 실측 버그):**
- `QA : [...]` 는 **bare 키 + 콤마 + 공백** 형식. **따옴표로 감싸지 말 것** (`["TEST-61","TEST-55"]` ❌ → `[TEST-61, TEST-55]` ✅)
- Python 에서 생성 시 `json.dumps(qa_list)` 금지. `"[" + ", ".join(qa_list) + "]"` 로 직접 조립
- `Counter Risk` 는 **예외적으로 따옴표** 필요 (`["R1:GOJIRA3-123"]`) — 콜론 포함 문자열이라 구분자 혼동 방지
- 항목 없는 리스트는 해당 필드를 빈 문자열 ""
- Description 은 한 줄로 (개행 공백 치환)

### 5.2 PUT /Issue

```bash
curl -s -X PUT "${GOJIRA_API}/Issue" \
  -H "Content-Type: application/json" -H "X-User-Id: ${USER}" \
  -d "$(jq -n \
    --arg p "$CODE" --arg k "$ADSN_KEY" --arg u "$USER" \
    --arg r "$RISK_YAML" --arg s "$SENS_YAML" \
    --arg m "$MITI_YAML" --arg c "$CONCERNS_YAML" \
    '{prjID:$p, key:$k, user:$u, data:{risk:$r, sensitivity:$s, mitigation:$m, concerns:$c}}')"
```

`concerns` 는 신규 필드. 값 없으면 빈 문자열 "" 로 PUT. 서버는 Mongo 에 그대로 저장 (schema-free).

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
