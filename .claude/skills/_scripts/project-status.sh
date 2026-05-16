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
GOJIRA_API="${GOJIRA_API:-http://10.211.55.3:44360/api/v2}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# 0) worktree 보장 + 호스트 경로 획득
WT=$(bash "${SCRIPT_DIR}/_worktree-ensure.sh" "$CODE" "$USER")

# 1) 이슈 통계 (HTTP 유지 — DB)
curl -s "${GOJIRA_API}/Issue" \
  -H "XBody: {\"prjID\":\"${CODE}\",\"infoType\":\"\"}" \
  -H "X-User-Id: ${USER}" > /tmp/gojira_issues.json

ISSUE_STATS=$(jq '{
  total: length,
  byType: (group_by(.issueType) | map({type: .[0].issueType, count: length}))
}' /tmp/gojira_issues.json)

# 2) 문서 목록 (worktree fs)
DOC_LIST=$(cd "$WT" && find . -type f -not -path './.git/*' -not -name '.git' -printf '%P\n' \
  | jq -Rn '[inputs]')

# 3) project-description.md 내용 (worktree fs — main 분기이므로 main 의 내용도 보임)
PROJDESC=""
if [ -f "${WT}/business/project-description.md" ]; then
  PROJDESC=$(cat "${WT}/business/project-description.md")
fi

# 4) 워크플로우 단계 판정
TOTAL=$(echo "$ISSUE_STATS" | jq '.total')
HAS_VOS=$(echo "$ISSUE_STATS" | jq '[.byType[] | select(.type=="VOS")] | length > 0')
HAS_FEATURE=$(echo "$ISSUE_STATS" | jq '[.byType[] | select(.type=="Feature")] | length > 0')
HAS_REQ=$(echo "$ISSUE_STATS" | jq '[.byType[] | select(.type=="Requirement")] | length > 0')
HAS_DRIVER=$(jq '[.[] | select(.issueType=="Requirement" and (.driverSelected // false) == true)] | length > 0' /tmp/gojira_issues.json)
HAS_NON_STYLE_ADSN=$(jq '[.[] | select((.issueType=="Architectural Design" or .issueType=="Detailed Design") and (.designPoint // "") != "시스템 스타일 결정")] | length > 0' /tmp/gojira_issues.json)
HAS_CHOSEN_STYLES=$(curl -s "${GOJIRA_API}/Project" \
  -H "XBody: {\"jiraCode\":\"${CODE}\"}" -H "X-User-Id: ${USER}" \
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
