---
name: schedule-analyze
description: GoJIRA 프로젝트 이슈 상태를 기반으로 진행률/지연 리스크를 분석합니다.
allowed-tools: Bash
---

전체 이슈의 status/dueDate 를 에이전트가 직접 집계해 일정 분석을 수행합니다.

## 환경 & 인자
- `GOJIRA_API` 기본값: `http://10.211.55.3:44360/api/v2`
- `CODE` — projectCode
- `USER` — X-User-Id

```bash
export CODE="GOJIRA3"
export USER="batman"
export GOJIRA_API="${GOJIRA_API:-http://10.211.55.3:44360/api/v2}"
```

## 조회
🚨 `RESP=$(curl ...)` 캡처 금지 — bash 가 응답 바이트를 손상시킴. **파일 저장** 후 jq 사용.

```bash
curl -s "${GOJIRA_API}/Issue" \
  -H "XBody: {\"prjID\":\"${CODE}\",\"infoType\":\"\"}" \
  -H "X-User-Id: ${USER}" \
  > /tmp/gojira_issues.json

jq '[.[] | {key, subkey, issueType, summary, status, dueDate, startDate}]' /tmp/gojira_issues.json
```

## 분석
1. 전체 진행률 (완료/진행중/미착수 비율)
2. 마일스톤/단계별 일정 적정성
3. 지연 리스크 항목 (dueDate 지난 미완료)
4. 개선 제안

결과는 한국어 요약 + 표.
