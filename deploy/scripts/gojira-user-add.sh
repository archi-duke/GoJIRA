#!/usr/bin/env bash
# gojira-user-add — GoJIRA 사용자에게 OpenCode 인스턴스 1개를 할당.
#
# 동작:
#   1) 다음 OPENCODE_PORT 계산 (Mongo 의 최대 port + 1, 비어있으면 OPENCODE_BASE_PORT)
#   2) /etc/gojira/opencode-<user>.env 생성 (mode 600)
#   3) Mongo opencodeInstances 컬렉션에 upsert (status=active)
#   4) systemctl enable --now opencode@<user>.service
#
# 전제:
#   - mongosh CLI 가 시스템에 설치됨 (apt: mongodb-mongosh)
#   - deploy/.env 가 MONGODB_SERVER / MONGO_USER / MONGO_PASS / MONGO_AUTH_SOURCE / PLATFORM_MONGO_DB 를 제공
#   - /etc/systemd/system/opencode@.service 가 설치됨 (deploy/systemd 참조)
#   - opencode 바이너리가 OPENCODE_BIN 경로에 존재 (env 파일에 기록)
#
# 사용:
#   sudo ./gojira-user-add.sh <userId> [--no-start]
#     --no-start: env 파일 + Mongo 등록까지만, systemctl 호출 안 함
set -euo pipefail

USAGE() {
  echo "사용: $0 <userId> [--no-start]" >&2
  exit 1
}

USER_ID="${1:-}"
NO_START="0"
[ -z "$USER_ID" ] && USAGE
if [ "${2:-}" = "--no-start" ]; then NO_START="1"; fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEPLOY_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

# .env 로드
if [ -f "${DEPLOY_DIR}/.env" ]; then
  set -a
  # shellcheck disable=SC1091
  . "${DEPLOY_DIR}/.env"
  set +a
fi

# 필수 변수 검사
: "${MONGODB_SERVER:?MONGODB_SERVER 필요 (deploy/.env)}"
: "${MONGO_USER:?MONGO_USER 필요}"
: "${MONGO_PASS:?MONGO_PASS 필요}"
MONGO_AUTH_SOURCE="${MONGO_AUTH_SOURCE:-admin}"
PLATFORM_MONGO_DB="${PLATFORM_MONGO_DB:-gojira}"
OPENCODE_BASE_PORT="${OPENCODE_BASE_PORT:-4096}"
OPENCODE_BIN="${OPENCODE_BIN:-/usr/local/bin/opencode}"
GOJIRA_NODE_HOST="${GOJIRA_NODE_HOST:-localhost}"
WORKING_DIR="${OPENCODE_WORKING_DIR:-/opt/gojira}"

# mongosh 존재 검사
if ! command -v mongosh >/dev/null 2>&1; then
  echo "ERROR: mongosh CLI 가 없습니다. apt install mongodb-mongosh 후 재시도하세요." >&2
  exit 2
fi

MONGO_URI="mongodb://${MONGO_USER}:${MONGO_PASS}@${MONGODB_SERVER}/?authSource=${MONGO_AUTH_SOURCE}"

# 1) 다음 port 계산 — db.NextOpencodePort 의 동일 로직
NEXT_PORT=$(mongosh --quiet "${MONGO_URI}" --eval "
  const db = db.getSiblingDB('${PLATFORM_MONGO_DB}');
  const top = db.opencodeInstances.find().sort({port:-1}).limit(1).toArray();
  print(top.length ? top[0].port + 1 : ${OPENCODE_BASE_PORT});
" | tail -1)

if ! [[ "$NEXT_PORT" =~ ^[0-9]+$ ]]; then
  echo "ERROR: 포트 계산 실패: $NEXT_PORT" >&2
  exit 3
fi

# 2) env 파일 생성 — 기존 있으면 보존 옵션
ENV_PATH="/etc/gojira/opencode-${USER_ID}.env"
if [ -f "$ENV_PATH" ]; then
  echo "WARN: ${ENV_PATH} 이미 존재 — port/password 유지하고 Mongo 등록만 갱신합니다." >&2
  # shellcheck disable=SC1090
  . "$ENV_PATH"
  NEXT_PORT="${OPENCODE_PORT:-$NEXT_PORT}"
else
  install -d -m 755 /etc/gojira
  PASSWORD=$(openssl rand -hex 16)
  umask 077
  cat > "$ENV_PATH" <<EOF
# 자동 생성 — $(date -Is)
OPENCODE_PORT=${NEXT_PORT}
OPENCODE_SERVER_PASSWORD=${PASSWORD}
OPENCODE_BIN=${OPENCODE_BIN}
WORKING_DIR=${WORKING_DIR}
EOF
  chmod 600 "$ENV_PATH"
  echo "생성: ${ENV_PATH} (port=${NEXT_PORT})"
fi

# 3) Mongo upsert
mongosh --quiet "${MONGO_URI}" --eval "
  const db = db.getSiblingDB('${PLATFORM_MONGO_DB}');
  const now = new Date();
  db.opencodeInstances.updateOne(
    { userId: '${USER_ID}' },
    {
      \$set: { host: '${GOJIRA_NODE_HOST}', port: ${NEXT_PORT}, status: 'active', lastHeartbeat: now, updatedAt: now },
      \$setOnInsert: { createdAt: now }
    },
    { upsert: true }
  );
  print('opencodeInstances upsert OK userId=${USER_ID} ${GOJIRA_NODE_HOST}:${NEXT_PORT}');
" | tail -1

# 4) systemd 시작 (옵션)
if [ "$NO_START" = "0" ]; then
  if ! command -v systemctl >/dev/null 2>&1; then
    echo "WARN: systemctl 없음 — 인스턴스를 직접 시작해야 합니다." >&2
  else
    systemctl daemon-reload
    systemctl enable --now "opencode@${USER_ID}.service"
    systemctl status --no-pager "opencode@${USER_ID}.service" | head -5
  fi
fi

echo "DONE: userId=${USER_ID} → ${GOJIRA_NODE_HOST}:${NEXT_PORT}"
