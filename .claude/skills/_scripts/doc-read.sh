#!/usr/bin/env bash
# doc-read.sh — worktree 에서 문서를 직접 read 해 stdout 으로 출력.
#
# 사용법:
#   bash doc-read.sh <CODE> <USER> <PATH>
#
# 예:
#   bash doc-read.sh GOJIRA3 batman business/project-description.md
#
# worktree 는 docs/<USER> 브랜치이고 origin/main 에서 분기되었기 때문에
# 사용자가 아직 변경하지 않은 파일도 main 의 내용을 그대로 가지고 있다.
# 따라서 별도의 main fallback 은 필요하지 않다.
set -euo pipefail

CODE="${1:?projectCode required}"
USER="${2:?user required}"
DOCPATH="${3:?document path required}"

# 로컬 clone 에서 직접 read (트랙 E — 서버 worktree 제거). 런처가 GOJIRA_REPO_ROOT 주입,
# 없으면 git toplevel. CODE/USER 인자는 호환 위해 받지만 로컬 read 에선 미사용.
ROOT="${GOJIRA_REPO_ROOT:-}"
if [ -z "$ROOT" ]; then ROOT=$(git rev-parse --show-toplevel 2>/dev/null || true); fi
if [ -z "$ROOT" ] || [ ! -d "$ROOT" ]; then
  echo "ERROR: repo 루트 미확인 — GOJIRA_REPO_ROOT 설정 또는 clone 안에서 실행" >&2
  exit 1
fi

FILE="${ROOT}/${DOCPATH}"
if [ ! -f "$FILE" ]; then
  echo "ERROR: file not found: ${DOCPATH} (root=${ROOT})" >&2
  exit 1
fi

cat "$FILE"
