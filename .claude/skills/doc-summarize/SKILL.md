---
name: doc-summarize
description: GoJIRA 프로젝트의 설계문서를 에이전트가 직접 읽어 요약합니다.
allowed-tools: Bash
---

Platform-API `GET /DocRepo/{code}/file` 로 문서를 읽어 에이전트(LLM)가 직접 한국어 요약을 작성합니다.

## 환경
- `PLATFORM_API` 기본값: `http://localhost:9000/api/v2`

## 인자
- `CODE` — projectCode
- `USER` — X-User-Id
- `FILE` — 요약할 문서 경로
- `REF` — (선택) 브랜치 (기본 `main`)

```bash
export CODE="GOJIRA3"
export USER="batman"
export FILE="architecture/architecture-integrated.md"
export REF="${REF:-main}"
export PLATFORM_API="${PLATFORM_API:-http://localhost:9000/api/v2}"
```

## 문서 읽기
```bash
bash "${PROJECT_ROOT}/.claude/skills/_scripts/doc-read.sh" "${CODE}" "${USER}" "${FILE}"
```

## 요약 규칙
- 문서의 목적 / 주요 섹션 / 핵심 결정사항을 한국어로 정리
- 불필요한 원문 재출력 지양
