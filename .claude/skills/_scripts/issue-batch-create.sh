#!/usr/bin/env bash
# issue-batch-create.sh — 이슈를 순차 생성하고 실패 시 재시도한다.
#
# 사용법:
#   bash issue-batch-create.sh <CODE> <USER> < items.json
#
# 입력(stdin): JSON array
#   [{"issueType":"VOS","summary":"...","desc":"...","client":"alice"}, ...]
#   필수: issueType, summary, desc
#   옵션: client / priority / assignee — 생성 후 PUT 으로 업데이트됨
#
# 출력(stdout): JSON
#   {"total":N,"created":K,"failed":F,"keys":[...],"errors":[...]}
#
# 재시도 정책: 5xx / 422(already exists) / 네트워크 에러 → 최대 5회 지수 백오프 (0.2/0.4/0.8/1.6/3.2s)
set -euo pipefail

CODE="${1:?projectCode required}"
USER="${2:?user knoxID required}"
GOJIRA_API="${GOJIRA_API:-http://localhost:9001/api/v2}"

INPUT=$(cat)
COUNT=$(jq 'length' <<< "$INPUT")

CREATED=0
FAILED=0
KEYS=()
ERRORS=()

for i in $(seq 0 $((COUNT - 1))); do
  ITEM=$(jq ".[$i]" <<< "$INPUT")
  SUM=$(jq -r '.summary' <<< "$ITEM")
  BODY=$(jq --arg p "$CODE" --arg u "$USER" \
    '{prjID:$p, user:$u, issueType:.issueType, summary:.summary, desc:.desc}
     + (if .reqType then {reqType:.reqType} else {} end)
     + (if .labels then {labels:.labels} else {} end)' <<< "$ITEM")

  RESP=""
  STATUS=""
  for attempt in 1 2 3 4 5; do
    TMP=$(mktemp)
    STATUS=$(curl -s -o "$TMP" -w '%{http_code}' -X POST "${GOJIRA_API}/Issue" \
      -H "Content-Type: application/json" -H "X-User-Id: ${USER}" \
      --data-binary "$BODY" || echo 000)
    RESP=$(cat "$TMP")
    rm -f "$TMP"
    if [[ "$STATUS" =~ ^2[0-9][0-9]$ ]]; then
      break
    fi
    # 재시도 대상: 5xx / 422 / 000(네트워크)
    if [[ "$STATUS" =~ ^(5[0-9][0-9]|422|000)$ ]]; then
      sleep $(awk "BEGIN{print 0.2 * (2 ^ ($attempt - 1))}")
      continue
    fi
    break
  done

  if [[ "$STATUS" =~ ^2[0-9][0-9]$ ]]; then
    KEY=$(jq -r '.key // empty' <<< "$RESP")
    SUBKEY=$(jq -r '.subkey // empty' <<< "$RESP")
    if [[ -z "$KEY" ]]; then
      FAILED=$((FAILED+1))
      ERRORS+=("$(jq -n --arg s "$SUM" --arg e "invalid response: $RESP" '{summary:$s,error:$e}')")
      continue
    fi
    CREATED=$((CREATED+1))
    KEYS+=("$KEY")

    # 선택 필드 PUT 업데이트
    EXTRA=$(jq '{
      client: (.client // ""),
      priority: (.priority // ""),
      assignee: (.assignee // "")
    } | with_entries(select(.value != ""))' <<< "$ITEM")
    if [[ "$(jq 'length' <<< "$EXTRA")" != "0" ]]; then
      curl -s -o /dev/null -X PUT "${GOJIRA_API}/Issue" \
        -H "Content-Type: application/json" -H "X-User-Id: ${USER}" \
        --data-binary "$(jq -n --arg p "$CODE" --arg k "$KEY" --arg u "$USER" --argjson d "$EXTRA" \
          '{prjID:$p, key:$k, user:$u, data:$d}')" || true
    fi
  else
    FAILED=$((FAILED+1))
    ERRORS+=("$(jq -n --arg s "$SUM" --arg st "$STATUS" --arg e "$RESP" '{summary:$s,status:$st,error:$e}')")
  fi

  # 요청 간 약간의 간격 — Gitea push race 완화
  sleep 0.15
done

jq -n \
  --argjson total "$COUNT" \
  --argjson created "$CREATED" \
  --argjson failed "$FAILED" \
  --argjson keys "$(printf '%s\n' "${KEYS[@]:-}" | jq -R . | jq -s 'map(select(length>0))')" \
  --argjson errors "$(printf '%s\n' "${ERRORS[@]:-}" | jq -s 'map(select(type=="object"))')" \
  '{total:$total, created:$created, failed:$failed, keys:$keys, errors:$errors}'
