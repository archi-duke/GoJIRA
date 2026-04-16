---
name: vos-collect2
description: 프로젝트의 project-description.md를 분석해 이해관계자 요구사항(VOS)을 추출하고 VOS 이슈를 자동 생성합니다. Self-loop 자가검증(최대 3회) + best-so-far 채택. Claude Code 네이티브 실행 (curl/jq 기반).
allowed-tools: Bash, Read
---

# 🚨 필수 실행 규칙

1. 이 스킬은 **반드시 실제 이슈를 생성**해야 한다. 서술만 하고 끝내면 **실패**.
2. Step 1~4 는 에이전트 내부 추론. 그러나 **Step 5 는 반드시 bash 실행**.
3. Step 5 는 **절대 개별 `curl -X POST /Issue` 를 반복 호출하지 말 것** — Gitea git push race + subkey 충돌로 대량 실패한다. 반드시 `bash "${PROJECT_ROOT}/.claude/skills/_scripts/issue-batch-create.sh"` 를 **한 번만** 호출해 배치 처리한다.
4. 실행 순서: Step 0 (bash) → Step 1~4 (내부 추론) → **Step 5 (batch 스크립트 1회)** → Step 6 (최종 보고).
5. Step 5 실행 없이 "점수 0.X 성공" 서술만 출력하면 **거짓 응답** — 절대 금지.
6. 생성 후 GET /Issue 로 실제 count 를 재조회해 보고에 포함하라.

## 환경

- `PLATFORM_API` 기본값: `http://localhost:9000/api/v2`
- `GOJIRA_API` 기본값: `http://localhost:9001/api/v2`

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

## Step 0 — 데이터 수집

### project-description.md 읽기 (docs/{user} 우선, 없으면 main)

```bash
PLATFORM_API="${PLATFORM_API:-http://localhost:9000/api/v2}"
GOJIRA_API="${GOJIRA_API:-http://localhost:9001/api/v2}"

# docs/{user} 우선
DESC=$(curl -s "${PLATFORM_API}/DocRepo/${CODE}/file?path=business/project-description.md&ref=docs/${USER}" \
  | jq -r '.content // empty' | base64 -d 2>/dev/null)

# main fallback
if [ -z "$DESC" ] || [ "$(echo "$DESC" | wc -c)" -lt 50 ]; then
  DESC=$(curl -s "${PLATFORM_API}/DocRepo/${CODE}/file?path=business/project-description.md&ref=main" \
    | jq -r '.content // empty' | base64 -d 2>/dev/null)
fi

echo "$DESC" | head -100
```

문서가 비거나 템플릿 수준(# 제목 외 실질 내용 < 30자)이면 **즉시 중단** 하고 사용자에게 "projdesc_interview 를 먼저 실행하세요" 보고.

### 기존 VOS 조회

```bash
curl -s "${GOJIRA_API}/Issue" \
  -H "XBody: {\"prjID\":\"${CODE}\",\"infoType\":\"\"}" \
  -H "X-User-Id: ${USER}" \
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
```
{
  "summary": "...",
  "description": "...",
  "client": "...",
  "mode": "create",
  "existingKey": "", "modifyReason": "", "deleteReason": ""
}
```

**사용자 관점 어투** ("~가 필요해요 / ~해야 합니다 / ~하면 좋겠어요") 로 작성.

---

## Step 2 — 자가검증

### 점검 항목 (critical 기준)
- 필수 필드(summary, description, client, mode) 누락
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

### create (새 이슈 생성) — ⚠️ 반드시 batch 스크립트 사용

개별 `curl -X POST` 를 **절대 반복 호출하지 말 것** — Gitea git push race + subkey 충돌 발생.
대신 `finalItem` 중 `mode == "create"` 만 모아 JSON 배열로 한 파일에 쓰고, 아래 스크립트를 **한 번** 실행한다. 스크립트가 순차 POST + 재시도 + PUT(client/priority/assignee) 까지 처리한다.

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
```bash
curl -s -X PUT "${GOJIRA_API}/Issue" \
  -H "Content-Type: application/json" \
  -H "X-User-Id: ${USER}" \
  -d "$(jq -n --arg p "$CODE" --arg k "EXISTING_KEY" --arg u "$USER" \
    '{prjID:$p, key:$k, user:$u, data:{summary:"...", description:"...", client:"..."}}')"
```

### reuse / delete_mark
- reuse: 아무 동작 안 함 (정보만 최종 보고에 포함)
- delete_mark: 현재는 실제 삭제하지 않고 **경고로 보고** (사용자가 수동 결정)

---

## Step 6 — 최종 보고

```
## VOS 수집 완료 (v2 Claude Code)

- 신규 생성: N건 (키 리스트)
- 수정: M건
- 재사용: K건
- 삭제 마킹: L건 (⚠ 사용자 검토 필요)

### Self-loop 요약
- iter N회 수행
- selected: iter X (score_effective Y)
- stop reason: pass | max_iter | repeat_critical | regression
- unresolvedCritical: [...] (있는 경우)
```

---

## 주의

- **모든 curl 호출에 `X-User-Id: ${USER}` 헤더 필수** — GoJIRA-API 가 user 누락 시 403 반환
- Platform-API 의 `/DocRepo/.../file` 응답 content 는 **base64** → `| base64 -d` 필수
- `jq`, `base64`, `curl` 이 PATH 에 있어야 함
- 문서 인코딩 UTF-8
- 한 번의 Claude Code 세션 안에서 Steps 1~5 모두 수행 (컨텍스트 유지). Step 0 과 Step 5 만 실제 Bash 호출, Step 1~4 는 에이전트 내부 추론.
