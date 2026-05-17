---
name: usecase-analyze2
description: VOS 를 User Story / Use Case / Feature 로 분석·도출하고 VOS↔Feature Contains 링크 + business/use-cases.md 를 작성/재정립합니다. 사용자가 "Feature 도출/Use Case 분석" 을 요청하면 MODE=full, "Feature-VOS 연결 재정리·재링크·링크만 다시" 류 요청이면 MODE=relink 로 호출 (기존 Feature/UC 이슈 보존, 링크·문서만 재작성, 차집합만 POST). VOS 커버리지 자가검증(모든 VOS 최소 1 Feature) + self-loop(최대 3회). Claude Code 네이티브(curl/jq).
allowed-tools: Bash, Read
---

## 이 스킬을 언제 호출하나

사용자 요청이 아래 의도에 해당하면 이 스킬을 호출하고 `MODE` 를 확정한다.

- **MODE=relink** — "Feature-VOS 연결 재정리", "링크 재정리", "재링크", "relink", "링크만 다시" 등 **기존 Feature/UseCase 를 유지하면서 VOS 와의 연결/문서만 재정립** 하려는 요청
- **MODE=full** — "Use Case 분석", "Feature 도출", "VOS 에서 Use Case 뽑아" 등 **VOS 에서 신규 Use Case/Feature 를 도출** 해 달라는 요청

단순 현황 조회(고아 Feature, 커버리지 점검 등)만 원하는 거면 먼저 `issue-analyze` 로 분석하고, 구조 변경(이슈·링크 수정) 의도가 확인된 뒤에만 이 스킬을 호출한다. 분석 결과만 요구한 턴에서 이 스킬을 "미리 돌려놓지" 말 것.

## 한 generation 당 제약 (GLM 계열 스톨 방지 — 필독)

- **한 번의 LLM generation 에서 Step 두 개 이상 처리 금지**. 각 Step 종료는 **반드시 Bash tool call 로 마무리** 해 generation 경계 형성
- **Step 1 은 1.1~1.5 로 분할** — Entities / Features / Scenarios / Diagrams / 결정적 조립. 각 sub-step 끝에 **반드시 `draft-io.sh merge` 또는 `save`** 호출로 generation 경계
- **결정적 조립 (Step 1.5)**: bash + jq + `usecase-assemble.py` 로 누적 draft 를 use-cases.md 로 변환한다. **LLM 호출 절대 금지**
- reasoning 은 각 Step (또는 sub-step) 당 3~5 줄 이내. 장기 추론 불필요
- Step 2 자가검증은 **반드시 `validate-usecases.py` 호출** — 에이전트가 직접 VOS 커버리지 / PlantUML 구문 / Actor 다양성 계산 금지
- critical 수정 후 다시 `draft-io.sh save` → 이어 `validate-usecases.py` 로 재검증. 한 generation 당 1 iter
- assistant 출력에는 긴 document JSON 뱉지 말 것. "저장 완료 / critical N건" 한두 줄만
- (참고) Feature ≤ 3 인 작은 프로젝트는 단일 generation 도 가능 — 추후 임계 분기 추가 고려. 현재는 항상 분할 적용 (스톨 회피 우선)

## 진행 상태 가시화 — `todowrite` (선택, OpenCode)

OpenCode 환경이면 각 Step 진입 시 내장 `todowrite` 로 현재/남은 단계를 알려주면 UX 가 좋다. 강제 아님, Claude Code 에서는 무시. 자세한 규칙은 `.claude/skills/_shared/todowrite-guide.md`.

이 스킬 Step 체크리스트:
```
# Todos
[ ] Step 0   — 진입 점검 (drift-check + 분기) + 데이터 수집
[ ] Step 1.1 — Entities (Domain + User Story + Use Case) — JSON only
[ ] Step 1.2 — Features (선정 + relatedVOS + mode) — 시나리오는 비움
[ ] Step 1.3 — Scenarios (basicflow / alternative / exception / pre·postcondition)
[ ] Step 1.4 — Diagrams (Context / Use Case / Sequence × Feature) — PlantUML 만
[ ] Step 1.5 — 결정적 문서 조립 (LLM 0회) — usecase-assemble.py
[ ] Step 2   — 자가검증 (validate-usecases.py --stage=final)
[ ] Step 3   — critical 수정 (반복, 최대 iter 3)
[ ] Step 5   — Feature/UC 이슈 + 링크 + use-cases.md 영속화
[ ] Step 6   — 최종 보고 + baseline 갱신
```

## 환경
- `PLATFORM_API` 기본값: `http://10.211.55.3:44350/api/v2`
- `GOJIRA_API` 기본값: `http://10.211.55.3:44360/api/v2`
- 임시 파일:
  - `/tmp/${CODE}-uc-draft.json` — 누적 draft (Step 1.1~1.4 가 단계별로 merge)
  - `/tmp/${CODE}-uc-validation.json` — 검증 결과
  - `/tmp/${CODE}-uc-final.md` — Step 1.5 결정적 조립 결과 (use-cases.md 미저장 사본)

## 인자 추출

사용자 메시지에서 다음 값들을 추출해 **먼저** 환경변수로 export 한 뒤 아래 bash 블록들을 실행하라. 누락 시 사용자에게 되물어 확정하라.

- `CODE` — projectCode (예: GOJIRA3)
- `USER` — user (예: batman)
- `MODE` — `full`(기본) | `relink`. `relink` 는 **기존 Feature 이슈를 보존**하고 VOS ↔ Feature 링크 / use-cases.md 만 재생성. 사용자가 "연결 재정리", "링크만", "재링크" 류의 표현을 쓰면 `relink` 로 해석

