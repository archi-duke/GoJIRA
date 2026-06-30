---
name: pr-list
description: GoJIRA 프로젝트의 Pull Request 목록을 조회합니다.
allowed-tools: Bash
---

Platform-API `GET /DocRepo/{code}/prs?state=...` 로 PR 목록을 조회합니다.

## 환경
- `PLATFORM_API` 필수 (launcher 가 주입). 미설정 시 fail-fast

## 인자
- `CODE` — projectCode
- `USER` — X-User-Id
- `STATE` — `open` | `closed` | `all` (기본 `open`)

```bash
export CODE="GOJIRA3"
export USER="batman"
export STATE="${STATE:-open}"
export PLATFORM_API="${PLATFORM_API:?PLATFORM_API env var required (launcher must inject)}"
```

## 조회
🚨 `RESP=$(curl ...)` 캡처 금지 — bash 가 응답 바이트를 손상시킴. **파일 저장** 후 jq 사용.

```bash
curl -s "${PLATFORM_API}/DocRepo/${CODE}/prs?state=${STATE}" \
  -H "X-User-Id: ${USER}" -H "Authorization: Bearer ${GOJIRA_SESSION_TOKEN}" \
  > /tmp/gojira_prs.json

jq '[.[] | {number, title, state, author: (.user.login // .author // null), head: (.head.ref // null)}]' /tmp/gojira_prs.json
```

결과를 한국어로 정리: PR 수 → 번호/제목/상태/작성자 마크다운 표.
