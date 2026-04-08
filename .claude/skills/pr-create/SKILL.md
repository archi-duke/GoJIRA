---
name: pr-create
description: 사용자 브랜치에서 main으로 Pull Request를 생성합니다
argument-hint: "[project-code] [--title \"...\"]"
allowed-tools: Bash
disable-model-invocation: true
---

사용자 브랜치에서 main 브랜치로 Pull Request를 생성합니다. 쓰기 작업이므로 실행 전 사용자 확인이 필요합니다.

## PR 생성
!`gojira-cli pr create --project $ARGUMENTS`

## 제목 지정
```
gojira-cli pr create --project PROJECT --title "PR 제목"
```

생성된 PR 번호와 URL을 사용자에게 알려줍니다.
title 생략 시 브랜치명 기반으로 자동 생성됩니다.