예:
```bash
export CODE="GOJIRA3"
export USER="batman"
export MODE="full"   # 또는 relink
```

## 역할
소프트웨어 아키텍트. **모든 VOS 가 User Story → Use Case → Feature 로 빠짐없이 파생되는 커버리지**를 책임진다.

### 핵심 파생 규칙
```
VOS ──(1:N)──▶ User Story ──(N:M)──▶ Use Case ──(선별)──▶ Feature
```
1. **VOS → User Story**: 각 VOS 에서 최소 1개 User Story 파생 (Happy/Alternative/Exception)
2. **User Story → Use Case**: 유사 목적 Story 를 묶어 Use Case 도출. 모든 Story 는 반드시 하나의 UC 에 매핑
3. **Use Case Diagram**: Feature 선정 **이전에** 모든 Use Case 로 다이어그램 작성
4. **Feature 선정**: Actor 와 1차적으로 연결되는 Use Case 만 Feature (include/extend/보조 제외)
   - 이 단계에서 VOS → User Story → Use Case 경로가 보장되므로 **모든 VOS 는 반드시 1개 이상의 Feature 에 연결됨**
5. **Feature 미선정 Use Case 유지**: Feature 로 선정되지 않은 Use Case 는 삭제하지 않음. Use Case 다이어그램도 수정하지 않음

### VOS ↔ Feature 연결 양방향 필수
- 모든 **VOS** 는 최소 1 개 Feature 와 연결 (기존 원칙)
- 모든 **Feature** 는 최소 1 개 VOS 와 연결 — Feature 는 VOS → UC → Feature 로 도출된 것이므로 연결할 VOS 가 없다는 건 Feature 자체의 도출 근거가 없다는 신호
- `relatedVOS` 는 최소 1 개 VOS 키 필수. 빈 배열 금지

### NFR 성격 VOS 도 Feature 확보 (면제 없음)
품질속성/정책/제약 성격이 강한 VOS 라도 **반드시 그 요구를 "구현하는" 기능 Feature 를 도출**한다. 면제(NFR 표기만) 금지.
- 원칙: "이 요구를 만족시키려면 시스템이 **무슨 동작**을 해야 하는가?" 로 환원 → 그 동작이 Feature
- Feature 가 확보되면 후속 Requirement 단계에서 그 Feature 에 NFR/CNST 속성 Requirement 를 연결하여 품질속성을 표현

파생 예시:
- "개인정보는 사용자 동의 없이 수집되지 않아야 함" → Feature **"사용자 동의 수집 및 데이터 수집 정책 표시"** (동의 플로우, 프라이버시 설정 UC)
- "네트워크 불안정 시 게임 상태가 유지되어야 함" → Feature **"세션 자동 복구/재접속"** (재접속 UC, 상태 스냅샷 UC)
- "접근성 준수" → Feature **"접근성 설정/키보드 네비게이션"**
- "응답시간 XX 이하" → Feature **"성능 모니터링/로드 상태 표시"** 또는 기능 Feature 자체에 NFR 로 연결

---

## Step 0 — 진입 점검 (drift-check) + 데이터 수집

### 0.1 drift-check (이슈 변동 여부 확인 — 필수)

본 스킬의 책임 단계는 **`usecase`** (workflow-graph.json). Step 0 진입 즉시 drift-check 를 호출해 **이슈 변동 여부 + 상위(`vos`) 변동 여부**를 확인하고, 그 결과로 진행 방식을 결정한다.

```bash
python3 "${PROJECT_ROOT}/.claude/skills/_scripts/drift-check.py" "${CODE}" "${USER}" check
```

### 0.2 drift-check 결과별 분기 (A/B/C 모델)

| drift-check 결과 | 흐름 | 동작 |
|---|---|---|
| `usecase` **NO-BASELINE** | 첫 실행 | Step 0.3 데이터 수집 → MODE=full |
| `usecase` **DRIFT** (이슈 변동 감지) | **B** | 본 단계 변경 영향 분석 + 부분 업데이트 (현재 미구현, MODE=full/relink 로 임시 진행) |
| `usecase` **CLEAN** + 상위(`vos`) **DRIFT** | **C** | 사용자 선택: ① 본 stage 재정립 (B 흐름 적용) / ② mark-deferred 보류 |
| **모두 CLEAN** | **A** | 0.2.1 의 3 메뉴 — 절대 자동 진행 금지 |

### 0.2.1 변경 없음 처리 메뉴 (A 흐름)

drift 모두 CLEAN 인 경우 **반드시 사용자에게 다음 3 옵션을 명시적으로 제시**. 추측 / 자동 선택 절대 금지.

```
변경된 이슈 / 산출물이 없습니다. 어떻게 처리할까요?
  [1] 종료 (default — 가장 안전, 아무것도 안 함)
  [2] 문서만 재생성 (이슈/artifact 보존, LLM 0회 결정적 조립)
  [3] 처음부터 다시 만들기 ⚠️ DESTRUCTIVE (이슈 모두 삭제 후 LLM 재도출)
```

### 선택별 동작

- **[1] 종료** — 즉시 Step 6 보고만 출력 (baseline 갱신 없음) → 종료
- **[2] 문서만 재생성** — `FAST_PATH=true` 설정 후 Step 1~Step 1.5 모두 스킵, Step 5.4 결정적 조립만. 이슈/artifact 절대 변경 안 함
- **[3] 처음부터 다시** — ⚠️ **이 SKILL 자체에서는 진행 금지**. 사용자에게 안내:
  > "destructive 작업입니다. 별도로 `reset-project` 스킬을 먼저 호출하시고 (CONFIRM=true 필요), 그 후 본 스킬을 다시 호출해주세요."
  
  본 SKILL 은 [3] 직접 수행하지 않음 — destructive 동작 격리 원칙.

