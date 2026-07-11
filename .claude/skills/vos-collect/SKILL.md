---
name: vos-collect
description: 프로젝트의 project-description.md를 분석해 이해관계자 요구사항(VOS)을 추출하고 VOS 이슈를 자동 생성합니다. Self-loop 자가검증(최대 3회) + best-so-far 채택. Claude Code 네이티브 실행 (curl/jq 기반).
allowed-tools: Bash, Read
---

# 🚨 필수 실행 규칙

1. 이 스킬은 **반드시 실제 이슈를 생성**해야 한다. 서술만 하고 끝내면 **실패**.
2. Step 1~4 는 에이전트 내부 추론. 그러나 **Step 5 는 반드시 bash 실행**.
3. Step 5 는 **절대 개별 `curl -X POST /Issue` 를 반복 호출하지 말 것** — Gitea git push race + subkey 충돌로 대량 실패한다. 반드시 `bash "${PROJECT_ROOT}/.claude/skills/_scripts/issue-batch-create.sh"` 를 **한 번만** 호출해 배치 처리한다.
4. 실행 순서: Step 0 (bash) → Step 1~4 (내부 추론) → **Step 5 (batch 스크립트 1회)** → Step 6 (최종 보고).
5. Step 5 실행 없이 "점수 0.X 성공" 서술만 출력하면 **거짓 응답** — 절대 금지.
6. 생성 후 `issue-query.sh` (로컬 clone 조회 — GET /Issue 아님, Mongo 는 PR 머지 시점에만
   갱신되므로 방금 만든 이슈가 안 보임) 로 실제 count 를 재조회해 보고에 포함하라.

## 한 generation 당 제약 (GLM 계열 스톨 방지 — 필독)

- reasoning 은 각 Step 당 **3~5 줄 이내**. 장기 추론 불필요 — 이해관계자 관점 분해는 표 형식으로 간결하게
- **한 번의 LLM generation 에서 Step 두 개 이상 처리 금지**. 각 Step 종료는 **반드시 Bash tool call 로 마무리** 해 generation 경계 형성 (Step 1 완료 → Step 0 에서 수집한 기존 VOS 재조회 같은 가벼운 curl 을 붙여 경계 만들어도 무방)
- assistant 출력에는 items JSON / document 를 그대로 뱉지 말 것. "초안 N건 / critical K건" 식 한두 줄 요약만. 상세는 Step 5 batch JSON 파일로만 유지

## 환경

- `PLATFORM_API` 필수 (launcher 가 주입). 미설정 시 fail-fast
- `GOJIRA_API` 필수 (launcher 가 주입). 미설정 시 fail-fast

## 인자 추출

사용자 메시지에서 다음 값들을 추출해 **먼저** 환경변수로 export 한 뒤 아래 bash 블록들을 실행하라. 누락 시 사용자에게 되물어 확정하라.

- `CODE` — projectCode (예: GOJIRA3)
- `USER` — user (예: batman) — `docs/{user}` 브랜치로 커밋, `X-User-Id` 헤더로 전송

예:
```bash
export CODE="GOJIRA3"
export USER="batman"
```

## 역할

노련한 시니어 기획자. 문서에 드러나지 않은 숨겨진 이해관계자를 발굴하고 다양한 관점의 요구사항을 검토한다. 직접 언급된 이해관계자뿐 아니라 유사 과제 경험상 관련될 수 있는 간접 이해관계자(운영자, 보안 담당, 감사/규제, 파트너사, 최종 사용자 등) 식별. **원본 과제와 관련성이 명확한 것만** — 무관한 가공 금지.

---

## Step 0 — 진입 점검 (drift-check) + 데이터 수집

### 0.1 drift-check (이슈 변동 여부 확인 — 필수)

본 스킬의 책임 단계는 **`vos`** (workflow-graph.json). Step 0 진입 즉시 drift-check 를 호출해 **이슈 변동 여부 + 상위(`projdesc`) 변동 여부**를 확인하고, 그 결과로 진행 방식을 결정한다.

```bash
python3 "${PROJECT_ROOT}/.claude/skills/_scripts/drift-check.py" "${CODE}" "${USER}" check
```

