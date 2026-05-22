#!/usr/bin/env bash
# 공통 헬퍼 — deploy/scripts/*.sh 가 source 한다.
# - 색상 로그
# - images.txt 파서 (base / service 분리, ${IMAGE_TAG} 치환)
# - .env 로드
set -euo pipefail

# 호출 스크립트 기준 deploy/ 디렉토리 위치 잡기 (어디서 실행해도 동작)
_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEPLOY_DIR="$(cd "${_LIB_DIR}/.." && pwd)"

# ── 로그 ─────────────────────────────────────────────────────────────────
_isatty=0
[[ -t 1 ]] && _isatty=1
if [[ $_isatty -eq 1 ]]; then
  C_R=$'\033[31m'; C_G=$'\033[32m'; C_Y=$'\033[33m'; C_B=$'\033[34m'; C_0=$'\033[0m'
else
  C_R=; C_G=; C_Y=; C_B=; C_0=
fi
log()  { printf "%s[ deploy ]%s %s\n" "$C_B" "$C_0" "$*"; }
ok()   { printf "%s[ deploy ]%s %s\n" "$C_G" "$C_0" "$*"; }
warn() { printf "%s[ deploy ]%s %s\n" "$C_Y" "$C_0" "$*" >&2; }
err()  { printf "%s[ deploy ]%s %s\n" "$C_R" "$C_0" "$*" >&2; }
die()  { err "$*"; exit 1; }

# ── docker 존재 확인 ─────────────────────────────────────────────────────
require_docker() {
  command -v docker >/dev/null 2>&1 || die "docker 명령을 찾을 수 없음. Docker Engine 설치 필요."
  docker compose version >/dev/null 2>&1 || die "docker compose v2 plugin 이 필요함 (docker-compose v1 아님)."
}

# ── .env 로드 (선택) ────────────────────────────────────────────────────
# - 없으면 silent skip (IMAGE_TAG 등은 default 적용)
load_env() {
  local env_file="${1:-${DEPLOY_DIR}/.env}"
  if [[ -f "$env_file" ]]; then
    # UID/EUID/PPID 등은 bash readonly built-in — 그대로 source 하면
    # 'readonly variable' 에러. 이들을 제외하고 source 하고, 컨테이너 권한용
    # UID/GID 는 docker compose 가 .env 에서 직접 읽으므로 여기선 검증용으로만
    # 별도 변수(ENV_UID/ENV_GID)에 추출한다.
    set -a
    # shellcheck disable=SC1090
    source <(grep -vE '^[[:space:]]*(UID|EUID|PPID|BASHPID)[[:space:]]*=' "$env_file")
    set +a
    ENV_UID="$(sed -nE 's/^[[:space:]]*UID[[:space:]]*=[[:space:]]*([^[:space:]#]+).*/\1/p' "$env_file" | tail -1)"
    ENV_GID="$(sed -nE 's/^[[:space:]]*GID[[:space:]]*=[[:space:]]*([^[:space:]#]+).*/\1/p' "$env_file" | tail -1)"
    : "${ENV_UID:=${UID:-1000}}"
    : "${ENV_GID:=${GID:-1000}}"
    export ENV_UID ENV_GID
  fi
  : "${IMAGE_TAG:=latest}"
  export IMAGE_TAG
}

# ── images.txt 파서 ─────────────────────────────────────────────────────
# 사용:
#   list_base_images   → 베이스 이미지 (외부망 pull 대상)
#   list_service_images → gojira/* 서비스 이미지 (build 결과)
#   list_all_images     → 전체
_images_file="${DEPLOY_DIR}/scripts/images.txt"
_parse_images() {
  [[ -f "$_images_file" ]] || die "images.txt 가 없음: $_images_file"
  # 주석/빈 줄 제거 + ${IMAGE_TAG} 치환 + 앞뒤 공백 정리
  sed -e 's/#.*$//' -e 's/[[:space:]]\+$//' -e 's/^[[:space:]]\+//' \
      -e "s|\${IMAGE_TAG}|${IMAGE_TAG}|g" "$_images_file" \
    | grep -v '^[[:space:]]*$'
}
list_all_images()    { _parse_images; }
list_base_images()   { _parse_images | grep -v '^gojira/'; }
list_service_images(){ _parse_images | grep    '^gojira/'; }
