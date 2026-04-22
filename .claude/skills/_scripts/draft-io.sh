#!/usr/bin/env bash
# draft-io.sh — generation 경계를 만들기 위한 중간 산출물 파일 I/O.
#
# 사용법:
#   # 저장 (stdin 에서 JSON/텍스트 읽어 파일로 flush)
#   echo "$DRAFT_JSON" | bash draft-io.sh save <CODE> <SKILL> <PHASE>
#
#   # 로드 (파일 내용을 stdout 으로 cat)
#   bash draft-io.sh load <CODE> <SKILL> <PHASE>
#
# 출력(save): JSON `{"path":"/tmp/...","bytes":N}` to stdout
# 출력(load): 파일 원문 그대로 to stdout (파일 없으면 exit 1)
#
# 파일 위치: /tmp/${CODE}-${SKILL}-${PHASE}.json
# (draft/validated/final 등 PHASE 자유 명명)
set -euo pipefail

MODE="${1:?mode required (save|load)}"
CODE="${2:?CODE required}"
SKILL="${3:?SKILL required}"
PHASE="${4:?PHASE required}"

FILE="/tmp/${CODE}-${SKILL}-${PHASE}.json"

case "$MODE" in
  save)
    TMP=$(mktemp)
    cat > "$TMP"
    mv "$TMP" "$FILE"
    BYTES=$(wc -c < "$FILE" | tr -d ' ')
    jq -n --arg path "$FILE" --argjson bytes "$BYTES" \
      '{path:$path, bytes:$bytes}'
    ;;
  load)
    if [[ ! -f "$FILE" ]]; then
      echo "ERROR: $FILE not found" >&2
      exit 1
    fi
    cat "$FILE"
    ;;
  *)
    echo "ERROR: unknown mode '$MODE' (use save|load)" >&2
    exit 2
    ;;
esac
