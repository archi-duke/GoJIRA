---
name: requirement-refine2
description: Requirement(FR/NFR/CNST) 를 도출·정제하고 Feature↔Requirement Contains 링크 + business/requirements.md 를 작성/재정립합니다. 사용자가 "요구사항 도출/생성" 을 요청하면 MODE=full, "연결 정리·재정립·재링크·relink·링크만 다시" 류 요청이면 MODE=relink 로 호출 (기존 이슈 보존, 링크·문서만 재작성, 차집합만 POST). Self-loop 자가검증(최대 3회) + Feature 커버리지 매트릭스. Claude Code 네이티브(curl/jq).
allowed-tools: Bash, Read
---

## 이 스킬을 언제 호출하나

사용자 요청이 아래 의도에 해당하면 이 스킬을 호출하고 `MODE` 를 확정한다.

- **MODE=relink** — "연결 정리", "링크 재정리", "관계 다시 맞춰", "재링크", "relink", "링크만 다시" 등 **기존 Requirement 를 유지하면서 Feature 와의 연결/문서만 재정립** 하려는 요청
- **MODE=full** — "Requirement 도출", "요구사항 생성", "FR/NFR/CNST 뽑아" 등 **Feature + use-cases 에서 신규 Requirement 를 만들어** 달라는 요청

단순 현황 조회(링크 몇 개, 고아 이슈 등)만 원하는 거면 먼저 `issue-analyze` 로 분석하고, 구조 변경(이슈·링크 수정) 의도가 확인된 뒤에만 이 스킬을 호출한다. 분석 결과만 요구한 턴에서 이 스킬을 "미리 돌려놓지" 말 것.

## 한 generation 당 제약 (GLM 계열 스톨 방지 — 필독)

- **한 번의 LLM generation 에서 Step 두 개 이상을 몰아 처리 금지**. 각 Step 종료는 **반드시 tool call (Bash) 로 마무리** 해서 generation 경계를 만든다
- reasoning 은 각 Step 당 3~5 줄 이내. 장기 추론 불필요. 상세 분석은 스크립트 output 으로
- Step 1 종료는 **반드시** `draft-io.sh save` 호출 — draft JSON 을 파일에 flush 후 그 generation 종료. 다음 generation 에서 Step 2 시작
- Step 2 자가검증은 **반드시** `validate-requirements.py` 호출로 수행 — 에이전트가 직접 필드/형식/커버리지 계산하지 말 것 (결정적 규칙은 전부 스크립트화됨)
- critical 수정 후에는 다시 `draft-io.sh save` → 이어 `validate-requirements.py` 로 재검증. 한 generation 당 1 iter
- assistant 출력에는 긴 JSON/문서를 그대로 뱉지 말 것. "/tmp/... 저장 완료" 식의 한두 줄 요약 + 핵심 지표만. 전체 draft 는 파일로만 유지

## 진행 상태 가시화 — `todowrite` (선택, OpenCode)

OpenCode 환경이면 각 Step 진입 시 내장 `todowrite` 로 현재/남은 단계를 알려주면 UX 가 좋다. 강제 아님, Claude Code 에서는 무시. 자세한 규칙은 `.claude/skills/_shared/todowrite-guide.md`.

이 스킬 Step 체크리스트:
```
# Todos
[ ] Step 0 — 데이터 수집
[ ] Step 1 — 초안 생성 (iter 1)
[ ] Step 2 — 자가검증 (validate-requirements.py)
[ ] Step 3 — critical 수정 (반복, 최대 iter 3)
[ ] Step 5 — 이슈/링크/문서 영속화
[ ] Step 6 — 최종 보고
```

## 환경
- `PLATFORM_API` 기본값: `http://localhost:9000/api/v2`
- `GOJIRA_API` 기본값: `http://localhost:9001/api/v2`
- 임시 파일 위치: `/tmp/${CODE}-reqref-{draft|validated}.json`

## 인자 추출

사용자 메시지에서 다음 값들을 추출해 **먼저** 환경변수로 export 한 뒤 아래 bash 블록들을 실행하라. 누락 시 사용자에게 되물어 확정하라.

- `CODE` — projectCode (예: GOJIRA3)
- `USER` — user (예: batman)
- `MODE` — `full`(기본) | `relink`. `relink` 는 **기존 Requirement 이슈를 보존**하고 Feature ↔ Requirement 링크 / requirements.md 만 재생성. 사용자가 "연결 재정리", "링크만", "재링크" 류의 표현을 쓰면 `relink` 로 해석

