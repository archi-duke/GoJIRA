---
name: requirement-refine2
description: Feature 와 use-cases.md 로부터 FR(User Story)/NFR(QAS)/CNST(제약) Requirement 를 도출하여 이슈를 생성하고 Feature contains Requirement 링크를 연결하며 business/requirements.md 를 작성합니다. Self-loop 자가검증(최대 3회) + Feature 커버리지 매트릭스. Claude Code 네이티브(curl/jq).
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
소프트웨어 아키텍트. Feature 와 Use Case 시나리오에서 **User Story(FR) / QAS(NFR) / 제약(CNST)** 을 도출. 모든 Feature 가 최소 1 개 FR 로 커버되도록 책임진다. User Story 형식과 QAS 6 요소 엄수.

---

## Step 0 — 데이터 수집

```bash
PLATFORM_API="${PLATFORM_API:-http://localhost:9000/api/v2}"
GOJIRA_API="${GOJIRA_API:-http://localhost:9001/api/v2}"

# project-description.md (main 우선, 비면 docs/{user})
DESC=$(curl -s "${PLATFORM_API}/DocRepo/${CODE}/file?path=business/project-description.md&ref=main" | jq -r '.content // empty' | base64 -d 2>/dev/null)
if [ -z "$DESC" ] || [ $(echo -n "$DESC" | wc -c) -lt 50 ]; then
  DESC=$(curl -s "${PLATFORM_API}/DocRepo/${CODE}/file?path=business/project-description.md&ref=docs/${USER}" | jq -r '.content // empty' | base64 -d 2>/dev/null)
fi

# use-cases.md
UC=$(curl -s "${PLATFORM_API}/DocRepo/${CODE}/file?path=business/use-cases.md&ref=main" | jq -r '.content // empty' | base64 -d 2>/dev/null)
if [ -z "$UC" ] || [ $(echo -n "$UC" | wc -c) -lt 50 ]; then
  UC=$(curl -s "${PLATFORM_API}/DocRepo/${CODE}/file?path=business/use-cases.md&ref=docs/${USER}" | jq -r '.content // empty' | base64 -d 2>/dev/null)
fi

echo "=== project-description.md ==="; echo "$DESC"
echo; echo "=== use-cases.md ==="; echo "$UC"

# Feature
echo; echo "=== Feature ==="
curl -s "${GOJIRA_API}/Issue" -H "XBody: {\"prjID\":\"${CODE}\",\"infoType\":\"\"}" -H "X-User-Id: ${USER}" \
  | jq '[.[] | select(.issueType=="Feature") | {key,subkey,summary,description,actor,precondition,basicflow,alternativeflow,exceptionflow,postcondition}]'

# VOS (참조용)
echo; echo "=== VOS ==="
curl -s "${GOJIRA_API}/Issue" -H "XBody: {\"prjID\":\"${CODE}\",\"infoType\":\"\"}" -H "X-User-Id: ${USER}" \
  | jq '[.[] | select(.issueType=="VOS") | {key,summary}]'

# 기존 Requirement
echo; echo "=== 기존 Requirement ==="
curl -s "${GOJIRA_API}/Issue" -H "XBody: {\"prjID\":\"${CODE}\",\"infoType\":\"\"}" -H "X-User-Id: ${USER}" \
  | jq '[.[] | select(.issueType=="Requirement") | {key,subkey,summary,reqtype,qa,measure}]'
```

- project-description.md 가 템플릿 수준(실질 < 30자) → **중단** 하고 `projdesc_interview` 먼저 권고
- Feature 0 건 → **중단** 하고 `usecase-analyze2` 먼저 권고

---

## Step 1 — 초안 생성 (iter 1)

각 Feature / Use Case 에서 FR / NFR / CNST 를 **다양한 관점에서 충분히** 도출.

### 다양성 가이드
- 각 Feature 는 보통 **2~5 개의 FR** 로 세분화. 단일 FR 은 세분화 부족 신호
- FR 관점: 기본 동작, 입력 검증, 조회/검색, CRUD, 권한/접근제어, 알림/피드백, 오류 처리
- NFR 은 품질 속성(QA)별로 다양: Performance / Security / Availability / Usability / Maintainability / Scalability
- CNST 는 과제 설명 / use-case 에서 **명확히 드러난 제약만** (기술 스택, 규정, 플랫폼, 표준). 추측성 금지

### FR 규칙
- User Story 형식 필수: `"{역할}은(는) {목적}을 위해 {기능}을 할 수 있다"`
- 한 Feature 의 여러 사용자 행위(생성/조회/수정/삭제/검색 등)는 개별 FR 로 분리

### NFR 규칙 (QAS)
- 필드 필수: `qa, source, stimulus, environment, artifact, response, measure, responseScenario`
- measure 원본에 없으면 합리적 수준으로 결정 후 `measureAuto: true` + document 비고에 `[자동결정]`

### CNST 규칙
- 반드시 지켜야 하는 사양/규제/표준만

### 수량 가이드
FR 10~30, NFR 3~10, CNST 0~5 수준.

### 중복 처리 (mode)
- `create` / `reuse` (existingKey) / `modify` (existingKey + modifyReason) / `delete_mark` (existingKey + deleteReason)

### docId
- FR → `FR-1, FR-2, ...` / NFR → `NFR-1, NFR-2, ...` / CNST → `CNST-1, CNST-2, ...`
- document 본문의 모든 Requirement 표기(표의 ID / (이슈 키) 열 / 헤딩 / 관련 Feature)는 **동일 docId 그대로**. Step 5.3 에서 실제 subkey(FR_NNN / QA_NNN / CNST_NNN) 로 치환
- `relatedFeatures` 에는 **실제 Feature 이슈 키**(`{code}-N`) 사용 (docId 아님)

