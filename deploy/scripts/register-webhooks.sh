#!/usr/bin/env bash
# register-webhooks.sh — 기존 DocRepo 에 Gitea webhook 을 소급 등록/갱신한다.
#
# 배경: PLATFORM_WEBHOOK_URL 이 비어 있으면 과제 생성 시 CreateWebhook 이 skip 되어
#       webhook 이 걸리지 않는다. 그러면 PR 머지(main push) 후 data/issues/*.yaml 변경이
#       Mongo 로 reconcile 되지 않아 웹 `공식` 뷰에 이슈가 반영되지 않는다.
#       설정을 뒤늦게 추가한 환경에서는 이미 만들어진 repo 에 이 스크립트로 소급 등록한다.
#
# 동작: 과제 목록의 DocRepo 를 순회하며
#        - webhook 없음        → 생성
#        - URL 이 다름          → 갱신(PATCH)
#        - URL 이 이미 동일     → 건너뜀
#
# 사용:
#   ./register-webhooks.sh              # dry-run (변경 없이 현황만)
#   ./register-webhooks.sh --apply      # 실제 등록/갱신
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/_lib.sh"
load_env

APPLY=0
[[ "${1:-}" == "--apply" ]] && APPLY=1

: "${PLATFORM_WEBHOOK_URL:?PLATFORM_WEBHOOK_URL 미설정 — deploy/.env 에 지정할 것}"
: "${PLATFORM_GITHUB_TOKEN:?PLATFORM_GITHUB_TOKEN 미설정}"
SECRET="${PLATFORM_WEBHOOK_SECRET:-gojira-webhook-secret}"
GITEA="http://${GITEA_SERVER:?GITEA_SERVER 미설정}"
GW="${URL_SCHEME:-http}://${GATEWAY_HOST:-127.0.0.1}:${GATEWAY_PORT:-44300}"

EVENTS='["pull_request","pull_request_review","push"]'

log "webhook URL : ${PLATFORM_WEBHOOK_URL}"
log "Gitea       : ${GITEA}"
[[ $APPLY -eq 1 ]] || warn "dry-run — 실제 변경하려면 --apply"

# 과제 목록에서 repoOwner/repoName 수집 (서비스 토큰으로 조회)
projects=$(curl -s -m 20 -H "X-Service-Token: ${SERVICE_TOKEN:-}" -H "X-User-Id: ${DEV_USER:-admin}" \
  "${GW}/gojira/api/v2/Project") || die "과제 목록 조회 실패"

echo "$projects" | python3 -c '
import sys, json
for p in json.load(sys.stdin):
    doc = p.get("documents") or {}
    rn = doc.get("repoName")
    if not rn:
        continue
    code = ((p.get("jiraCodes") or {}).get("main") or {}).get("jiraCode") or "?"
    owner = doc.get("repoOwner") or "gojira"
    print(code + "\t" + owner + "\t" + rn)
' | while IFS=$'\t' read -r code owner repo; do
  hooks=$(curl -s -m 10 -H "Authorization: token ${PLATFORM_GITHUB_TOKEN}" \
    "${GITEA}/api/v1/repos/${owner}/${repo}/hooks" 2>/dev/null) || hooks="[]"

  hook_id=$(echo "$hooks" | python3 -c '
import sys, json
want = sys.argv[1]
try:
    hs = json.load(sys.stdin)
except Exception:
    hs = []
for h in hs:
    url = (h.get("config") or {}).get("url", "")
    if "/webhook/github" in url:
        print(h.get("id"), "SAME" if url == want else "DIFF")
        break
' "${PLATFORM_WEBHOOK_URL}" 2>/dev/null || true)

  if [[ -z "$hook_id" ]]; then
    printf '  %-10s %-34s 없음 → 생성\n' "$code" "${owner}/${repo}"
    [[ $APPLY -eq 1 ]] && curl -s -o /dev/null -X POST \
      -H "Authorization: token ${PLATFORM_GITHUB_TOKEN}" -H "Content-Type: application/json" \
      -d "{\"type\":\"gitea\",\"active\":true,\"events\":${EVENTS},\"config\":{\"url\":\"${PLATFORM_WEBHOOK_URL}\",\"content_type\":\"json\",\"secret\":\"${SECRET}\"}}" \
      "${GITEA}/api/v1/repos/${owner}/${repo}/hooks"
  else
    id=${hook_id%% *}; state=${hook_id##* }
    if [[ "$state" == "SAME" ]]; then
      printf '  %-10s %-34s 최신 — 건너뜀\n' "$code" "${owner}/${repo}"
    else
      printf '  %-10s %-34s URL 다름 → 갱신(#%s)\n' "$code" "${owner}/${repo}" "$id"
      [[ $APPLY -eq 1 ]] && curl -s -o /dev/null -X PATCH \
        -H "Authorization: token ${PLATFORM_GITHUB_TOKEN}" -H "Content-Type: application/json" \
        -d "{\"active\":true,\"events\":${EVENTS},\"config\":{\"url\":\"${PLATFORM_WEBHOOK_URL}\",\"content_type\":\"json\",\"secret\":\"${SECRET}\"}}" \
        "${GITEA}/api/v1/repos/${owner}/${repo}/hooks/${id}"
    fi
  fi
done

log "완료"
