---
name: issue-manage
description: GoJIRA 이슈를 생성, 수정, 삭제하거나 이슈 간 관계를 설정합니다.
argument-hint: "<create|update|delete|link> [options]"
disable-model-invocation: true
allowed-tools: Bash
---

이슈를 관리합니다. 아래 서브커맨드를 사용하세요.

## 이슈 생성
```
gojira-cli issue create --project PROJECT --type TYPE --summary "제목" --description "설명"
```
- TYPE: VOS, Feature, Requirement, "Architectural Design", "Detailed Design"
- 이슈 키는 자동 할당됩니다

## 이슈 수정
```
gojira-cli issue update --key GOJIRA-1 --status "In Progress"
gojira-cli issue update --key GOJIRA-1 --summary "새 제목" --priority High
```
- 변경할 필드만 지정합니다

## 이슈 삭제
```
gojira-cli issue delete --key GOJIRA-1
```

## 이슈 관계 설정
```
gojira-cli issue link --from GOJIRA-1 --to GOJIRA-5 --type contain
```
- 관계 유형: contain, relate, duplicate, block
- contain: 상위→하위 포함 관계 (VOS→Feature→Requirement→ADSN)

실행 전 사용자에게 확인을 요청합니다.
생성/수정/삭제 결과를 JSON으로 출력합니다.
