#!/usr/bin/env bash
# 외부망 (인터넷 가능) 에서 실행 — Go/Node 의존성 캐시 이미지 2종을 빌드해
# build-cache.tar 로 묶는다. 폐쇄망에 반입 후 `docker load -i build-cache.tar`
# 하면 그 안의 의존성으로 모든 서비스 빌드 가능.
#
# 갱신 시점:
#   - go.mod / go.sum / package-lock.json / pumlex package.json /
#     JIRA-API/requirements.txt 가 변경된 경우만
#   - 일반 코드 수정은 폐쇄망에서 docker compose build 만으로 끝
#
# 사용:
#   ./build-cache-update.sh                       # → ./build-cache.tar (linux/amd64)
#   ./build-cache-update.sh /tmp/build-cache.tar  # 출력 경로
#
# 타깃 플랫폼: 기본 linux/amd64 (사내 Windows Server 등 x86_64). 서비스 이미지가 이
# 캐시를 FROM 으로 쓰므로, 캐시와 서비스 이미지의 플랫폼이 같아야 한다. Apple Silicon
# 등 arm64 호스트에서도 amd64 캐시를 만들도록 강제(QEMU). 다른 arch 면
# DOCKER_DEFAULT_PLATFORM 을 미리 export 하거나 native 로 지정.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
OUT="${1:-${REPO_ROOT}/build-cache.tar}"

# 서비스 이미지와 동일 플랫폼으로 캐시를 빌드 — 기본 linux/amd64.
export DOCKER_DEFAULT_PLATFORM="${DOCKER_DEFAULT_PLATFORM:-linux/amd64}"
echo "[ deploy ] 타깃 플랫폼: ${DOCKER_DEFAULT_PLATFORM} (서비스 이미지와 일치해야 함)"

echo "[ deploy ] build-cache-go 이미지 빌드"
docker build -f "${REPO_ROOT}/deploy/images/build-cache-go/Dockerfile" \
  -t gojira/build-cache-go:latest "${REPO_ROOT}"

echo "[ deploy ] build-cache-node 이미지 빌드 (GoJIRA-App·Platform-App·pumlex)"
docker build -f "${REPO_ROOT}/deploy/images/build-cache-node/Dockerfile" \
  -t gojira/build-cache-node:latest "${REPO_ROOT}"

echo "[ deploy ] build-cache-py 이미지 빌드 (jira-api wheel)"
docker build -f "${REPO_ROOT}/deploy/images/build-cache-py/Dockerfile" \
  -t gojira/build-cache-py:latest "${REPO_ROOT}"

echo "[ deploy ] tar 패키징 → ${OUT}"
docker save -o "${OUT}" \
  gojira/build-cache-go:latest \
  gojira/build-cache-node:latest \
  gojira/build-cache-py:latest

size="$(du -h "${OUT}" | awk '{print $1}')"
echo "[ deploy ] 완료: ${OUT} (${size})"
echo "[ deploy ] 폐쇄망 적재: docker load -i $(basename "${OUT}")"
