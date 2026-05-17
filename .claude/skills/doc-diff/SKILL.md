---
name: doc-diff
description: main 브랜치와 사용자 브랜치(docs/{user}) 간의 문서 차이를 분석합니다.
allowed-tools: Bash
---

Platform-API `/DocRepo/{code}/file` 로 두 브랜치의 파일을 가져와 diff 합니다.

## 환경
- `PLATFORM_API` 기본값: `http://10.211.55.3:44350/api/v2`

## 인자
- `CODE` — projectCode
- `USER` — X-User-Id
- `FILE` — 비교할 문서 경로 (예: `business/project-description.md`)

```bash
export CODE="GOJIRA3"
export USER="batman"
export FILE="business/project-description.md"
export PLATFORM_API="${PLATFORM_API:-http://10.211.55.3:44350/api/v2}"
```

## 파일 조회 + diff
```bash
# main 브랜치 문서 (doc-read.sh 는 docs/{user} fallback 하므로, main 고정을 위해 USER=main 전달)
bash "${PROJECT_ROOT}/.claude/skills/_scripts/doc-read.sh" "${CODE}" "main" "${FILE}" > /tmp/doc-main.txt 2>/dev/null || true
# docs/{user} 브랜치 문서
bash "${PROJECT_ROOT}/.claude/skills/_scripts/doc-read.sh" "${CODE}" "${USER}" "${FILE}" > /tmp/doc-user.txt 2>/dev/null || true
diff -u /tmp/doc-main.txt /tmp/doc-user.txt || true
```

결과를 한국어로 정리: 추가/삭제/수정 구분 + 요약.
