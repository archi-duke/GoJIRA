---
name: driver-select2
description: Requirement(FR/NFR/CNST) 중 아키텍처 결정에 영향이 큰 항목을 Architecture Driver 로 선정하고 QAD/QAI/Priority/risk/sensitivity 를 평가합니다. 해당 Requirement 를 업데이트(ASR 라벨 부여)하고 architecture/drivers.md + 정규화 시스템 속성(style_define 입력)을 산출합니다. Self-loop 자가검증(최대 3회). Claude Code 네이티브(curl/jq).
allowed-tools: Bash, Read
---

## 한 generation 당 제약 (GLM 계열 스톨 방지 — 필독)

- reasoning 은 각 Step 당 **3~5 줄 이내**. 장기 추론 불필요 — QAD/QAI 산정·정규화 속성 선택의 상세 분석은 문서에 짧게 근거만 적고 넘어갈 것
- **한 번의 LLM generation 에서 Step 두 개 이상 처리 금지**. 각 Step 종료는 **반드시 Bash tool call 로 마무리** 해 generation 경계 형성
- Step 5 는 Requirement PUT / ASR 라벨 / normalizedAttributes 저장 / drivers.md 저장을 **개별 Bash 호출로 분할** — 한 generation 에 몰지 말 것 (5.4 에서 155s 대기 관찰 이력 있음)
- assistant 출력에는 document / JSON 전체를 뱉지 말 것. "iter N 완료 / critical K건" 식 한두 줄 요약만

## 환경
- `PLATFORM_API` 기본값: `http://localhost:9000/api/v2`
- `GOJIRA_API` 기본값: `http://localhost:9001/api/v2`
- 모든 요청에 `X-User-Id: ${USER}` 헤더 필수

## 인자 추출

사용자 메시지에서 다음 값들을 추출해 **먼저** 환경변수로 export 한 뒤 아래 bash 블록들을 실행하라. 누락 시 사용자에게 되물어 확정하라.

- `CODE` — projectCode (예: GOJIRA3)
- `USER` — user (예: batman)
- `W_QAD` — (선택) qad 가중치, 기본 `0.5`
- `W_QAI` — (선택) qai 가중치, 기본 `0.5`

예:
```bash
export CODE="GOJIRA3"
export USER="batman"
# 선택:
# export W_QAD="0.6"
# export W_QAI="0.4"
```

## 역할
노련한 소프트웨어 아키텍트. 요구사항 중 **ASR(Architecturally Significant Requirement)** 를 선별하고, 각 드라이버에 QAD/QAI/Priority/risk/sensitivity/카테고리를 부여. 부산물로 정규화된 시스템 속성(style_define 입력)을 산출.

---

## Step 0 — 진입 점검 (drift-check) + 데이터 수집

### 0.1 drift-check (이슈 변동 여부 확인 — 필수)

본 스킬의 책임 단계는 **`driver`** (workflow-graph.json). Step 0 진입 즉시 drift-check 를 호출해 **이슈 변동 여부 + 상위(`requirement`) 변동 여부**를 확인하고, 그 결과로 진행 방식을 결정한다.

```bash
python3 "${PROJECT_ROOT}/.claude/skills/_scripts/drift-check.py" "${CODE}" "${USER}" check
```

### 0.2 drift-check 결과별 분기 (A/B/C 모델)

| drift-check 결과 | 흐름 | 동작 |
|---|---|---|
| `driver` **NO-BASELINE** | 첫 실행 | Step 0.3 데이터 수집 → MODE=full |
| `driver` **DRIFT** (이슈 변동 감지) | **B** | 본 단계 변경 영향 분석 + 부분 업데이트 (현재 미구현, MODE=full 로 임시 진행) |
| `driver` **CLEAN** + 상위(`requirement`) **DRIFT** | **C** | 사용자 선택: ① 본 stage 재정립 / ② mark-deferred 보류 |
| **모두 CLEAN** | **A** | 0.2.1 의 3 메뉴 — 절대 자동 진행 금지 |

### 0.2.1 변경 없음 처리 메뉴 (A 흐름)

drift 모두 CLEAN 인 경우 **반드시 사용자에게 다음 3 옵션을 명시적으로 제시**. 추측 / 자동 선택 절대 금지.

```
변경된 이슈 / 산출물이 없습니다. 어떻게 처리할까요?
  [1] 종료 (default — 가장 안전, 아무것도 안 함)
  [2] 문서만 재생성 (이슈/archSettings 보존, LLM 0회 결정적 조립)
  [3] 처음부터 다시 만들기 ⚠️ DESTRUCTIVE (이슈 모두 삭제 후 LLM 재도출)
```

