---
name: issue-query
description: GoJIRA 프로젝트의 이슈 목록을 조회합니다. 타입별 필터링 및 Requirement 세부 타입(QA/NFR, FR, CNST) 필터링을 지원합니다.
argument-hint: "[project-code] [--type TYPE] [--reqtype REQTYPE]"
allowed-tools: Bash
---

프로젝트의 이슈 목록을 조회합니다.

## 전체 이슈 조회
!`gojira-cli issue list --project $0`

## 타입별 필터링
```
gojira-cli issue list --project PROJECT --type "VOS"
gojira-cli issue list --project PROJECT --type "Feature"
gojira-cli issue list --project PROJECT --type "Requirement"
gojira-cli issue list --project PROJECT --type "Architectural Design"
```

## Requirement 세부 타입 필터링
Requirement 이슈는 reqType으로 세부 분류 가능:
```
gojira-cli issue list --project PROJECT --type Requirement --reqtype NFR   # QA/품질속성 요구사항
gojira-cli issue list --project PROJECT --type Requirement --reqtype FR    # 기능 요구사항
gojira-cli issue list --project PROJECT --type Requirement --reqtype CNST  # 제약사항
```

- QA 또는 NFR(Quality Attribute) → --reqtype NFR
- FR(Functional Requirement) → --reqtype FR
- CNST(Constraint) → --reqtype CNST

결과를 한국어로 정리합니다:
- 이슈 수를 먼저 알려줍니다
- key, 타입, reqType, 요약, 상태를 마크다운 표로 정리합니다
