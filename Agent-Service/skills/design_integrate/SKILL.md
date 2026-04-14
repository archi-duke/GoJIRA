---
name: design_integrate
description: ADSN/DDSN 설계 결정과 시스템 스타일을 논리/프로세스/배포/개발 관점으로 통합해 architecture/architecture-integrated.md 를 작성합니다. 이슈 변경 없이 문서만 산출.
---

# design_integrate

Phase 3 ⑧ 디자인 통합.

## Parameters

```yaml
projectCode:
  type: string
  required: true
user:
  type: string
```

## 선행 의존

- ADSN/DDSN 이슈 존재 (design_decide 완료). 없으면 `design_decide` 자동 위임

## 동작

1. ADSN/DDSN 이슈 + chosenStyles + normalizedAttributes 수집
2. LLM 으로 최소 2개 관점(Logical/Process/Deployment/Development) 통합 서술
3. 관점 간 일관성 검증 + Mermaid 다이어그램 인라인
4. critique 루프 (workflow-architecture-design.md ⑧ SSOT)
5. `architecture/architecture-integrated.md` 저장

## 산출

- `architecture/architecture-integrated.md` 문서
- 이슈 생성/변경 없음
