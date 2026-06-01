#!/usr/bin/env bash
# 외부망 — 모든 이미지를 단일 tar 로 추출. USB / approved file transfer 채널로
# 폐쇄망에 반입한 뒤 load-images.sh 로 적재.
#
# 사용:
#   ./save-images.sh                       # → deploy/gojira-images.tar
#   ./save-images.sh /tmp/out.tar          # 출력 경로 지정
#   IMAGE_TAG=v1.2.3 ./save-images.sh      # 특정 태그
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/_lib.sh"

require_docker
load_env

OUT="${1:-${DEPLOY_DIR}/gojira-images.tar}"

# 출력 디렉토리 보장
out_dir="$(dirname "$OUT")"
[[ -d "$out_dir" ]] || die "출력 디렉토리 없음: $out_dir"

# 누락 이미지 사전 점검
# (mapfile 은 bash 4+ — macOS 기본 bash 3.2 호환 위해 while-read 로 수집)
missing=()
IMAGES=()
while IFS= read -r img; do
  [[ -n "$img" ]] && IMAGES+=("$img")
done < <(list_all_images)
for img in "${IMAGES[@]}"; do
  if ! docker image inspect "$img" >/dev/null 2>&1; then
    missing+=("$img")
  fi
done
if [[ ${#missing[@]} -gt 0 ]]; then
  err "다음 이미지가 로컬에 없음. 먼저 ./build-images.sh 실행 필요:"
  printf '  - %s\n' "${missing[@]}" >&2
  exit 1
fi

log "이미지 ${#IMAGES[@]}개 → ${OUT}"
docker save -o "$OUT" "${IMAGES[@]}"

size="$(du -h "$OUT" | awk '{print $1}')"
ok "save 완료: ${OUT} (${size})"
log "폐쇄망에 반입할 파일: ${OUT} + deploy/ 디렉토리 전체 (.env 포함)"
