---
name: issue_analyze
description: 프로젝트의 이슈 관계(contain, relate 등)를 분석하고, 누락된 관계나 고아 이슈를 감지합니다. VOS→Feature→Requirement→ADSN contain 체인을 검증합니다.
parameters:
  projectCode:
    type: string
    description: 프로젝트 코드
    required: true
---

## 이슈 관계 분석 규칙

- 프로젝트의 전체 이슈와 issueLinks를 조회합니다
- 다음 항목을 분석합니다:
  1. contain 체인 검증: VOS → Feature → Requirement → ADSN 연결 여부
  2. 고아 이슈: 어떤 상위 이슈에도 contain되지 않는 이슈
  3. 누락 관계 제안: 내용 기반으로 연결이 필요해 보이는 이슈 쌍
  4. 타입별 통계: VOS/Feature/Requirement/ADSN/DDSN 수량
  5. 관계 통계: contain/relate/block 수량
- 결과를 마크다운으로 정리합니다
