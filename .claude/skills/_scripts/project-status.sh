#!/usr/bin/env bash
# project-status.sh — 프로젝트 현재 상태를 한 번에 수집.
#
# 사용법:
#   bash project-status.sh <CODE> <USER>
#
# 출력 JSON: {code, user, nextStage, issues, docs, projectDescription, flags}
#
# 이슈 통계는 GoJIRA-API (HTTP) 로, 문서는 worktree fs 직접 read.
set -euo pipefail

CODE="${1:?projectCode required}"
USER="${2:?user required}"
GOJIRA_API="${GOJIRA_API:?GOJIRA_API env var required (launcher must inject)}"

# 0) 로컬 clone 루트 (트랙 E — 서버 worktree 제거). 런처가 GOJIRA_REPO_ROOT 주입.
ROOT="${GOJIRA_REPO_ROOT:-}"
if [ -z "$ROOT" ]; then ROOT=$(git rev-parse --show-toplevel 2>/dev/null || true); fi
if [ -z "$ROOT" ] || [ ! -d "$ROOT" ]; then
  echo "ERROR: repo 루트 미확인 — GOJIRA_REPO_ROOT 설정 또는 clone 안에서 실행" >&2
  exit 1
fi

# 1) 이슈 통계 (HTTP 유지 — DB)
curl -s "${GOJIRA_API}/Issue" \
  -H "XBody: {\"prjID\":\"${CODE}\",\"infoType\":\"\"}" \
  -H "X-User-Id: ${USER}" -H "Authorization: Bearer ${GOJIRA_SESSION_TOKEN}" > /tmp/gojira_issues.json

ISSUE_STATS=$(jq '{
  total: length,
  byType: (group_by(.issueType) | map({type: .[0].issueType, count: length}))
}' /tmp/gojira_issues.json)

# 2) 문서 목록 (로컬 clone fs)
DOC_LIST=$(cd "$ROOT" && find . -type f -not -path './.git/*' -not -name '.git' -printf '%P\n' \
  | jq -Rn '[inputs]')

# 3) project-description.md 내용 (로컬 clone fs)
PROJDESC=""
if [ -f "${ROOT}/business/project-description.md" ]; then
  PROJDESC=$(cat "${ROOT}/business/project-description.md")
fi

# 4) 워크플로우 단계 판정
TOTAL=$(echo "$ISSUE_STATS" | jq '.total')
HAS_VOS=$(echo "$ISSUE_STATS" | jq '[.byType[] | select(.type=="VOS")] | length > 0')
HAS_FEATURE=$(echo "$ISSUE_STATS" | jq '[.byType[] | select(.type=="Feature")] | length > 0')
HAS_REQ=$(echo "$ISSUE_STATS" | jq '[.byType[] | select(.type=="Requirement")] | length > 0')
HAS_DRIVER=$(jq '[.[] | select(.issueType=="Requirement" and (.driverSelected // false) == true)] | length > 0' /tmp/gojira_issues.json)
HAS_NON_STYLE_ADSN=$(jq '[.[] | select((.issueType=="Architectural Design" or .issueType=="Detailed Design") and (.designPoint // "") != "시스템 스타일 결정")] | length > 0' /tmp/gojira_issues.json)
HAS_CHOSEN_STYLES=$(curl -s "${GOJIRA_API}/Project" \
  -H "XBody: {\"jiraCode\":\"${CODE}\"}" -H "X-User-Id: ${USER}" -H "Authorization: Bearer ${GOJIRA_SESSION_TOKEN}" \
  | jq '[.[] | (.archSettings.chosenStyles.primary // [])] | flatten | length > 0' 2>/dev/null || echo "false")
PROJDESC_LEN=$(echo "$PROJDESC" | wc -c | tr -d ' ')

STAGE="unknown"
if [ "$TOTAL" = "0" ] && [ "$PROJDESC_LEN" -lt 100 ]; then
  STAGE="projdesc"
elif [ "$TOTAL" = "0" ] || [ "$HAS_VOS" = "false" ]; then
  STAGE="vos"
elif [ "$HAS_FEATURE" = "false" ]; then
  STAGE="usecase"
elif [ "$HAS_REQ" = "false" ]; then
  STAGE="requirement"
elif [ "$HAS_DRIVER" = "false" ]; then
  STAGE="driver"
elif [ "$HAS_CHOSEN_STYLES" = "false" ]; then
  STAGE="style"
elif [ "$HAS_NON_STYLE_ADSN" = "false" ]; then
  STAGE="design"
else
  STAGE="complete"
fi

jq -n \
  --arg code "$CODE" \
  --arg user "$USER" \
  --arg stage "$STAGE" \
  --argjson issues "$ISSUE_STATS" \
  --argjson docs "$DOC_LIST" \
  --arg projdesc "$PROJDESC" \
  --argjson hasDriver "$HAS_DRIVER" \
  --argjson hasChosenStyles "$HAS_CHOSEN_STYLES" \
  --argjson hasNonStyleAdsn "$HAS_NON_STYLE_ADSN" \
  '{code:$code, user:$user, nextStage:$stage, issues:$issues, docs:$docs, projectDescription:$projdesc,
    flags:{hasDriver:$hasDriver, hasChosenStyles:$hasChosenStyles, hasNonStyleAdsn:$hasNonStyleAdsn}}'
