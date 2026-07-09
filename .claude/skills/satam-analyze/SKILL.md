---
name: satam-analyze
description: GoJIRA 프로젝트의 SATAM 매트릭스(NFR/ADSN 의 risk/sensitivity/mitigation)를 분석합니다.
allowed-tools: Bash
---

NFR Requirement 와 Architectural Design 이슈의 risk/sensitivity/mitigation 필드를 에이전트가 직접 평가합니다.
(이슈는 로컬 clone data/issues/*.yaml 에서 읽는다 — Mongo 는 PR 머지 시점에만 갱신되므로
세션 중 GoJIRA-API `/Issue` 로는 방금 만든 이슈가 보이지 않는다.)

## 환경 & 인자
- `CODE` — projectCode (참고용)

```bash
export CODE="GOJIRA3"
```

## NFR + ADSN 조회
🚨 `RESP=$(curl ...)` 같은 command substitution 캡처 금지 — bash 가 응답 바이트를 손상시킴. **파일 저장** 후 jq 로 필터.

```bash
bash "${PROJECT_ROOT}/.claude/skills/_scripts/issue-query.sh" > /tmp/gojira_issues.json

jq '[.[]
    | select((.issueType=="Requirement" and .reqtype=="NFR")
             or .issueType=="Architectural Design")
    | {key, subkey, issueType, summary, risk, sensitivity, mitigation, tradeoffs}]' \
  /tmp/gojira_issues.json
```

## 분석 항목
- 위험도 높은 항목 (risk)
- 민감도 높은 항목 (sensitivity)
- 경감조치 미완료 (mitigation 비어있음)
- 전체 커버리지 통계

결과는 한국어 표 + 권고 순서로 보고.
