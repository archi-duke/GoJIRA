#!/usr/bin/env bash
# doc-save.sh — DocRepo 에 문서를 저장한다 (docs/{user} 브랜치).
#
# 사용법:
#   bash doc-save.sh <CODE> <USER> <PATH> < content.md
#   bash doc-save.sh <CODE> <USER> <PATH> --file /tmp/content.md
#
# 예:
#   echo "# hello" | bash doc-save.sh GOJIRA3 batman business/use-cases.md
#   bash doc-save.sh GOJIRA3 batman business/use-cases.md --file /tmp/uc.md
#
# 재시도: 5xx → 최대 5회 지수 백오프
set -euo pipefail

CODE="${1:?projectCode required}"
USER="${2:?user required}"
DOCPATH="${3:?document path required (e.g. business/use-cases.md)}"
shift 3

PLATFORM_API="${PLATFORM_API:-http://localhost:9000/api/v2}"

# 본문 읽기
if [[ "${1:-}" == "--file" ]]; then
  CONTENT=$(cat "${2:?file path required}")
else
  CONTENT=$(cat)
fi

# .md 파일이면 PlantUML 결정적 자동 수정 (~30ms, 안전 — 변경 없으면 no-op)
# DOC_FIX=0 환경변수로 비활성화 가능
if [[ "${DOC_FIX:-1}" == "1" && "$DOCPATH" == *.md ]]; then
  TMPMD=$(mktemp)
  printf '%s' "$CONTENT" > "$TMPMD"
  python3 "$(dirname "$0")/plantuml-fix.py" "$TMPMD" >/dev/null 2>&1 || true
  CONTENT=$(cat "$TMPMD")
  rm -f "$TMPMD"
fi

# base64 불필요 — Platform-API PUT /DocRepo/{code}/file 은 plain text content 수용
BODY=$(jq -n --arg p "$DOCPATH" --arg c "$CONTENT" --arg m "[doc] ${DOCPATH} 저장" \
  '{path:$p, content:$c, message:$m}')

STATUS=""
RESP=""
for attempt in 1 2 3 4 5; do
  TMP=$(mktemp)
  STATUS=$(curl -s -o "$TMP" -w '%{http_code}' -X PUT "${PLATFORM_API}/DocRepo/${CODE}/file" \
    -H "Content-Type: application/json" -H "X-User-Id: ${USER}" \
    --data-binary "$BODY" || echo 000)
  RESP=$(cat "$TMP")
  rm -f "$TMP"
  if [[ "$STATUS" =~ ^2[0-9][0-9]$ ]]; then
    break
  fi
  if [[ "$STATUS" =~ ^(5[0-9][0-9]|000)$ ]]; then
    sleep $(awk "BEGIN{print 0.3 * (2 ^ ($attempt - 1))}")
    continue
  fi
  break
done

if [[ "$STATUS" =~ ^2[0-9][0-9]$ ]]; then
  jq -n --arg path "$DOCPATH" --arg status "$STATUS" '{ok:true, path:$path, status:$status}'
else
  jq -n --arg path "$DOCPATH" --arg status "$STATUS" --arg error "$RESP" '{ok:false, path:$path, status:$status, error:$error}'
fi
