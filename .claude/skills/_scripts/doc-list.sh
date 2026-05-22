#!/usr/bin/env bash
# doc-list.sh — worktree 의 문서 파일 목록을 JSON 으로 출력.
#
# 사용법:
#   bash doc-list.sh <CODE> <USER>
#
# 출력: [{path, size}, ...]   ← 기존 sha 필드는 더 이상 채우지 않음 (use git ls-tree if needed)
#
# 참고: worktree 는 docs/<USER> 브랜치 (origin/main 분기). main 의 파일도 그대로 보인다.
set -euo pipefail

CODE="${1:?projectCode required}"
USER="${2:?user required}"

# 로컬 clone 에서 직접 목록화 (트랙 E — 서버 worktree 제거). 런처가 GOJIRA_REPO_ROOT 주입.
# CODE/USER 인자는 호환 위해 받지만 로컬 read 에선 미사용.
ROOT="${GOJIRA_REPO_ROOT:-}"
if [ -z "$ROOT" ]; then ROOT=$(git rev-parse --show-toplevel 2>/dev/null || true); fi
if [ -z "$ROOT" ] || [ ! -d "$ROOT" ]; then
  echo "ERROR: repo 루트 미확인 — GOJIRA_REPO_ROOT 설정 또는 clone 안에서 실행" >&2
  exit 1
fi

# .git 자체 + .git/ 디렉토리 모두 제외. relative path 로 출력.
cd "$ROOT"
find . -type f -not -path './.git/*' -not -name '.git' -printf '%P\t%s\n' \
  | jq -Rn '[inputs | split("\t") | {path: .[0], size: (.[1] | tonumber)}]'
