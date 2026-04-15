---
name: issue-delete
description: GoJIRA 이슈를 삭제합니다. 되돌릴 수 없으므로 사용자 확인 필수.
allowed-tools: Bash
disable-model-invocation: true
---

GoJIRA-API `DELETE /Issue/{key}` 로 이슈를 삭제합니다. **파괴적 작업 — 실행 전 반드시 사용자 확인.**

## 환경
- `GOJIRA_API` 기본값: `http://localhost:9001/api/v2`

## 인자
- `USER` — X-User-Id (필수)
- `KEY` — 삭제할 이슈 키 (필수)

```bash
export USER="batman"
export KEY="GOJIRA3-30"
export GOJIRA_API="${GOJIRA_API:-http://localhost:9001/api/v2}"
```

## 삭제
```bash
curl -s -X DELETE "${GOJIRA_API}/Issue/${KEY}" \
  -H "X-User-Id: ${USER}"
```

결과(성공/실패)를 사용자에게 보고합니다.
