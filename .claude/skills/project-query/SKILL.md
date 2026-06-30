---
name: project-query
description: GoJIRA 프로젝트 목록을 조회합니다.
allowed-tools: Bash
---

GoJIRA-API `/Project` 에서 프로젝트 목록을 조회합니다.

## 환경
- `GOJIRA_API` 필수 (launcher 가 주입). 미설정 시 fail-fast

## 인자
- `USER` — X-User-Id (필수)
- `CODE` — (선택) 특정 프로젝트만 조회

```bash
export USER="batman"
export CODE=""
export GOJIRA_API="${GOJIRA_API:?GOJIRA_API env var required (launcher must inject)}"
```

## 조회
🚨 `RESP=$(curl ...)` 캡처 금지 — bash 가 응답 바이트를 손상시킴. **파일 저장** 후 jq 사용.

```bash
curl -s "${GOJIRA_API}/Project" -H "X-User-Id: ${USER}" -H "Authorization: Bearer ${GOJIRA_SESSION_TOKEN}" > /tmp/gojira_projects.json
if [ -n "$CODE" ]; then
  jq --arg c "$CODE" '[.[] | select(.jiraCodes.main.jiraCode==$c) | {code:.jiraCodes.main.jiraCode, name, status}] | .[0]' /tmp/gojira_projects.json
else
  jq '[.[] | {code:(.jiraCodes.main.jiraCode // ""), name, status}]' /tmp/gojira_projects.json
fi
```

결과를 한국어 마크다운 표(프로젝트 수, code, name, status)로 보고합니다.
