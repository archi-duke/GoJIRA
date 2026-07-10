---
name: pr-list
description: GoJIRA 프로젝트의 Pull Request 목록을 조회합니다.
allowed-tools: Bash
---

PR 목록을 `pr-list.sh` 로 조회합니다. **curl 로 `${PLATFORM_API}/DocRepo/.../prs` 를 직접 호출하지 말 것** — URL/헤더 조립은 스크립트가 전담한다.

## 인자
- `CODE` — projectCode (필수)
- `USER` — X-User-Id (필수, 시스템 컨텍스트의 knoxID)
- `STATE` — `open` | `closed` | `all` (선택, 기본 `open`)

## 실행 — 이 bash 블록을 그대로 사용

🚨 `RESP=$(bash ...)` 캡처 금지 — 반드시 **파일로 저장** 후 jq 사용.

```bash
export CODE="GOJIRA3"            # 실제 프로젝트 코드로 교체
export USER="batman"             # 시스템 컨텍스트의 knoxID 로 교체
export STATE="open"              # open | closed | all

bash "${PROJECT_ROOT}/.claude/skills/_scripts/pr-list.sh" "$CODE" "$USER" "$STATE" > /tmp/gojira_prs.json

jq . /tmp/gojira_prs.json
```

## 결과 보고 (한국어)
PR 수 → 번호 / 제목 / 상태 / 작성자 마크다운 표.
