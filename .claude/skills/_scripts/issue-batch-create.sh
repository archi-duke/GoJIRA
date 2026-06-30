#!/usr/bin/env bash
# issue-batch-create.sh — 이슈를 순차 생성하고 실패 시 재시도한다.
#
# 사용법:
#   bash issue-batch-create.sh <CODE> <USER> < items.json
#
# 입력(stdin): JSON array
#   [{"issueType":"VOS","summary":"...","desc":"...","client":"alice"}, ...]
#   필수: issueType, summary, desc
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
COUNT=$(jq 'length' <<< "$INPUT")

CREATED=0
FAILED=0
KEYS=()
ERRORS=()

for i in $(seq 0 $((COUNT - 1))); do
  ITEM=$(jq ".[$i]" <<< "$INPUT")
  SUM=$(jq -r '.summary' <<< "$ITEM")
  BODY=$(jq --arg p "$CODE" --arg u "$USER" \
    '{prjID:$p, user:$u, issueType:.issueType, summary:.summary, desc:.desc}
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
      PUTBODY=$(jq -n --arg p "$CODE" --arg k "$KEY" --arg u "$USER" --argjson d "$EXTRA" \
        '{prjID:$p, key:$k, user:$u, data:$d}')
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
