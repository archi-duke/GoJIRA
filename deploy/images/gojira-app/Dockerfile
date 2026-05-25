### GoJIRA-App — React build → nginx 정적 서빙 (런타임 config)
#
# 빌드된 번들은 host-agnostic — REACT_APP_* 를 빌드시점에 inline 하지 않는다.
# nginx 컨테이너 startup 시 entrypoint.sh 가 env → /usr/share/nginx/html/env.js
# 를 생성해 window._env_ 로 노출. React (src/config.js) 가 1순위로 읽음.

FROM node:lts-alpine AS builder
# craco.config.js 가 ../gojira-shared-web 으로 alias → 작업디렉토리 /src 의
# 상위(`/`) 에 공유 패키지를 복사. 컨텍스트는 레포 루트.
COPY gojira-shared-web/ /gojira-shared-web/
WORKDIR /src
COPY GoJIRA-App/package.json GoJIRA-App/package-lock.json* ./
RUN npm ci --no-audit --no-fund
COPY GoJIRA-App/ ./
RUN npm run build

FROM nginx:alpine
COPY deploy/images/gojira-app/nginx.conf /etc/nginx/conf.d/default.conf
COPY deploy/images/gojira-app/entrypoint.sh /docker-entrypoint.d/40-env-js.sh
COPY --from=builder /src/build /usr/share/nginx/html
RUN chmod +x /docker-entrypoint.d/40-env-js.sh
EXPOSE 3001
