#!/usr/bin/env bash
# 외부망 빌드 — 베이스 이미지 일괄 pull → docker compose build.
# 폐쇄망 반입 전 단일 명령으로 외부망 작업을 완료한다.
#
# 타깃 플랫폼: 기본 linux/amd64 (사내 Windows Server 등 x86_64). Apple Silicon 등
# arm64 빌드 호스트에서도 amd64 이미지를 만들도록 강제한다(QEMU 에뮬레이션). 안 그러면
# arm64 이미지가 조용히 만들어져 사내 amd64 서버에서 컨테이너가 안 뜬다. 빌드 호스트
# 아키텍처 그대로 쓰려면 `--platform native` (또는 DOCKER_DEFAULT_PLATFORM 직접 지정).
# ⚠️ 서비스 이미지 4종은 FROM gojira/build-cache-* 이므로, 같은 플랫폼의 build-cache
#    이미지가 먼저 로컬에 있어야 한다 → 사전에 build-cache-update.sh 동일 플랫폼으로 실행.
#
# 사용:
#   ./build-images.sh                     # 전체 (pull + build), linux/amd64
#   ./build-images.sh --platform native   # 빌드 호스트 arch 그대로 (교차 빌드 안 함)
#   ./build-images.sh --no-pull           # 베이스 이미지 pull 생략
#   ./build-images.sh --service platform-app gojira-app
#                                         # 특정 서비스만 재빌드 (React 호스트 변경 시)
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/_lib.sh"

require_docker
load_env

DO_PULL=1
SERVICES=()
# 기본 타깃 = linux/amd64. 환경변수 DOCKER_DEFAULT_PLATFORM 이 이미 있으면 그걸 존중.
PLATFORM="${DOCKER_DEFAULT_PLATFORM:-linux/amd64}"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --no-pull) DO_PULL=0; shift ;;
    --platform) shift; PLATFORM="${1:?--platform 값 필요 (예: linux/amd64 | native)}"; shift ;;
    --service) shift; while [[ $# -gt 0 && "$1" != --* ]]; do SERVICES+=("$1"); shift; done ;;
    -h|--help) sed -n '2,18p' "$0"; exit 0 ;;
    *) die "알 수 없는 옵션: $1 (--help 참조)" ;;
  esac
done

# 플랫폼 강제 — docker pull / compose build 둘 다 DOCKER_DEFAULT_PLATFORM 을 존중한다.
if [[ "$PLATFORM" == "native" ]]; then
  unset DOCKER_DEFAULT_PLATFORM || true
  warn "platform=native — 빌드 호스트 아키텍처 그대로 빌드 (사내 서버 arch 와 다르면 컨테이너 안 뜸)"
else
  export DOCKER_DEFAULT_PLATFORM="$PLATFORM"
  log "타깃 플랫폼: ${PLATFORM} (사내 서버 arch 와 반드시 일치해야 함)"
fi

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
