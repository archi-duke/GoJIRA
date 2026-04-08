---
name: issue-create
description: GoJIRA 프로젝트에 새 이슈를 생성합니다. Requirement 이슈는 reqType(NFR/FR/CNST)을 지정해야 합니다.
argument-hint: "[project-code] --type TYPE [--reqtype REQTYPE] --summary \"...\""
allowed-tools: Bash
disable-model-invocation: true
---

프로젝트에 새 이슈를 생성합니다. 쓰기 작업이므로 실행 전 사용자 확인이 필요합니다.

## 이슈 생성
```
gojira-cli issue create --project PROJECT --type TYPE --summary "제목" [--description "설명"] [--priority Medium]
```

## Requirement 이슈 생성 시 reqType 필수
```
gojira-cli issue create --project PROJECT --type Requirement --reqtype NFR --summary "품질속성 요구사항"
gojira-cli issue create --project PROJECT --type Requirement --reqtype FR --summary "기능 요구사항"
gojira-cli issue create --project PROJECT --type Requirement --reqtype CNST --summary "제약사항"
```

## reqType 매핑
- QA/품질속성(Quality Attribute) → --reqtype NFR
- 기능 요구사항(Functional Requirement) → --reqtype FR
- 제약사항(Constraint) → --reqtype CNST

## 사용 가능한 이슈 타입
- VOS, Feature, Requirement, Architectural Design, Detailed Design

생성 후 반환된 이슈 키를 사용자에게 알려줍니다.