### 항목 구조
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

NFR 은 위에 + `qa/source/stimulus/environment/artifact/response/measure/responseScenario/measureAuto`.

---

## Step 2 — 자가검증 (커버리지 + 형식)

### Feature 커버리지 매트릭스 (필수)
Feature 목록의 **모든 키**가 최소 1 개 FR 의 `relatedFeatures` 에 포함되는지 점검.
- 미커버 Feature 가 있으면 **FR 추가** (Step 1 반복) 또는 `coverageMatrix[feat_key] = ["(CNST/NFR 전역)"]` 으로 전역 품질/제약에 의한 간접 커버 표시
- 억지 매핑 금지

### critical 기준
- 필수 필드 누락: FR(summary, description, docId, relatedFeatures, mode), NFR(+ QAS 6 요소), CNST(summary, description, docId, mode)
- FR description 이 User Story 형식 아님
- NFR 에 QAS 필드 누락
- `relatedFeatures` 가 Feature 목록에 없는 키 참조 (hallucination)
- `mode=reuse/modify/delete_mark` 인데 `existingKey` 가 기존 Requirement 에 없음
- Feature 1 건이라도 FR 미매핑 (전역 표기 시 면제)
- Feature 당 평균 FR < 1.5 (세분화 부족)
- QA 품질 속성 다양성 부족 (Performance/Security/Availability 중 2 개 이상 누락인데 NFR 은 존재)
- CNST 에 추측성 제약 (원문 근거 없음)
- docId 중복/누락 또는 reqType 접두 불일치

### 점수
- `score`: 0.0~1.0
- `score_effective = score - 0.5 × |critical|`
- `pass`: `score_effective >= 0.8`

---

## Step 3 — 수정 (pass=false)

critical + suggestions 만 **국소 수정**. 나머지 원문 유지. 수정 후 Step 2 재진입.

---

## Step 4 — 종료 조건

1. pass=true
2. iter 3 완료
3. 같은 critical 2회 연속 재발
4. 새 수정이 이전 best 보다 score_effective 낮음 → 포기

**best-so-far**: 매 iter 의 `(requirements, document, coverageMatrix, score_effective)` 기록. 종료 시 최댓값 iter 를 selected.

---

## Step 5 — 이슈 생성 + 링크 + 문서 저장

### 5.1 Requirement 이슈 생성 (create)

FR / NFR / CNST 공통:
```bash
RESP=$(curl -s -X POST "${GOJIRA_API}/Issue" \
  -H "Content-Type: application/json" -H "X-User-Id: ${USER}" \
  -d @- <<EOF
{
  "prjID": "${CODE}",
  "issueType": "Requirement",
  "summary": "요구사항 제목",
  "desc": "요구사항 본문",
  "reqType": "FR",
  "user": "${USER}"
}
EOF
)
KEY=$(echo "$RESP" | jq -r '.key')
SUBKEY=$(echo "$RESP" | jq -r '.subkey')
```

- NFR 은 추가로 PUT /Issue 로 QAS 필드 업데이트:
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

### 5.2 Feature → Requirement Contains 링크

각 Requirement 의 `relatedFeatures` 순회 → Feature(outward) contains Requirement(inward):
```bash
curl -s -X POST "${GOJIRA_API}/Issue/link" \
  -H "Content-Type: application/json" -H "X-User-Id: ${USER}" \
  -d "$(jq -n --arg p "$CODE" --arg iw "$REQ_KEY" --arg ow "$FEAT_KEY" --arg lt "Contains(WBSGantt)" --arg u "$USER" \
    '{prjID:$p, inwardKey:$iw, outwardKey:$ow, linkType:$lt, user:$u}')"
```

링크 실패는 **silent drop 금지** — 최종 보고에 누락 건수 명시.

### 5.3 docId → subkey 치환

생성 응답의 `subkey` 로 document 치환:
- FR-N → FR_NNN
- NFR-N → QA_NNN
- CNST-N → CNST_NNN

```bash
DOCUMENT=$(echo "$DOCUMENT" | sed "s/\bFR-1\b/FR_001/g" | sed "s/\bNFR-1\b/QA_001/g" | sed "s/\bCNST-1\b/CNST_001/g")
```

### 5.4 requirements.md 저장
```bash
curl -s -X PUT "${PLATFORM_API}/DocRepo/${CODE}/file" \
  -H "Content-Type: application/json" -H "X-User-Id: ${USER}" \
  -d "$(jq -n --arg c "$DOCUMENT" '{path:"business/requirements.md", content:$c, message:"Requirement 정제 (requirement-refine2)"}')"
```

---

## Step 6 — 최종 보고

```
## Requirement 정제 완료 (v2 Claude Code)

- FR 생성 N / NFR 생성 M / CNST 생성 K
- 수정 X / 재사용 Y / 삭제 마킹 Z (⚠ 검토 필요)
- Feature-Requirement 링크: L건
- 문서 저장: business/requirements.md
- ⚠ 자동 결정 NFR Measure: W건 (검토/조정 필요)

### Feature 커버리지 매트릭스
| Feature 키 | 매핑 Requirement | 비고 |
|---|---|---|
| GOJIRA3-5 | FR_001, FR_002 | |
| GOJIRA3-6 | (CNST 전역) | 전역 제약으로 간접 커버 |

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
- Step 0 과 Step 5 만 실제 Bash 호출, Step 1~4 는 에이전트 내부 추론 (단일 Claude Code 세션)
