#!/usr/bin/env bash
# 외부망 빌드 — 베이스 이미지 일괄 pull → docker compose build.
# 폐쇄망 반입 전 단일 명령으로 외부망 작업을 완료한다.
#
# 사용:
#   ./build-images.sh           # 전체 (pull + build)
#   ./build-images.sh --no-pull # 베이스 이미지 pull 생략
#   ./build-images.sh --service platform-app gojira-app
#                               # 특정 서비스만 재빌드 (React 호스트 변경 시)
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/_lib.sh"

require_docker
load_env

DO_PULL=1
SERVICES=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    --no-pull) DO_PULL=0; shift ;;
    --service) shift; while [[ $# -gt 0 && "$1" != --* ]]; do SERVICES+=("$1"); shift; done ;;
    -h|--help) sed -n '2,12p' "$0"; exit 0 ;;
    *) die "알 수 없는 옵션: $1 (--help 참조)" ;;
  esac
done

if [[ $DO_PULL -eq 1 ]]; then
  log "베이스 이미지 pull (외부망 네트워크 필요)"
  while IFS= read -r img; do
    log "  ↓ $img"
    docker pull "$img"
  done < <(list_base_images)
  ok "베이스 이미지 pull 완료"
else
  warn "--no-pull: 베이스 이미지 pull 생략"
fi

cd "${DEPLOY_DIR}"
if [[ ${#SERVICES[@]} -eq 0 ]]; then
  log "전체 서비스 빌드 (docker compose build)"
  docker compose build
else
  log "선택 서비스 빌드: ${SERVICES[*]}"
  docker compose build "${SERVICES[@]}"
fi

ok "빌드 완료. 다음 단계: ./save-images.sh"