### ⚠️ 절대 금지 (LLM 행동 가드)

다음 중 어느 것도 LLM 자율 판단으로 호출 금지:
- `DELETE /Issue` (이슈 삭제)
- `PUT /Issue` (이슈 수정 — 단 Step 5.1 batch 흐름 내부는 예외)
- `POST /Issue/link` 의 신규 링크 (단 Step 5.2 차집합 후 NEW 만 POST 는 OK)
- `archSettings` `$unset`

**FAST_PATH=true 진입 시**: 새 이슈 생성 / 기존 이슈 수정 / 삭제 / artifact 삭제 모두 금지. 오직 doc 재생성만. 위 호출이 필요해 보이면 즉시 SKILL 중단 + 사용자에게 보고.

진행 결정 후 Step 0.3 (데이터 수집) 으로 이동. [1] 종료면 Step 6 보고만 출력하고 baseline 갱신 없이 끝낸다.

### 0.3 데이터 수집

```bash
PLATFORM_API="${PLATFORM_API:-http://10.211.55.3:44350/api/v2}"
GOJIRA_API="${GOJIRA_API:-http://10.211.55.3:44360/api/v2}"

# project-description.md (docs/{user} 우선, fallback main — 스크립트 자동 처리)
DESC=$(bash "${PROJECT_ROOT}/.claude/skills/_scripts/doc-read.sh" "${CODE}" "${USER}" "business/project-description.md")
echo "=== project-description.md ==="
echo "$DESC"

# VOS 목록 (issueLinks 포함 — relink 차집합 계산 / Feature 미연결 VOS 검출용)
echo
echo "=== VOS 목록 ==="
curl -s "${GOJIRA_API}/Issue" -H "XBody: {\"prjID\":\"${CODE}\",\"infoType\":\"\"}" -H "X-User-Id: ${USER}" \
  | jq '[.[] | select(.issueType=="VOS") | {key,summary,description,client,issueLinks}]'

# 기존 Feature (issueLinks + 전체 본문 — relink/강제 재생성 에서 원문 그대로 보존)
echo
echo "=== 기존 Feature ==="
curl -s "${GOJIRA_API}/Issue" -H "XBody: {\"prjID\":\"${CODE}\",\"infoType\":\"\"}" -H "X-User-Id: ${USER}" \
  | jq '[.[] | select(.issueType=="Feature") | {key,subkey,summary,description,actor,precondition,basicflow,alternativeflow,exceptionflow,postcondition,issueLinks}]'
```

- VOS 가 0건 → **중단** 하고 "vos-collect2 를 먼저 실행하세요" 보고
- `MODE=relink` 인데 기존 Feature 0 건 → **중단** 하고 `MODE=full` 로 전환 권고 (재링크 대상 없음)
- **강제 재생성 모드**인데 기존 Feature 0 건 → **중단** (재생성할 대상 없음)

### 0.4 누적 draft 초기화

다음 step 1.1~1.4 가 누적해서 채우는 draft 파일을 빈 객체로 시작 (`vosKeys` / `existingFeatureKeys` 만 채워둔다).

```bash
# vosKeys / existingFeatureKeys 는 위에서 수집한 결과로 채움
jq -n --argjson vos "$VOS_KEYS_JSON" --argjson feat "$EXISTING_FEAT_KEYS_JSON" \
   '{vosKeys: $vos, existingFeatureKeys: $feat,
     domain: {}, userStories: [], useCases: [],
     features: [], diagrams: {}, document: ""}' \
  | bash "${PROJECT_ROOT}/.claude/skills/_scripts/draft-io.sh" save "${CODE}" "uc" "draft" "init"
```

이 tool call 로 Step 0 종료. 다음 generation 에서 Step 1.1 시작.

---

## Step 1 — 분할 생성 (iter 1)

Step 1 은 **1.1 ~ 1.5 의 5 sub-step** 으로 나뉜다. 각 sub-step 은 한 generation 안에서만 처리하고, 끝에 반드시 `draft-io.sh save|merge` 로 generation 경계를 만든다.

### Artifact fast path 분기 (Step 0.2.1 결과 적용)

`MODE=relink / 강제 재생성` + `ART_EXISTS=true` → **Step 1.1~1.4 모두 스킵**, 환경변수 `FAST_PATH=true` 로 두고 바로 Step 1.5 (`--from-mongo` 모드) 호출. LLM 호출 없음.

```bash
export FAST_PATH=true
```

→ 다음 generation 에서 Step 1.5 진입.

### MODE=relink / 강제 재생성 모드 축약 규칙 (artifact 없는 legacy 케이스 — sub-step 공통)

이슈 생성/수정/삭제 금지. 기존 Feature 원문(summary/actor/flows) 그대로 보존. sub-step 별로:

- **1.1**: 기존 use-cases.md 가 있으면 거기서 도메인/User Story/UC 표를 그대로 추출. 신규 도출 금지
- **1.2**: 기존 Feature 의 summary/actor/subkey 를 그대로 옮기고 `mode=reuse`, `existingKey`= 기존 key, `docId` = 기존 subkey (예: UC_001). **`relatedVOS` 만 VOS 목록 대조해 재산정 — 빈 배열 금지**
- **1.3**: 기존 Feature 본문에서 basicflow/alternative/exception/precondition/postcondition 그대로 취해 features 배열에 채움
- **1.4**: 기존 use-cases.md 의 PlantUML 블록을 그대로 추출해 diagrams.context / usecase / sequences 에 보존
- **1.5**: MODE=full 과 동일 — 결정적 조립 (LLM 호출 0회)

이후 Step 2 self-loop 는 MODE=full 과 동일하게 진행.

