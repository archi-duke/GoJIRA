#!/usr/bin/env bash
# issue-batch-link.sh — 이슈 링크를 순차 생성한다.
#
# 사용법:
#   bash issue-batch-link.sh <CODE> <USER> [SOURCE] < links.json
#
# 입력(stdin): JSON array
#   [{"inwardKey":"GOJIRA3-1","outwardKey":"GOJIRA3-10","linkType":"Contains(WBSGantt)"}, ...]
#
# SOURCE (선택): 호출 SKILL 이름 (예: "requirement-refine2"). 전달 시 link 에 _addedBy 메타
#   부착되어 drift-check 가 "정상 워크플로우 자동 생성" 으로 분류 → hash 에서 제외.
#   미전달/빈 값이면 메타 없음 = 사용자 수작업 link 와 동일 처리.
#
# 출력(stdout): JSON
#   {"total":N,"created":K,"failed":F,"errors":[...]}
#
# 재시도: 5xx → 최대 3회 지수 백오프
set -euo pipefail

CODE="${1:?projectCode required}"
USER="${2:?user required}"
SOURCE="${3:-}"
GOJIRA_API="${GOJIRA_API:-http://10.211.55.3:44360/api/v2}"

INPUT=$(cat)
COUNT=$(jq 'length' <<< "$INPUT")

CREATED=0
FAILED=0
ERRORS=()

for i in $(seq 0 $((COUNT - 1))); do
  ITEM=$(jq ".[$i]" <<< "$INPUT")
  IW=$(jq -r '.inwardKey' <<< "$ITEM")
  OW=$(jq -r '.outwardKey' <<< "$ITEM")
  LT=$(jq -r '.linkType // "Contains(WBSGantt)"' <<< "$ITEM")

  BODY=$(jq -n --arg p "$CODE" --arg iw "$IW" --arg ow "$OW" --arg lt "$LT" --arg u "$USER" --arg src "$SOURCE" \
    'if $src == "" then
       {prjID:$p, inwardKey:$iw, outwardKey:$ow, linkType:$lt, user:$u}
     else
       {prjID:$p, inwardKey:$iw, outwardKey:$ow, linkType:$lt, user:$u, source:$src}
     end')

  STATUS=""
  for attempt in 1 2 3; do
    TMP=$(mktemp)
    STATUS=$(curl -s -o "$TMP" -w '%{http_code}' -X POST "${GOJIRA_API}/Issue/link" \
      -H "Content-Type: application/json" -H "X-User-Id: ${USER}" \
      --data-binary "$BODY" || echo 000)
    RESP=$(cat "$TMP")
    rm -f "$TMP"
    if [[ "$STATUS" =~ ^2[0-9][0-9]$ ]]; then
      break
    fi
    if [[ "$STATUS" =~ ^(5[0-9][0-9]|000)$ ]]; then
      sleep $(awk "BEGIN{print 0.2 * (2 ^ ($attempt - 1))}")
      continue
    fi
    break
  done

  if [[ "$STATUS" =~ ^2[0-9][0-9]$ ]]; then
    CREATED=$((CREATED+1))
  else
    FAILED=$((FAILED+1))
    ERRORS+=("$(jq -n --arg iw "$IW" --arg ow "$OW" --arg st "$STATUS" --arg e "$RESP" '{inward:$iw,outward:$ow,status:$st,error:$e}')")
  fi

  sleep 0.1
done

jq -n \
  --argjson total "$COUNT" \
  --argjson created "$CREATED" \
  --argjson failed "$FAILED" \
  --argjson errors "$(printf '%s\n' "${ERRORS[@]:-}" | jq -s 'map(select(type=="object"))')" \
  '{total:$total, created:$created, failed:$failed, errors:$errors}'
