---
name: vos_collect
description: 프로젝트의 project-description.md를 분석하여 이해관계자 요구사항(VOS)을 추출하고 VOS 이슈를 자동 생성합니다. 기존 VOS가 있으면 중복을 피해 추가분만 생성합니다.
---

## Parameters

```yaml
projectCode:
  type: string
  description: "프로젝트 코드 (예: GOJIRA)"
  required: true
user:
  type: string
  description: "요청 사용자 ID (문서 브랜치 조회용)"
```

## VOS 수집 절차

1. `business/project-description.md` 문서를 조회합니다 (main → docs/{user} 순서)
2. 기존 VOS 이슈를 조회합니다
3. 문서 내용을 분석하여 이해관계자별 요구사항(VOS)을 추출합니다
4. 기존 VOS와 중복되지 않는 항목만 VOS 이슈로 생성합니다

## 분석 관점

- 과제 배경에서 도출되는 비즈니스 요구
- 과제 목적에서 도출되는 핵심 목표
- 주요 사항에서 도출되는 구체적 요구사항
- 이해관계자(사용자, 관리자, 운영자, 외부 시스템 등)별 관점

## VOS 이슈 필드

- issueType: VOS
- summary: 요구사항 제목 (간결하게)
- description: 요구사항 상세 내용 (배경, 근거 포함)
- client: 요구사항을 제기한 이해관계자 역할명

## 규칙

- 문서가 없거나 템플릿만 있는 경우 문서 보강을 요청합니다
- 기존 VOS는 수정하거나 삭제하지 않습니다
- summary가 유사한 기존 VOS가 있으면 해당 항목은 스킵합니다
- 추가할 내용이 없으면 빈 목록을 반환합니다
