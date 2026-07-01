#!/bin/sh
# GoJIRA 게이트웨이 — 컨테이너 startup 시 URL_SCHEME 에 따라 nginx 설정을 생성.
#   URL_SCHEME=https → 인증서로 TLS 종단 (fullchain.pem + privkey.pem 필요)
#   그 외            → 평문 HTTP
# 백엔드는 항상 컨테이너 내부 HTTP — 스킴 변경 시 React/백엔드 재빌드 불요, 컨테이너
# recreate (docker compose up -d) 만으로 반영. env.js 생성 패턴과 동일 사상.
#
# nginx:alpine 의 docker-entrypoint.sh 가 /docker-entrypoint.d/*.sh 를 실행한 뒤
# CMD(nginx) 를 exec 하므로, 이 스크립트는 설정만 쓰고 종료한다 (nginx 직접 exec X).
set -eu

SCHEME="${URL_SCHEME:-http}"
PORT="${GATEWAY_PORT:-8080}"
CERT="${TLS_CERT_FILE:-/etc/gojira/tls/fullchain.pem}"
KEY="${TLS_KEY_FILE:-/etc/gojira/tls/privkey.pem}"
CONF="/etc/nginx/conf.d/default.conf"

if [ "$SCHEME" = "https" ]; then
    if [ ! -r "$CERT" ] || [ ! -r "$KEY" ]; then
        echo "[gateway] URL_SCHEME=https 인데 인증서를 읽을 수 없음:" >&2
        echo "[gateway]   cert=$CERT  key=$KEY" >&2
        echo "[gateway] 호스트의 TLS_CERT_DIR 에 fullchain.pem + privkey.pem 을 두고 재시작하세요." >&2
        exit 1
    fi
    LISTEN="listen ${PORT} ssl;
    http2 on;
    ssl_certificate ${CERT};
    ssl_certificate_key ${KEY};
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;
    ssl_prefer_server_ciphers off;"
    FWD="https"
else
    LISTEN="listen ${PORT};"
    FWD="http"
fi

cat > "$CONF" <<EOF
# Generated at container startup by entrypoint.sh — DO NOT EDIT
map \$http_upgrade \$connection_upgrade {
    default upgrade;
    ''      close;
}

server {
    ${LISTEN}
    server_name _;

    # 외부에서 들어온 실제 스킴 — 백엔드에 X-Forwarded-Proto 로 전달.
    set \$forwarded_proto ${FWD};

    include /etc/nginx/gojira-locations.conf;
}
EOF

echo "[gateway] URL_SCHEME=${SCHEME} listen=${PORT} — wrote ${CONF}"