예:
```bash
export CODE="GOJIRA3"
export USER="batman"
export MODE="full"   # 또는 relink
```

## 역할
소프트웨어 아키텍트. Feature 와 Use Case 시나리오에서 **User Story(FR) / QAS(NFR) / 제약(CNST)** 을 도출. 모든 Feature 가 최소 1 개 Requirement(FR/NFR/CNST 구분 없이) 로 커버되도록 책임진다. User Story 형식과 QAS 6 요소 엄수.

---

## Step 0 — 데이터 수집

```bash
PLATFORM_API="${PLATFORM_API:-http://localhost:9000/api/v2}"
GOJIRA_API="${GOJIRA_API:-http://localhost:9001/api/v2}"

# project-description.md (docs/{user} 우선, fallback main — 스크립트 자동 처리)
DESC=$(bash "${PROJECT_ROOT}/.claude/skills/_scripts/doc-read.sh" "${CODE}" "${USER}" "business/project-description.md" 2>/dev/null || true)

# use-cases.md
UC=$(bash "${PROJECT_ROOT}/.claude/skills/_scripts/doc-read.sh" "${CODE}" "${USER}" "business/use-cases.md" 2>/dev/null || true)

echo "=== project-description.md ==="; echo "$DESC"
echo; echo "=== use-cases.md ==="; echo "$UC"

# Feature (issueLinks 포함 — relink 차집합 계산에 필요)
echo; echo "=== Feature ==="
curl -s "${GOJIRA_API}/Issue" -H "XBody: {\"prjID\":\"${CODE}\",\"infoType\":\"\"}" -H "X-User-Id: ${USER}" \
  | jq '[.[] | select(.issueType=="Feature") | {key,subkey,summary,description,actor,precondition,basicflow,alternativeflow,exceptionflow,postcondition,issueLinks}]'

# VOS (참조용)
echo; echo "=== VOS ==="
curl -s "${GOJIRA_API}/Issue" -H "XBody: {\"prjID\":\"${CODE}\",\"infoType\":\"\"}" -H "X-User-Id: ${USER}" \
  | jq '[.[] | select(.issueType=="VOS") | {key,summary}]'

# 기존 Requirement (description/QAS 전체 + issueLinks — relink 모드에서 원문 그대로 보존)
echo; echo "=== 기존 Requirement ==="
curl -s "${GOJIRA_API}/Issue" -H "XBody: {\"prjID\":\"${CODE}\",\"infoType\":\"\"}" -H "X-User-Id: ${USER}" \
  | jq '[.[] | select(.issueType=="Requirement") | {key,subkey,summary,description,reqtype,qa,source,stimulus,environment,artifact,response,measure,responseScenario,measureAuto,issueLinks}]'
```

- project-description.md 가 템플릿 수준(실질 < 30자) → **중단** 하고 `projdesc_interview` 먼저 권고
- Feature 0 건 → **중단** 하고 `usecase-analyze2` 먼저 권고
- `MODE=relink` 인데 기존 Requirement 0 건 → **중단** 하고 `MODE=full` 로 전환 권고 (재링크 대상이 없음)

---

## Step 1 — 초안 생성 (iter 1)

### MODE=relink 축약 규칙

- 모든 항목 `mode=reuse` 고정, `existingKey` = Step 0 의 기존 key, `docId` = 기존 subkey 그대로
- 신규 FR/NFR/CNST 추가 금지 (커버리지 부족은 critical 보고만)
- `relatedFeatures` 만 Feature 스코프 대조해 재산정

### MODE=full (기본)

각 Feature / Use Case 에서 FR / NFR / CNST 를 **다양한 관점에서 충분히** 도출.

### 다양성 가이드
- 각 Feature 는 보통 **2~5 개의 FR** 로 세분화. 단일 FR 은 세분화 부족 신호
- FR 관점: 기본 동작, 입력 검증, 조회/검색, CRUD, 권한/접근제어, 알림/피드백, 오류 처리
- NFR 은 품질 속성(QA)별로 다양: Performance / Security / Availability / Usability / Maintainability / Scalability
- CNST 는 과제 설명 / use-case 에서 **명확히 드러난 제약만** (기술 스택, 규정, 플랫폼, 표준). 추측성 금지
- **Feature 연결 방침 (FR/NFR/CNST 공통)**: 모든 Requirement 는 **최소 1 개 Feature** 와 Contains 링크 필수. Requirement 는 Feature 로부터 도출된 것이므로 `relatedFeatures` 가 빈 배열(`[]`) 이 되는 경우는 없음. 영향 범위가 전체 Feature 라면 6개 중 적용되는 전부를 열거한다 (cross-cutting). 하나도 못 찾으면 Requirement 자체의 도출 근거가 약한 것 — critical

