---
name: satam-analyze
description: GoJIRA 프로젝트의 SATAM 매트릭스를 분석합니다. Risk, Sensitivity, Mitigation 상태를 평가합니다.
argument-hint: "[project-code]"
allowed-tools: Bash
---

프로젝트의 SATAM 매트릭스를 분석합니다.

## 분석 실행
```
gojira-cli schedule analyze --project $ARGUMENTS
```

또는 이슈를 직접 조회하여 분석:
```
gojira-cli issue list --project $ARGUMENTS --type Requirement --reqtype NFR
gojira-cli issue list --project $ARGUMENTS --type "Architectural Design"
```

분석 항목:
- 위험도 높은 항목 (risk 필드)
- 민감도 높은 항목 (sensitivity 필드)
- 경감조치 미완료 (mitigation 없음)
- 전체 커버리지 통계