### 선택별 동작

- **[1] 종료** — 즉시 Step 6 보고만 출력 (baseline 갱신 없음) → 종료
- **[2] 문서만 재생성** — `FAST_PATH=true` 설정 후 Step 1 + Step 5.1~5.3 스킵, Step 5.4 결정적 조립만. **이슈/archSettings 절대 변경 안 함**
- **[3] 처음부터 다시** — ⚠️ **이 SKILL 자체에서는 진행 금지**. 사용자에게 안내:
  > "destructive 작업입니다. 별도로 `reset-project` 스킬을 먼저 호출하시고 (CONFIRM=true 필요), 그 후 본 스킬을 다시 호출해주세요."

### ⚠️ 절대 금지 (LLM 행동 가드)

다음 중 어느 것도 LLM 자율 판단으로 호출 금지:
- `DELETE /Issue` (이슈 삭제)
- `PUT /Issue` driverSelected/qad/qai 등 필드 변경 (Step 5.1 batch 외 금지)
- `archSettings` `$unset`

**FAST_PATH=true 진입 시**: 이슈/archSettings 변경 모두 금지. 오직 doc 재생성만. 위 호출이 필요해 보이면 즉시 SKILL 중단 + 사용자에게 보고.

진행 결정 후 Step 0.3 (데이터 수집) 으로 이동. [1] 종료면 Step 6 보고만 출력하고 baseline 갱신 없이 끝낸다.

### 0.3 데이터 수집

```bash
PLATFORM_API="${PLATFORM_API:-http://localhost:9000/api/v2}"
GOJIRA_API="${GOJIRA_API:-http://localhost:9001/api/v2}"

# project-description.md (docs/{user} 우선, fallback main — 스크립트 자동 처리)
DESC=$(bash "${PROJECT_ROOT}/.claude/skills/_scripts/doc-read.sh" "${CODE}" "${USER}" "business/project-description.md" 2>/dev/null || true)
echo "=== project-description.md ==="
echo "$DESC"

# Requirement 목록
echo
echo "=== Requirement 목록 ==="
curl -s "${GOJIRA_API}/Issue" \
  -H "XBody: {\"prjID\":\"${CODE}\",\"infoType\":\"\"}" \
  -H "X-User-Id: ${USER}" \
  | jq '[.[] | select(.issueType=="Requirement") | {key, subkey, summary, description, reqtype, qa, measure}]'

# Feature (참고)
echo
echo "=== Feature (참고) ==="
curl -s "${GOJIRA_API}/Issue" \
  -H "XBody: {\"prjID\":\"${CODE}\",\"infoType\":\"\"}" \
  -H "X-User-Id: ${USER}" \
  | jq '[.[] | select(.issueType=="Feature") | {key, summary}]'

# priorityWeight (projects.archSettings 에서 조회 — 없으면 0.5/0.5)
# W_QAD/W_QAI 가 미리 export 되어 있으면 사용, 아니면 기본 0.5/0.5
W_QAD="${W_QAD:-0.5}"; W_QAI="${W_QAI:-0.5}"
echo "priorityWeight: qad=${W_QAD}, qai=${W_QAI}"
```

Requirement 가 0건이면 **중단** 하고 "requirement_refine 을 먼저 실행하세요" 보고.

---

## Step 1 — 초안 생성 (iter 1)

### 1.1 Driver 후보 검토
- 모든 NFR 은 기본 Driver 후보
- FR 중 **구조적 복잡도가 큰 것**(분산/이벤트/대량 처리) 포함
- CNST 중 **아키텍처 선택을 제한하는 제약**(기술/규정/플랫폼) 포함

### 1.2 Driver 카테고리 태깅
각 driver 에 `category` 지정:
- `Functional` — FR 기반 구조 드라이버
- `QualityAttribute` — NFR (성능/가용성/보안 등)
- `Constraint` — CNST
- `Business` — 사업 목표/비용/시장 제약

### 1.3 QAD / QAI (1~5 정수) + rationale
- QAD (난이도): 구현·설계 관점의 기술적 난이도
- QAI (중요도): 비즈니스·품질 관점의 중요도
- 근거 없이 모두 동일 값 금지

### 1.4 Priority
`priority = qad * W_QAD + qai * W_QAI`