### MODE=full (기본)

---

## Step 1.1 — Entities (Domain + User Story + Use Case)

**관심**: 도메인 경계 / 핵심 개념 / Actor 식별 / 도메인 규칙 + VOS → User Story → Use Case 파생.

작성 규칙:
- 본 sub-step 은 **JSON 만** 산출 — markdown / 다이어그램 절대 금지 (다음 단계들에서 채움)
- 모든 VOS 가 최소 1개 User Story 를 가져야 함 (`vosKey` 필드)
- User Story 분류: **A=Happy / B=Alternative / C=Exception**
- 모든 User Story 는 반드시 하나의 Use Case 에 매핑 (`ucId` 필드)
- Use Case 는 비슷한 목적 Story 묶기. "게임 시작 / 추측 입력 / 게임 종료" 처럼 다른 시점/입출력은 별도 UC
- Actor 식별: Primary(가치 요청 트리거) / Supporting(협력)
- 암묵적 Story 능동 발굴: 계정/인증, 중단/이탈, 다중 사용자 동기화, 데이터 영속성

### Step 1.1 종료 — draft merge + artifact PUT

```bash
# 본 generation 에서 도출한 entities 를 partial JSON 으로 merge
cat > /tmp/${CODE}-uc-step1.1.json <<'JSON'
{
  "domain": {
    "scope": "...시스템 책임 영역...",
    "concepts": [{"name":"...","desc":"..."}],
    "primaryActors": [{"name":"...","role":"..."}],
    "supportingActors": [{"name":"...","role":"..."}],
    "rules": ["..."]
  },
  "userStories": [
    {"id":"US-1","actor":"사용자","category":"A","story":"...은(는) ...할 수 있다","vosKey":"TEST-1","ucId":"UC-1"}
  ],
  "useCases": [
    {"id":"UC-1","summary":"...","stories":["US-1","US-2"],"feature":true}
  ]
}
JSON

bash "${PROJECT_ROOT}/.claude/skills/_scripts/draft-io.sh" merge "${CODE}" "uc" "draft" "entities" \
  < /tmp/${CODE}-uc-step1.1.json

# **artifact 영속화** — 다음 호출(MODE=relink/강제 재생성) 에서 LLM 0회 재생성용
GOJIRA_API="${GOJIRA_API:-http://10.211.55.3:44360/api/v2}"
jq '{domain, userStories, useCases}' /tmp/${CODE}-uc-step1.1.json \
  | curl -s -X PUT "${GOJIRA_API}/Project/${CODE}/usecase-artifacts" \
      -H "Content-Type: application/json" -H "X-User-Id: ${USER}" \
      --data-binary @- | jq '.updated'
```

(선택) 단계별 자가검증:
```bash
bash "${PROJECT_ROOT}/.claude/skills/_scripts/draft-io.sh" load "${CODE}" "uc" "draft" \
  | python3 "${PROJECT_ROOT}/.claude/skills/_scripts/validate-usecases.py" --stage=entities
```

이 tool call 로 generation 종료. 다음 generation 에서 Step 1.2 시작.

---

## Step 1.2 — Features 선정 (relatedVOS + mode)

**관심**: Actor 와 1차 연결되는 Use Case 만 Feature 로 승격. include/extend/보조 UC 는 제외.

작성 규칙:
- features 배열에 `docId / summary / actor / relatedVOS / mode / existingKey` **만** 채움
- **`basicflow` / `alternativeflow` / `exceptionflow` / `precondition` / `postcondition` 는 빈 문자열로 두기** — Step 1.3 에서 채움
- `docId` = `UC-1`, `UC-2`, ... 1부터 순차. Use Case id 와 정합되어야 함 (1.1 에서 정한 UC 식별자 그대로)
- `relatedVOS`: VOS 목록 실존 키만, **최소 1개 필수, 빈 배열 금지**
- `mode`: `create` / `reuse` / `modify` / `delete_mark`
- 과도 통합 금지: "게임 플레이" 하나로 [시작/입력/결과/이력] 묶지 말 것

### Step 1.2 종료 — draft merge

```bash
cat > /tmp/${CODE}-uc-step1.2.json <<'JSON'
{
  "features": [
    {"docId":"UC-1","summary":"...","actor":"사용자","relatedVOS":["TEST-1"],"mode":"create","existingKey":"",
     "precondition":"","basicflow":"","alternativeflow":"","exceptionflow":"","postcondition":""}
  ]
}
JSON

bash "${PROJECT_ROOT}/.claude/skills/_scripts/draft-io.sh" merge "${CODE}" "uc" "draft" "features" \
  < /tmp/${CODE}-uc-step1.2.json
```

(선택) 단계별 검증:
```bash
bash "${PROJECT_ROOT}/.claude/skills/_scripts/draft-io.sh" load "${CODE}" "uc" "draft" \
  | python3 "${PROJECT_ROOT}/.claude/skills/_scripts/validate-usecases.py" --stage=features
```

merge 는 docId 기준으로 항목별 머지. 같은 docId 가 있으면 필드 추가/갱신, 없으면 append.

이 tool call 로 generation 종료. 다음 generation 에서 Step 1.3.

---

## Step 1.3 — Scenarios

**관심**: 각 Feature 에 Basic / Alternative / Exception Flow + Pre/Postcondition 작성.

작성 규칙:
- features 배열을 그대로 두고 각 항목에 `basicflow` 등을 채워서 patch 한다 (docId 매칭)
- Basic Flow 가 5단계 이상이면 별도 UC 분해 검토 (1.1 로 되돌아가야 할 신호 — 가능하면 sub-step 안에서 분해 안 함, 보고만)
- 텍스트만 작성 — **다이어그램 작성 금지** (Step 1.4 에서)
- Step 1.2 에서 만든 features 항목과 동일 `docId` 로 patch

