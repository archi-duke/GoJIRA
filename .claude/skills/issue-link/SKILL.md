---
name: issue-link
description: GoJIRA 두 이슈 간의 관계를 설정합니다 (contain, relate, block 등)
argument-hint: "[from-key] [to-key] --type TYPE"
allowed-tools: Bash
disable-model-invocation: true
---

두 이슈 간의 관계를 설정합니다. 쓰기 작업이므로 실행 전 사용자 확인이 필요합니다.

## 이슈 링크 생성
!`gojira-cli issue link --from $0 --to $1 --type $2`

## 사용 가능한 링크 타입
- contain: 포함 관계 (상위 이슈가 하위 이슈를 포함)
- relate: 관련 관계
- duplicate: 중복 관계
- block: 차단 관계 (선행 이슈가 후행 이슈를 차단)

링크 생성 후 결과를 사용자에게 알려줍니다.
