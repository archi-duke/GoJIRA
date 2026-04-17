#!/usr/bin/env bash
# doc-list.sh — 문서 파일 목록을 JSON 으로 출력한다.
#
# 사용법:
#   bash doc-list.sh <CODE> <USER> [REF]
#
# 예:
#   bash doc-list.sh GOJIRA3 batman           # docs/batman 브랜치
#   bash doc-list.sh GOJIRA3 batman main      # main 브랜치
#
# 출력: [{path, type, size, sha}, ...]
set -euo pipefail

CODE="${1:?projectCode required}"
USER="${2:?user required}"
REF="${3:-docs/${USER}}"
PLATFORM_API="${PLATFORM_API:-http://localhost:9000/api/v2}"

curl -s "${PLATFORM_API}/DocRepo/${CODE}/tree?ref=$(printf %s "$REF" | jq -sRr @uri)" \
  -H "X-User-Id: ${USER}" \
  > /tmp/gojira_doctree.json

jq '[.tree[] | select(.type=="blob") | {path, size, sha}]' /tmp/gojira_doctree.json
