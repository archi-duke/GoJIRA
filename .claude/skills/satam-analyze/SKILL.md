---
name: satam-analyze
description: GoJIRA 프로젝트의 SATAM 매트릭스(NFR/ADSN 의 risk/sensitivity/mitigation)를 분석합니다.
allowed-tools: Bash
---

NFR Requirement 와 Architectural Design 이슈의 risk/sensitivity/mitigation 필드를 에이전트가 직접 평가합니다.

## 환경 & 인자
- `GOJIRA_API` 기본값: `http://localhost:9001/api/v2`
- `CODE` — projectCode
- `USER` — X-User-Id

```bash
export CODE="GOJIRA3"
export USER="batman"
export GOJIRA_API="${GOJIRA_API:-http://localhost:9001/api/v2}"
```

## NFR + ADSN 조회
```bash
curl -s "${GOJIRA_API}/Issue" \
  -H "XBody: {\"prjID\":\"${CODE}\",\"infoType\":\"\"}" \
  -H "X-User-Id: ${USER}" \
  | jq '[.[]
      | select((.issueType=="Requirement" and .reqType=="NFR")
               or .issueType=="Architectural Design")
      | {key, subkey, issueType, summary, risk, sensitivity, mitigation, tradeoffs}]'
```

## 분석 항목
- 위험도 높은 항목 (risk)
- 민감도 높은 항목 (sensitivity)
- 경감조치 미완료 (mitigation 비어있음)
- 전체 커버리지 통계

결과는 한국어 표 + 권고 순서로 보고.
