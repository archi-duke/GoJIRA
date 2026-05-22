#!/usr/bin/env bash
# 폐쇄망 — 반입된 tar 를 docker 에 적재.
#
# 사용:
#   ./load-images.sh                       # deploy/gojira-images.tar 사용
#   ./load-images.sh /path/to/images.tar   # 경로 지정
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/_lib.sh"

require_docker
load_env

TAR="${1:-${DEPLOY_DIR}/gojira-images.tar}"
[[ -f "$TAR" ]] || die "tar 파일 없음: $TAR"

log "load: $TAR"
docker load -i "$TAR"

log "기대 이미지 존재 여부 확인"
all_ok=1
while IFS= read -r img; do
  if docker image inspect "$img" >/dev/null 2>&1; then
    ok "  ✓ $img"
  else
    err "  ✗ $img (미적재)"
    all_ok=0
  fi
done < <(list_all_images)

[[ $all_ok -eq 1 ]] || die "일부 이미지 누락 — tar 가 outdated 일 수 있음"
ok "load 완료. 다음 단계: ./preflight.sh && docker compose up -d"
