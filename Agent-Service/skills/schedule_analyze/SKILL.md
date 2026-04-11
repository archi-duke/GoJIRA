---
name: schedule_analyze
description: 프로젝트의 마일스톤 일정을 분석하여 리스크와 진행 상태를 보고합니다.
---

## Parameters

```yaml
projectCode:
  type: string
  description: 프로젝트 코드
  required: true
```

## 일정 분석 규칙

- 프로젝트의 마일스톤(PA, EA, ER1, ER2, CA 등) 일정을 조회합니다
- 이슈 상태별 통계(총/완료/진행중/미착수)를 집계합니다
- 오늘 날짜 기준으로 분석합니다
- LLM에 일정 정보를 전달하여 다음을 분석합니다:
  1. 전체 진행률
  2. 마일스톤 간 일정 적정성
  3. 지연 리스크 항목
  4. 개선 제안
