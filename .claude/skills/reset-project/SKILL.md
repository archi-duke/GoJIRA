---
name: reset-project
description: "[개발/테스트 전용] 프로젝트의 이슈(Mongo+Gitea YAML) / archSettings / business·architecture 문서를 초기 상태로 리셋합니다. 파괴적 작업이므로 CONFIRM=true 없이는 dry-run(영향 범위)만 반환합니다. Skill tool 로도 호출 가능하지만 CONFIRM=true 는 반드시 사용자 명시 동의 후에만 설정."
allowed-tools: Bash
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
- `RESET_ARCH_SETTINGS` — (선택) `true`/`false`, 기본 `true`
- `RESET_DOCS` — (선택) `true`/`false`, 기본 `true`

예 (dry-run):
```bash
export CODE="GOJIRA3"
export USER="batman"
# CONFIRM 은 사용자 명시 동의 후에만 설정
```

실제 실행 (사용자 동의 후):
```bash
export CONFIRM="true"
```

## 경고

이 스킬은 **파괴적(destructive)** 입니다.
- 프로젝트의 모든 이슈를 삭제하고(Gitea YAML + Mongo 양쪽), archSettings 를 초기화하고, 문서를 초기 템플릿으로 되돌립니다
- **반드시** 사용자에게 명시적 동의를 얻은 뒤에만 `CONFIRM=true` 로 실행
- 기본 동작은 dry-run. dry-run 결과를 보여주고, 사용자가 "확인/진행/yes" 등 명시 동의 후에만 두 번째 호출에서 `CONFIRM=true`
- 서버측 일괄 삭제 엔드포인트(`POST /Project/{code}/reset`)를 사용하므로 클라이언트 bash 루프 없음 — sandbox 의 bulk-destructive 감지에 덜 민감

## 1. 사전 체크

```bash
PLATFORM_API="${PLATFORM_API:-http://localhost:9000/api/v2}"
GOJIRA_API="${GOJIRA_API:-http://localhost:9001/api/v2}"
CONFIRM="${CONFIRM:-false}"
RESET_ISSUES="${RESET_ISSUES:-true}"
RESET_ARCH_SETTINGS="${RESET_ARCH_SETTINGS:-true}"
RESET_DOCS="${RESET_DOCS:-true}"

if [ -z "$CODE" ] || [ -z "$USER" ]; then
  echo "ERROR: projectCode 와 user 모두 필수"
  exit 1
fi
```

## 2. 영향 범위 산정 — `POST /Project/{code}/reset` dry-run

서버측 엔드포인트가 영향 범위(이슈 키 목록, archSettings 존재 여부) 를 반환한다.

```bash
curl -s -X POST "${GOJIRA_API}/Project/${CODE}/reset" \
  -H "Content-Type: application/json" -H "X-User-Id: ${USER}" \
  -d "$(jq -n --argjson ri "$RESET_ISSUES" --argjson ra "$RESET_ARCH_SETTINGS" \
    '{confirm:false, resetIssues:$ri, resetArchSettings:$ra}')" \
  | jq
```

응답 예:
```json
{
  "jiraCode": "TPRJ",
  "mode": "dry-run",
  "wouldDelete": {
    "issueCount": 67,
    "issueKeys": ["TPRJ-1", "TPRJ-2", ...],
    "resetArchSettings": true,
    "archSettingsPresent": true
  },
  "message": "DRY-RUN: 67이슈 삭제 예정. 실제 실행은 confirm=true 로 재호출."
}
```

`RESET_DOCS=true` 라면 아래 8 파일이 덮어써짐(보고에만 포함, 서버측 처리 아님 — Step 4-2 에서 스킬이 직접 PUT):
- `business/project-description.md`
- `business/use-cases.md`
- `business/requirements.md`
- `architecture/architecture.md`
- `architecture/drivers.md`
- `architecture/architecture-style.md`
- `architecture/design-decisions.md`
- `architecture/architecture-integrated.md`

