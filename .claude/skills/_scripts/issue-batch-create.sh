#!/usr/bin/env bash
# issue-batch-create.sh — 이슈를 순차 생성하고 실패 시 재시도한다.
#
# 사용법:
#   bash issue-batch-create.sh <CODE> <USER> < items.json
#
# 입력(stdin): JSON array
#   [{"issueType":"VOS","summary":"...","desc":"...","client":"alice"}, ...]
#   필수: issueType, summary, desc — 하나라도 비면 전체 중단(아무것도 생성 안 함)
#   "description" 은 규약 위반이지만 desc 부재 시 자동 정규화된다(+stderr 경고)
#   POST 본문 포함: issueType / summary / desc / reqType / labels
#   PUT 업데이트 필드 (생성 후 자동 반영):
#     공통:      client / priority / assignee
#     Feature:   actor / precondition / basicflow / alternativeflow / exceptionflow / postcondition
#     NFR:       qa / source / stimulus / environment / artifact / response / measure / responseScenario / measureAuto
#     Driver:    qad / qai / priority / riskLevel / sensitivity / driverSelected
#     ADSN/DDSN: designLevel / designPoint / dmodel / objectives / approaches / decision / decisionRationale
#                pros / cons / score / eval / tradeoffs / risk / sensitivity / mitigation
#                adiagram / bdiagram
#   그 외 필드는 무시됨
#
# 출력(stdout): JSON
#   {"total":N,"created":K,"failed":F,"keys":[...],"errors":[...]}
#
# 재시도 정책: 5xx / 422(already exists) / 네트워크 에러 → 최대 5회 지수 백오프 (0.2/0.4/0.8/1.6/3.2s)
set -euo pipefail

# 트랙 E: 서버 worktree 대신 로컬 clone 에 YAML 기록 (X-Local-Write + localWrites 적용).
DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
source "${DIR}/_local-write.sh"

CODE="${1:?projectCode required}"
USER="${2:?user knoxID required}"
GOJIRA_API="${GOJIRA_API:-${GOJIRA_API_URL:?GOJIRA_API or GOJIRA_API_URL env var required (launcher must inject)}}"

INPUT=$(cat)

# ── 입력 사전 검증 + 별칭 정규화 (생성 시작 전, 원자적) ──────────────────────
# 에이전트가 초안 스키마의 "description" 을 배치 입력에 그대로 가져오는 사고가 반복됨.
# 배치 규약은 "desc" (POST /Issue wire 필드) — 미지 필드는 무시되므로 그대로 두면
# 본문이 빈 이슈가 조용히 생긴다. → description 만 있으면 desc 로 자동 정규화(+경고),
# 필수 필드(issueType/summary/desc) 가 하나라도 비면 아무것도 생성하지 않고 중단.
if [ "$(jq -r 'type' <<< "$INPUT" 2>/dev/null)" != "array" ]; then
  echo "ERROR: 입력은 JSON 배열이어야 함 — 받은 것: $(head -c 200 <<< "$INPUT")" >&2
  exit 1
fi
ALIASED=$(jq '[.[] | select((.desc // "")=="" and (.description // "")!="")] | length' <<< "$INPUT")
if [ "$ALIASED" -gt 0 ]; then
  echo "WARN: ${ALIASED}개 항목의 'description' 을 'desc' 로 정규화함 — 배치 입력 규약은 desc (초안 필드명을 바꾸지 말고 그대로 desc 로 작성할 것)" >&2
  INPUT=$(jq 'map(if (.desc // "")=="" and (.description // "")!="" then .desc = .description else . end)' <<< "$INPUT")
