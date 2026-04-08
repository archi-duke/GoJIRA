---
name: issue-analyze
description: GoJIRA 프로젝트의 이슈 관계를 분석합니다. contain 체인 검증, 고아 이슈 감지, 누락 관계 제안을 합니다.
argument-hint: "[project-code]"
allowed-tools: Bash
---

프로젝트의 이슈 관계를 분석합니다.

## 분석 실행
전체 이슈를 조회하여 관계를 분석합니다:
```
gojira-cli issue list --project $ARGUMENTS
```

분석 항목:
1. contain 체인 검증: VOS → Feature → Requirement → ADSN 연결 여부
2. 고아 이슈: 상위 이슈에 contain되지 않는 이슈
3. 타입별/관계별 통계