### FR 규칙
- User Story 형식 필수: `"{역할}은(는) {목적}을 위해 {기능}을 할 수 있다"`
- 한 Feature 의 여러 사용자 행위(생성/조회/수정/삭제/검색 등)는 개별 FR 로 분리

### NFR 규칙 (QAS)
- 필드 필수: `qa, source, stimulus, environment, artifact, response, measure, responseScenario`
- measure 원본에 없으면 합리적 수준으로 결정 후 `measureAuto: true` + document 비고에 `[자동결정]`

### CNST 규칙
- 반드시 지켜야 하는 사양/규제/표준만
- 특정 Feature 만 구속하는 제약은 해당 키만 지정. cross-cutting 제약(예: "전 시스템 PCI-DSS 준수") 은 적용되는 Feature **전부** 를 열거 (빈 배열 금지)

### 수량 가이드
FR 10~30, NFR 3~10, CNST 0~5 수준.

### 중복 처리 (mode)
- `create` / `reuse` (existingKey) / `modify` (existingKey + modifyReason) / `delete_mark` (existingKey + deleteReason)

### docId
- FR → `FR-1, FR-2, ...` / NFR → `NFR-1, NFR-2, ...` / CNST → `CNST-1, CNST-2, ...`
- document 본문의 모든 Requirement 표기(표의 ID / (이슈 키) 열 / 헤딩 / 관련 Feature)는 **동일 docId 그대로**. Step 5.3 에서 실제 subkey(FR_NNN / QA_NNN / CNST_NNN) 로 치환
- `relatedFeatures` 에는 **실제 Feature 이슈 키**(`{code}-N`) 사용 (docId 아님)

### 항목 구조

`relatedFeatures` 는 FR / NFR / CNST 모두 **최소 1 개 이상의 Feature 키** 를 담은 배열. **빈 배열 금지** — Requirement 는 Feature 로부터 도출된 것이므로 연결할 Feature 가 없다는 건 Requirement 자체가 부적절하다는 신호.

FR:
```
{
  "docId": "FR-1",
  "reqType": "FR",
  "summary": "...",
  "description": "{역할}은(는) {목적}을 위해 {기능}을 할 수 있다",
  "relatedFeatures": ["GOJIRA3-5"],
  "mode": "create",
  "existingKey": "", "modifyReason": "", "deleteReason": ""
}
```

NFR (위 필드 + QAS 6 요소):
```
{
  "docId": "NFR-1",
  "reqType": "NFR",
  "summary": "로그인 응답 < 500ms",
  "description": "...",
  "relatedFeatures": ["GOJIRA3-5"],
  "mode": "create",
  "qa": "Performance",
  "source": "...", "stimulus": "...", "environment": "...",
  "artifact": "...", "response": "...",
  "measure": "500ms",
  "responseScenario": "...",
  "measureAuto": false
}
```

CNST:
```
{
  "docId": "CNST-1",
  "reqType": "CNST",
  "summary": "PCI-DSS 준수",
  "description": "...",
  "relatedFeatures": ["GOJIRA3-7"],
  "mode": "create"
}
```

cross-cutting NFR/CNST 예 (여러 Feature 열거): `"relatedFeatures": ["GOJIRA3-14","GOJIRA3-15","GOJIRA3-16","GOJIRA3-17","GOJIRA3-18","GOJIRA3-19"]`.

### 문서 템플릿 (requirements.md)

**반드시** 아래 템플릿 파일의 구조를 따른다. 섹션 번호/이름을 그대로 유지:

```bash
cat "${PROJECT_ROOT}/.claude/skills/requirement-refine2/templates/requirements.md"
```

- 섹션 1 개요 / 2 FR / 3 NFR / 4 CNST / 5 상세 / 6 Feature 커버리지 매트릭스 전부 포함
- User Story 형식 (`~은(는) ~을 위해 ~할 수 있다`) 엄수 — validator 가 잡음

