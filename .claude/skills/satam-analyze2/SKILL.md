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

## 재분석 규칙 (skip 방지)

ADSN 의 `risk / sensitivity / mitigation / concerns` 필드가 **비어있지 않아도 자동 skip 금지**. 기존 값이 **SATAM v2 YAML 포맷** (`ID : R1` / `QA : [...]` / `Description : ...` / `Counter Risk : [...]`) 이 아니면 **design-decide2 단계 잔여 원문으로 간주** 하고 재분석한다.

판정 규칙 (Step 0 에서 실시):
- `risk` 필드 원문에 `"ID :"` 문자열이 **없으면** → 원문/미분석 → 이번 스킬로 재작성
- `"ID :"` 가 있으면 → v2 구조화 완료 → `Step 5` PUT 단계에서만 참조 (재분석 불필요)

사용자가 "이전에 돌렸다" 고 해도 위 포맷 체크로 직접 판정. 신뢰하지 말 것.

---

## 진행 상태 가시화 — `todowrite` (선택, OpenCode)

OpenCode 환경이면 각 Step 진입 시 내장 `todowrite` 로 현재/남은 단계를 알려주면 UX 가 좋다. 강제 아님, Claude Code 에서는 무시. 자세한 규칙은 `.claude/skills/_shared/todowrite-guide.md`.

이 스킬 Step 체크리스트:
```
# Todos
[ ] Step 0 — 진입 점검 (drift-check + 분기) + ADSN + NFR(QA) 데이터 수집
[ ] Step 1 — risk/sensitivity/mitigation 매핑 초안
[ ] Step 2 — 자가검증 (validate-satam.py)
[ ] Step 3 — critical 수정 (반복)
[ ] Step 5 — ADSN PUT + satamDone 플래그 영속화
[ ] Step 6 — 최종 보고 + baseline 갱신
```

## 환경
- `PLATFORM_API` 기본값: `http://10.211.55.3:44350/api/v2`
- `GOJIRA_API` 기본값: `http://10.211.55.3:44360/api/v2`
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

## 역할 / 핵심 목적

소프트웨어 아키텍트. ATAM 변형(SATAM)으로 **각 아키텍처 설계(ADSN)가 프로젝트의 Requirement 달성을 위협하는 지점** 을 구조화한다.

> **SATAM 의 한 줄 정의**: *Mitigation 되지 않은 Risk 를 찾는 것*.

### v3 구조 (2026-04-24) — design × requirement 판정

기존 버전은 ADSN 의 원문(risk/sensitivity 필드)을 분해하는 방식이어서 LLM 이 "자기가 만든 risk 를 자기가 해결" 하는 완화 쌍을 기록하는 문제가 있었다. v3 는 **판정 시점에 이미 self-mitigated 를 제외** 한다.

**핵심 규칙**: 디자인이 자기 risk 를 **스스로 극복 가능** 하면 **risk 자체가 성립하지 않음** — 등록 금지.

**Mitigation 의미 재정의**: "이 디자인이 **다른 ADSN 의 risk 를 해결** 한다" 는 cross-ADSN 관계만 인정. 같은 ADSN 내에 R 과 해당 R 을 가리키는 M 이 함께 등장하면 **critical 오류**.

### 판정 알고리즘

모든 `(design, requirement)` 조합을 순회하며 아래 규칙으로 판정:

```
for each ADSN d in 채택된 ADSN 목록:
  for each Requirement r in 프로젝트 Requirement 전체 (FR/NFR/CNST):
    # FR 중 ASR(driverSelected=true) 아닌 것은 건너뜀
    if r.reqtype == "FR" and not r.driverSelected: continue

    verdict = judge(d, r)
    # judge() 는 LLM 이 다음 로직으로 판정:

    if r.reqtype == "NFR" (QA):
      if   d 로 인해 r 의 QAS 달성이 "명확히 실패" 함을 확인 가능
           AND d 가 그 실패를 스스로 극복 불가능
        → risks.append({ADSN: d, requirement: r, description})
      elif d 로 인해 r 의 QAS 달성이 "조건/상황 변동 시 risk 가 될" 수 있음
        → sensitivities.append({...})
      else: non-risk (무기록)

    elif r.reqtype == "CNST":
      if   d 로 인해 r 의 CNST 위배 발생이 확인 가능 AND 자체 극복 불가
        → risks
      elif d 로 인해 r 의 CNST 위배가 조건 변동 시 발생 가능
        → sensitivities
      else: non-risk

    elif r.reqtype == "FR" and r.driverSelected:
      if   d 로 인해 r 의 구현이 "불가능" 해짐 (어떻게든 되는 경우 제외)
           AND 자체 극복 불가
        → risks
      elif d 로 인해 r 의 구현이 조건 변동 시 불가능해질 수 있음
        → sensitivities
      else: non-risk

# 모든 risk 판정 완료 후, 각 디자인의 mitigation 검토:
for each ADSN d:
  for each Risk r_other in (d 외 다른 ADSN 들의 risks):
    if d 가 r_other 를 해결 가능:
      mitigations[d].append({targetADSN: r_other.adsn, targetRisk: r_other.id, description})
```

