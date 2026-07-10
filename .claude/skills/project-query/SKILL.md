---
name: project-query
description: GoJIRA 프로젝트 목록을 조회합니다.
allowed-tools: Bash
---

프로젝트 목록/단건을 `project-query.sh` 로 조회합니다. **curl 로 `${GOJIRA_API}/Project` 를 직접 호출하지 말 것** — URL/헤더 조립은 스크립트가 전담한다.

## 인자
- `USER` — X-User-Id (필수, 시스템 컨텍스트의 knoxID)
- `CODE` — (선택) 특정 프로젝트만 조회. 지정 시 그 프로젝트의 **전체 오브젝트**(archSettings 포함) 1건.

## 실행 — 이 bash 블록을 그대로 사용

🚨 `RESP=$(bash ...)` 캡처 금지 — 반드시 **파일로 저장** 후 jq 사용.

```bash
export USER="batman"             # 시스템 컨텍스트의 knoxID 로 교체
export CODE=""                   # 특정 프로젝트만 볼 때 지정

# 전체 목록(요약: code/name/status) 또는 CODE 지정 시 1건(전체 필드)
bash "${PROJECT_ROOT}/.claude/skills/_scripts/project-query.sh" "$USER" $CODE > /tmp/gojira_projects.json

jq . /tmp/gojira_projects.json
```

## 결과 보고 (한국어)
- 목록 조회: 프로젝트 수 + code / name / status 마크다운 표
- 단건 조회: 주요 필드(code, name, status, archSettings 유무 등) 요약