### Step 1 종료 — draft flush (필수)

draft 를 파일로 저장하고 **이 generation 을 종료**. 다음 generation 에서 Step 2 시작.

**JSON 필드명은 스키마 그대로 사용**. validator 는 아래 키만 인식 — `desc` / `reqtype` / `related` 등 변형 금지.

```bash
cat > /tmp/${CODE}-reqref-draft-input.json <<'JSON'
{
  "requirements": [
    {"docId":"FR-1","reqType":"FR","summary":"...","description":"{역할}은(는) ... 할 수 있다","relatedFeatures":["GOJIRA3-5"],"mode":"create","existingKey":"","modifyReason":"","deleteReason":""},
    {"docId":"NFR-1","reqType":"NFR","summary":"...","description":"...","relatedFeatures":["GOJIRA3-5"],"mode":"create","qa":"Performance","source":"...","stimulus":"...","environment":"...","artifact":"...","response":"...","measure":"500ms","responseScenario":"...","measureAuto":false},
    {"docId":"CNST-1","reqType":"CNST","summary":"...","description":"...","relatedFeatures":[],"mode":"create"}
  ],
  "featureKeys": ["GOJIRA3-5","GOJIRA3-6"],
  "existingRequirementKeys": []
}
JSON

bash "${PROJECT_ROOT}/.claude/skills/_scripts/draft-io.sh" save "${CODE}" "reqref" "draft" \
  < /tmp/${CODE}-reqref-draft-input.json
```

출력 한 줄(`{"path":"...","bytes":N}`)만 assistant 메시지에 포함. 전체 JSON 재출력 금지.

---

## Step 2 — 자가검증 (스크립트 기반)

**반드시 `validate-requirements.py` 호출로 수행**. 에이전트가 직접 필드/형식/커버리지 계산하지 말 것.

```bash
bash "${PROJECT_ROOT}/.claude/skills/_scripts/draft-io.sh" load "${CODE}" "reqref" "draft" \
  | python3 "${PROJECT_ROOT}/.claude/skills/_scripts/validate-requirements.py" \
  > /tmp/${CODE}-reqref-validation.json
cat /tmp/${CODE}-reqref-validation.json
```

출력 스키마:
```json
{
  "pass": bool,
  "score": 0.0~1.0,
  "score_effective": number,
  "critical": [{"type":"...","docId":"...","detail":"..."}],
  "suggestions": [...],
  "stats": {"FR":N,"NFR":M,"CNST":K,"features":F,"coveredFeatures":C}
}
```

스크립트가 검사하는 critical 항목 (에이전트가 다시 계산할 필요 없음):
- 필수 필드 누락 (reqType 별 각각)
- docId 중복 / reqType 접두 불일치
- `relatedFeatures` 가 Feature 목록에 없는 키 참조 (hallucination)
- FR description User Story 형식 미준수 (정규식 검사)
- `mode=reuse/modify/delete_mark` 인데 `existingKey` 누락 또는 기존 Requirement 에 없음
- **Feature 1 건이라도 Requirement 미매핑** (cross-cutting 도 실제 Feature 키 열거 필요)
- `relatedFeatures` 가 배열 아님 / 빈 배열은 critical 아닌 suggestion (정책상 빈 배열 허용하지 않으면 수작업으로 critical 로 격상)

suggestions (검토 필요, critical 아님):
- FR-커버된 Feature 당 평균 FR < 1.5 (세분화 부족)
- QA 다양성 부족 (Performance/Security/Availability 중 2 개 이상 누락)
- 드문 QA 속성명

### 통과 기준
- `pass == true` (critical 없음 + score ≥ 0.8) → Step 4/5 진입
- `pass == false` → Step 3 수정

### 추가 — CNST "추측성" 판단
이것 하나만 **Type C (LLM 판단)**. critical 리스트에 자동 포함되지 않으므로 에이전트가 validation output 을 본 뒤 별도 검토 (원문 근거 없이 새로 지어낸 CNST 가 있는지 한 줄 확인).

---

## Step 3 — 수정 (pass=false)

validation output 의 critical 항목 **하나씩만** 국소 수정. 나머지 원문 유지.

수정 후 다시 **draft 파일을 갱신**하고 Step 2 재진입:

