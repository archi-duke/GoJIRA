#!/bin/sh
# nginx 컨테이너 startup 훅 — Platform-App 과 동일 패턴.
# GoJIRA-App 이 실제로 사용하는 REACT_APP_* 만 직렬화.
set -eu

ENV_JS="/usr/share/nginx/html/env.js"

esc() { printf '%s' "${1:-}" | sed 's/\\/\\\\/g; s/"/\\"/g'; }

cat > "$ENV_JS" <<EOF
// Generated at container startup by entrypoint.sh — DO NOT EDIT
window._env_ = {
  REACT_APP_PLATFORM_API: "$(esc "${REACT_APP_PLATFORM_API:-}")",
  REACT_APP_GOJIRA_API: "$(esc "${REACT_APP_GOJIRA_API:-}")",
  REACT_APP_PUMLEX_SERVER: "$(esc "${REACT_APP_PUMLEX_SERVER:-}")",
  REACT_APP_SSO_URL: "$(esc "${REACT_APP_SSO_URL:-}")",
  REACT_APP_USE_SSO: "$(esc "${REACT_APP_USE_SSO:-}")",
  REACT_APP_DEV_USER: "$(esc "${REACT_APP_DEV_USER:-}")"
};
EOF

echo "[entrypoint] wrote $ENV_JS:"
cat "$ENV_JS"

exec nginx -g 'daemon off;'