### 판정 지침

- **"명확히 실패"**: 추측·가능성 수준 배제. 현 설계가 그대로 돌아가면 requirement 를 못 만족함이 설계상 확정적
- **"자체 극복 가능"**: 설계의 `cons`/`tradeoffs`/`decisionRationale` 에 *이 risk 에 대한 대응 방안이 이미 포함* 돼있으면 스스로 해결 가능 → risk 등록 금지
- **"어떻게든 되는 경우"**: FR 판정에서 성능·품질 타협 있어도 기본 구현은 가능하면 → non-risk
- **Cross-ADSN Mitigation**: 다른 ADSN 의 구조·결정이 이 ADSN 의 risk 를 해소하는 경우만. 막연한 "다른 ADSN 과 함께 쓰면 괜찮음" 은 제외 — 구체적 해결 메커니즘 명시

### Concerns

위 판정 밖의 관찰 사항(팀 숙련도, 장기 모니터링 등) 은 `concerns` 로 별도 기록. 선택사항.

---

## Step 0 — 진입 점검 (drift-check) + 데이터 수집

### 0.1 drift-check (이슈 변동 여부 확인 — 필수)

본 스킬의 책임 단계는 **`satam`** (workflow-graph.json). Step 0 진입 즉시 drift-check 를 호출해 **이슈 변동 여부 + 상위(`design-decide`) 변동 여부**를 확인하고, 그 결과로 진행 방식을 결정한다.

```bash
python3 "${PROJECT_ROOT}/.claude/skills/_scripts/drift-check.py" "${CODE}" "${USER}" check
```

### 0.2 결과에 따른 분기

| drift-check 결과 | 동작 |
|---|---|
| `satam` **NO-BASELINE** | 첫 실행 — Step 0.3 데이터 수집 후 본 작업 진행 |
| `satam` **DRIFT** (ADSN risk/sensitivity/mitigation 변동 감지) | 본 작업 진행 |
| `satam` **CLEAN** + 상위(`design-decide`) **DRIFT** | 사용자에게 통지: "Design Decisions 가 변경됐습니다. SATAM 재분석이 필요할 수 있어요. (a) 지금 진행 (b) `mark-deferred` 로 보류" — 답변 받고 진행/종료 결정 |
| **모두 CLEAN** | **즉시 종료** — 본 스킬은 문서 산출이 없고 ADSN 이슈 필드 update + archSettings.satamDone 만 갱신하므로, 이슈/상위 모두 변동이 없으면 **강제 재생성도 의미 없음**. 사용자에게 yes/no 질문 없이 바로 Step 6 보고 출력 후 종료 (baseline 갱신 없음) |

진행 결정 후 Step 0.3 (데이터 수집) 으로 이동.

### 0.3 데이터 수집