## 3. Dry-run 최종 보고 (confirm != true)

`CONFIRM` 이 `true` 가 아니면 여기서 종료. 사용자에게 Step 2 응답 + 문서 목록을 정리해 보고하고 "`CONFIRM=true` 로 재호출 필요" 안내.

## 4. 실제 실행 (confirm=true)

### 4-1. 이슈 + archSettings 일괄 리셋 (서버측 1회 호출)

```bash
RESET_RESULT=$(curl -s -X POST "${GOJIRA_API}/Project/${CODE}/reset" \
  -H "Content-Type: application/json" -H "X-User-Id: ${USER}" \
  -d "$(jq -n --argjson ri "$RESET_ISSUES" --argjson ra "$RESET_ARCH_SETTINGS" \
    '{confirm:true, resetIssues:$ri, resetArchSettings:$ra}')")
echo "$RESET_RESULT" | jq '{mode, deletedCount, failedCount, archSettingsCleared}'
```

응답에 `failedCount > 0` 이면 최종 보고에 `failed[]` 내용을 포함. 그래도 Step 4-2 진행(부분 성공이어도 문서 초기화는 시도).

### 4-2. 문서 초기화 (사용자 브랜치 docs/{USER})

`.claude/skills/reset-project/templates/` 하위 8개 파일을 Platform-API `PUT /DocRepo/{code}/file` 로 각각 덮어쓴다. 템플릿은 Agent-Service `_TEMPLATES` 와 동일한 내용이다.

```bash
put_template() {
  local REL_PATH="$1"
  local CONTENT
  CONTENT=$(cat "${PROJECT_ROOT}/.claude/skills/reset-project/templates/${REL_PATH}")
  curl -s -X PUT "${PLATFORM_API}/DocRepo/${CODE}/file" \
    -H "Content-Type: application/json" -H "X-User-Id: ${USER}" \
    -d "$(jq -n --arg p "$REL_PATH" --arg c "$CONTENT" --arg m "[reset-project] ${REL_PATH} 초기화" \
      '{path:$p, content:$c, message:$m}')" \
    -o /dev/null -w "PUT ${REL_PATH} -> %{http_code}\n"
}

if [ "$CONFIRM" = "true" ] && [ "$RESET_DOCS" = "true" ]; then
  for F in \
    business/project-description.md \
    business/use-cases.md \
    business/requirements.md \
    architecture/architecture.md \
    architecture/drivers.md \
    architecture/architecture-style.md \
    architecture/design-decisions.md \
    architecture/architecture-integrated.md; do
    put_template "$F"
  done

  # main 브랜치 누락 파일 보충
  curl -s -X POST "${PLATFORM_API}/DocRepo/${CODE}/init-files" \
    -H "X-User-Id: ${USER}" \
    -o /dev/null -w "INIT-FILES -> %{http_code}\n"
fi
```

## 5. 최종 보고

- **dry-run**: 이슈 수 / archSettings 존재 여부 / 덮어쓸 파일 목록 + `CONFIRM=true` 재호출 안내
- **executed**: 삭제된 이슈 수/실패 수 + archSettingsCleared + 문서 PUT 결과(HTTP 코드) + init-files 응답

모든 보고는 한국어 마크다운. 실패(4xx/5xx) 는 반드시 보고에 명시.

## 주의

- `CONFIRM=true` 는 반드시 사용자 명시 동의 후에만 설정
- 서버측 `POST /Project/{code}/reset` 이 없는 옛 버전 GoJIRA-API 에서는 404 반환 → 최신 빌드로 업그레이드 필요
- 템플릿 8개는 `.claude/skills/reset-project/templates/` 하위에 있으며 Agent-Service `reset_project._TEMPLATES` 와 동일하게 유지
- 문서 PUT 응답 content 는 Platform-API 가 서버측에서 base64 처리 — 호출은 plain UTF-8
