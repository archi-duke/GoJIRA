---
name: issue-delete
description: GoJIRA 이슈를 삭제합니다. 되돌릴 수 없으므로 사용자 확인 필수.
allowed-tools: Bash
disable-model-invocation: true
---

GoJIRA-API `DELETE /Issue` (body 에 prjID/key/user) 로 이슈를 삭제합니다. **파괴적 작업 — 실행 전 반드시 사용자 확인.**

## 환경
- `GOJIRA_API` 기본값: `http://localhost:9001/api/v2`

## 인자
- `CODE` — projectCode (필수)
- `USER` — X-User-Id (필수)
- `KEY` — 삭제할 이슈 키 (필수)

```bash
export CODE="GOJIRA3"
export USER="batman"
export KEY="GOJIRA3-30"
export GOJIRA_API="${GOJIRA_API:-http://localhost:9001/api/v2}"
```

## 삭제
⚠️ **`DELETE /Issue/{KEY}` path 방식 금지** — 404 발생. 반드시 body 에 JSON 으로 전달.

```bash
curl -s -X DELETE "${GOJIRA_API}/Issue" \
  -H "Content-Type: application/json" \
  -H "X-User-Id: ${USER}" \
  -d "{\"prjID\":\"${CODE}\",\"key\":\"${KEY}\",\"user\":\"${USER}\"}"
```

결과(성공/실패)를 사용자에게 보고합니다.