### Step 1.3 종료 — draft merge

```bash
cat > /tmp/${CODE}-uc-step1.3.json <<'JSON'
{
  "features": [
    {"docId":"UC-1",
     "precondition":"미로그인 상태",
     "basicflow":"1. 사용자가 ID 입력\n2. 시스템이 인증 확인\n3. 세션 발급",
     "alternativeflow":"- 자동 로그인 토큰이 있으면 ID 입력 생략",
     "exceptionflow":"- 인증 실패 시 오류 메시지 표시",
     "postcondition":"세션 생성됨"}
  ]
}
JSON

bash "${PROJECT_ROOT}/.claude/skills/_scripts/draft-io.sh" merge "${CODE}" "uc" "draft" "scenarios" \
  < /tmp/${CODE}-uc-step1.3.json
```

(선택) 단계별 검증:
```bash
bash "${PROJECT_ROOT}/.claude/skills/_scripts/draft-io.sh" load "${CODE}" "uc" "draft" \
  | python3 "${PROJECT_ROOT}/.claude/skills/_scripts/validate-usecases.py" --stage=scenarios
```

이 tool call 로 generation 종료. 다음 generation 에서 Step 1.4.

---

## Step 1.4 — Diagrams (PlantUML)

**관심**: Context / Use Case / Sequence × Feature 다이어그램 작성. 가장 큰 출력 + 가장 큰 스톨 위험 — 격리해서 한 generation 에서 모두 생성.

작성 규칙:
- 모든 다이어그램은 ` ```plantuml ... ``` ` 코드펜스 **없이** **PlantUML 본문만** (`@startuml ... @enduml`) 산출 — 코드펜스는 Step 1.5 결정적 조립이 자동으로 감싼다
- **반드시 `@startuml` / `@enduml` 쌍** 포함
- **Context Diagram** — actor + rectangle 로 시스템 경계 표현. 본문에 `Context` 또는 `컨텍스트` 라벨 포함
- **Use Case Diagram** — `actor` + `usecase "..." as UC_NNN` + `rectangle "..."` 묶음. **모든 Use Case 포함** (Feature 미선정 UC 도). include/extend 는 점선 화살표 (`UC1 ..> UC2 : <<include>>`)
- **Sequence Diagram** — **각 Feature(docId) 마다 1개 필수**. `participant "이름" as 별칭` 또는 공백 없는 별칭 사용
- 기본 actor 스타일: `skinparam actorStyle stick` (use case / context 권장)
- 레이아웃: `left to right direction` (use case / context 권장)

**금지**
- `class` / `interface` / `abstract` / `enum` 키워드 (Use Case 를 class 다이어그램으로 표현 절대 금지)
- 외부 이미지 참조 (`![...](...png)`)
- `@startuml` / `@enduml` 쌍 누락
- participant/actor 이름에 공백 있는데 따옴표도 `as` 도 없는 선언 (`participant 후공 사용자` — 잘못. `participant "후공 사용자" as U2` 또는 `participant U2 as "후공 사용자"` 로)

### Step 1.4 종료 — draft merge + artifact PUT

```bash
# diagrams.sequences 의 키는 features[].docId 와 정확히 일치해야 한다 (validator 가 매칭)
cat > /tmp/${CODE}-uc-step1.4.json <<'JSON'
{
  "diagrams": {
    "context": "@startuml\nskinparam actorStyle stick\nleft to right direction\nactor 사용자 as User\nrectangle \"시스템\" as Sys\nrectangle \"외부 시스템\" as Ext\nUser --> Sys\nSys --> Ext\n' Context\n@enduml",
    "usecase": "@startuml\nskinparam actorStyle stick\nleft to right direction\nactor 사용자 as User\nrectangle \"시스템\" {\n  usecase \"로그인\" as UC1\n  usecase \"세션 관리\" as UC2\n}\nUser --> UC1\nUC1 ..> UC2 : <<include>>\n@enduml",
    "sequences": {
      "UC-1": "@startuml\nactor 사용자 as User\nparticipant \"시스템\" as Sys\nUser -> Sys: 로그인 요청\nSys --> User: 세션 발급\n@enduml"
    }
  }
}
JSON

bash "${PROJECT_ROOT}/.claude/skills/_scripts/draft-io.sh" merge "${CODE}" "uc" "draft" "diagrams" \
  < /tmp/${CODE}-uc-step1.4.json

# **artifact 영속화** — diagrams + vosKeys 도 함께
GOJIRA_API="${GOJIRA_API:-http://10.211.55.3:44360/api/v2}"
jq --argjson vos "$VOS_KEYS_JSON" '{diagrams, vosKeys: $vos}' /tmp/${CODE}-uc-step1.4.json \
  | curl -s -X PUT "${GOJIRA_API}/Project/${CODE}/usecase-artifacts" \
      -H "Content-Type: application/json" -H "X-User-Id: ${USER}" \
      --data-binary @- | jq '.updated'
```

(선택) 단계별 검증:
```bash
bash "${PROJECT_ROOT}/.claude/skills/_scripts/draft-io.sh" load "${CODE}" "uc" "draft" \
  | python3 "${PROJECT_ROOT}/.claude/skills/_scripts/validate-usecases.py" --stage=diagrams
```

이 tool call 로 generation 종료. 다음 generation 에서 Step 1.5 결정적 조립.

---

## Step 1.5 — 결정적 문서 조립 (LLM 0회 — 필수)

