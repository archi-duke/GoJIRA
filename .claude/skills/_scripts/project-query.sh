#!/usr/bin/env bash
# project-query.sh — GoJIRA-API /Project 프로젝트 목록을 JSON 으로 stdout 에 출력한다.
#
# 사용법:
#   bash project-query.sh <USER>            > /tmp/gojira_projects.json   # 전체 목록(요약)
#   bash project-query.sh <USER> <CODE>     > /tmp/gojira_project.json    # 해당 프로젝트 1건(전체 필드)
#
# 출력:
#   CODE 미지정 → [{code, name, status}] 요약 배열
#   CODE 지정   → 그 프로젝트의 전체 오브젝트 (archSettings 등 포함). 미존재 시 null.
#
# 프로젝트 메타는 Mongo 가 진실(이슈와 달리 로컬 clone 에 없음) — HTTP 조회가 맞다.
# URL/헤더 조립은 이 스크립트가 전담한다 — 에이전트가 curl 을 직접 만들지 말 것.
set -euo pipefail

USER="${1:?user (X-User-Id) required}"
CODE="${2:-}"
GOJIRA_API="${GOJIRA_API:?GOJIRA_API env var required (launcher must inject)}"

TMP=$(mktemp /tmp/.gojira_projects.XXXXXX)
trap 'rm -f "$TMP"' EXIT

curl -s "${GOJIRA_API}/Project" \
  -H "X-User-Id: ${USER}" -H "Authorization: Bearer ${GOJIRA_SESSION_TOKEN:-}" > "$TMP"

if [ -n "$CODE" ]; then
  jq --arg c "$CODE" '[.[] | select(.jiraCodes.main.jiraCode==$c)] | .[0]' "$TMP"
else
  jq '[.[] | {code:(.jiraCodes.main.jiraCode // ""), name, status}]' "$TMP"
fi
