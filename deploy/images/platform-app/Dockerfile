### Platform-App — React build → nginx 정적 서빙
# REACT_APP_* 는 빌드 시점에 inline 되므로 build-arg 로 받는다.
# 빌드 후 호스트 변경 시 재빌드 필요 (docker-compose build platform-app).

FROM node:lts-alpine AS builder
WORKDIR /src
COPY Platform-App/package.json Platform-App/package-lock.json* ./
RUN npm ci --no-audit --no-fund
COPY Platform-App/ ./
# build-arg → env (CRA 의 .env 처리)
ARG REACT_APP_PLATFORM_API
ARG REACT_APP_GOJIRA_API
ARG REACT_APP_AGENT_API
ARG REACT_APP_GOJIRA_APP_REMOTE
ARG REACT_APP_SSO_URL
ENV REACT_APP_PLATFORM_API=${REACT_APP_PLATFORM_API} \
    REACT_APP_GOJIRA_API=${REACT_APP_GOJIRA_API} \
    REACT_APP_AGENT_API=${REACT_APP_AGENT_API} \
    REACT_APP_GOJIRA_APP_REMOTE=${REACT_APP_GOJIRA_APP_REMOTE} \
    REACT_APP_SSO_URL=${REACT_APP_SSO_URL}
RUN npm run build

FROM nginx:alpine
COPY deploy/images/platform-app/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /src/build /usr/share/nginx/html
EXPOSE 3000
