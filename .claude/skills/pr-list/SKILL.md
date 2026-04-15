---
name: pr-list
description: GoJIRA 프로젝트의 Pull Request 목록을 조회합니다.
allowed-tools: Bash
---

Platform-API `GET /DocRepo/{code}/prs?state=...` 로 PR 목록을 조회합니다.

## 환경
- `PLATFORM_API` 기본값: `http://localhost:9000/api/v2`

## 인자
- `CODE` — projectCode
- `USER` — X-User-Id
- `STATE` — `open` | `closed` | `all` (기본 `open`)

```bash
export CODE="GOJIRA3"
export USER="batman"
export STATE="${STATE:-open}"
export PLATFORM_API="${PLATFORM_API:-http://localhost:9000/api/v2}"
```

## 조회
```bash
curl -s "${PLATFORM_API}/DocRepo/${CODE}/prs?state=${STATE}" \
  -H "X-User-Id: ${USER}" \
  | jq '[.[] | {number, title, state, author: (.user.login // .author // null), head: (.head.ref // null)}]'
```

결과를 한국어로 정리: PR 수 → 번호/제목/상태/작성자 마크다운 표.
