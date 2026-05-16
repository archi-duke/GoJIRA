#!/usr/bin/env bash
# doc-save.sh — 사용자별 worktree 에 문서를 직접 write + SSE notify.
#
# 사용법:
#   bash doc-save.sh <CODE> <USER> <PATH> < content.md
#   bash doc-save.sh <CODE> <USER> <PATH> --file /tmp/content.md
#
# 흐름:
#   1) EditSession lock acquire (holderKind=skill)  — UI 와 동시 편집 방지
#   2) PlantUML 자동 수정 (md 파일, DOC_FIX=0 으로 비활성화)
#   3) ${WT}/${PATH} 에 fs 직접 write (mkdir -p)
#   4) Platform-API notify-change 호출 — SSE broadcast 트리거 (UI DocEditor 갱신)
#   5) post-save sanity 검사 (DOC_POST_VALIDATE=0 으로 비활성화)
#
# lock 은 자동 release 하지 않음 — 작업 종료 시 _edit-release.sh 명시 호출 권장.
# 같은 user 의 reentry 는 heartbeat 갱신이므로 매 save 마다 호출해도 부담 없음.
set -euo pipefail

CODE="${1:?projectCode required}"
USER="${2:?user required}"
DOCPATH="${3:?document path required (e.g. business/use-cases.md)}"
shift 3

PLATFORM_API="${PLATFORM_API:-http://localhost:9000/api/v2}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# 본문 읽기
if [[ "${1:-}" == "--file" ]]; then
  CONTENT=$(cat "${2:?file path required}")
else
  CONTENT=$(cat)
fi

# 1) lock acquire + worktree 경로 획득
WT=$(bash "${SCRIPT_DIR}/_edit-acquire.sh" "$CODE" "$USER")

# 2) PlantUML 결정적 자동 수정 (md 파일)
if [[ "${DOC_FIX:-1}" == "1" && "$DOCPATH" == *.md ]]; then
  TMPMD=$(mktemp)
  printf '%s' "$CONTENT" > "$TMPMD"
  python3 "${SCRIPT_DIR}/plantuml-fix.py" "$TMPMD" >/dev/null 2>&1 || true
  CONTENT=$(cat "$TMPMD")
  rm -f "$TMPMD"
fi

# 3) fs write
FILE="${WT}/${DOCPATH}"
mkdir -p "$(dirname "$FILE")"
printf '%s' "$CONTENT" > "$FILE"

# 4) SSE broadcast 트리거
NOTIFY_BODY=$(jq -n --arg p "$DOCPATH" '{path:$p, op:"save"}')
curl -sS -X POST "${PLATFORM_API}/DocRepo/${CODE}/notify-change" \
  -H "Content-Type: application/json" \
  -H "X-User-Id: ${USER}" \
  --data-binary "$NOTIFY_BODY" >/dev/null || true   # broadcast 실패는 save 자체에 영향 없음

# 5) post-save sanity 검사
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
