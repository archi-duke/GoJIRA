---
name: project-query
description: GoJIRA 프로젝트 목록을 조회합니다.
allowed-tools: Bash
---

GoJIRA-API `/Project` 에서 프로젝트 목록을 조회합니다.

## 환경
- `GOJIRA_API` 기본값: `http://localhost:9001/api/v2`

## 인자
- `USER` — X-User-Id (필수)
- `CODE` — (선택) 특정 프로젝트만 조회

```bash
export USER="batman"
export CODE=""
export GOJIRA_API="${GOJIRA_API:-http://localhost:9001/api/v2}"
```

## 조회
🚨 `RESP=$(curl ...)` 캡처 금지 — bash 가 응답 바이트를 손상시킴. **파일 저장** 후 jq 사용.

```bash
if [ -n "$CODE" ]; then
  curl -s "${GOJIRA_API}/Project/${CODE}" -H "X-User-Id: ${USER}" > /tmp/gojira_project.json
  jq '{code:.code, name:.name, status:.status}' /tmp/gojira_project.json
else
  curl -s "${GOJIRA_API}/Project" -H "X-User-Id: ${USER}" > /tmp/gojira_projects.json
  jq '[.[] | {code, name, status}]' /tmp/gojira_projects.json
fi
```

결과를 한국어 마크다운 표(프로젝트 수, code, name, status)로 보고합니다.