누적된 draft (`/tmp/${CODE}-uc-draft.json`) 또는 `usecase_artifacts` mongo 컬렉션에서 entities + features + diagrams 를 가져와 `usecase-assemble.py` 가 템플릿에 채워 use-cases.md 본문을 생성한다. **이 step 에서 LLM 호출 절대 금지**.

```bash
TEMPLATE="${PROJECT_ROOT}/.claude/skills/usecase-analyze2/templates/use-cases.md"

if [ "${FAST_PATH:-false}" = "true" ]; then
  # fast path: artifact + Feature/VOS 이슈 mongo 직접 fetch (Step 1.1~1.4 스킵 케이스)
  python3 "${PROJECT_ROOT}/.claude/skills/_scripts/usecase-assemble.py" \
    --from-mongo "${CODE}" --user "${USER}" --template "$TEMPLATE" \
    > /tmp/${CODE}-uc-final.md
else
  # 기존 흐름: draft.json 로드 → assemble
  bash "${PROJECT_ROOT}/.claude/skills/_scripts/draft-io.sh" load "${CODE}" "uc" "draft" \
    | python3 "${PROJECT_ROOT}/.claude/skills/_scripts/usecase-assemble.py" --template "$TEMPLATE" \
    > /tmp/${CODE}-uc-final.md
fi
echo "use-cases.md 길이=$(wc -c < /tmp/${CODE}-uc-final.md | tr -d ' ')"

# draft 의 document 필드에도 본문을 주입 (Step 2 validator 가 document 키를 본다)
# fast path 의 경우 draft.json 이 비어있을 수 있으니 먼저 init 한 뒤 merge.
if [ "${FAST_PATH:-false}" = "true" ] && [ ! -f "/tmp/${CODE}-uc-draft.json" ]; then
  # 검증에 필요한 최소 골격 (vosKeys + features) 을 mongo 에서 한 번 더 빌드
  python3 - <<PY > /tmp/${CODE}-uc-draft-init.json
import json, urllib.request, sys
api="${GOJIRA_API:-http://10.211.55.3:44360/api/v2}"
def get(u):
    req=urllib.request.Request(u, headers={"X-User-Id":"${USER}"})
    return json.loads(urllib.request.urlopen(req,timeout=10).read())
art=get(f"{api}/Project/${CODE}/usecase-artifacts")
issues_req=urllib.request.Request(f"{api}/Issue", headers={"X-User-Id":"${USER}","XBody":json.dumps({"prjID":"${CODE}","infoType":""})})
issues=json.loads(urllib.request.urlopen(issues_req,timeout=15).read())
print(json.dumps({
  "vosKeys": [i["key"] for i in issues if i.get("issueType")=="VOS"],
  "domain": art.get("domain") or {},
  "userStories": art.get("userStories") or [],
  "useCases": art.get("useCases") or [],
  "features": [],
  "diagrams": art.get("diagrams") or {}
}))
PY
  bash "${PROJECT_ROOT}/.claude/skills/_scripts/draft-io.sh" save "${CODE}" "uc" "draft" "init" \
    < /tmp/${CODE}-uc-draft-init.json
fi
DOC_JSON=$(jq -Rs . < /tmp/${CODE}-uc-final.md)
jq -n --argjson doc "$DOC_JSON" '{document: $doc}' \
  | bash "${PROJECT_ROOT}/.claude/skills/_scripts/draft-io.sh" merge "${CODE}" "uc" "draft" "final"
```

이 tool call 로 generation 종료. 다음 generation 에서 Step 2 자가검증.

---

## Step 2 — 자가검증 (스크립트 기반)

**반드시 `validate-usecases.py --stage=final` 호출**. 에이전트가 직접 커버리지 / PlantUML 구문 / Actor 다양성 계산 금지.

```bash
bash "${PROJECT_ROOT}/.claude/skills/_scripts/draft-io.sh" load "${CODE}" "uc" "draft" \
  | python3 "${PROJECT_ROOT}/.claude/skills/_scripts/validate-usecases.py" --stage=final \
  > /tmp/${CODE}-uc-validation.json
cat /tmp/${CODE}-uc-validation.json
```

> 단계별 (`entities` / `features` / `scenarios` / `diagrams`) 자가검증은 Step 1.x sub-step 내부에서 선택적으로 실행 가능. Step 2 는 항상 `--stage=final` 로 누적 결과를 한 번 더 검증한다.

출력 스키마:
```json
{
  "pass": bool,
  "score": 0.0~1.0,
  "score_effective": number,
  "critical": [{"type":"...","docId":"...","detail":"..."}],
  "suggestions": [...],
  "stats": {"vosKeys":N,"storyVosCovered":V1,"featureVosCovered":V2,
            "features":F,"sequenceDiagramCount":SD,
            "useCaseDiagramFound":bool,"contextDiagramFound":bool,
            "plantumlBlocks":M,"plantumlBadPair":X}
}
```

스크립트가 잡는 critical:
- `vos_no_story` — VOS 가 User Story 에 미파생
- `vos_no_feature` — VOS 가 Feature relatedVOS 에 미포함 (NFR 면제 없음)
- `missing_field` / `empty_field` — Feature 필수 필드 누락 (summary/actor/basicflow/docId/relatedVOS)
- `relatedVOS_empty` / `relatedVOS_hallucination` / `bad_relatedVOS_type`
- `duplicate_docId`
- `over_integration` — Feature 1건에 VOS 3건 이상 묶음
- `single_actor_all` — Feature ≥3 이 모두 동일 Actor
- `class_diagram_for_uc` — class 다이어그램으로 UC 표현
- `missing_usecase_diagram` / `missing_sequence_diagram`
- `plantuml_missing_pair` — `@startuml` / `@enduml` 쌍 누락
- `external_image` — 외부 이미지 참조
- `nfr_placeholder` — (NFR) 플레이스홀더