```bash
PLATFORM_API="${PLATFORM_API:-http://10.211.55.3:44350/api/v2}"
GOJIRA_API="${GOJIRA_API:-http://10.211.55.3:44360/api/v2}"

# 채택된 ADSN 목록 (decision 필드 있음) — 논리 판정에 필요한 전체 맥락 포함
echo "=== 채택 ADSN 목록 ==="
curl -s "${GOJIRA_API}/Issue" \
  -H "XBody: {\"prjID\":\"${CODE}\",\"infoType\":\"\"}" \
  -H "X-User-Id: ${USER}" \
  | jq '[.[] | select(.issueType=="Architectural Design" and .decision != null and .decision != "")
         | {key, subkey, summary, designPoint, designLevel,
            objectives, decision, decisionRationale, approaches,
            pros, cons, eval, tradeoffs, adiagram, bdiagram}]'

# Requirement 전체 — FR/NFR/CNST, ASR 플래그 포함
echo
echo "=== Requirement 전체 (FR/NFR/CNST + driverSelected) ==="
curl -s "${GOJIRA_API}/Issue" \
  -H "XBody: {\"prjID\":\"${CODE}\",\"infoType\":\"\"}" \
  -H "X-User-Id: ${USER}" \
  | jq '[.[] | select(.issueType=="Requirement")
         | {key, subkey, summary, description, reqtype,
            driverSelected: (.driverSelected // false),
            qa, source, stimulus, environment, artifact, response, measure, responseScenario}]'
```

ADSN(decision 필드 있는) 이 0건이면 **중단** — "design_decide2 를 먼저 실행하세요".

v3 SATAM 대상 Requirement:
- **NFR 전부** (모든 QA)
- **CNST 전부**
- **FR 중 `driverSelected=true` (ASR)** — driverSelected=false 인 FR 은 제외

즉 `driverSelected=true` 인 FR + NFR + CNST = SATAM 판정 대상 requirement 집합.

### Step 0 종료 — 수집 데이터 flush (필수)

수집한 ADSN 목록과 QA 참조 키를 context 파일로 저장. Step 1 이후 모든 generation 이 이 파일을 load 해서 참조.

```bash
ADSN_JSON=$(curl -s "${GOJIRA_API}/Issue" \
  -H "XBody: {\"prjID\":\"${CODE}\",\"infoType\":\"\"}" -H "X-User-Id: ${USER}" \
  | jq '[.[] | select(.issueType=="Architectural Design" and .decision != null and .decision != "")
         | {key, summary, designPoint, objectives, decision, decisionRationale,
            approaches, pros, cons, eval, tradeoffs, adiagram, bdiagram}]
        | reduce .[] as $i ({}; . + {($i.key): $i})')

REQ_JSON=$(curl -s "${GOJIRA_API}/Issue" \
  -H "XBody: {\"prjID\":\"${CODE}\",\"infoType\":\"\"}" -H "X-User-Id: ${USER}" \
  | jq '[.[] | select(.issueType=="Requirement")
         | select(.reqtype=="NFR" or .reqtype=="CNST" or (.reqtype=="FR" and (.driverSelected // false) == true))
         | {key, summary, description, reqtype,
            driverSelected: (.driverSelected // false),
            qa, measure, responseScenario}]
        | reduce .[] as $i ({}; . + {($i.key): $i})')

jq -n --argjson a "$ADSN_JSON" --argjson r "$REQ_JSON" '{adsns:$a, requirements:$r}' \
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

### 1.1 판정 대상 & 분류 정의

v3 는 **design × requirement 이중 루프** 로 각 쌍의 관계를 판정해 Risk/Sensitivity 를 생성. 원문 분해가 아닌 **논리적 판정**.

| 카테고리 | 의미 | 기록 조건 |
|---|---|---|
| **Risk (R)** | 이 설계가 이 requirement 달성을 **명확히 실패** 시킴 | 현 설계 그대로 돌아가면 requirement 를 못 만족함이 확정적. **그리고 이 설계 자체로 극복 불가** |
| **Sensitivity (S)** | 현재는 괜찮지만 **조건/상황 변동 시 risk 로 전환** 가능 | "10만 명 초과 시", "지역 확장 시", "인프라 부재 시" 등 명시적 조건부 |
| **Mitigation (M)** | 이 설계가 **다른 ADSN 의 risk 를 해결** 함 (cross-ADSN 전용) | 다른 ADSN 에 등록된 Risk 의 ID 를 `targetRisk` 로 참조 |
| **Concerns (C)** | 위 3가지에 해당 안 되는 관찰·검토 사항 | 선택사항 |

### 1.2 판정 알고리즘 (각 ADSN 마다 전 Requirement 순회)

```
risks[adsn] = []
sensitivities[adsn] = []
# mitigations 는 Risk 모두 판정 완료 후 2단계에서 생성

