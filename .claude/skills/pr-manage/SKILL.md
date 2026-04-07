---
name: pr-manage
description: GoJIRA 프로젝트의 Pull Request를 생성하거나 목록을 조회합니다.
argument-hint: "<create|list> [options]"
disable-model-invocation: true
allowed-tools: Bash
---

Pull Request를 관리합니다.

## PR 생성
```
gojira-cli pr create --project PROJECT --title "제목" --body "본문"
```
- GOJIRA_USER 환경변수의 사용자 브랜치(docs/{user})에서 main으로 PR을 생성합니다
- title 생략 시 자동 생성됩니다

## PR 목록 조회
```
gojira-cli pr list --project PROJECT
gojira-cli pr list --project PROJECT --state all
gojira-cli pr list --project PROJECT --state closed
```
- 기본 상태 필터는 "open"입니다

실행 전 사용자에게 확인을 요청합니다.
