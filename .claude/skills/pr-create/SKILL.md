---
name: pr-create
description: 사용자 브랜치(docs/{user})에서 main 으로 Pull Request 를 생성합니다. 쓰기 작업.
allowed-tools: Bash
disable-model-invocation: true
---

Platform-API `POST /DocRepo/{code}/pr` 로 PR 을 생성합니다. 쓰기 작업 — 실행 전 사용자 확인.

## 환경
- `PLATFORM_API` 필수 (launcher 가 주입). 미설정 시 fail-fast

## 인자
- `CODE` — projectCode
- `USER` — X-User-Id (head 브랜치는 `docs/${USER}`)
- `TITLE` — (선택) PR 제목 (생략 시 `[CODE] 설계문서 배포 요청`)
- `BODY` — (선택) PR 본문

```bash
export CODE="GOJIRA3"
export USER="batman"
export TITLE=""
export BODY=""
export PLATFORM_API="${PLATFORM_API:?PLATFORM_API env var required (launcher must inject)}"
[ -z "$TITLE" ] && TITLE="[${CODE}] 설계문서 배포 요청"
```

## 생성
```bash
curl -s -X POST "${PLATFORM_API}/DocRepo/${CODE}/pr" \
  -H "Content-Type: application/json" -H "X-User-Id: ${USER}" \
  -d "$(jq -n --arg t "$TITLE" --arg b "$BODY" --arg h "docs/${USER}" \
    '{title:$t, body:$b, head:$h, base:"main"}')"
```

결과의 PR 번호와 URL 을 사용자에게 보고합니다.