for r in requirements:
  if r.reqtype == "FR" and r.driverSelected != true:
    continue

  if r.reqtype == "NFR":
    # QA(NFR) 판정: QAS 달성 관점
    if 이 ADSN 으로 인해 r 의 QAS(응답 시간, 가용성 등) 달성이 명확히 실패:
      if 이 ADSN 스스로 극복 가능 (cons/tradeoffs/decisionRationale 에 대응 명시):
        continue   # risk 아님 — self-mitigated
      else:
        risks.append({requirement: r.key, reqType: "NFR", description})
    elif 조건 변동 시 QAS 달성 실패 가능:
      sensitivities.append({requirement: r.key, reqType: "NFR", description})
    else:
      continue   # non-risk

  elif r.reqtype == "CNST":
    if 이 ADSN 으로 인해 r 의 CNST 위배 확인:
      if 자체 극복 가능: continue
      else: risks.append({requirement: r.key, reqType: "CNST", ...})
    elif 조건 변동 시 CNST 위배 가능:
      sensitivities.append({...})

  elif r.reqtype == "FR" and r.driverSelected:
    if 이 ADSN 으로 인해 r 의 구현이 "불가능" 해짐:
      # 어떻게든 성능 타협으로라도 되는 경우는 제외
      if 자체 극복 가능: continue
      else: risks.append({requirement: r.key, reqType: "FR", ...})
    elif 조건 변동 시 구현 불가해질 수 있음:
      sensitivities.append({...})

# Risk 목록이 모두 확정된 후, mitigation 생성:
for d in adsns:
  mitigations[d] = []
  for r_other in (다른 ADSN 의 risks 전체):
    if d 가 r_other 를 구체적 메커니즘으로 해결 가능:
      mitigations[d].append({
        targetADSN: r_other.adsn,
        targetRisk: r_other.id,
        description: "이 디자인이 어떤 메커니즘으로 해결하는지"
      })
