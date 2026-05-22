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
  REACT_APP_AGENT_API: "$(esc "${REACT_APP_AGENT_API:-}")",
  REACT_APP_PLANTUML_SERVER: "$(esc "${REACT_APP_PLANTUML_SERVER:-}")",
  REACT_APP_PUMLEX_SERVER: "$(esc "${REACT_APP_PUMLEX_SERVER:-}")",
  REACT_APP_SSO_URL: "$(esc "${REACT_APP_SSO_URL:-}")"
};
EOF

echo "[entrypoint] wrote $ENV_JS:"
cat "$ENV_JS"

exec nginx -g 'daemon off;'
