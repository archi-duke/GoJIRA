#!/usr/bin/env bash
# pr-list.sh — Platform-API /DocRepo/{code}/prs PR 목록을 JSON 배열로 stdout 에 출력한다.
#
# 사용법:
#   bash pr-list.sh <CODE> <USER> [STATE]   > /tmp/gojira_prs.json
#   STATE: open | closed | all (기본 open)
#
# 출력: [{number, title, state, author, head}]
#
# URL/헤더 조립은 이 스크립트가 전담한다 — 에이전트가 curl 을 직접 만들지 말 것.
set -euo pipefail

CODE="${1:?projectCode required}"
USER="${2:?user (X-User-Id) required}"
STATE="${3:-open}"
PLATFORM_API="${PLATFORM_API:?PLATFORM_API env var required (launcher must inject)}"

case "$STATE" in
  open|closed|all) ;;
  *) echo "ERROR: STATE 는 open|closed|all 중 하나 (입력: $STATE)" >&2; exit 1 ;;
esac

TMP=$(mktemp /tmp/.gojira_prs.XXXXXX)
trap 'rm -f "$TMP"' EXIT

curl -s "${PLATFORM_API}/DocRepo/${CODE}/prs?state=${STATE}" \
  -H "X-User-Id: ${USER}" -H "Authorization: Bearer ${GOJIRA_SESSION_TOKEN:-}" > "$TMP"

if [ "$(jq -r 'type' "$TMP" 2>/dev/null)" != "array" ]; then
  echo "ERROR: PR 조회 실패 — 응답: $(head -c 300 "$TMP")" >&2
  exit 1
fi

jq '[.[] | {number, title, state, author: (.user.login // .author // null), head: (.head.ref // null)}]' "$TMP"