### 0.2 결과에 따른 분기

| drift-check 결과 | 동작 |
|---|---|
| `vos` **NO-BASELINE** | 첫 실행 — Step 0.3 데이터 수집 후 본 작업 진행 |
| `vos` **DRIFT** (이슈 변동 감지) | 본 작업 진행 |
| `vos` **CLEAN** + 상위(`projdesc`) **DRIFT** | 사용자에게 통지: "project-description 이 변경됐습니다. VOS 재정립이 필요할 수 있어요. (a) 지금 진행 (b) `mark-deferred` 로 보류" — 답변 받고 진행/종료 결정 |
| **모두 CLEAN** | 사용자에게 질문: "이슈 변동이 없습니다. 강제로 다시 돌릴까요? (예: 템플릿 변경, 정책 변경 등)" — `no` 면 **즉시 종료**, `yes` 면 강제 재생성 모드로 진행 |

> **강제 재생성 모드 규칙**: drift CLEAN + 사용자 yes 케이스에서는 **이슈 생성/수정 도구 호출(`issue-batch-create.sh` / `PUT /Issue` 등) 을 모두 스킵** 하고 archSettings/문서 갱신만 수행. 본 스킬은 VOS 이슈 자체를 만드는 게 핵심이라 강제 재생성 시 수행할 작업이 거의 없음 — Step 5 통째로 스킵하고 Step 6 보고만 출력해도 무방. 사실상 종료에 가깝다.

진행 결정 후 Step 0.3 (데이터 수집) 으로 이동. 종료 결정이면 Step 6 보고만 출력하고 baseline 갱신 없이 끝낸다.

### 0.3 데이터 수집

#### project-description.md 읽기 (docs/{user} 우선, 없으면 main)

```bash
PLATFORM_API="${PLATFORM_API:?PLATFORM_API env var required (launcher must inject)}"
GOJIRA_API="${GOJIRA_API:?GOJIRA_API env var required (launcher must inject)}"

# docs/{user} 우선, fallback main (스크립트가 자동 처리)
DESC=$(bash "${PROJECT_ROOT}/.claude/skills/_scripts/doc-read.sh" "${CODE}" "${USER}" "business/project-description.md")

echo "$DESC" | head -100
```

