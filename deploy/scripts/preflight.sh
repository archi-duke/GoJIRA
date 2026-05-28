#!/usr/bin/env bash
# 폐쇄망 — `docker compose up -d` 전에 .env 와 외부 인프라 도달성을 검증.
# fail-fast: 한 항목이라도 실패하면 비-0 종료 (운영자가 즉시 조치).
#
# 검사 항목:
#   1. .env 존재 + ?required 항목 비어있지 않음
#   2. ${GOJIRA_REPOS_DIR} 디렉토리 존재 + 호스트 ${UID}:${GID} 소유
#   3. Mongo / Gitea TCP 도달 (nc -zv)
#   4. docker compose config 평가 가능 (변수 치환 실패 없음)
#   5. 필요 이미지 로컬 존재 (load-images.sh 이후 검증용)
#
# 사용:
#   ./preflight.sh                 # 전체
#   ./preflight.sh --skip-net      # 네트워크 도달 검사 생략
#   ./preflight.sh --skip-images   # 이미지 존재 검사 생략 (build 전이라면)
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/_lib.sh"

SKIP_NET=0
SKIP_IMAGES=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --skip-net) SKIP_NET=1; shift ;;
    --skip-images) SKIP_IMAGES=1; shift ;;
    -h|--help) sed -n '2,18p' "$0"; exit 0 ;;
    *) die "알 수 없는 옵션: $1" ;;
  esac
done

require_docker

# ── 1) .env 존재 + required 항목 ────────────────────────────────────────
ENV_FILE="${DEPLOY_DIR}/.env"
[[ -f "$ENV_FILE" ]] || die ".env 가 없음: $ENV_FILE  (cp .env.example .env 후 채우기)"
load_env "$ENV_FILE"

# docker-compose.yml 의 ${X:?required} 변수와 1:1 매칭
REQUIRED_VARS=(
  MONGODB_SERVER MONGO_USER MONGO_PASS
  GITEA_SERVER PLATFORM_GITHUB_TOKEN PLATFORM_GITHUB_ORG
  PLATFORM_API_SERVER PLATFORM_API_PORT
  GOJIRA_API_SERVER  GOJIRA_API_PORT
  PLATFORM_APP_SERVER PLATFORM_APP_PORT
  GOJIRA_APP_SERVER   GOJIRA_APP_PORT
  PUMLEX_SERVER       PUMLEX_PORT
)
log "1) .env required 변수 검증"
fail=0
for v in "${REQUIRED_VARS[@]}"; do
  val="${!v:-}"
  # placeholder pattern (< > 로 감싼 .env.example 기본값) 도 미설정 취급
  if [[ -z "$val" || "$val" == *'<'*'>'* ]]; then
    err "  ✗ ${v} 미설정 또는 placeholder"
    fail=1
  fi
done
[[ $fail -eq 0 ]] && ok "  → 전부 채워짐"
[[ $fail -eq 1 ]] && die "필수 변수 누락 — .env 보완 필요"

# ── 2) GOJIRA_REPOS_DIR 디렉토리 + 권한 ─────────────────────────────────
log "2) bind mount 호스트 디렉토리 검증"
REPOS_DIR="${GOJIRA_REPOS_DIR:-../repos}"
# 상대경로면 deploy/ 기준 절대경로로
[[ "$REPOS_DIR" = /* ]] || REPOS_DIR="$(cd "${DEPLOY_DIR}" && cd "$(dirname "$REPOS_DIR")" 2>/dev/null && pwd)/$(basename "$REPOS_DIR")" || true
if [[ ! -d "$REPOS_DIR" ]]; then
  warn "  ⚠ ${REPOS_DIR} 없음 — Platform-API 가 startup 시 mkdir 시도하나 권한 없으면 실패"
  warn "    조치:  mkdir -p ${REPOS_DIR} && sudo chown -R ${ENV_UID}:${ENV_GID} ${REPOS_DIR}"
else
  owner_uid="$(stat -c '%u' "$REPOS_DIR" 2>/dev/null || stat -f '%u' "$REPOS_DIR")"
  if [[ "$owner_uid" != "${ENV_UID}" ]]; then
    warn "  ⚠ ${REPOS_DIR} 소유자 uid=${owner_uid}, .env UID=${ENV_UID}"
    warn "    조치:  sudo chown -R ${ENV_UID}:${ENV_GID} ${REPOS_DIR}"
  else
    ok "  ✓ ${REPOS_DIR} (uid=${owner_uid})"
  fi
fi

# ── 3) 외부 인프라 TCP 도달 ─────────────────────────────────────────────
if [[ $SKIP_NET -eq 0 ]]; then
  log "3) 외부 인프라 TCP 도달 (nc -zv, 5s timeout)"
  _check_tcp() {
    local label="$1" hostport="$2"
    local host="${hostport%%:*}" port="${hostport##*:}"
    [[ "$host" == "$port" ]] && { warn "  ⚠ ${label}: host:port 형식 아님 ($hostport) — skip"; return; }
    if command -v nc >/dev/null 2>&1; then
      if nc -z -w5 "$host" "$port" 2>/dev/null; then
        ok "  ✓ ${label}: ${host}:${port}"
      else
        err "  ✗ ${label}: ${host}:${port} 도달 불가"
        fail=1
      fi
    else
      warn "  ⚠ nc 미설치 — ${label} 도달 검사 생략"
    fi
  }
  _check_tcp "MongoDB" "${MONGODB_SERVER}"
  _check_tcp "Gitea"   "${GITEA_SERVER}"
  [[ $fail -eq 1 ]] && die "외부 인프라 도달 실패 — 방화벽/host 값 확인"
else
  warn "3) --skip-net: 외부 인프라 도달 검사 생략"
fi

# ── 4) docker compose config 평가 ───────────────────────────────────────
log "4) docker compose config 평가 (변수 치환)"
cd "${DEPLOY_DIR}"
if docker compose config -q 2>/tmp/preflight.err; then
  ok "  ✓ compose config OK"
else
  err "  ✗ compose config 실패:"
  cat /tmp/preflight.err >&2
  die "docker-compose.yml 변수 치환 실패 — 위 메시지의 변수 확인"
fi

# ── 5) 필요 이미지 로컬 존재 ────────────────────────────────────────────
if [[ $SKIP_IMAGES -eq 0 ]]; then
  log "5) 이미지 로컬 적재 확인"
  missing=()
  while IFS= read -r img; do
    docker image inspect "$img" >/dev/null 2>&1 || missing+=("$img")
  done < <(list_all_images)
  if [[ ${#missing[@]} -eq 0 ]]; then
    ok "  ✓ 전체 이미지 적재됨"
  else
    err "  ✗ 다음 이미지 누락:"
    printf '    - %s\n' "${missing[@]}" >&2
    die "./load-images.sh 또는 ./build-images.sh 먼저 실행"
  fi
else
  warn "5) --skip-images: 이미지 존재 검사 생략"
fi

ok "preflight 전체 통과 — docker compose up -d 진행 가능"
