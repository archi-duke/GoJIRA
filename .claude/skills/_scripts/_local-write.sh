#!/usr/bin/env bash
# _local-write.sh — 이슈 로컬 write 공통 헬퍼 (트랙 E 이슈 스킬 로컬화, local-agent-issue-model.md §3.2).
#
# GoJIRA-API 가 `X-Local-Write: true` 응답에 동봉하는 localWrites
#   [{path, content} | {path, delete:true}]
# 를 로컬 clone(GOJIRA_REPO_ROOT|git toplevel) 에 반영한다.
# raw curl 대신 이 헬퍼를 거치면 서버 worktree 의존 없이 로컬 YAML 이 갱신된다.
#
# 사용: source 한 뒤 gj_apply_local_writes "<json-response>" 호출.
#   - 응답에 localWrites 가 없으면(구버전/비로컬) 조용히 무시 → 하위호환.

# gj_repo_root — 로컬 clone 루트. GOJIRA_REPO_ROOT(런처 주입) 우선, 없으면 git toplevel.
gj_repo_root() {
  local root="${GOJIRA_REPO_ROOT:-}"
  if [[ -z "$root" ]]; then
    root=$(git rev-parse --show-toplevel 2>/dev/null || true)
  fi
  if [[ -z "$root" || ! -d "$root" ]]; then
    echo "ERROR: repo 루트 미확인 — GOJIRA_REPO_ROOT 설정 또는 clone 안에서 실행하세요." >&2
    return 1
  fi
  printf '%s' "$root"
}

# gj_apply_local_writes <json-response>
# 응답의 .localWrites[] 를 ROOT 기준으로 기록(content)/삭제(delete)한다.
gj_apply_local_writes() {
  local resp="$1"
  local n
  n=$(jq -r '(.localWrites // []) | length' <<< "$resp" 2>/dev/null || echo 0)
  [[ -z "$n" || "$n" == "0" ]] && return 0

  local root
  root=$(gj_repo_root) || return 1

  local i path del full
  for i in $(seq 0 $((n - 1))); do
    path=$(jq -r ".localWrites[$i].path // empty" <<< "$resp")
    [[ -z "$path" ]] && continue
    # path traversal 차단 — 절대경로/.. 금지 (서버가 만든 경로지만 방어적).
    case "$path" in
      /*|*..*) echo "WARN: 비정상 localWrites 경로 무시: $path" >&2; continue ;;
    esac
    full="${root}/${path}"
    del=$(jq -r ".localWrites[$i].delete // false" <<< "$resp")
    if [[ "$del" == "true" ]]; then
      rm -f "$full"
    else
      mkdir -p "$(dirname "$full")"
      jq -r ".localWrites[$i].content // \"\"" <<< "$resp" > "$full"
    fi
  done
}
