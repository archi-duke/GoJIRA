#!/usr/bin/env bash
# project-status.sh — 프로젝트 현재 상태를 한 번에 수집한다.
#
# 사용법:
#   bash project-status.sh <CODE> <USER>
#
# 출력: JSON
#   {project, issues, docs, projectDescription}
set -euo pipefail

CODE="${1:?projectCode required}"
USER="${2:?user required}"
GOJIRA_API="${GOJIRA_API:-http://localhost:9001/api/v2}"
PLATFORM_API="${PLATFORM_API:-http://localhost:9000/api/v2}"

# 1) 이슈 통계
curl -s "${GOJIRA_API}/Issue" \
  -H "XBody: {\"prjID\":\"${CODE}\",\"infoType\":\"\"}" \
  -H "X-User-Id: ${USER}" > /tmp/gojira_issues.json

ISSUE_STATS=$(jq '{
  total: length,
  byType: (group_by(.issueType) | map({type: .[0].issueType, count: length}))
}' /tmp/gojira_issues.json)

# 2) 문서 목록
DOC_LIST=$(curl -s "${PLATFORM_API}/DocRepo/${CODE}/tree?ref=docs/${USER}" \
  -H "X-User-Id: ${USER}" | jq '[.tree[] | select(.type=="blob") | .path]' 2>/dev/null || echo '[]')

# 3) project-description.md 내용
PROJDESC=""
RAW=$(curl -s "${PLATFORM_API}/DocRepo/${CODE}/file?path=business/project-description.md&ref=docs/${USER}" \
  -H "X-User-Id: ${USER}" | jq -r '.content // empty')
if [ -n "$RAW" ]; then
  PROJDESC=$(echo "$RAW" | base64 -d 2>/dev/null || echo "$RAW")
fi
if [ -z "$PROJDESC" ] || [ "$(echo "$PROJDESC" | wc -c)" -lt 50 ]; then
  RAW=$(curl -s "${PLATFORM_API}/DocRepo/${CODE}/file?path=business/project-description.md&ref=main" \
    -H "X-User-Id: ${USER}" | jq -r '.content // empty')
  if [ -n "$RAW" ]; then
    PROJDESC=$(echo "$RAW" | base64 -d 2>/dev/null || echo "$RAW")
  fi
fi

# 4) 워크플로우 단계 판정
STAGE="unknown"
TOTAL=$(echo "$ISSUE_STATS" | jq '.total')
HAS_VOS=$(echo "$ISSUE_STATS" | jq '[.byType[] | select(.type=="VOS")] | length > 0')
HAS_FEATURE=$(echo "$ISSUE_STATS" | jq '[.byType[] | select(.type=="Feature")] | length > 0')
HAS_REQ=$(echo "$ISSUE_STATS" | jq '[.byType[] | select(.type=="Requirement")] | length > 0')
HAS_DRIVER=$(jq '[.[] | select(.issueType=="Requirement" and (.driverSelected // false) == true)] | length > 0' /tmp/gojira_issues.json)
# style-define2 는 designPoint="시스템 스타일 결정" ADSN 을 만들고, design-decide2 는 그 외 designPoint 로 ADSN/DDSN 을 만든다.
# 설계 결정 완료 판정에는 시스템 스타일용 ADSN 을 제외해야 함.
HAS_NON_STYLE_ADSN=$(jq '[.[] | select((.issueType=="Architectural Design" or .issueType=="Detailed Design") and (.designPoint // "") != "시스템 스타일 결정")] | length > 0' /tmp/gojira_issues.json)
# 스타일 선정 여부는 archSettings.chosenStyles.primary 존재로 판정
HAS_CHOSEN_STYLES=$(curl -s "${GOJIRA_API}/Project" \
  -H "XBody: {\"jiraCode\":\"${CODE}\"}" -H "X-User-Id: ${USER}" \
  | jq '[.[] | (.archSettings.chosenStyles.primary // [])] | flatten | length > 0' 2>/dev/null || echo "false")
PROJDESC_LEN=$(echo "$PROJDESC" | wc -c | tr -d ' ')

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
