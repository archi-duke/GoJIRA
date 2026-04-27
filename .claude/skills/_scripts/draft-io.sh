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
#   # 머지 (stdin 에서 부분 JSON 을 받아 기존 draft 와 deep merge)
#   echo "$PARTIAL_JSON" | bash draft-io.sh merge <CODE> <SKILL> <PHASE> [<STAGE_LABEL>]
#
# 출력(save/merge): JSON `{"path":"/tmp/...","bytes":N,"stage":"<label>"}` to stdout
# 출력(load): 파일 원문 그대로 to stdout (파일 없으면 exit 1)
#
# merge 동작:
#   - 기존 파일이 없거나 비어 있으면 stdin 입력을 그대로 저장
#   - 기존 파일이 있으면 jq 의 `*` 연산자로 deep merge (입력 키만 갱신)
#   - 입력에 `features` 배열이 있으면 docId 기준으로 항목별 merge (object spread).
#     같은 docId 가 있으면 필드 추가/갱신, 없으면 새 항목으로 append.
#   - STAGE_LABEL 은 결과 JSON 의 `stage` 필드로 echo (단순 trace 용도)
#
# 파일 위치: /tmp/${CODE}-${SKILL}-${PHASE}.json
# (draft/validated/final 등 PHASE 자유 명명)
set -euo pipefail

MODE="${1:?mode required (save|load|merge)}"
CODE="${2:?CODE required}"
SKILL="${3:?SKILL required}"
PHASE="${4:?PHASE required}"
STAGE_LABEL="${5:-}"

FILE="/tmp/${CODE}-${SKILL}-${PHASE}.json"

case "$MODE" in
  save)
    TMP=$(mktemp)
    cat > "$TMP"
    mv "$TMP" "$FILE"
    BYTES=$(wc -c < "$FILE" | tr -d ' ')
    jq -n --arg path "$FILE" --argjson bytes "$BYTES" --arg stage "$STAGE_LABEL" \
      '{path:$path, bytes:$bytes, stage:$stage}'
    ;;
  load)
    if [[ ! -f "$FILE" ]]; then
      echo "ERROR: $FILE not found" >&2
      exit 1
    fi
    cat "$FILE"
    ;;
  merge)
    INPUT=$(mktemp)
    cat > "$INPUT"
    if [[ ! -s "$INPUT" ]]; then
      echo "ERROR: empty stdin for merge" >&2
      rm -f "$INPUT"
      exit 2
    fi
    OUT=$(mktemp)
    if [[ ! -f "$FILE" ]] || [[ ! -s "$FILE" ]]; then
      # 기존 draft 없음 → 입력을 그대로 저장
      cp "$INPUT" "$OUT"
    else
      # deep merge — features 는 docId 키 기준으로 항목별 머지
      # jq 의 `*` 연산자: object 는 deep merge, scalar/array 는 우측 우선
      # features 는 별도 처리: 좌/우의 docId 매핑을 합쳐 동일 docId 는 object spread, 없으면 append
      jq -n \
        --slurpfile base "$FILE" \
        --slurpfile patch "$INPUT" \
        '
        ($base[0] // {}) as $b
        | ($patch[0] // {}) as $p
        | ($b * $p) as $merged
        | if ($b.features? | type == "array") and ($p.features? | type == "array")
          then
            ($b.features | map({key: (.docId // .id // ""), value: .}) | from_entries) as $bm
            | ($p.features | map({key: (.docId // .id // ""), value: .}) | from_entries) as $pm
            | ($bm * $pm) as $fm
            | ($b.features | map(.docId // .id // "")) as $border
            | ($p.features | map(.docId // .id // "")
                           | map(select(. as $k | ($border | index($k) | not)))) as $newOrder
            | ($border + $newOrder) as $allOrder
            | $merged + {features: ($allOrder | map($fm[.]) | map(select(. != null)))}
          else
            $merged
          end
        ' > "$OUT"
    fi
    mv "$OUT" "$FILE"
    rm -f "$INPUT"
    BYTES=$(wc -c < "$FILE" | tr -d ' ')
    jq -n --arg path "$FILE" --argjson bytes "$BYTES" --arg stage "$STAGE_LABEL" \
      '{path:$path, bytes:$bytes, stage:$stage, mode:"merge"}'
    ;;
  *)
    echo "ERROR: unknown mode '$MODE' (use save|load|merge)" >&2
    exit 2
    ;;
esac