```bash
cat > /tmp/${CODE}-reqref-draft-input.json <<'JSON'
{ ...수정된 requirements / featureKeys / existingRequirementKeys... }
JSON
bash "${PROJECT_ROOT}/.claude/skills/_scripts/draft-io.sh" save "${CODE}" "reqref" "draft" \
  < /tmp/${CODE}-reqref-draft-input.json
```

이 tool call 로 generation 종료. 다음 generation 에서 validate 재실행.

---

## Step 4 — 종료 조건

1. pass=true
2. iter 3 완료
3. 같은 critical 2회 연속 재발
4. 새 수정이 이전 best 보다 score_effective 낮음 → 포기

**best-so-far**: 매 iter 의 `(requirements, document, coverageMatrix, score_effective)` 기록. 종료 시 최댓값 iter 를 selected.

---

## Step 5 — 이슈 생성 + 링크 + 문서 저장

### 5.1 Requirement 이슈 일괄 생성 (create)

**MODE=relink 이면 이 섹션 전체 스킵** — 이슈 생성/수정/삭제 모두 하지 않는다. 바로 5.2 로.

FR / NFR / CNST 모든 create 모드 항목을 JSON 배열로 모아 batch 스크립트 **1회** 실행.
`issue-batch-create.sh` 가 순차 POST + PUT(추가 필드) + 재시도를 자동 처리한다.

`relatedFeatures` 는 Step 5.2 링크 생성에만 쓰이며 Issue 본문에는 저장되지 않음 — batch JSON 에 포함시켜도 무해하나, 순수 생성용 페이로드만 전달해도 됨.

```bash
# reqType / qa / source 등 추가 필드를 포함시키면 PUT 으로 자동 업데이트됨
cat > /tmp/req_items.json <<'JSON'
[
  {"issueType":"Requirement","summary":"...","desc":"...","reqType":"FR"},
  {"issueType":"Requirement","summary":"...","desc":"...","reqType":"NFR","qa":"Performance","source":"...","stimulus":"...","environment":"...","artifact":"...","response":"...","measure":"...","responseScenario":"...","measureAuto":false},
  {"issueType":"Requirement","summary":"...","desc":"...","reqType":"CNST"}
]
JSON

bash "${PROJECT_ROOT}/.claude/skills/_scripts/issue-batch-create.sh" "${CODE}" "${USER}" < /tmp/req_items.json \
  > /tmp/req_result.json
cat /tmp/req_result.json
```

결과의 `keys` 배열에서 생성된 Requirement key 목록 확인. NFR 의 QAS 필드는 생성 후 PUT /Issue 로 업데이트:
```bash
curl -s -X PUT "${GOJIRA_API}/Issue" \
  -H "Content-Type: application/json" -H "X-User-Id: ${USER}" \
  -d "$(jq -n --arg p "$CODE" --arg k "$KEY" --arg u "$USER" \
    --arg qa "$QA" --arg src "$SRC" --arg stm "$STM" --arg env "$ENV" \
    --arg art "$ART" --arg resp "$RESP_F" --arg m "$MEASURE" --arg rs "$RSCEN" \
    --argjson auto $MEASURE_AUTO \
    '{prjID:$p, key:$k, user:$u, data:{qa:$qa, source:$src, stimulus:$stm, environment:$env, artifact:$art, response:$resp, measure:$m, responseScenario:$rs, measureAuto:$auto}}')"
```

- modify: `PUT /Issue` 로 해당 key 필드 업데이트 + 변경 코멘트
- reuse: 무동작 (보고에만 포함)
- delete_mark: 경고로 보고 (실제 삭제는 사용자 결정)

### 5.2 Feature → Requirement Contains 링크 일괄 생성

FR / NFR / CNST 모든 Requirement 의 `relatedFeatures` 를 순회해 링크 JSON 배열을 구성 (빈 배열은 Step 2 에서 이미 critical 로 걸러짐 — 여기까지 오면 전부 1개 이상).

> **중복 제거 필수** — `POST /Issue/link` 은 idempotent 아님 (매 호출마다 새 linkID 로 `$push`). Step 0 에서 수집한 Feature/Requirement 의 `issueLinks[].{inwardIssue.key, outwardIssue.key, type.name}` 로 기존 튜플 집합 `EXISTING` 을 만들고, 새로 만들 집합 `DESIRED` 와 차집합을 계산:
> - `NEW = DESIRED - EXISTING` → `/tmp/req_links.json` 에 이것만 씀
> - `KEPT = DESIRED ∩ EXISTING` → 보고에만 노출, POST 안 함
> - `REMOVAL_CANDIDATES = EXISTING - DESIRED` (Requirement↔Feature 범위만) → **자동 삭제 금지**, 최종 보고에 ⚠ 목록으로만 표시 (유저가 검토 후 수동 처리)

