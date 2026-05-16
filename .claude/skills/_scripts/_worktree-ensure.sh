#!/usr/bin/env bash
# _worktree-ensure.sh — fs 직접 read 하는 스킬용 헬퍼.
#
# Platform-API 의 worktree-info endpoint 를 호출해 worktree 를 보장하고
# 호스트 측 절대경로를 stdout 한 줄로 출력한다. EditSession lock 은 잡지 않는다.
#
# 사용법:
#   WT=$(bash _worktree-ensure.sh <CODE> <USER>)
#   cat "$WT/business/project-description.md"
#
# 필수 환경변수:
#   PLATFORM_API       — 예: http://localhost:9000/api/v2
#   GOJIRA_REPOS_DIR   — 호스트 측 bare clone + worktree 루트 (Platform-API 와 공유)
set -euo pipefail

CODE="${1:?projectCode required}"
USER="${2:?user required}"
PLATFORM_API="${PLATFORM_API:-http://localhost:9000/api/v2}"
GOJIRA_REPOS_DIR="${GOJIRA_REPOS_DIR:-${PROJECT_ROOT:-.}/repos}"

RESP=$(curl -sS -w '\n%{http_code}' \
  -H "X-User-Id: ${USER}" \
  "${PLATFORM_API}/DocRepo/${CODE}/worktree-info")
HTTP=$(printf '%s\n' "$RESP" | tail -1)
BODY=$(printf '%s\n' "$RESP" | sed '$d')

if [ "$HTTP" != "200" ]; then
  echo "ERROR: worktree-info HTTP ${HTTP}: ${BODY}" >&2
  exit 1
fi

REL=$(echo "$BODY" | jq -r '.worktreeRelPath // empty')
if [ -z "$REL" ]; then
  echo "ERROR: worktreeRelPath missing in response: ${BODY}" >&2
  exit 1
fi

WT="${GOJIRA_REPOS_DIR%/}/${REL}"
if [ ! -d "$WT" ]; then
  echo "ERROR: worktree 경로가 호스트에 없음: ${WT} (GOJIRA_REPOS_DIR 가 Platform-API 의 bind mount 와 같은 디렉토리를 가리키는지 확인)" >&2
  exit 1
fi

echo "$WT"
