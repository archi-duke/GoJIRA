---
name: pr-list
description: GoJIRA 프로젝트의 Pull Request 목록을 조회합니다
argument-hint: "[project-code] [--state STATE]"
allowed-tools: Bash
---

프로젝트의 Pull Request 목록을 조회합니다.

## 전체 PR 조회 (기본: open)
!`gojira-cli pr list --project $ARGUMENTS`

## 상태별 필터링
```
gojira-cli pr list --project PROJECT --state open
gojira-cli pr list --project PROJECT --state closed
gojira-cli pr list --project PROJECT --state all
```

결과를 한국어로 정리합니다:
- PR 수를 먼저 알려줍니다
- PR 번호, 제목, 상태, 작성자를 마크다운 표로 정리합니다
