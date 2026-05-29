#!/usr/bin/env bash
# 외부망 (인터넷 가능) 에서 실행 — Go/Node 의존성 캐시 이미지 2종을 빌드해
# build-cache.tar 로 묶는다. 폐쇄망에 반입 후 `docker load -i build-cache.tar`
# 하면 그 안의 의존성으로 모든 서비스 빌드 가능.
#
# 갱신 시점:
#   - go.mod / go.sum / package-lock.json 이 변경된 경우만
#   - 일반 코드 수정은 폐쇄망에서 docker compose build 만으로 끝
#
# 사용:
#   ./build-cache-update.sh                       # → ./build-cache.tar
#   ./build-cache-update.sh /tmp/build-cache.tar  # 출력 경로
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
OUT="${1:-${REPO_ROOT}/build-cache.tar}"

echo "[ deploy ] build-cache-go 이미지 빌드"
docker build -f "${REPO_ROOT}/deploy/images/build-cache-go/Dockerfile" \
  -t gojira/build-cache-go:latest "${REPO_ROOT}"

echo "[ deploy ] build-cache-node 이미지 빌드"
docker build -f "${REPO_ROOT}/deploy/images/build-cache-node/Dockerfile" \
  -t gojira/build-cache-node:latest "${REPO_ROOT}"

echo "[ deploy ] tar 패키징 → ${OUT}"
docker save -o "${OUT}" gojira/build-cache-go:latest gojira/build-cache-node:latest

size="$(du -h "${OUT}" | awk '{print $1}')"
echo "[ deploy ] 완료: ${OUT} (${size})"
echo "[ deploy ] 폐쇄망 적재: docker load -i $(basename "${OUT}")"
