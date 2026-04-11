---
name: vos-collect
description: 프로젝트의 project-description.md를 분석하여 이해관계자 요구사항(VOS)을 추출하고 이슈를 생성합니다.
argument-hint: "[project-code]"
allowed-tools: Bash
---

프로젝트의 `business/project-description.md` 문서를 분석하여 이해관계자(Stakeholder)의 요구사항(VOS)을 추출하고, VOS 타입 이슈를 생성합니다.

## 실행 절차

### 1단계: 문서 조회
프로젝트의 business/project-description.md 내용을 조회합니다.
```
gojira-cli doc summarize --project $ARGUMENTS --file "business/project-description.md"
```

### 2단계: 기존 VOS 조회
이미 생성된 VOS 이슈를 확인합니다.
```
gojira-cli issue list --project $ARGUMENTS --type VOS
```

### 3단계: VOS 추출 및 생성
project-description.md 내용을 분석하여 이해관계자별 요구사항(VOS)을 추출합니다.

**분석 관점:**
- 과제 배경에서 도출되는 비즈니스 요구
- 과제 목적에서 도출되는 핵심 목표
- 주요 사항에서 도출되는 구체적 요구사항
- 이해관계자(사용자, 관리자, 운영자, 외부 시스템 등)별 관점

**VOS 이슈 생성 규칙:**
- 문서가 없거나 템플릿만 있는 경우 문서 보강을 요청하고 중단
- 기존 VOS가 있으면 중복되지 않는 신규 항목만 추가 생성
- 기존 VOS는 수정하거나 삭제하지 않음
- 추가할 내용이 없으면 "추가 VOS 없음"으로 응답

**VOS 이슈 필드:**
- issueType: VOS
- summary: 요구사항 제목 (간결하게)
- description: 요구사항 상세 내용
- client: 요구사항을 제기한 이해관계자 (knoxID 또는 역할명)

```
gojira-cli issue create --project $ARGUMENTS --type VOS --summary "요구사항 제목" --description "상세 내용"
```

### 4단계: 결과 보고
생성된 VOS 목록을 마크다운 표로 정리합니다:
- 이슈 키, 요약, 이해관계자(client), 도출 근거
- 기존 VOS 대비 추가된 항목 수