### 1.5 중요도 / 난이도 (테이블 평가 컬럼)
- 중요도: QAI 기반, 비즈니스·품질 관점에서 해당 점수를 부여한 사유를 한 줄로 기술
- 난이도: QAD 기반, 구현·설계 관점에서 해당 점수를 부여한 사유를 한 줄로 기술

### 1.6 Driver 선정
Priority 상위 5~10건. 단일 속성/카테고리 편향 금지.

### 1.7 정규화 속성 (normalizedAttributes)

허용 어휘만 사용 (관련 없으면 생략):

- **품질속성**: scalability, modifiability, availability, performance, security, consistency, latency, auditability, debuggability, testability, simplicity, portability, decoupling
- **요구 패턴/워크로드**: event_volume, async_processing_need, synchronous_low_latency, real_time_processing, read_write_ratio_skewed, sporadic_workload, simple_crud, data_volume, domain_complexity, long_running_job, stateful_heavy, short_lifecycle, long_term_maintenance
- **프론트엔드**: large_ui_codebase, polyglot_frontend_tech, strict_ui_consistency, ui_team_independence
- **인프라/규모**: service_count, multi_cluster, inter_service_traffic_control, observability_requirement, strong_vendor_independence
- **조직/운영**: team_size(small/medium/large), ops_maturity(low/medium/high), release_frequency(low/medium/high), team_independent_deploy(low/medium/high), strong_consistency(low/medium/high)

값 체계: `low`/`medium`/`high` 또는 `required`. 불명은 **생략**.

### 1.8 document (drivers.md 본문)

```
# Architecture Drivers

## 1. 선정 Driver 목록
| # | Requirement | 타입 | 카테고리 | 이슈 키 | QAD | QAI | Priority | 중요도 | 난이도 |

## 2. Driver 별 상세 근거
### QA_001 — (이슈 요약)
- QAD 근거 / QAI 근거 / 중요도 / 난이도

## 3. 정규화된 시스템 속성 (style_define 입력)
| 속성 | 값 | 근거 Requirement |

## 4. 선정 편향 점검
```

이슈 키 칸에는 `subkey`(FR_001/QA_001/CNST_001) 를 사용. PlantUML 은 선택 — `component` 다이어그램 권장 (`class`/`interface`/`abstract`/`enum` 키워드 금지), 블록은 ` ```plantuml ` 로 감싸고 `@startuml` / `@enduml` 쌍 필수.

---

## Step 2 — 자가검증

### critical 기준
- 필수 필드(key, qad, qai, priority, category, rationale) 누락
- `key` 가 입력 Requirement 목록에 없음 (hallucination)
- `qad`/`qai` 가 1~5 범위 밖, 혹은 근거 없이 모두 동일 값
- 카테고리 편향 (QualityAttribute 0건, 단일 카테고리만 존재)
- NFR 인데 Driver 에 포함되지 않은 항목 존재
- `normalizedAttributes` 가 허용 어휘 밖의 키/값 사용
- document 에서 `subkey` 대신 이슈 key / placeholder 노출
- PlantUML 을 class 다이어그램으로 작성 (`class`/`interface` 등), 또는 외부 이미지 참조

### 점수
- `score`: 0.0~1.0
- `score_effective = score - 0.5 × |critical|`
- `pass`: `score_effective >= 0.8`

---

## Step 3 — 수정 (pass=false)
critical + suggestions 항목만 국소 수정. 나머지 원문 유지. 수정 후 Step 2.

---

## Step 4 — 종료 조건
1. pass=true
2. iter 3 완료
3. 같은 critical 2회 연속 재발
4. 새 수정이 이전 best 보다 낮은 score_effective → 포기

**best-so-far**: `(drivers, normalizedAttributes, document, score_effective)` 매 iter 기록. 종료 시 최댓값 iter 의 결과를 selected.

---

## Step 5 — Requirement 업데이트 + 라벨 + 문서 저장

### 5.1 각 driver 에 대해 Requirement 업데이트

**강제 재생성 모드(Step 0.2 의 drift CLEAN + 사용자 yes) 이면 이 섹션 전체 스킵** — Requirement 이슈 PUT 하지 않는다.

```bash
curl -s -X PUT "${GOJIRA_API}/Issue" \
  -H "Content-Type: application/json" -H "X-User-Id: ${USER}" \
  -d "$(jq -n --arg p "$CODE" --arg k "$KEY" --arg u "$USER" \
    --argjson qad $QAD --argjson qai $QAI --argjson pri $PRIORITY \
    --arg imp "$IMPORTANCE" --arg diff "$DIFFICULTY" \
    '{prjID:$p, key:$k, user:$u, data:{qad:$qad, qai:$qai, priority:$pri, importance:$imp, difficulty:$diff, driverSelected:true}}')"
