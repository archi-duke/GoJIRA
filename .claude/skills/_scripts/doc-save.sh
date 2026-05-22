#!/usr/bin/env bash
# doc-save.sh — 로컬 clone 작업본에 문서를 직접 write (local-agent 모델).
#
# 사용법:
#   bash doc-save.sh <CODE> <USER> <PATH> < content.md
#   bash doc-save.sh <CODE> <USER> <PATH> --file /tmp/content.md
#
# 흐름 (로컬 전용 — 서버 왕복 없음):
#   1) repo 루트 결정: GOJIRA_REPO_ROOT (런처 주입) 우선, 없으면 git toplevel
#   2) PlantUML 자동 수정 (md 파일, DOC_FIX=0 으로 비활성화)
#   3) ${ROOT}/${PATH} 에 fs 직접 write (mkdir -p)
#   4) post-save sanity 검사 (DOC_POST_VALIDATE=0 으로 비활성화)
#
# 동시성:
#   - 로컬 작업본은 단일 writer (사용자 PC 의 clone 한 개) 이므로 lock 불필요.
#   - 서버 EditSession lock / SSE notify 는 사용하지 않는다.
#
# CODE/USER 인자는 8개 doc 스킬 호환을 위해 유지(현재 미사용 가능).
set -euo pipefail

CODE="${1:?projectCode required}"
USER="${2:?user required}"
DOCPATH="${3:?document path required (e.g. business/use-cases.md)}"
shift 3

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# 본문 읽기
if [[ "${1:-}" == "--file" ]]; then
  CONTENT=$(cat "${2:?file path required}")
else
  CONTENT=$(cat)
fi

# 1) repo 루트 결정 — 런처가 GOJIRA_REPO_ROOT 주입, 없으면 git toplevel fallback
ROOT="${GOJIRA_REPO_ROOT:-}"
if [[ -z "$ROOT" ]]; then
  ROOT=$(git rev-parse --show-toplevel 2>/dev/null || true)
fi
if [[ -z "$ROOT" || ! -d "$ROOT" ]]; then
  echo "ERROR: repo 루트를 찾을 수 없음 — GOJIRA_REPO_ROOT 를 설정하거나 clone 안에서 실행하세요." >&2
  exit 1
fi

# 2) PlantUML 결정적 자동 수정 (md 파일)
if [[ "${DOC_FIX:-1}" == "1" && "$DOCPATH" == *.md ]]; then
  TMPMD=$(mktemp)
  printf '%s' "$CONTENT" > "$TMPMD"
  python3 "${SCRIPT_DIR}/plantuml-fix.py" "$TMPMD" >/dev/null 2>&1 || true
  CONTENT=$(cat "$TMPMD")
  rm -f "$TMPMD"
fi

# 3) fs write (단일 writer — flock 불필요)
FILE="${ROOT}/${DOCPATH}"
mkdir -p "$(dirname "$FILE")"
printf '%s' "$CONTENT" > "$FILE"

# 4) post-save sanity 검사
WARNINGS_JSON="[]"
if [[ "${DOC_POST_VALIDATE:-1}" == "1" && "$DOCPATH" == *.md ]]; then
  WARNS=()
  START_COUNT=$(printf '%s' "$CONTENT" | grep -c '@startuml' || true)
  END_COUNT=$(printf '%s' "$CONTENT" | grep -c '@enduml' || true)
  if [[ "$START_COUNT" != "$END_COUNT" ]]; then
    WARNS+=("plantuml_pair_mismatch: @startuml=$START_COUNT @enduml=$END_COUNT")
  fi
  OPEN_FENCE=$(printf '%s' "$CONTENT" | grep -c '^\x60\x60\x60plantuml' || true)
  if [[ "$OPEN_FENCE" -gt "$START_COUNT" ]]; then
    WARNS+=("plantuml_fence_extra: fence_open=$OPEN_FENCE startuml=$START_COUNT")
  fi
  PLACEHOLDER=$(printf '%s' "$CONTENT" | grep -c '미작성 — critical' || true)
  if [[ "$PLACEHOLDER" -gt 0 ]]; then
    WARNS+=("placeholder_remaining: '미작성 — critical' x$PLACEHOLDER — assemble 회귀 가능성")
  fi
  if [[ ${#WARNS[@]} -gt 0 ]]; then
    printf '[doc-save] post-save warnings for %s:\n' "$DOCPATH" >&2
    for w in "${WARNS[@]}"; do
      printf '  - %s\n' "$w" >&2
    done
    WARNINGS_JSON=$(printf '%s\n' "${WARNS[@]}" | jq -R . | jq -s .)
  fi
fi

BYTES=$(wc -c < "$FILE" | tr -d ' ')
jq -n --arg path "$DOCPATH" --arg status "200" --argjson bytes "$BYTES" --argjson warnings "$WARNINGS_JSON" \
  '{ok:true, path:$path, status:$status, bytes:$bytes, warnings:$warnings}'