### 통과 기준
- `pass == true` → Step 5 진입
- `pass == false` → Step 3 수정

---

## Step 3 — 수정 (pass=false)

validation output 의 critical 만 **국소 수정**. 수정할 키만 partial JSON 으로 만들어 `merge` 로 갱신.

```bash
# 예 1: 특정 Feature 의 relatedVOS 빈 배열 수정 (features 배열에 docId 매칭으로 머지)
cat > /tmp/${CODE}-uc-fix.json <<'JSON'
{
  "features": [
    {"docId":"UC-3","relatedVOS":["TEST-2"]}
  ]
}
JSON
bash "${PROJECT_ROOT}/.claude/skills/_scripts/draft-io.sh" merge "${CODE}" "uc" "draft" "fix" \
  < /tmp/${CODE}-uc-fix.json

# 예 2: 특정 Feature 의 Sequence Diagram 누락 수정
jq -n '{diagrams:{sequences:{"UC-3":"@startuml\n...\n@enduml"}}}' \
  | bash "${PROJECT_ROOT}/.claude/skills/_scripts/draft-io.sh" merge "${CODE}" "uc" "draft" "fix"

# 수정 반영 후 Step 1.5 결정적 조립을 다시 실행해 document 갱신 (필수)
TEMPLATE="${PROJECT_ROOT}/.claude/skills/usecase-analyze2/templates/use-cases.md"
bash "${PROJECT_ROOT}/.claude/skills/_scripts/draft-io.sh" load "${CODE}" "uc" "draft" \
  | python3 "${PROJECT_ROOT}/.claude/skills/_scripts/usecase-assemble.py" --template "$TEMPLATE" \
  > /tmp/${CODE}-uc-final.md
DOC_JSON=$(jq -Rs . < /tmp/${CODE}-uc-final.md)
jq -n --argjson doc "$DOC_JSON" '{document: $doc}' \
  | bash "${PROJECT_ROOT}/.claude/skills/_scripts/draft-io.sh" merge "${CODE}" "uc" "draft" "final"
```

이 tool call 로 generation 종료. 다음 generation 에서 Step 2 validate 재실행.

---

## Step 4 — 종료 조건
1. pass=true
2. iter 3 완료
3. 같은 critical 2회 연속 재발
4. 새 수정이 이전 best 보다 낮은 score_effective → 포기

**best-so-far**: `(features, document, coverageMatrix, score_effective)` 기록. 종료 시 최댓값 iter 를 selected.

---

## Step 5 — 이슈 생성 + 링크 + 문서 저장

### ⚠️ 실행 규칙
- **개별 curl POST/PUT 반복 절대 금지** — 반드시 batch 스크립트 사용.
- 스크립트 경로: `${PROJECT_ROOT}/.claude/skills/_scripts/`

### 5.1 Feature 이슈 일괄 생성

**MODE=relink 또는 강제 재생성 모드(Step 0.2 의 drift CLEAN + 사용자 yes) 이면 이 섹션 전체 스킵** — 이슈 생성/수정/삭제 모두 하지 않는다. 바로 5.2 로.

create 모드 항목을 JSON 배열로 모아 batch 스크립트 **1회** 실행.
`issue-batch-create.sh` 는 순차 POST + PUT(추가 필드) + 재시도를 자동 처리한다.

```bash
# items 에 actor/precondition/basicflow 등을 포함시키면 PUT 으로 자동 업데이트됨
cat > /tmp/uc_items.json <<'JSON'
[
  {"issueType":"Feature","summary":"...","desc":"...","actor":"...","precondition":"...","basicflow":"...","alternativeflow":"...","exceptionflow":"...","postcondition":"..."},
  ...
]
JSON

bash "${PROJECT_ROOT}/.claude/skills/_scripts/issue-batch-create.sh" "${CODE}" "${USER}" < /tmp/uc_items.json \
  > /tmp/uc_result.json
cat /tmp/uc_result.json
```

결과의 `keys` 배열에서 생성된 Feature key 목록 확인.

### 5.2 VOS → Feature Contains 링크 일괄 생성

각 Feature 의 `relatedVOS` 를 순회해 링크 JSON 배열을 구성. 링크 방향: **VOS 가 inward, Feature 가 outward**.

> **중복 제거 필수** — `POST /Issue/link` 은 idempotent 아님 (매 호출마다 새 linkID 로 `$push`). Step 0 에서 수집한 VOS/Feature 의 `issueLinks[].{inwardIssue.key, outwardIssue.key, type.name}` 로 기존 튜플 집합 `EXISTING` 을 만들고, 새로 만들 집합 `DESIRED` 와 차집합을 계산:
> - `NEW = DESIRED - EXISTING` → `/tmp/uc_links.json` 에 이것만 씀
> - `KEPT = DESIRED ∩ EXISTING` → 보고에만 노출, POST 안 함
> - `REMOVAL_CANDIDATES = EXISTING - DESIRED` (VOS↔Feature 범위만) → **자동 삭제 금지**, 최종 보고에 ⚠ 목록으로만 표시

batch 스크립트 **1회** 실행 (NEW 만 POST):

```bash
cat > /tmp/uc_links.json <<'JSON'
[
  {"inwardKey":"GOJIRA3-1","outwardKey":"GOJIRA3-21","linkType":"Contains(WBSGantt)"},
  ...
]
JSON

bash "${PROJECT_ROOT}/.claude/skills/_scripts/issue-batch-link.sh" "${CODE}" "${USER}" "usecase-analyze2" < /tmp/uc_links.json \
  > /tmp/uc_link_result.json
cat /tmp/uc_link_result.json
```