```

### 1.3 판정 지침 (중요 — 편법 방지)

**현실적 판정 — 없는 risk 찾지 말 것**:
- 분량 채우려고 억지 risk 만들기 금지. 판정 결과 **전 ADSN 의 risks·sensitivities 가 빈 배열** 이어도 완전히 정상
- "가능성 있음", "~할 수도 있음", "일반적으로 ~" 같은 확률·추측 수준은 risk 아님. 설계상 확정적 impact 만
- ADSN 의 cons 에 "약간의 오버헤드" 같은 문구가 있다고 기계적으로 risk 로 올리지 말 것 — requirement(QAS/CNST/ASR-FR) 달성을 실제로 방해하는 수준인지 매번 검토

**자체 극복 가능 판정**: 이 ADSN 의 `cons`/`tradeoffs`/`decisionRationale` 에 *해당 requirement 에 대한 대응* 이 명시돼 있는지 확인. 있으면 risk 성립 안 함
- 예: "Redis 단일 장애점" 이 cons 에 나오지만 tradeoffs 에 "RDB/AOF 로 복구 가능" 이 있으면 → self-mitigated → **risk 기록 금지**

**"어떻게든 되는 경우 제외"** (FR 판정):
- 성능·UX 타협이 있어도 기능 자체가 구현 가능하면 → non-risk
- "구현 불가능" 이 명확한 경우만 risk (예: 선택한 프로토콜이 요구 프로토콜과 호환 불가)

**Cross-ADSN Mitigation**: 막연히 "다른 ADSN 과 함께 쓰면" 이 아니라 *이 디자인의 구체적 특성이 어떻게 다른 ADSN 의 risk 를 해소하는지* 명시. 못 찾으면 `mitigations: []` 로 둘 것

**self-mitigation 금지**: 같은 ADSN 의 risk 를 같은 ADSN 의 mitigation 이 가리키는 건 validator critical. 이미 self-mitigated 는 risk 등록 자체가 금지이므로 이 쌍은 구조적으로 나올 수 없어야 함

### 1.4 ID 규칙

- **R / S / M**: 프로젝트 전역 유일. 의미 동일한 기존 ID 재사용, 신규는 max+1
- **C**: ADSN 국소

### 1.5 JSON 스키마 (v3)

```json
{
  "GOJIRA3-23": {
    "risks": [
      {
        "id": "R1",
        "requirement": "GOJIRA3-15",
        "reqType": "NFR",
        "description": "이 설계가 어떻게 이 QA 의 달성을 명확히 실패시키는지"
      }
    ],
    "sensitivities": [
      {
        "id": "S1",
        "requirement": "GOJIRA3-16",
        "reqType": "NFR",
        "description": "어떤 조건 변화 시 risk 로 전환되는지"
      }
    ],
    "mitigations": [
      {
        "id": "M1",
        "targetADSN": "GOJIRA3-30",
        "targetRisk": "R5",
        "description": "이 디자인이 다른 ADSN 의 R5 를 구체적으로 어떻게 해결하는지"
      }
    ],
    "concerns": [
      {"id": "C1", "description": "팀 숙련도 등 기타 관찰사항"}
    ]
  }
}
```

**스키마 변경점 (v2 대비)**:
- `tradeoffs` → `mitigations` (필드명 변경, cross-ADSN 의미 반영)
- `risks[].qa` → `risks[].requirement` + `reqType` (단일 requirement, 타입 명시)
- `mitigations` 는 `targetADSN` + `targetRisk` 로 **다른 ADSN 의 R 을 참조**
- `same_adsn_mitigation` 은 critical (targetADSN 이 자기 자신이면 오류)

**키 규칙**: 모든 key 는 JIRA 이슈 key (예: `GOJIRA3-15`). subkey 금지.

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

스크립트가 잡는 critical (v3):
- `unknown_adsn` — Step 0 목록에 없는 키
- `bad_id_format` — R1/S1/M1 형식 위반
- `duplicate_id` — ADSN 내부 id 중복
- `requirement_hallucination` — `requirement` 가 Step 0 requirements 풀에 없는 키
- `fr_not_asr` — reqType=FR 인데 해당 requirement 의 driverSelected=false
- `bad_reqType` — reqType 이 "FR"/"NFR"/"CNST" 셋 중 하나 아님
- `same_adsn_mitigation` — mitigation.targetADSN 이 자기 자신 → **SATAM 본연 위반** (self-mitigated risk 는 애초에 등록 금지)
- `mitigation_target_unknown` — targetADSN 또는 targetRisk 가 존재하지 않음
- `missing_description`

### 통과 기준
- `pass == true` → Step 5 진입
- `pass == false` → Step 3 수정

---

## Step 3 — 수정 (pass=false)

critical 항목만 **국소 수정** 후 draft 재저장. 아래 처리 지침 참조:

### critical 유형별 처리 (v3)

| critical type | 조치 |
|---|---|
| `same_adsn_mitigation` | **SATAM 본연 위반** — mitigation.targetADSN 이 자기 자신. 해당 mitigation 삭제 + 관련 risk 도 self-mitigated 이므로 삭제 (애초에 등록 금지) |
| `mitigation_target_unknown` | targetADSN/targetRisk 참조 실패 → 해당 mitigation 제거 |
| `requirement_hallucination` | requirement key 가 프로젝트 풀에 없음 → 올바른 key 로 교체 또는 삭제 |
| `fr_not_asr` | reqType=FR 인데 driverSelected=false → 해당 risk/sensitivity 삭제 (ASR 아닌 FR 은 대상 아님) |
| `bad_reqType` | "FR" / "NFR" / "CNST" 중 하나로 수정 |
| `bad_id_format` / `duplicate_id` | ID 재할당 (전역 R/S/M 순서 유지, ADSN 내 C 국소) |
| `missing_description` | description 채우기 |

**`same_adsn_mitigation` 은 SATAM 본연 위반** — iter 종료 전 0건이어야 함. self-mitigated risk 는 risk 로 **등록하지 말 것** (non-risk 로 무기록).

실제 수정 코드:

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

v3 스키마 (`requirement` + `reqType`) 를 저장 YAML 로 변환. UI parseRST 는 `QA:` 라벨을 기대하므로 **라벨명은 QA 유지**, 값은 requirement key 1개를 배열 형태로.

Risk:
```
- ID : R1
  QA : [GOJIRA3-123]
  ReqType : NFR
  Description : ...
```

Sensitivity:
```
- ID : S1
  QA : [GOJIRA3-124]
  ReqType : CNST
  Description : 조건 변화 시 ...