batch 스크립트 **1회** 실행 (NEW 만 POST):
```bash
cat > /tmp/req_links.json <<'JSON'
[
  {"inwardKey":"GOJIRA3-21","outwardKey":"GOJIRA3-5","linkType":"Contains(WBSGantt)"},
  ...
]
JSON

bash "${PROJECT_ROOT}/.claude/skills/_scripts/issue-batch-link.sh" "${CODE}" "${USER}" < /tmp/req_links.json \
  > /tmp/req_link_result.json
cat /tmp/req_link_result.json
```

링크 실패는 **silent drop 금지** — 최종 보고에 누락 건수 명시.

### 5.3 docId → subkey 치환

생성 응답의 `subkey` 로 document 치환 (relink 모드는 docId 가 이미 subkey 형식이라 자연스럽게 no-op):
- FR-N → FR_NNN
- NFR-N → QA_NNN
- CNST-N → CNST_NNN

```bash
DOCUMENT=$(echo "$DOCUMENT" | sed "s/\bFR-1\b/FR_001/g" | sed "s/\bNFR-1\b/QA_001/g" | sed "s/\bCNST-1\b/CNST_001/g")
```

### 5.4 requirements.md 저장
```bash
echo "$DOCUMENT" | bash "${PROJECT_ROOT}/.claude/skills/_scripts/doc-save.sh" "${CODE}" "${USER}" "business/requirements.md"
```

---

## Step 6 — 최종 보고

```
## Requirement 정제 완료 (v2 Claude Code) — MODE={full|relink}

- FR 생성 N / NFR 생성 M / CNST 생성 K   (relink 은 모두 0)
- 수정 X / 재사용 Y / 삭제 마킹 Z (⚠ 검토 필요)
- Feature-Requirement 링크: 신규 NEW / 기존 유지 KEPT / ⚠ 제거 후보 REM (REM>0 이면 key 쌍 나열 + 수동 처리 안내)
  · FR L1 / NFR L2 / CNST L3 (모든 Requirement 는 ≥1 Feature 연결 보장)
- 문서 저장: business/requirements.md
- ⚠ 자동 결정 NFR Measure: W건 (검토/조정 필요)

### Feature 커버리지 매트릭스
| Feature 키 | 매핑 Requirement (FR/NFR/CNST) |
|---|---|
| GOJIRA3-5 | FR_001, FR_002, QA_001, CNST_001 |
| GOJIRA3-6 | QA_002, CNST_002 |
| GOJIRA3-7 | QA_002, CNST_002 |

### Self-loop
- iter N회 / selected=iter X (score_eff Y) / stop=pass|max_iter|repeat_critical|regression
- unresolvedCritical: [...]
```

---

## 주의
- 모든 curl 에 `X-User-Id: ${USER}` 헤더 필수 (403 방지)
- 링크 linkType 은 정확히 `Contains(WBSGantt)` (JIRA DC 표준). Feature 가 outward, Requirement 가 inward
- Platform-API `/file` 응답 content 는 base64 → `base64 -d`; 저장 시에는 plain UTF-8 (서버 측 인코딩)
- NFR Measure 가 자동 결정된 항목은 document 본문에 `[자동결정]` 표기 + 최종 보고에 별도 목록
- docId 규칙 엄수 — "(이슈 키)" 칸 빈칸 금지. docId 그대로 적고 Step 5.3 에서 치환
- User Story 어투: `~은(는) ~을 위해 ~할 수 있다` 준수
- Step 0, 1 종료 flush, 2 validate, 3 재-flush, 5 이슈/링크/문서 저장이 **모두 Bash tool call**. 에이전트가 "한 generation 에 몰아서" 처리하지 말 것 — 이게 GLM 스톨 방지의 핵심
- MODE 요약: `full` — 이슈 create + 링크 + 문서 재작성 / `relink` — 이슈 생성·수정 없음, 링크 차집합 POST + 문서 재작성. 두 모드 모두 `/Issue/link` 가 idempotent 아니므로 POST 전 기존 `issueLinks` 와 차집합 필수
