---
name: issue-delete
description: GoJIRA 이슈를 삭제합니다
argument-hint: "[issue-key]"
allowed-tools: Bash
disable-model-invocation: true
---

이슈를 삭제합니다. 삭제는 되돌릴 수 없으므로 반드시 사용자 확인이 필요합니다.

## 이슈 삭제
!`gojira-cli issue delete --key $ARGUMENTS`

삭제 완료 후 결과를 사용자에게 알려줍니다.
