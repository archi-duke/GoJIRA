#!/usr/bin/env bash
# dev-opencode-register — 개발 환경에서 OpenCode 인스턴스 1건을 Mongo 에 upsert.
#
# 사내 폐쇄망 운영의 gojira-user-add.sh 와 동일한 라우팅 정보를 Mongo 에 기록한다.
# 다만 systemd / env 파일 / password 는 만들지 않음 — 개발은 start-all.bat 이 직접 OpenCode 를 띄움.
#
# 사용 (예 — Windows start-all.bat 안에서):
#   wsl bash -lc "cd /mnt/c/path/to/GoJIRA && bash deploy/scripts/dev-opencode-register.sh batman 4096"
#
# 환경:
#   deploy/.env 의 MONGODB_SERVER / MONGO_USER / MONGO_PASS / MONGO_AUTH_SOURCE / PLATFORM_MONGO_DB
#   GOJIRA_NODE_HOST (default localhost)
#
# mongosh CLI 가 WSL 또는 호스트 어디든 PATH 에 있어야 함. 없으면 조용히 종료 (개발 편의).
set -euo pipefail

USER_ID="${1:?userId required}"
PORT="${2:?port required}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEPLOY_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

if [ -f "${DEPLOY_DIR}/.env" ]; then
  set -a
  # shellcheck disable=SC1091
  . "${DEPLOY_DIR}/.env"
  set +a
fi

: "${MONGODB_SERVER:?MONGODB_SERVER 필요 (deploy/.env)}"
: "${MONGO_USER:?MONGO_USER 필요}"
: "${MONGO_PASS:?MONGO_PASS 필요}"
MONGO_AUTH_SOURCE="${MONGO_AUTH_SOURCE:-admin}"
PLATFORM_MONGO_DB="${PLATFORM_MONGO_DB:-gojira}"
GOJIRA_NODE_HOST="${GOJIRA_NODE_HOST:-localhost}"

if ! command -v mongosh >/dev/null 2>&1; then
  echo "[dev-opencode-register] mongosh 없음 — 건너뜀 (필요 시 apt install mongodb-mongosh)" >&2
  exit 0
fi

MONGO_URI="mongodb://${MONGO_USER}:${MONGO_PASS}@${MONGODB_SERVER}/?authSource=${MONGO_AUTH_SOURCE}"

mongosh --quiet "${MONGO_URI}" --eval "
  const db = db.getSiblingDB('${PLATFORM_MONGO_DB}');
  const now = new Date();
  db.opencodeInstances.updateOne(
    { userId: '${USER_ID}' },
    {
      \$set: { host: '${GOJIRA_NODE_HOST}', port: ${PORT}, status: 'active', lastHeartbeat: now, updatedAt: now },
      \$setOnInsert: { createdAt: now }
    },
    { upsert: true }
  );
  print('[dev-opencode-register] upsert OK ${USER_ID} → ${GOJIRA_NODE_HOST}:${PORT}');
" | tail -1
