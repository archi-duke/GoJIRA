#!/usr/bin/env bash
# _edit-release.sh — 스킬이 잡고 있던 EditSession lock 을 명시적으로 해제.
#
# 사용법:
#   bash _edit-release.sh <CODE> <USER>
#
# 스킬 작업 마지막에 호출. 호출 안 해도 30분 TTL 후 자동 해제되지만,
# 같은 사용자가 UI 로 옮겨가 곧바로 편집을 시작하려면 명시 해제가 필요.
set -euo pipefail

CODE="${1:?projectCode required}"
USER="${2:?user required}"
PLATFORM_API="${PLATFORM_API:-http://10.211.55.3:44350/api/v2}"

curl -sS -X POST \
  -H "X-User-Id: ${USER}" \
  "${PLATFORM_API}/DocRepo/${CODE}/edit/release" >/dev/null
echo "released"