```

### 5.2 ASR 라벨 부여 (선정된 Driver + 모든 NFR)

**강제 재생성 모드(Step 0.2 의 drift CLEAN + 사용자 yes) 이면 이 섹션 전체 스킵** — labels 갱신 PUT 하지 않는다.

```bash
# 현재 labels 조회 (XBody 헤더 방식) → ASR 누락 시 append
LABELS=$(curl -s "${GOJIRA_API}/Issue" \
  -H "XBody: {\"prjID\":\"${CODE}\",\"key\":\"${KEY}\"}" \
  -H "X-User-Id: ${USER}" \
  | jq -c '.[0].labels // []')
NEW_LABELS=$(echo "$LABELS" | jq -c 'if index("ASR") then . else . + ["ASR"] end')
curl -s -X PUT "${GOJIRA_API}/Issue" \
  -H "Content-Type: application/json" -H "X-User-Id: ${USER}" \
  -d "$(jq -n --arg p "$CODE" --arg k "$KEY" --arg u "$USER" --argjson l "$NEW_LABELS" \
    '{prjID:$p, key:$k, user:$u, data:{labels:$l}}')"
```

선정되지 않았더라도 **모든 NFR Requirement** 에 ASR 라벨 부여.

### 5.3 archSettings 영속화 (normalizedAttributes + priorityWeight)

`PATCH /Project/{CODE}/archSettings` 한 번 호출로 두 필드 동시 merge. 다른 archSettings 필드(chosenStyles/satamDone 등)는 건드리지 않음.

**HTTP status 검증 필수** — 실패 시 silent 통과하면 하류 스킬(style-define2/design-decide2)이 drivers.md fallback 파싱 (~70초 비효율) 으로 빠진다. 본 단계가 실패하면 Step 5.4/6.2 진행 전에 사용자에게 알리고 수동 점검 유도.

```bash
# NORM_JSON: Step 1.7 에서 만든 normalizedAttributes JSON 객체
# 가중치는 Step 0 에서 결정된 W_QAD / W_QAI 사용
RESP=$(curl -s -w '\n%{http_code}' -X PATCH "${GOJIRA_API}/Project/${CODE}/archSettings" \
  -H "Content-Type: application/json" -H "X-User-Id: ${USER}" \
  -d "$(jq -n --argjson n "$NORM_JSON" --argjson wq "$W_QAD" --argjson wi "$W_QAI" \
    '{normalizedAttributes:$n, priorityWeight:{qad:$wq, qai:$wi}}')")
STATUS=$(printf '%s' "$RESP" | tail -1)
BODY=$(printf '%s' "$RESP" | sed '$d')

if [[ "$STATUS" =~ ^2[0-9][0-9]$ ]]; then
  echo "$BODY" | jq '{updated, normalizedAttributes_keys: (.archSettings.normalizedAttributes | keys), priorityWeight: .archSettings.priorityWeight}'
  echo "✓ archSettings PATCH 성공 (status=$STATUS)"

  # 사후 GET 으로 실제 영속화 확인 (선택, 강력 권장)
  GET_BODY=$(curl -s "${GOJIRA_API}/Project" -H "XBody: {\"jiraCode\":\"${CODE}\"}" -H "X-User-Id: ${USER}")
  HAS_NORM=$(echo "$GET_BODY" | jq '.[0].archSettings.normalizedAttributes | length')
  HAS_W=$(echo "$GET_BODY" | jq '.[0].archSettings.priorityWeight | length')
  if [[ "$HAS_NORM" == "0" || "$HAS_W" == "0" ]]; then
    echo "⚠ PATCH 응답은 200 이지만 GET 검증에서 normalizedAttributes/priorityWeight 가 비어있음 (norm=$HAS_NORM, weight=$HAS_W). 최종 보고에 ⚠ 표기 + 수동 재시도 권고."
  fi
else
  echo "⚠ archSettings PATCH 실패 (status=$STATUS)"
  echo "$BODY"
  echo "→ 하류 스킬(style-define2/design-decide2) 에서 ~70초 fallback 파싱 비효율 발생 예상."
  echo "→ 수동 재시도하거나 사용자에게 알려 archSettings 직접 점검 권고. Step 5.4/6.2 진행 전 사용자 결정 필요."
