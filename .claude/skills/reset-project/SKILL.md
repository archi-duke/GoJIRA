---
name: reset-project
description: "[개발/테스트 전용] 프로젝트의 모든 이슈(DB+JIRA)와 business/architecture 문서를 초기 템플릿 상태로 리셋합니다. 파괴적 작업이므로 CONFIRM=true 없이는 dry-run(영향 범위)만 반환합니다."
allowed-tools: Bash
disable-model-invocation: true
---

## 환경

- `PLATFORM_API` 기본값: `http://localhost:9000/api/v2`
- `GOJIRA_API` 기본값: `http://localhost:9001/api/v2`

## 인자 추출

사용자 메시지에서 다음 값들을 추출해 **먼저** 환경변수로 export 한 뒤 아래 bash 블록들을 실행하라. 누락 시 사용자에게 되물어 확정하라.

- `CODE` — projectCode (필수)
- `USER` — user (필수, `X-User-Id` 및 `docs/{user}` 브랜치)
- `CONFIRM` — (선택) `true` 이면 실제 실행, 그 외 dry-run. **절대 기본 true 로 두지 말 것.** 미지정/미확인 시 dry-run 유지
- `RESET_ISSUES` — (선택) `true`/`false`, 기본 `true`
- `RESET_DOCS` — (선택) `true`/`false`, 기본 `true`

예 (dry-run):
```bash
export CODE="GOJIRA3"
export USER="batman"
# CONFIRM 은 사용자 명시 동의 후에만 설정
```

실제 실행 시 (사용자 동의 후):
```bash
export CONFIRM="true"
```

## 경고

이 스킬은 **파괴적(destructive)** 입니다.
- 프로젝트의 모든 이슈를 삭제하고 문서를 초기 템플릿으로 되돌립니다.
- **반드시** 사용자에게 명시적 동의를 얻은 뒤에만 `confirm=true`로 실행하세요.
- 기본 동작은 dry-run 입니다. dry-run 결과를 보여주고, 사용자가 "확인/진행/yes" 등 명시 동의 후에만 두 번째 호출에서 `confirm=true` 사용.
- 자동 호출(모델이 선별 실행) 금지 — `disable-model-invocation: true`.

## 1. 사전 체크

```bash
PLATFORM_API="${PLATFORM_API:-http://localhost:9000/api/v2}"
GOJIRA_API="${GOJIRA_API:-http://localhost:9001/api/v2}"
CONFIRM="${CONFIRM:-false}"
RESET_ISSUES="${RESET_ISSUES:-true}"
RESET_DOCS="${RESET_DOCS:-true}"

if [ -z "$CODE" ] || [ -z "$USER" ]; then
  echo "ERROR: projectCode 와 user 모두 필수"
  exit 1
fi
```

## 2. 영향 범위 산정 (항상 수행)

### 이슈 목록

```bash
if [ "$RESET_ISSUES" = "true" ]; then
  ISSUE_KEYS=$(curl -s "${GOJIRA_API}/Issue" \
    -H "XBody: {\"prjID\":\"${CODE}\",\"infoType\":\"\"}" \
    -H "X-User-Id: ${USER}" \
    | jq -r '[.[] | .key] | .[]' 2>/dev/null)
  ISSUE_COUNT=$(echo "$ISSUE_KEYS" | grep -c . || echo 0)
else
  ISSUE_KEYS=""
  ISSUE_COUNT=0
fi
echo "영향 이슈: ${ISSUE_COUNT}건"
```

### 대상 문서 (resetDocs 시)

고정 템플릿 경로 (Agent-Service `reset_project` 와 동일):
- `business/project-description.md`
- `business/use-cases.md`
- `business/requirements.md`
- `architecture/architecture.md`
- `architecture/drivers.md`
- `architecture/architecture-style.md`
- `architecture/design-decisions.md`
- `architecture/architecture-integrated.md`

## 3. Dry-run (confirm != true)

`CONFIRM` 이 `true` 가 아니면 아래 요약만 출력하고 **종료**. 실제 삭제/쓰기 금지.

```
DRY-RUN
- projectCode: $CODE
- 영향 이슈: $ISSUE_COUNT 건 (최대 50개 미리보기)
- 사용자 브랜치(docs/$USER) 덮어쓰기 예정 파일: 위 8개
- main 브랜치: init-files 로 누락 파일만 생성
실제 실행: confirm=true 로 다시 호출 (사용자 명시 동의 필요)
```

## 4. 실제 실행 (confirm=true)

### 4-1. 이슈 삭제

```bash
if [ "$CONFIRM" = "true" ] && [ "$RESET_ISSUES" = "true" ]; then
  echo "$ISSUE_KEYS" | while read -r KEY; do
    [ -z "$KEY" ] && continue
    curl -s -X DELETE "${GOJIRA_API}/Issue" \
      -H "Content-Type: application/json" \
      -H "X-User-Id: ${USER}" \
      -d "{\"key\":\"${KEY}\",\"prjID\":\"${CODE}\"}" \
      -o /dev/null -w "DEL ${KEY} -> %{http_code}\n"
  done
fi
```

### 4-2. 문서 초기화

사용자 브랜치 각 파일을 초기 템플릿 내용으로 `PUT /DocRepo/{code}/file` 덮어쓰기.

템플릿 예시 (project-description.md):

```
# Project Description

과제의 배경, 목적, 주요 사항을 기록하는 문서입니다.

## 과제 배경

(작성 필요)

## 과제 목적

(작성 필요)

## 주요 사항

(작성 필요)
```

덮어쓰기 호출:

```bash
put_file() {
  local PATH_="$1"
  local CONTENT="$2"
  local B64=$(printf '%s' "$CONTENT" | base64)
  curl -s -X PUT "${PLATFORM_API}/DocRepo/${CODE}/file" \
    -H "Content-Type: application/json" \
    -H "X-User-Id: ${USER}" \
    -d "$(jq -n --arg p "$PATH_" --arg c "$CONTENT" --arg m "[reset-project] $PATH_ 초기화" \
          '{path:$p, content:$c, message:$m}')" \
    -o /dev/null -w "PUT ${PATH_} -> %{http_code}\n"
}

if [ "$CONFIRM" = "true" ] && [ "$RESET_DOCS" = "true" ]; then
  put_file "business/project-description.md" "$(cat <<'EOF'
# Project Description

과제의 배경, 목적, 주요 사항을 기록하는 문서입니다.

## 과제 배경

(작성 필요)

## 과제 목적

(작성 필요)

## 주요 사항

(작성 필요)
EOF
)"
  # use-cases.md, requirements.md, architecture/*.md 동일 패턴으로 반복
  # (Agent-Service reset_project/executor.py 의 _TEMPLATES 참고)

  # main 브랜치 누락 파일 보충
  curl -s -X POST "${PLATFORM_API}/DocRepo/${CODE}/init-files" \
    -H "X-User-Id: ${USER}" \
    -o /dev/null -w "INIT-FILES -> %{http_code}\n"
fi
```

## 5. 보고

- dry-run: 영향 이슈 수 + 파일 목록 + "confirm=true 재호출 안내".
- executed: 삭제된 이슈 수/실패 수, 덮어쓴 파일 목록, init-files 응답코드 요약.
- 모든 보고는 한국어 마크다운.