문서가 비거나 템플릿 수준(# 제목 외 실질 내용 < 30자)이면 **즉시 중단** 하고 사용자에게 "projdesc_interview 를 먼저 실행하세요" 보고.

### 기존 VOS 조회

🚨 `curl ${GOJIRA_API}/Issue` 로 직접 조회하지 말 것 — 이슈 쓰기는 Mongo 를 건드리지 않고
로컬 clone 에만 기록되고 Mongo 는 PR 머지 시점에만 갱신되므로, 세션 중 방금 만든 VOS 가
API 응답에 안 보여 **중복 생성**으로 이어질 수 있다. 반드시 로컬 clone(`issue-query.sh`)에서 조회:

```bash
bash "${PROJECT_ROOT}/.claude/skills/_scripts/issue-query.sh" \
  | jq '[.[] | select(.issueType == "VOS") | {key, summary, description, client}]'
```

---

## Step 1 — 초안 생성 (iter 1)

project-description 내용과 기존 VOS 를 놓고 **새로 추가할 VOS** 항목을 도출한다.

### 다양성 가이드
- 입력 문장 하나를 그대로 옮기지 말 것. 여러 이해관계자 관점으로 분해·확장
- 각 이해관계자별 최소 1개 이상
- 관점 예시 (과제 해당되는 것만):
  - 최종 사용자: 기능 편의, 학습성, 접근성, 개인정보
  - 관리자/운영자: 관리 UI, 모니터링, 장애 대응
  - 보안 담당: 인증/인가, 암호화, 감사 로그
  - 개발자: API 명세, 테스트, 로컬 개발환경
  - 기획/비즈니스: 지표/KPI, 보고서
  - 감사/규제: 규정 준수, 개인정보, 추적성
  - 파트너/외부 시스템: 연동 API, SLA
- 품질 속성(성능/안정성/보안/유지보수/사용성)도 VOS 로 표현
- 암묵적 요구를 명시적으로 드러내기

### 수량
일반적으로 **8~20건**. 3건 미만이면 검토 부족, 30건 초과면 과도.

### 중복 처리 (mode 판정)
각 VOS 에 `mode` 지정:
- `create`: 기존과 의미적으로 다른 새 VOS
- `reuse`: 기존과 동일 (existingKey 필수)
- `modify`: 기존 보강 필요 (existingKey + modifyReason)
- `delete_mark`: 기존 폐기 (existingKey + deleteReason)

### 항목 형식 (내부 구조)

⚠️ 본문 필드명은 **`desc`** — `description` 아님. 이 초안 항목이 Step 5 배치 입력으로 **필드명 변경 없이 그대로** 들어간다 (조회 응답의 `description` 은 저장 필드명일 뿐, 생성 입력 규약은 `desc`).

```
{
  "summary": "...",
  "desc": "...",
  "client": "...",
  "mode": "create",
  "existingKey": "", "modifyReason": "", "deleteReason": ""
}
```

**사용자 관점 어투** ("~가 필요해요 / ~해야 합니다 / ~하면 좋겠어요") 로 작성.

---

## Step 2 — 자가검증

### 점검 항목 (critical 기준)
- 필수 필드(summary, desc, client, mode) 누락 — 본문 필드가 `description` 으로 되어 있으면 그 자체가 critical (규약은 `desc`)
- `mode=reuse/modify/delete_mark` 인데 `existingKey` 가 기존 VOS 에 없음
- 동일 의미 VOS 중복 (summary 유사도 과도)
- 이해관계자 관점 편향 (사용자만 있고 운영/보안 등 누락)
- 총 항목 수 부적절 (3건 미만 또는 30건 초과)
- 품질 속성 관점 누락 (성능/보안/안정성 어느 하나도 없음)
- "~다" 체 / 기술 명세 투 (사용자 관점 어투 아님)

### 점수 계산
- `score`: 0.0~1.0 (검증 기준 충족도 종합)
- `score_effective = score - 0.5 × |critical|`
- `pass`: `score_effective >= 0.8`

---

## Step 3 — 수정 (pass=false 시)

critical + suggestions 항목만 **국소 수정**. 나머지는 원문 유지. 수정 후 다시 Step 2.

---

## Step 4 — 종료 조건

아래 중 하나라도 충족하면 중단:
1. `pass=true`
2. iter 3 완료
3. 같은 critical 항목이 **2회 연속** 재발
4. 새 수정본이 이전 best 보다 `score_effective` 낮음 → 포기

### best-so-far
매 iter 마다 `(items, score_effective)` 를 기록. 종료 시 `score_effective` 최댓값의 items 를 **최종 채택** (마지막 iter 가 아닐 수 있음).

---

## Step 5 — 이슈 생성 (최종 items 반영)

**강제 재생성 모드(Step 0.2 의 drift CLEAN + 사용자 yes) 이면 이 섹션 전체 스킵** — 이슈 생성/수정/삭제 모두 하지 않는다. 바로 Step 6 으로.

### create (새 이슈 생성) — ⚠️ 반드시 batch 스크립트 사용

개별 `curl -X POST` 를 **절대 반복 호출하지 말 것** — Gitea git push race + subkey 충돌 발생.
대신 `finalItem` 중 `mode == "create"` 만 모아 JSON 배열로 한 파일에 쓰고, 아래 스크립트를 **한 번** 실행한다. 스크립트가 순차 POST + 재시도 + PUT(client/priority/assignee) 까지 처리한다.

⚠️ 배치 파일의 필드명은 초안 그대로 — 특히 본문은 **`desc`** (`description` 으로 바꿔 쓰면 검증 경고/실패). `mode`/`existingKey` 등 여분 필드는 스크립트가 무시하므로 지우지 않아도 된다.

```bash
# 1) 생성할 items 를 JSON 배열로 작성
cat > /tmp/vos_items.json <<'JSON'
[
  {"issueType":"VOS","summary":"...","desc":"...","client":"alice.kim"},
  {"issueType":"VOS","summary":"...","desc":"...","client":"bob.park"}
]
JSON

# 2) 배치 스크립트 실행 (한 번만)
bash "${PROJECT_ROOT}/.claude/skills/_scripts/issue-batch-create.sh" "${CODE}" "${USER}" < /tmp/vos_items.json \
  > /tmp/vos_result.json
cat /tmp/vos_result.json
```

결과 JSON 에서 `keys` 배열 · `created` · `failed` · `errors` 를 최종 보고에 반영.

### modify (기존 이슈 업데이트)

⚠️ 비대칭 주의: **생성 입력은 `desc`, 수정(PUT data)은 저장 필드명 `description`** — API wire 규약이 다르다. 아래 블록 그대로 사용.

```bash
SCRIPTS="${GOJIRA_REPO_ROOT:-$(git rev-parse --show-toplevel)}/.claude/skills/_scripts"
jq -n --arg p "$CODE" --arg k "EXISTING_KEY" --arg u "$USER" \
    '{prjID:$p, key:$k, user:$u, data:{summary:"...", description:"...", client:"..."}}' \
  | bash "${SCRIPTS}/issue-write.sh" PUT /Issue
```

### reuse / delete_mark
- reuse: 아무 동작 안 함 (정보만 최종 보고에 포함)
- delete_mark: 현재는 실제 삭제하지 않고 **경고로 보고** (사용자가 수동 결정)

---

## Step 6 — 최종 보고 + baseline 갱신

### 6.1 보고 출력

```
## VOS 수집 완료 (v2 Claude Code) — MODE={full|forced-doc-regen}

- 진입 drift: vos=<NO-BASELINE|CLEAN|DRIFT>, projdesc=<...> (Step 0.1 결과)
- 신규 생성: N건 (키 리스트)   (forced-doc-regen 은 0)
- 수정: M건
- 재사용: K건
- 삭제 마킹: L건 (⚠ 사용자 검토 필요)

### Self-loop 요약
- iter N회 수행
- selected: iter X (score_effective Y)
- stop reason: pass | max_iter | repeat_critical | regression
- unresolvedCritical: [...] (있는 경우)
```

### 6.2 baseline 갱신 (필수 — 본 작업이 실제로 진행된 경우)

본 작업이 진행됐다면(NO-BASELINE 첫 실행 / DRIFT 처리 / 강제 재생성) **반드시** `vos` stage 의 baseline 을 새 hash 로 갱신한다. Step 0.2 에서 "종료" 결정으로 끝난 경우는 호출하지 않는다.

```bash
python3 "${PROJECT_ROOT}/.claude/skills/_scripts/drift-check.py" "${CODE}" "${USER}" set-baseline --stage vos
```

이 명령은 archSettings.lastRunHash[vos] / lastRunAt[vos] 만 갱신 — 다른 stage 의 baseline 은 건드리지 않는다.

---

## 주의

- **모든 curl 호출에 `X-User-Id: ${USER}` 헤더 필수** — GoJIRA-API 가 user 누락 시 403 반환
- Platform-API 의 `/DocRepo/.../file` 응답 content 는 **base64** → `| base64 -d` 필수
- `jq`, `base64`, `curl` 이 PATH 에 있어야 함
- 문서 인코딩 UTF-8
- 한 번의 Claude Code 세션 안에서 Steps 1~5 모두 수행 (컨텍스트 유지). Step 0 과 Step 5 만 실제 Bash 호출, Step 1~4 는 에이전트 내부 추론.
- 진입 정책: Step 0.1 의 drift-check 가 작업 진행 여부를 정한다. 모두 CLEAN 인데 사용자가 강제 재생성을 거부하면 즉시 종료 (LLM 호출 0회). 강제 재생성 yes 인 경우 이슈 생성/수정 step 들은 자동 스킵하고 문서/링크만 재생성
- baseline 갱신: 본 작업 진행 시 Step 6.2 의 `set-baseline --stage vos` 호출이 필수. 안 하면 다음 호출에서 또 DRIFT 로 잡혀 재실행 압력이 생긴다
