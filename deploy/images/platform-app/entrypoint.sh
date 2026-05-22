#!/bin/sh
# nginx 컨테이너 startup 훅:
# - 컨테이너 환경변수 → /usr/share/nginx/html/env.js 로 직렬화
# - React (src/config.js) 가 window._env_ 로 1순위 읽어감
# - 운영자는 docker compose 의 environment / .env 만 바꾸고 컨테이너 recreate 만 하면 됨
#   (React 재빌드 불필요)
set -eu

ENV_JS="/usr/share/nginx/html/env.js"

# 직렬화 — JS 문법 안전을 위해 큰따옴표 escape
esc() { printf '%s' "${1:-}" | sed 's/\\/\\\\/g; s/"/\\"/g'; }

cat > "$ENV_JS" <<EOF
// Generated at container startup by entrypoint.sh — DO NOT EDIT
window._env_ = {
  REACT_APP_PLATFORM_API: "$(esc "${REACT_APP_PLATFORM_API:-}")",
  REACT_APP_GOJIRA_API: "$(esc "${REACT_APP_GOJIRA_API:-}")",
  REACT_APP_GOJIRA_APP_REMOTE: "$(esc "${REACT_APP_GOJIRA_APP_REMOTE:-}")",
  REACT_APP_SSO_URL: "$(esc "${REACT_APP_SSO_URL:-}")"
};
EOF

echo "[entrypoint] wrote $ENV_JS:"
cat "$ENV_JS"

exec nginx -g 'daemon off;'
