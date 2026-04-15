---
name: issue-update
description: GoJIRA 기존 이슈의 필드(status/summary/priority 등)를 수정합니다. 쓰기 작업.
allowed-tools: Bash
disable-model-invocation: true
---

GoJIRA-API `PUT /Issue` 로 이슈 필드를 부분 업데이트합니다. 쓰기 작업이므로 실행 전 사용자 확인이 필요합니다.

## 환경
- `GOJIRA_API` 기본값: `http://localhost:9001/api/v2`
- `X-User-Id` 헤더 필수

## 인자 추출
- `CODE` — projectCode (필수)
- `USER` — user (X-User-Id, 필수)
- `KEY` — 이슈 키 (필수, 예: GOJIRA-1)
- `STATUS` — (선택) `To Do` | `In Progress` | `Done`
- `SUMMARY` — (선택) 새 제목
- `PRIORITY` — (선택) `Highest` | `High` | `Medium` | `Low` | `Lowest`

```bash
export CODE="GOJIRA3"
export USER="batman"
export KEY="GOJIRA3-1"
export STATUS=""
export SUMMARY=""
export PRIORITY=""
export GOJIRA_API="${GOJIRA_API:-http://localhost:9001/api/v2}"
```

## 업데이트 (빈 필드는 제외)
```bash
DATA=$(jq -n \
  --arg s "$STATUS" --arg sm "$SUMMARY" --arg p "$PRIORITY" '
  [ {status:$s}, {summary:$sm}, {priority:$p} ]
  | map(to_entries[] | select(.value != ""))
  | from_entries')

curl -s -X PUT "${GOJIRA_API}/Issue" \
  -H "Content-Type: application/json" -H "X-User-Id: ${USER}" \
  -d "$(jq -n --arg p "$CODE" --arg k "$KEY" --arg u "$USER" --argjson d "$DATA" \
    '{prjID:$p, key:$k, user:$u, data:$d}')"
```

결과에서 `key` 와 변경된 필드를 사용자에게 보고합니다.
