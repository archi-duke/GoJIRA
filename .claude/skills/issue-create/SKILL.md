---
name: issue-create
description: GoJIRA 프로젝트에 새 이슈를 생성합니다
argument-hint: "[project-code] --type TYPE --summary \"...\""
allowed-tools: Bash
disable-model-invocation: true
---

프로젝트에 새 이슈를 생성합니다. 쓰기 작업이므로 실행 전 사용자 확인이 필요합니다.

## 이슈 생성
!`gojira-cli issue create --project $0 --type "$1" --summary "$2"`

## 옵션 포함
```
gojira-cli issue create --project PROJECT --type "VOS" --summary "이슈 제목" --description "상세 설명" --priority Medium
```

## 사용 가능한 이슈 타입
- VOS
- Feature
- Requirement
- Architectural Design
- Detailed Design

## 사용 가능한 우선순위
- Highest / High / Medium / Low / Lowest

생성 후 반환된 이슈 키를 사용자에게 알려줍니다.
