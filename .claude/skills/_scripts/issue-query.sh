#!/usr/bin/env bash
# issue-query.sh — 로컬 clone data/issues/*.yaml 을 GET /Issue 응답과 동일한 JSON
# 배열로 stdout 에 출력한다.
#
# 배경: 이슈 쓰기(POST/PUT /Issue, X-Local-Write:true)는 Mongo 를 건드리지 않고
# 로컬 clone 에만 YAML 을 기록하고, Mongo 는 PR 머지 시점에만 갱신된다. 그래서 세션
# 중(PR 머지 전) `curl ${GOJIRA_API}/Issue` 로 조회하면 방금 만든 이슈가 전혀 안
# 보인다 — 이슈 조회는 반드시 이 스크립트로 로컬 clone 에서 읽어야 한다.
#
# 사용법:
#   bash issue-query.sh > /tmp/gojira_issues.json
set -euo pipefail

DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

ROOT="${GOJIRA_REPO_ROOT:-}"
if [ -z "$ROOT" ]; then ROOT=$(git rev-parse --show-toplevel 2>/dev/null || true); fi
if [ -z "$ROOT" ] || [ ! -d "$ROOT" ]; then
  echo "ERROR: repo 루트 미확인 — GOJIRA_REPO_ROOT 설정 또는 clone 안에서 실행" >&2
  exit 1
fi

python3 -c "
import sys, json
sys.path.insert(0, sys.argv[1])
import issue_local
print(json.dumps(issue_local.load_local_issues(sys.argv[2])))
" "$DIR" "$ROOT"