링크 실패는 **silent drop 금지** — 최종 보고에 누락 건수 명시.

### 5.3 (제거됨)

이전엔 docId→subkey 치환을 LLM 이 markdown 본문에 적용했지만, 5.4 가 mongo 직접 fetch 로 변경되어 불필요. mongo Feature 이슈는 이미 실제 subkey 보유.

### 5.4 use-cases.md 저장 — 결정적 조립 (LLM 0회)

**MODE=full / fast path 동일 흐름**: Step 5.1 직후 mongo Feature 이슈 + Step 1.1·1.4 에서 PUT 한 `usecase_artifacts` 가 갖춰져 있으므로 `usecase-assemble.py --from-mongo` 가 결정적 조립. doc-save.sh 가 plantuml-fix 결정적 자동 수정 적용.

```bash
TEMPLATE="${PROJECT_ROOT}/.claude/skills/usecase-analyze2/templates/use-cases.md"
python3 "${PROJECT_ROOT}/.claude/skills/_scripts/usecase-assemble.py" \
  --from-mongo "${CODE}" --user "${USER}" --template "$TEMPLATE" \
  | bash "${PROJECT_ROOT}/.claude/skills/_scripts/doc-save.sh" "${CODE}" "${USER}" "business/use-cases.md"
echo "use-cases.md 저장 완료 (결정적 조립)"
```

**FAST_PATH=true 면** Step 1 + 5.1/5.2 모두 스킵하고 위 명령만 실행 후 `exit 0`.

---

## Step 6 — 최종 보고 + baseline 갱신

### 6.1 보고 출력

```
## Use Case 분석 완료 (v2 Claude Code) — MODE={full|relink|forced-doc-regen}

- 진입 drift: usecase=<NO-BASELINE|CLEAN|DRIFT>, vos=<...> (Step 0.1 결과)
- Feature 생성: N건 (키 리스트)   (relink / forced-doc-regen 은 0)
- VOS-Feature 링크: 신규 NEW / 기존 유지 KEPT / ⚠ 제거 후보 REM (REM>0 이면 key 쌍 나열 + 수동 처리 안내)
- 문서 저장: business/use-cases.md
- PlantUML 다이어그램: Context / Use Case / Sequence

### VOS 커버리지 매트릭스 (전 VOS ≥1 Feature, 전 Feature ≥1 VOS 연결)
| VOS 키 | 매핑된 Feature | 비고 |
|---|---|---|
| GOJIRA3-N | UC-1, UC-3 | |
| GOJIRA3-M | UC-5 | NFR 성격 — 구현 기능 Feature 확보 후 Requirement 단계에서 NFR 로 속성화

### Self-loop
- iter N회 / selected=iter X (score_eff Y) / stop=pass|max_iter|...
- unresolvedCritical: [...]
```

### 6.2 baseline 갱신 (필수 — 본 작업이 실제로 진행된 경우)

본 작업이 진행됐다면(NO-BASELINE 첫 실행 / DRIFT 처리 / 강제 재생성) **반드시** `usecase` stage 의 baseline 을 새 hash 로 갱신한다. Step 0.2 에서 "종료" 결정으로 끝난 경우는 호출하지 않는다.

```bash
python3 "${PROJECT_ROOT}/.claude/skills/_scripts/drift-check.py" "${CODE}" "${USER}" set-baseline --stage usecase
```

이 명령은 archSettings.lastRunHash[usecase] / lastRunAt[usecase] 만 갱신 — 다른 stage 의 baseline 은 건드리지 않으므로 하류 단계의 영향 추적이 유지된다.

---

## 주의
- 모든 curl 에 `X-User-Id: ${USER}` 헤더 필수 (403 방지)
- 링크 linkType 은 정확히 `Contains(WBSGantt)` (JIRA DC 표준)
- Platform-API `/file` 응답 content 는 base64 → `base64 -d`
- use-cases.md 본문은 **UTF-8 plain markdown** (Platform-API 가 서버 측에서 base64 인코딩 처리)
- PlantUML 코드블록은 반드시 ` ```plantuml ` ... ` ``` ` 로 감싸고 `@startuml` / `@enduml` 쌍 포함
- Feature 생성 후 subkey 를 응답에서 받아 docId 치환에 사용
- MODE 요약: `full` — 신규 Feature create + 링크 + 문서 작성 / `relink` — 이슈 생성·수정 없음, 링크 차집합 POST + 문서 재작성. 두 모드 모두 `/Issue/link` 가 idempotent 아니므로 POST 전 기존 `issueLinks` 와 차집합 필수
- 진입 정책: Step 0.1 의 drift-check 가 작업 진행 여부를 정한다. 모두 CLEAN 인데 사용자가 강제 재생성을 거부하면 즉시 종료 (LLM 호출 0회). 강제 재생성 yes 인 경우 MODE 와 무관하게 relink 흐름으로 강등
- baseline 갱신: 본 작업 진행 시 Step 6.2 의 `set-baseline --stage usecase` 호출이 필수. 이걸 안 하면 다음 호출 때 또 DRIFT 로 잡혀 재실행 압력이 생긴다
- **Step 1 분할**: 1.1~1.4 는 sub-step 1개 = generation 1개. 한 generation 에 두 sub-step 같이 처리하지 말 것 (스톨 방지). 각 sub-step 종료는 반드시 `draft-io.sh save|merge` Bash tool call
- **Step 1.5 결정적 조립**: `usecase-assemble.py` + 템플릿만으로 use-cases.md 본문 산출 — LLM 호출 0회. PlantUML 본문은 Step 1.4 의 raw 형태 (`@startuml ... @enduml`) 그대로 두고 코드펜스는 1.5 에서 자동 추가됨
