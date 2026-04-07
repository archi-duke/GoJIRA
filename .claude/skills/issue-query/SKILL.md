---
name: issue-query
description: GoJIRA 프로젝트의 이슈 목록을 조회합니다. 타입별 필터링, 이슈 현황 파악에 사용합니다.
argument-hint: "[project-code] [--type TYPE]"
allowed-tools: Bash
---

프로젝트의 이슈 목록을 조회합니다.

## 전체 이슈 조회
!`gojira-cli issue list --project $0`

## 타입별 필터링
특정 이슈 타입만 조회하려면:
```
gojira-cli issue list --project $0 --type "VOS"
gojira-cli issue list --project $0 --type "Feature"
gojira-cli issue list --project $0 --type "Requirement"
gojira-cli issue list --project $0 --type "Architectural Design"
```

결과를 한국어로 정리합니다:
- 이슈 수를 먼저 알려줍니다
- key, 타입, 요약, 상태를 마크다운 표로 정리합니다
- 타입별 통계가 있으면 함께 제공합니다