fi
```

실패(4xx/5xx) 시 최종 보고에 ⚠ 로 노출. 다음 스킬(style-define2) 은 fallback 체인으로 우회 가능하지만, 세션 경계를 넘으면 복구 불가이므로 사용자에게 경고.

### 5.4 architecture/drivers.md 저장 — 결정적 조립 (LLM 0회)

**MODE=full / fast path 동일 흐름**: Step 5.1~5.3 직후 mongo 이슈 + archSettings.normalizedAttributes 가 갖춰져 있으므로 `driver-assemble.py` 가 결정적 조립. LLM `$DOCUMENT` echo 폐기.

```bash
python3 "${PROJECT_ROOT}/.claude/skills/_scripts/driver-assemble.py" \
  --from-mongo "${CODE}" --user "${USER}" \
  | bash "${PROJECT_ROOT}/.claude/skills/_scripts/doc-save.sh" "${CODE}" "${USER}" "architecture/drivers.md"
echo "drivers.md 저장 완료 (결정적 조립)"
```

**FAST_PATH=true 면** Step 1 + 5.1~5.3 모두 스킵하고 위 명령만 실행 후 `exit 0`.

---

## Step 6 — 최종 보고 + baseline 갱신

### 6.1 보고 출력

```
## Architecture Driver 선정 완료 (v2 Claude Code) — MODE={full|forced-doc-regen}

- 진입 drift: driver=<NO-BASELINE|CLEAN|DRIFT>, requirement=<...> (Step 0.1 결과)
- Driver 선정: N건 (키 + 카테고리 분포)   (forced-doc-regen 은 PUT 0)
- Requirement 업데이트: qad/qai/priority/risk/sensitivity   (forced-doc-regen 은 스킵)
- ASR 라벨: Driver + 모든 NFR   (forced-doc-regen 은 스킵)
- 정규화 속성: {scalability: high, ...}
- 문서 저장: architecture/drivers.md

### 카테고리 분포
| 카테고리 | 건수 |
|---------|-----|
| QualityAttribute | X |
| Functional | Y |
| Constraint | Z |
| Business | W |

### Self-loop
- iter N회 / selected=iter X (score_eff Y) / stop=pass|max_iter|repeat_critical|regression
- unresolvedCritical: [...]
```

### 6.2 baseline 갱신 (필수 — 본 작업이 실제로 진행된 경우)

본 작업이 진행됐다면(NO-BASELINE 첫 실행 / DRIFT 처리 / 강제 재생성) **반드시** `driver` stage 의 baseline 을 새 hash 로 갱신한다. Step 0.2 에서 "종료" 결정으로 끝난 경우는 호출하지 않는다.

```bash
python3 "${PROJECT_ROOT}/.claude/skills/_scripts/drift-check.py" "${CODE}" "${USER}" set-baseline --stage driver
```

이 명령은 archSettings.lastRunHash[driver] / lastRunAt[driver] 만 갱신 — 다른 stage 의 baseline 은 건드리지 않는다.

---

## 주의
- 모든 curl 에 `X-User-Id: ${USER}` 헤더 필수 (403 방지)
- Requirement 의 `key` 는 입력 목록 실존 key 만 사용 (hallucination 금지)
- document 의 "이슈 키" 칸은 **subkey** (FR_001/QA_001/CNST_001)
- Platform-API `/file` 응답 content 는 base64 → `base64 -d`, 저장 시 plain UTF-8 전달
- normalizedAttributes 는 허용 어휘 외 사용 금지
- 한 세션에서 Steps 1~4 는 추론, Step 0 과 Step 5 는 실제 Bash 호출. 단 **Step 5 내부는 5.1~5.4 각각을 별도 Bash tool call 로 분할** — 한 generation 에 몰지 말 것
- 진입 정책: Step 0.1 의 drift-check 가 작업 진행 여부를 정한다. 모두 CLEAN 인데 사용자가 강제 재생성을 거부하면 즉시 종료 (LLM 호출 0회). 강제 재생성 yes 인 경우 이슈 생성/수정 step 들은 자동 스킵하고 문서/링크만 재생성
- baseline 갱신: 본 작업 진행 시 Step 6.2 의 `set-baseline --stage driver` 호출이 필수. 안 하면 다음 호출에서 또 DRIFT 로 잡혀 재실행 압력이 생긴다