```

Mitigation (cross-ADSN 대상만):
```
- ID : M1
  Target ADSN : GOJIRA3-30
  Target Risk : R5
  Description : 이 디자인이 R5 를 어떻게 해결하는지
```

Concerns:
```
- ID : C1
  Description : 팀 숙련도 등 관찰사항
```

**⚠ 포맷 주의**:
- `QA : [...]` 는 **bare 키 + 콤마 + 공백** 형식 (따옴표 없음)
- Python 에서 `json.dumps(list)` 금지. `"[" + ", ".join(list) + "]"` 로 조립
- Target ADSN / Target Risk 는 **bare key** 한 개씩 (배열 아님)
- 항목 없는 리스트는 해당 필드 빈 문자열 ""
- Description 한 줄 (개행 공백 치환)

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

## Step 6 — 최종 보고 + baseline 갱신

### 6.1 보고 출력

```
## SATAM 분석 완료 (v2 Claude Code) — MODE={full}

- 진입 drift: satam=<NO-BASELINE|CLEAN|DRIFT>, design-decide=<...> (Step 0.1 결과)
- ADSN 업데이트: N건 (키 리스트)
- **Unmitigated Risks (핵심 산출물)**: U건 — 후속 설계 필요
- 총계: Risks X / Sensitivities Y / Mitigations Z / Concerns C
- 실패: F건 (키 + 사유)

### ⚠ Unmitigated Risk 목록 (후속 design-decide2 또는 추가 ADSN/DDSN 대상)

| ADSN | Risk ID | QA | Description | 제안 후속 설계 |
|---|---|---|---|---|
| ADSN_001 | R1 | QA_004 | 세션 소실 위험 | 세션 복구 전략 DP 추가 검토 |

- 이 목록이 비어있으면 "모든 Risk 가 Mitigation 으로 커버됨" — SATAM 관점에서 현 설계 안정
- 1건 이상이면 다음 design-decide2 호출 시 이 Risk 들을 새 DP 로 다루기 권장

### QA 매핑 요약
| ADSN | Risks | Sensitivities | Mitigations | Concerns | 참조 QA 수 |

### Self-loop
- iter N회 / selected=iter X (score_eff Y) / stop=pass|max_iter|...
- unresolvedCritical: [...]
```

이어서 `ui_navigate(page=satam)` 로 SATAM 화면 전환 안내.

### 6.2 baseline 갱신 (필수 — 본 작업이 실제로 진행된 경우)

본 작업이 진행됐다면(NO-BASELINE 첫 실행 / DRIFT 처리) **반드시** `satam` stage 의 baseline 을 새 hash 로 갱신한다. Step 0.2 의 "모두 CLEAN" 즉시 종료 또는 상위 DRIFT 보류 결정으로 끝난 경우는 호출하지 않는다.

```bash
python3 "${PROJECT_ROOT}/.claude/skills/_scripts/drift-check.py" "${CODE}" "${USER}" set-baseline --stage satam
```

이 명령은 archSettings.lastRunHash[satam] / lastRunAt[satam] 만 갱신 — 다른 stage 의 baseline 은 건드리지 않는다.

---

## 주의
- 모든 curl 에 `X-User-Id: ${USER}` 헤더 필수 (403 방지)
- `qa` 배열 및 ADSN 매핑 키는 반드시 **이슈 key** (subkey 금지)
- Trade-off 의 `counterRisk` 는 **같은 ADSN 내** Risk ID 만 참조 (`R<id>:<QA_KEY>` 형식)
- 문서 파일 생성 절대 금지 — 이슈 필드 3종 반영만
- Step 0 flush / Step 1 draft save / Step 2 validate / Step 3 재-save / Step 5 PUT 루프 **모두 Bash tool call** 로 종료. 한 generation 에 몰아서 처리 금지
- 진입 정책: Step 0.1 의 drift-check 가 작업 진행 여부를 정한다. 본 스킬은 문서 산출이 없어 강제 재생성 모드 자체가 무의미 — 모두 CLEAN 이면 사용자 질문 없이 즉시 종료
- baseline 갱신: 본 작업 진행 시 Step 6.2 의 `set-baseline --stage satam` 호출이 필수. 안 하면 다음 호출에서 또 DRIFT 로 잡혀 재실행 압력이 생긴다
