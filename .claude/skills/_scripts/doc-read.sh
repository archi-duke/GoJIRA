#!/usr/bin/env bash
# doc-read.sh — 문서 파일 내용을 읽어 stdout 으로 출력한다.
#
# 사용법:
#   bash doc-read.sh <CODE> <USER> <PATH>
#
# 예:
#   bash doc-read.sh GOJIRA3 batman business/project-description.md
#
# docs/{USER} 브랜치 우선, fallback main.
set -euo pipefail

CODE="${1:?projectCode required}"
USER="${2:?user required}"
DOCPATH="${3:?document path required}"
PLATFORM_API="${PLATFORM_API:-http://localhost:9000/api/v2}"

# docs/{user} 우선
CONTENT=$(curl -s "${PLATFORM_API}/DocRepo/${CODE}/file?path=$(printf %s "$DOCPATH" | jq -sRr @uri)&ref=docs/${USER}" \
  -H "X-User-Id: ${USER}" | jq -r '.content // empty')

if [ -n "$CONTENT" ]; then
  echo "$CONTENT" | base64 -d 2>/dev/null || echo "$CONTENT"
  exit 0
fi

# fallback: main
CONTENT=$(curl -s "${PLATFORM_API}/DocRepo/${CODE}/file?path=$(printf %s "$DOCPATH" | jq -sRr @uri)&ref=main" \
  -H "X-User-Id: ${USER}" | jq -r '.content // empty')

if [ -n "$CONTENT" ]; then
  echo "$CONTENT" | base64 -d 2>/dev/null || echo "$CONTENT"
  exit 0
fi

echo "ERROR: file not found: ${DOCPATH}" >&2
exit 1
