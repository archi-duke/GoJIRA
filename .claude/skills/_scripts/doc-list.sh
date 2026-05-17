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

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WT=$(bash "${SCRIPT_DIR}/_worktree-ensure.sh" "$CODE" "$USER")

# .git 자체 + .git/ 디렉토리 모두 제외. relative path 로 출력.
cd "$WT"
find . -type f -not -path './.git/*' -not -name '.git' -printf '%P\t%s\n' \
  | jq -Rn '[inputs | split("\t") | {path: .[0], size: (.[1] | tonumber)}]'
