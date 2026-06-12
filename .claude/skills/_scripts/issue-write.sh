#!/usr/bin/env bash
# issue-write.sh — 단건 이슈 write(생성/수정/삭제/링크) 로컬화 래퍼 (트랙 E).
#
# raw `curl -X POST/PUT/DELETE ${GOJIRA_API}/Issue...` 대신 이 스크립트를 쓰면:
#   1) X-Local-Write: true 헤더로 호출 → 서버가 worktree 에 손대지 않고 YAML 본문을 응답에 동봉
#   2) 응답의 localWrites 를 로컬 clone(GOJIRA_REPO_ROOT|git toplevel) 에 기록/삭제
#   3) 서버 응답 JSON 을 그대로 stdout 으로 출력 (key/subkey 등 후속 파싱용)
#
# 사용법:
#   echo '<body-json>' | bash issue-write.sh <METHOD> <path>
#     METHOD : POST | PUT | DELETE
#     path   : /Issue | /Issue/link  (GOJIRA_API 기준 상대경로)
# 예)
#   echo '{"prjID":"X","key":"X-1","user":"u","data":{"summary":"..."}}' \
#     | bash issue-write.sh PUT /Issue
#   echo '{"key":"X-1","user":"u"}' | bash issue-write.sh DELETE /Issue
set -euo pipefail

DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
source "${DIR}/_local-write.sh"

METHOD="${1:?METHOD required (POST|PUT|DELETE)}"
APIPATH="${2:?path required (/Issue|/Issue/link)}"
GOJIRA_API="${GOJIRA_API:-${GOJIRA_API_URL:-http://localhost:9001/api/v2}}"

BODY=$(cat)
USER="${GOJIRA_USER:-}"
[[ -z "$USER" ]] && USER=$(jq -r '.user // empty' <<< "$BODY" 2>/dev/null || true)

TMP=$(mktemp)
# body 는 stdin(@-)으로 — argv 전달 시 Windows 네이티브 curl 이 UTF-8→ANSI 변환해 한글 깨짐.
STATUS=$(curl -s -o "$TMP" -w '%{http_code}' -X "$METHOD" "${GOJIRA_API}${APIPATH}" \
  -H "Content-Type: application/json" -H "X-User-Id: ${USER}" -H "X-Local-Write: true" \
  --data-binary @- <<< "$BODY" || echo 000)
RESP=$(cat "$TMP")
rm -f "$TMP"

if [[ "$STATUS" =~ ^2[0-9][0-9]$ ]]; then
  gj_apply_local_writes "$RESP"
  printf '%s' "$RESP"
else
  echo "ERROR: $METHOD ${APIPATH} → HTTP $STATUS: $RESP" >&2
  exit 1
fi
