---
name: issue-update
description: GoJIRA 기존 이슈의 필드를 수정합니다
argument-hint: "[issue-key] [--status STATUS] [--summary \"...\"]"
allowed-tools: Bash
disable-model-invocation: true
---

기존 이슈의 필드를 수정합니다. 쓰기 작업이므로 실행 전 사용자 확인이 필요합니다.

## 상태 변경
!`gojira-cli issue update --key $0 --status "$1"`

## 제목 변경
```
gojira-cli issue update --key GOJIRA-1 --summary "새 제목"
```

## 복합 수정
```
gojira-cli issue update --key GOJIRA-1 --status "In Progress" --priority "High" --summary "수정된 제목"
```

## 사용 가능한 상태
- To Do / In Progress / Done

## 사용 가능한 우선순위
- Highest / High / Medium / Low / Lowest

변경할 필드만 전달합니다 (부분 업데이트).
