#!/usr/bin/env bash
# issue-write.sh — 단건 이슈 write(생성/수정/삭제/링크) 로컬화 래퍼 (트랙 E).
#
# raw `curl -X POST/PUT/DELETE ${GOJIRA_API}/Issue...` 대신 이 스크립트를 쓰면:
#   1) X-Local-Write: true 헤더로 호출 → 서버가 worktree 에 손대지 않고 YAML 본문을 응답에 동봉
#   2) 응답의 localWrites 를 로컬 clone(GOJIRA_REPO_ROOT|git toplevel) 에 기록/삭제
#   3) 서버 응답 JSON 을 그대로 stdout 으로 출력 (key/subkey 등 후속 파싱용)
#
# 사용법:
#   echo '<body-json>' | bash issue-write.sh <METHOD> <path>
#     METHOD : POST | PUT | DELETE
#     path   : /Issue | /Issue/link  (GOJIRA_API 기준 상대경로)
# 예)
#   echo '{"prjID":"X","key":"X-1","user":"u","data":{"summary":"..."}}' \
#     | bash issue-write.sh PUT /Issue
#   echo '{"key":"X-1","user":"u"}' | bash issue-write.sh DELETE /Issue
set -euo pipefail

DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
source "${DIR}/_local-write.sh"

METHOD="${1:?METHOD required (POST|PUT|DELETE)}"
APIPATH="${2:?path required (/Issue|/Issue/link)}"
GOJIRA_API="${GOJIRA_API:-${GOJIRA_API_URL:?GOJIRA_API or GOJIRA_API_URL env var required (launcher must inject)}}"

BODY=$(cat)
USER="${GOJIRA_USER:-}"
[[ -z "$USER" ]] && USER=$(jq -r '.user // empty' <<< "$BODY" 2>/dev/null || true)

# Phase 2(이슈 쓰기 시점 이동) — Mongo 는 PR 머지 시점에만 갱신되므로, 여기서 로컬 YAML
# 원문을 body 에 동봉해 서버가 그걸 base 로 병합하게 한다(Mongo 무접촉).
case "$METHOD" in
  POST)
    case "$APIPATH" in
      /Issue)
        BODY=$(jq '. + {localOnly:true}' <<< "$BODY")
        ;;
      /Issue/link)
        IWKEY=$(jq -r '.inwardKey // empty' <<< "$BODY")
        OWKEY=$(jq -r '.outwardKey // empty' <<< "$BODY")
        IWFILE=$(gj_find_issue_yaml "$IWKEY" 2>/dev/null) || IWFILE=""
        OWFILE=$(gj_find_issue_yaml "$OWKEY" 2>/dev/null) || OWFILE=""
        if [[ -z "$IWFILE" && -z "$OWFILE" ]]; then
          echo "ERROR: 로컬 clone에서 이슈 YAML을 찾지 못함: ${IWKEY:-?}, ${OWKEY:-?} — 과제 clone 안에서 실행했는지 확인" >&2
          exit 1
        fi
        [[ -n "$IWFILE" ]] && BODY=$(jq --rawfile y "$IWFILE" '. + {inwardYaml:$y}' <<< "$BODY")
        [[ -n "$OWFILE" ]] && BODY=$(jq --rawfile y "$OWFILE" '. + {outwardYaml:$y}' <<< "$BODY")
        ;;
    esac
    ;;
  PUT)
    if [[ "$APIPATH" == "/Issue" ]]; then
      KEY=$(jq -r '.key // empty' <<< "$BODY")
      if [[ -z "$KEY" ]]; then
        echo "ERROR: PUT /Issue 요청에 key 없음" >&2
        exit 1
      fi
      YFILE=$(gj_find_issue_yaml "$KEY") || { echo "ERROR: 로컬 clone에서 이슈 YAML을 찾지 못함: $KEY — 과제 clone 안에서 실행했는지 확인" >&2; exit 1; }
      BODY=$(jq --rawfile y "$YFILE" '. + {baseYaml:$y}' <<< "$BODY")
    fi
    ;;
  DELETE)
    if [[ "$APIPATH" == "/Issue" ]]; then
      KEY=$(jq -r '.key // empty' <<< "$BODY")
      if [[ -z "$KEY" ]]; then
        echo "ERROR: DELETE /Issue 요청에 key 없음" >&2
        exit 1
      fi
      YFILE=$(gj_find_issue_yaml "$KEY") || { echo "ERROR: 로컬 clone에서 이슈 YAML을 찾지 못함: $KEY — 과제 clone 안에서 실행했는지 확인" >&2; exit 1; }
      BODY=$(jq --rawfile y "$YFILE" '. + {baseYaml:$y}' <<< "$BODY")
    elif [[ "$APIPATH" == /Issue/link* ]]; then
      LINKID=$(jq -r '.linkId // empty' <<< "$BODY")
      if [[ -z "$LINKID" ]]; then
        # /Issue/link/{linkId}/of/{key} 경로 세그먼트에서 추출.
        LINKID=$(printf '%s' "$APIPATH" | sed -nE 's#^/Issue/link/([^/]+)(/of/.*)?$#\1#p')
      fi
      if [[ -z "$LINKID" ]]; then
        echo "ERROR: DELETE link 요청에 linkId 없음(body 또는 path)" >&2
        exit 1
      fi
      FILES=$(gj_find_link_files "$LINKID") || { echo "ERROR: 로컬 clone에서 linkId 관련 YAML을 찾지 못함: $LINKID — 과제 clone 안에서 실행했는지 확인" >&2; exit 1; }
      BASEFILES_JSON="[]"
      while IFS= read -r f; do
        [[ -z "$f" ]] && continue
        BASEFILES_JSON=$(jq -n --argjson arr "$BASEFILES_JSON" --rawfile c "$f" '$arr + [$c]')
      done <<< "$FILES"
      if [[ "$BASEFILES_JSON" == "[]" ]]; then
        echo "ERROR: 로컬 clone에서 linkId 관련 YAML을 찾지 못함: $LINKID — 과제 clone 안에서 실행했는지 확인" >&2
        exit 1
      fi
      BODY=$(jq --argjson bf "$BASEFILES_JSON" '. + {baseFiles: $bf}' <<< "$BODY")
    fi
    ;;
esac

TMP=$(mktemp)
# body 는 stdin(@-)으로 — argv 전달 시 Windows 네이티브 curl 이 UTF-8→ANSI 변환해 한글 깨짐.
STATUS=$(curl -s -o "$TMP" -w '%{http_code}' -X "$METHOD" "${GOJIRA_API}${APIPATH}" \
  -H "Content-Type: application/json" -H "X-User-Id: ${USER}" -H "Authorization: Bearer ${GOJIRA_SESSION_TOKEN}" -H "X-Local-Write: true" \
  --data-binary @- <<< "$BODY" || echo 000)
RESP=$(cat "$TMP")
rm -f "$TMP"

if [[ "$STATUS" =~ ^2[0-9][0-9]$ ]]; then
  gj_apply_local_writes "$RESP"
  printf '%s' "$RESP"
else
  echo "ERROR: $METHOD ${APIPATH} → HTTP $STATUS: $RESP" >&2
  exit 1
fi
