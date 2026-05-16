#!/usr/bin/env bash
# _edit-acquire.sh — fs 직접 write 하는 스킬용 EditSession lock + worktree 헬퍼.
#
# Platform-API 의 edit/start endpoint 를 holderKind=skill 로 호출해
# 사용자별 worktree 를 보장하고 호스트 측 절대경로를 stdout 한 줄로 출력한다.
#
# 사용법:
#   WT=$(bash _edit-acquire.sh <CODE> <USER>)
#   echo "..." > "$WT/path/to/file.md"
#
# 충돌 동작:
#   - 같은 user 가 UI 로 편집 중이면 (다른 holderKind) → exit 2 + stderr 에 안내
#   - 그 외 에러 → exit 1
#
# 필수 환경변수: PLATFORM_API, GOJIRA_REPOS_DIR
set -euo pipefail

CODE="${1:?projectCode required}"
USER="${2:?user required}"
PLATFORM_API="${PLATFORM_API:-http://localhost:9000/api/v2}"
GOJIRA_REPOS_DIR="${GOJIRA_REPOS_DIR:-${PROJECT_ROOT:-.}/repos}"

RESP=$(curl -sS -w '\n%{http_code}' -X POST \
  -H "X-User-Id: ${USER}" \
  -H "Content-Type: application/json" \
  -d '{"holderKind":"skill"}' \
  "${PLATFORM_API}/DocRepo/${CODE}/edit/start")
HTTP=$(printf '%s\n' "$RESP" | tail -1)
BODY=$(printf '%s\n' "$RESP" | sed '$d')

if [ "$HTTP" = "409" ]; then
  HK=$(echo "$BODY" | jq -r '.holderKind // "unknown"')
  echo "ERROR: ${USER} 의 EditSession 이 다른 도구(${HK})에 의해 사용 중입니다. UI 편집 종료 후 다시 시도해 주세요." >&2
  exit 2
fi
if [ "$HTTP" != "200" ]; then
  echo "ERROR: edit/start HTTP ${HTTP}: ${BODY}" >&2
  exit 1
fi

OWNER=$(echo "$BODY" | jq -r '.owner // empty')
REPO=$(echo "$BODY" | jq -r '.repo // empty')
if [ -z "$OWNER" ] || [ -z "$REPO" ]; then
  echo "ERROR: owner/repo missing in edit/start response: ${BODY}" >&2
  exit 1
fi

WT="${GOJIRA_REPOS_DIR%/}/${OWNER}/${REPO}.worktrees/${USER}"
if [ ! -d "$WT" ]; then
  echo "ERROR: worktree 경로가 호스트에 없음: ${WT}" >&2
  exit 1
fi

echo "$WT"
