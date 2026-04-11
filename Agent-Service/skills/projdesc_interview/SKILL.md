---
name: projdesc_interview
description: project-description.md 작성을 위한 인터뷰를 진행합니다. 문서가 없거나 내용이 부족하면 질문 목록을 반환하고, 답변을 수집하여 projdesc_generate 스킬로 문서를 생성합니다.
---

## Parameters

```yaml
projectCode:
  type: string
  description: "프로젝트 코드 (예: GOJIRA)"
  required: true
answers:
  type: string
  description: "이전 질문에 대한 사용자 답변 (JSON 형식). 최초 호출 시 비워두면 질문 목록을 반환합니다."
```

## 역할

projdesc_generate 워크플로우의 **문서 상태 확인 + 질문 생성** 단계를 담당합니다.

## 인터뷰 진행 규칙

1. 먼저 "문서 작성을 위한 인터뷰를 진행하겠습니다"라고 안내한다
2. 질문은 **한 번에 하나씩** 자연스럽게 대화하듯 진행한다
3. 사용자 답변이 짧거나 모호하면 **구체적으로 다시 물어본다**
4. 모든 항목이 충분히 확보되면 answers를 JSON으로 정리하여 projdesc_generate를 호출한다
5. projdesc_generate 호출 시 반드시 mode="generate"와 answers를 전달한다

## 질문 항목 (필수)

- background: 이 과제를 시작하게 된 배경이나 동기
- purpose: 이 과제가 달성하려는 핵심 목표
- stakeholders: 이 과제의 주요 사용자나 이해관계자
- requirements: 반드시 충족해야 할 주요 요구사항
- constraints: 기술적 또는 비즈니스적 제약사항 (없으면 '없음')

## 반환 상태

- `sufficient`: 문서에 내용이 충분함 → 보강(enrich) 가능
- `needs_interview`: 문서가 없거나 부족 → 인터뷰 질문 목록 반환
- `ready`: 인터뷰 답변이 모두 수집됨 → 문서 생성 가능
- `incomplete`: 일부 답변 부족 → 미답변 질문 목록 반환