fi
INVALID=$(jq -r 'to_entries
  | map(select((.value.issueType // "")=="" or (.value.summary // "")=="" or (.value.desc // "")==""))
  | map("  항목[\(.key)] summary=\(.value.summary // "(없음)") — issueType/summary/desc 는 필수(비어있으면 안 됨)")
  | .[]' <<< "$INPUT")
if [ -n "$INVALID" ]; then
  echo "ERROR: 입력 검증 실패 — 아무 이슈도 생성하지 않음. 항목을 고쳐 전체 배열로 다시 호출할 것." >&2
  echo "$INVALID" >&2
  exit 1
fi

COUNT=$(jq 'length' <<< "$INPUT")

CREATED=0
FAILED=0
KEYS=()
ERRORS=()

for i in $(seq 0 $((COUNT - 1))); do
  ITEM=$(jq ".[$i]" <<< "$INPUT")
  SUM=$(jq -r '.summary' <<< "$ITEM")
  # Phase 2(이슈 쓰기 시점 이동): localOnly:true 로 Mongo 캐시 upsert 를 생략시킨다(키 할당은 그대로).
  BODY=$(jq --arg p "$CODE" --arg u "$USER" \
    '{prjID:$p, user:$u, issueType:.issueType, summary:.summary, desc:.desc, localOnly:true}
     + (if .reqType then {reqType:.reqType} else {} end)
     + (if .labels then {labels:.labels} else {} end)' <<< "$ITEM")

  RESP=""
  STATUS=""
  for attempt in 1 2 3 4 5; do
    TMP=$(mktemp)
    # body 는 stdin(@-)으로 전달 — argv 로 넘기면 Windows 네이티브 curl 이 UTF-8 을
    # ANSI(CP949)로 변환해 한글이 깨진다(서버 json 파싱 시 U+FFFD). stdin 은 바이트 보존.
    STATUS=$(curl -s -o "$TMP" -w '%{http_code}' -X POST "${GOJIRA_API}/Issue" \
      -H "Content-Type: application/json" -H "X-User-Id: ${USER}" -H "Authorization: Bearer ${GOJIRA_SESSION_TOKEN}" -H "X-Local-Write: true" \
      --data-binary @- <<< "$BODY" || echo 000)
    RESP=$(cat "$TMP")
    rm -f "$TMP"
    if [[ "$STATUS" =~ ^2[0-9][0-9]$ ]]; then
      break
    fi
    # 재시도 대상: 5xx / 422 / 000(네트워크)
    if [[ "$STATUS" =~ ^(5[0-9][0-9]|422|000)$ ]]; then
      sleep $(awk "BEGIN{print 0.2 * (2 ^ ($attempt - 1))}")
      continue
    fi
    break
  done

  if [[ "$STATUS" =~ ^2[0-9][0-9]$ ]]; then
    KEY=$(jq -r '.key // empty' <<< "$RESP")
    SUBKEY=$(jq -r '.subkey // empty' <<< "$RESP")
    if [[ -z "$KEY" ]]; then
      FAILED=$((FAILED+1))
      ERRORS+=("$(jq -n --arg s "$SUM" --arg e "invalid response: $RESP" '{summary:$s,error:$e}')")
      continue
    fi
    CREATED=$((CREATED+1))
    KEYS+=("$KEY")
    # 생성 YAML 을 로컬 clone 에 기록 (서버 worktree 미사용).
    gj_apply_local_writes "$RESP"

    # 선택 필드 PUT 업데이트 — 공통 + Feature + NFR + Driver + ADSN/DDSN 필드 화이트리스트
    # 값이 빈 문자열이 아닌 것만 PUT 에 포함 (false, 0, [] 등은 유지)
    EXTRA=$(jq '{
      client:            .client,
      priority:          .priority,
      assignee:          .assignee,
      actor:             .actor,
      precondition:      .precondition,
      basicflow:         .basicflow,
      alternativeflow:   .alternativeflow,
      exceptionflow:     .exceptionflow,
      postcondition:     .postcondition,
      qa:                .qa,
      source:            .source,
      stimulus:          .stimulus,
      environment:       .environment,
      artifact:          .artifact,
      response:          .response,
      measure:           .measure,
      responseScenario:  .responseScenario,
      measureAuto:       .measureAuto,
      qad:               .qad,
      qai:               .qai,
      riskLevel:         .riskLevel,
      driverSelected:    .driverSelected,
      designLevel:       .designLevel,
      designPoint:       .designPoint,
      dmodel:            .dmodel,
      objectives:        .objectives,
      approaches:        .approaches,
      decision:          .decision,
      decisionRationale: .decisionRationale,
      pros:              .pros,
      cons:              .cons,
      score:             .score,
      eval:              .eval,
      tradeoffs:         .tradeoffs,
      risk:              .risk,
      sensitivity:       .sensitivity,
      mitigation:        .mitigation,
      concerns:          .concerns,
      adiagram:          .adiagram,
      bdiagram:          .bdiagram
    } | with_entries(select(.value != null and .value != ""))' <<< "$ITEM")
    if [[ "$(jq 'length' <<< "$EXTRA")" != "0" ]]; then
      PTMP=$(mktemp)
      # Phase 2: 생성이 localOnly 라 Mongo 에 이슈가 없다 — 방금 생성된 YAML(응답
      # localWrites[0].content)을 baseYaml 로 동봉해 서버가 그걸 base 로 병합하게 한다.
      # 없이 보내면 legacy(Mongo) 경로가 404 로 떨어져 추가 필드가 유실된다.
      BASEY=$(jq -r '.localWrites[0].content // empty' <<< "$RESP")
      PUTBODY=$(jq -n --arg p "$CODE" --arg k "$KEY" --arg u "$USER" --argjson d "$EXTRA" --arg by "$BASEY" \
        '{prjID:$p, key:$k, user:$u, data:$d, baseYaml:$by}')
      # body 는 stdin(@-)으로 — argv 전달 시 Windows 에서 UTF-8 한글 깨짐(U+FFFD).
      curl -s -o "$PTMP" -X PUT "${GOJIRA_API}/Issue" \
        -H "Content-Type: application/json" -H "X-User-Id: ${USER}" -H "Authorization: Bearer ${GOJIRA_SESSION_TOKEN}" -H "X-Local-Write: true" \
        --data-binary @- <<< "$PUTBODY" || true
      # PUT 후 갱신된 YAML 을 로컬 clone 에 반영.
      gj_apply_local_writes "$(cat "$PTMP")"
      rm -f "$PTMP"
    fi
  else
    FAILED=$((FAILED+1))
    ERRORS+=("$(jq -n --arg s "$SUM" --arg st "$STATUS" --arg e "$RESP" '{summary:$s,status:$st,error:$e}')")
  fi

  # 요청 간 약간의 간격 — Gitea push race 완화
  sleep 0.15
done

jq -n \
  --argjson total "$COUNT" \
  --argjson created "$CREATED" \
  --argjson failed "$FAILED" \
  --argjson keys "$(printf '%s\n' "${KEYS[@]:-}" | jq -R . | jq -s 'map(select(length>0))')" \
  --argjson errors "$(printf '%s\n' "${ERRORS[@]:-}" | jq -s 'map(select(type=="object"))')" \
  '{total:$total, created:$created, failed:$failed, keys:$keys, errors:$errors}'
