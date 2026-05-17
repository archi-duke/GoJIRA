#!/usr/bin/env bash
# gojira-user-remove — GoJIRA 사용자의 OpenCode 인스턴스 등록 해제.
#
# 동작:
#   1) systemctl disable --now opencode@<user>.service
#   2) /etc/gojira/opencode-<user>.env 삭제
#   3) Mongo opencodeInstances 컬렉션에서 사용자 제거
#
# 보존:
#   - worktree (${GOJIRA_REPOS_DIR}/.../worktrees/<user>) 는 보존. 명시적 정리는 별도 절차.
#   - GoJIRA 사용자 계정(users 컬렉션) 은 건드리지 않음.
#
# 사용:
#   sudo ./gojira-user-remove.sh <userId>
set -euo pipefail

USAGE() {
  echo "사용: $0 <userId>" >&2
  exit 1
}

USER_ID="${1:-}"
[ -z "$USER_ID" ] && USAGE

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

if ! command -v mongosh >/dev/null 2>&1; then
  echo "ERROR: mongosh CLI 가 없습니다." >&2
  exit 2
fi
MONGO_URI="mongodb://${MONGO_USER}:${MONGO_PASS}@${MONGODB_SERVER}/?authSource=${MONGO_AUTH_SOURCE}"

# 1) systemd
if command -v systemctl >/dev/null 2>&1; then
  systemctl disable --now "opencode@${USER_ID}.service" 2>&1 || true
else
  echo "WARN: systemctl 없음 — 수동으로 인스턴스를 정지해야 합니다." >&2
fi

# 2) env 파일
ENV_PATH="/etc/gojira/opencode-${USER_ID}.env"
if [ -f "$ENV_PATH" ]; then
  rm -f "$ENV_PATH"
  echo "삭제: ${ENV_PATH}"
fi

# 3) Mongo 삭제
mongosh --quiet "${MONGO_URI}" --eval "
  const db = db.getSiblingDB('${PLATFORM_MONGO_DB}');
  const r = db.opencodeInstances.deleteOne({ userId: '${USER_ID}' });
  print('opencodeInstances delete userId=${USER_ID} deletedCount=' + r.deletedCount);
" | tail -1

echo "DONE: userId=${USER_ID} (worktree 는 보존됨)"
