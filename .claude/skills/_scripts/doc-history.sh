#!/usr/bin/env bash
# doc-history.sh — Platform-API /DocRepo/{code}/commits 커밋 이력을 JSON 배열로 stdout 에 출력한다.
#
# 사용법:
#   bash doc-history.sh <CODE> [BRANCH] [LIMIT]   > /tmp/gojira_commits.json
#   BRANCH: 기본 main. 사용자 브랜치는 docs/{user}
#   LIMIT : 기본 20
#
# 출력: [{sha, message(첫 줄), author, date}]  — 조회 실패/비배열 응답이면 에러 종료.
#
# URL/헤더 조립(branch URL 인코딩 포함)은 이 스크립트가 전담한다 — 에이전트가 curl 을 직접 만들지 말 것.
set -euo pipefail

CODE="${1:?projectCode required}"
BRANCH="${2:-main}"
LIMIT="${3:-20}"
PLATFORM_API="${PLATFORM_API:?PLATFORM_API env var required (launcher must inject)}"

TMP=$(mktemp /tmp/.gojira_commits.XXXXXX)
trap 'rm -f "$TMP"' EXIT

curl -s "${PLATFORM_API}/DocRepo/${CODE}/commits?branch=$(printf %s "$BRANCH" | jq -sRr @uri)&perPage=${LIMIT}" \
  -H "Authorization: Bearer ${GOJIRA_SESSION_TOKEN:-}" > "$TMP"

if [ "$(jq -r 'type' "$TMP" 2>/dev/null)" != "array" ]; then
  echo "ERROR: 커밋 조회 실패 — 응답: $(head -c 300 "$TMP")" >&2
  exit 1
fi

jq '[.[] | {
  sha: (.sha // "")[0:7],
  message: ((.commit.message // "") | split("\n")[0]),
  author: (.commit.author.name // ""),
  date: (.commit.author.date // "")
}]' "$TMP"
