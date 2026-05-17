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

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WT=$(bash "${SCRIPT_DIR}/_worktree-ensure.sh" "$CODE" "$USER")

FILE="${WT}/${DOCPATH}"
if [ ! -f "$FILE" ]; then
  echo "ERROR: file not found: ${DOCPATH} (worktree=${WT})" >&2
  exit 1
fi

cat "$FILE"
