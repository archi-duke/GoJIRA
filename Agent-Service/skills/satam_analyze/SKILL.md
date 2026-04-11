---
name: satam_analyze
description: 프로젝트의 SATAM(Sensitivity, Architecture Tradeoff Analysis Method) 매트릭스를 분석합니다. Requirement와 Design 이슈의 risk, sensitivity, mitigation 상태를 평가합니다.
---

## Parameters

```yaml
projectCode:
  type: string
  description: 프로젝트 코드
  required: true
```

## SATAM 분석 규칙

- Requirement(QA/NFR) 이슈와 Architectural Design 이슈를 조회합니다
- 각 이슈의 risk, sensitivity, mitigation 필드를 분석합니다
- 다음 항목을 보고합니다:
  - 위험도 높은 항목 (risk 있음)
  - 민감도 높은 항목 (sensitivity 있음)
  - 경감조치 미완료 항목 (risk/sensitivity 있는데 mitigation 없음)
  - 전체 커버리지 통계
- 결과를 마크다운 표로 정리합니다
