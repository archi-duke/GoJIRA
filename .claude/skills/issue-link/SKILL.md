---
name: issue-link
description: GoJIRA 두 이슈 간의 관계(Contains/Relates/Blocks 등)를 설정합니다. 쓰기 작업.
allowed-tools: Bash
disable-model-invocation: true
---

GoJIRA-API `POST /Issue/link` 로 두 이슈 간 링크를 생성합니다. 쓰기 작업.

## 환경
- `GOJIRA_API` 기본값: `http://10.211.55.3:44360/api/v2`

## 인자 추출
- `CODE` — projectCode
- `USER` — user (X-User-Id)
- `FROM` — inward(상위) 이슈 키
- `TO` — outward(하위) 이슈 키
- `TYPE` — linkType. 표준값:
  - `Contains(WBSGantt)` — 포함(상위→하위)
  - `Relates` — 관련
  - `Duplicate` — 중복
  - `Blocks` — 차단

```bash
export CODE="GOJIRA3"
export USER="batman"
export FROM="GOJIRA3-10"
export TO="GOJIRA3-20"
export TYPE="Contains(WBSGantt)"
export GOJIRA_API="${GOJIRA_API:-http://10.211.55.3:44360/api/v2}"
```

## 링크 생성
```bash
curl -s -X POST "${GOJIRA_API}/Issue/link" \
  -H "Content-Type: application/json" -H "X-User-Id: ${USER}" \
  -d "$(jq -n --arg p "$CODE" --arg iw "$FROM" --arg ow "$TO" --arg lt "$TYPE" --arg u "$USER" \
    '{prjID:$p, inwardKey:$iw, outwardKey:$ow, linkType:$lt, user:$u}')"
```

결과를 사용자에게 보고. 실패 시 silent drop 금지.
