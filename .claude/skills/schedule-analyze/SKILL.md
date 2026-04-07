---
name: schedule-analyze
description: GoJIRA 프로젝트의 일정을 분석하여 진행률, 리스크, 개선 제안을 제공합니다.
argument-hint: "[project-code]"
allowed-tools: Bash
---

프로젝트의 일정을 분석합니다.

## 일정 분석 실행
```
gojira-cli schedule analyze --project $ARGUMENTS
```

분석 항목:
1. 전체 진행률 (완료/진행중/미착수 비율)
2. 마일스톤 간 일정 적정성
3. 지연 리스크 항목
4. 개선 제안

결과를 한국어로 정리합니다.
