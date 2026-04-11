---
name: projdesc_generate
description: 인터뷰 답변 또는 기존 내용을 기반으로 project-description.md를 생성하거나 보강합니다. 사용자 브랜치에 저장합니다.
---

## Parameters

```yaml
projectCode:
  type: string
  description: "프로젝트 코드 (예: GOJIRA)"
  required: true
answers:
  type: string
  description: "인터뷰 답변 (JSON). background, purpose, stakeholders, requirements, constraints 키"
content:
  type: string
  description: "기존 문서 내용 (보강 모드에서 사용). answers와 content 중 하나 필수"
mode:
  type: string
  description: "생성 모드"
  enum: [generate, enrich]
```

## 동작 모드

### generate (기본)
인터뷰 답변(answers)을 기반으로 project-description.md를 새로 생성합니다.

### enrich
기존 문서 내용(content)을 분석하여 더 풍부하게 보강합니다.

## 생성 규칙

- 마크다운 형식으로 구조화
- 섹션: 과제 배경, 과제 목적, 주요 사항, 이해관계자
- VOS 자동생성 참조 안내 포함
- 사용자 브랜치(docs/{user})에 저장
