#!/usr/bin/env bash
# _notify-change.sh — fs 직접 write 한 스킬이 SSE broadcast 를 트리거.
#
# 사용법:
#   bash _notify-change.sh <CODE> <USER> <DOCPATH> [<OP>]
#     OP: save | delete  (기본 save)
#
# 호출 측에서 fs write 완료 직후 호출.
# 실패해도 save 자체에 영향 없도록 호출자가 || true 처리 권장.
#
# 필수 환경변수: PLATFORM_API
set -euo pipefail

CODE="${1:?projectCode required}"
USER="${2:?user required}"
DOCPATH="${3:?document path required}"
OP="${4:-save}"

PLATFORM_API="${PLATFORM_API:-http://localhost:9000/api/v2}"

BODY=$(jq -n --arg p "$DOCPATH" --arg o "$OP" '{path:$p, op:$o}')
curl -sS -X POST "${PLATFORM_API}/DocRepo/${CODE}/notify-change" \
  -H "Content-Type: application/json" \
  -H "X-User-Id: ${USER}" \
  --data-binary "$BODY" >/dev/null
