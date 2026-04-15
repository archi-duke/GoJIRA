---
name: doc-list
description: GoJIRA 프로젝트의 설계문서 파일 목록을 조회합니다.
allowed-tools: Bash
---

Platform-API `GET /DocRepo/{code}/tree` 로 문서 파일 트리를 조회합니다.

## 환경
- `PLATFORM_API` 기본값: `http://localhost:9000/api/v2`

## 인자
- `CODE` — projectCode
- `USER` — X-User-Id
- `REF` — (선택) 브랜치 (기본: `main`, 개인 작업은 `docs/{USER}`)

```bash
export CODE="GOJIRA3"
export USER="batman"
export REF="${REF:-main}"
export PLATFORM_API="${PLATFORM_API:-http://localhost:9000/api/v2}"
```

## 조회
```bash
curl -s "${PLATFORM_API}/DocRepo/${CODE}/tree?ref=$(printf %s "$REF" | jq -sRr @uri)" \
  -H "X-User-Id: ${USER}" \
  | jq '[.[] | {path, name, type, lastCommit: (.lastCommit // null)}]'
```

결과를 한국어로 정리: 파일 수 → 경로/이름/최종 수정일 마크다운 표.
