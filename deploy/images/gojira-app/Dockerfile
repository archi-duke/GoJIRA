### GoJIRA-App — React build → nginx 정적 서빙 (런타임 config)
#
# 빌드된 번들은 host-agnostic — REACT_APP_* 를 빌드시점에 inline 하지 않는다.
# nginx 컨테이너 startup 시 entrypoint.sh 가 env → /usr/share/nginx/html/env.js
# 를 생성해 window._env_ 로 노출. React (src/config.js) 가 1순위로 읽음.
#
# Builder base = gojira/build-cache-node — node_modules 가 이미 들어있어
# 외부 npm registry 없이 빌드 가능 (폐쇄망 자체 빌드용).

FROM gojira/build-cache-node:latest AS builder
WORKDIR /src
# 캐시 이미지의 node_modules 를 그대로 끌어옴 (npm ci 재실행 불필요)
RUN cp -r /cache/GoJIRA-App/node_modules ./node_modules && \
    cp /cache/GoJIRA-App/package.json ./package.json && \
    cp /cache/GoJIRA-App/package-lock.json* ./ 2>/dev/null || true
COPY GoJIRA-App/ ./
# 위 COPY 가 package.json 도 덮어쓰지만 lock 과 일관되도록 유지 (캐시 이미지가 같은 lock 사용 가정)
# 새 의존성 추가 시 build-cache-node 도 갱신 필요.
# @platform/web-ui 소스 패키지 — vite.config 이 ../packages/web-ui/src 로 alias 해석
# (/src 의 상위 = /packages). 신규 npm 의존성이 아니므로 build-cache-node 는 무관.
COPY packages/ /packages/
RUN npm run build

FROM nginx:alpine
COPY deploy/images/gojira-app/nginx.conf /etc/nginx/conf.d/default.conf
COPY deploy/images/gojira-app/entrypoint.sh /docker-entrypoint.d/40-env-js.sh
COPY --from=builder /src/build /usr/share/nginx/html

# 정적 파일 권한 설정 — nginx 가 읽을 수 있도록 (644: 파일, 755: 디렉토리)
RUN chmod +x /docker-entrypoint.d/40-env-js.sh && \
    find /usr/share/nginx/html -type d -exec chmod 755 {} \; && \
    find /usr/share/nginx/html -type f -exec chmod 644 {} \;
    
EXPOSE 3001
