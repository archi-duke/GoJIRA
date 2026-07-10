---
name: doc-history
description: 프로젝트 문서 저장소(DocRepo)의 커밋 이력을 조회합니다. 브랜치별 커밋 로그와 변경 이력을 확인합니다.
allowed-tools: Bash
---

커밋 이력을 `doc-history.sh` 로 조회합니다. **curl 로 `${PLATFORM_API}/DocRepo/.../commits` 를 직접 호출하지 말 것** — URL/헤더 조립(branch 인코딩 포함)은 스크립트가 전담한다.

## 인자

사용자 메시지에서 추출. `CODE` 누락 시 사용자에게 되물어 확정하라.

- `CODE` — projectCode (필수, 예: GOJIRA)
- `BRANCH` — (선택) 기본 `main`. 사용자 브랜치는 `docs/{user}`
- `LIMIT` — (선택) 기본 `20`

## 실행 — 이 bash 블록을 그대로 사용

🚨 `RESP=$(bash ...)` 캡처 금지 — 반드시 **파일로 저장** 후 jq 사용.

```bash
export CODE="GOJIRA"             # 실제 프로젝트 코드로 교체
export BRANCH="main"             # 사용자 브랜치는 docs/{user}
export LIMIT="20"

bash "${PROJECT_ROOT}/.claude/skills/_scripts/doc-history.sh" "$CODE" "$BRANCH" "$LIMIT" > /tmp/gojira_commits.json

jq -r '
  "| SHA | 메시지 | 작성자 | 날짜 |",
  "|-----|--------|--------|------|",
  (.[] | "| \(.sha) | \(.message) | \(.author) | \(.date) |")
' /tmp/gojira_commits.json
```

## 보고 형식 (한국어)

1. 상단에 `프로젝트 {CODE} / 브랜치 {BRANCH} — 커밋 {N}건` 요약.
2. 이어서 마크다운 표 출력.
