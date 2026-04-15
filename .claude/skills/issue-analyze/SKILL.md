---
name: issue-analyze
description: GoJIRA 프로젝트의 이슈 관계(Contains 체인, 고아 이슈, 타입 통계)를 분석합니다.
allowed-tools: Bash
---

GoJIRA-API `/Issue` 에서 전체 이슈를 가져와 에이전트가 직접 관계를 분석합니다.

## 환경 & 인자
- `GOJIRA_API` 기본값: `http://localhost:9001/api/v2`
- `CODE` — projectCode
- `USER` — X-User-Id

```bash
export CODE="GOJIRA3"
export USER="batman"
export GOJIRA_API="${GOJIRA_API:-http://localhost:9001/api/v2}"
```

## 이슈 전체 조회
```bash
curl -s "${GOJIRA_API}/Issue" \
  -H "XBody: {\"prjID\":\"${CODE}\",\"infoType\":\"\"}" \
  -H "X-User-Id: ${USER}" \
  | jq '[.[] | {key, subkey, issueType, reqType, summary, status, links}]'
```

## 분석 항목 (에이전트가 직접 수행)
1. **Contains 체인**: VOS → Feature → Requirement → ADSN 연결 여부
2. **고아 이슈**: 어떤 상위에도 Contains 되지 않는 이슈
3. **타입/관계별 통계**

결과는 한국어 마크다운 표 + 개선 제안으로 정리.
