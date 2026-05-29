### Platform-App — React build → nginx 정적 서빙 (런타임 config)
#
# 빌드된 번들은 host-agnostic — REACT_APP_* 를 빌드시점에 inline 하지 않는다.
# nginx 컨테이너 startup 시 entrypoint.sh 가 env → /usr/share/nginx/html/env.js
# 를 생성해 window._env_ 로 노출. React (src/config.js) 가 1순위로 읽음.
# → 폐쇄망에서 .env 만 바꾸고 docker compose up -d 하면 끝 (재빌드 불요).
#
# Builder base = gojira/build-cache-node — node_modules 캐시 포함, 외부 npm 불필요.

FROM gojira/build-cache-node:latest AS builder
WORKDIR /src
RUN cp -r /cache/Platform-App/node_modules ./node_modules && \
    cp /cache/Platform-App/package.json ./package.json && \
    cp /cache/Platform-App/package-lock.json* ./ 2>/dev/null || true
COPY Platform-App/ ./
RUN npm run build

FROM nginx:alpine
COPY deploy/images/platform-app/nginx.conf /etc/nginx/conf.d/default.conf
COPY deploy/images/platform-app/entrypoint.sh /docker-entrypoint.d/40-env-js.sh
COPY --from=builder /src/build /usr/share/nginx/html
# 40-env-js.sh 는 nginx:alpine 의 docker-entrypoint.sh 가 자동으로 source/exec 함.
# (10-listen-on-ipv6-by-default.sh, 20-envsubst-on-templates.sh 와 동일 메커니즘)
RUN chmod +x /docker-entrypoint.d/40-env-js.sh
EXPOSE 3000
