### GoJIRA-App — React build → nginx 정적 서빙

FROM node:lts-alpine AS builder
WORKDIR /src
COPY GoJIRA-App/package.json GoJIRA-App/package-lock.json* ./
RUN npm ci --no-audit --no-fund
COPY GoJIRA-App/ ./
ARG REACT_APP_PLATFORM_API
ARG REACT_APP_GOJIRA_API
ARG REACT_APP_AGENT_API
ARG REACT_APP_PLANTUML_SERVER
ARG REACT_APP_PUMLEX_SERVER
ARG REACT_APP_SSO_URL
ENV REACT_APP_PLATFORM_API=${REACT_APP_PLATFORM_API} \
    REACT_APP_GOJIRA_API=${REACT_APP_GOJIRA_API} \
    REACT_APP_AGENT_API=${REACT_APP_AGENT_API} \
    REACT_APP_PLANTUML_SERVER=${REACT_APP_PLANTUML_SERVER} \
    REACT_APP_PUMLEX_SERVER=${REACT_APP_PUMLEX_SERVER} \
    REACT_APP_SSO_URL=${REACT_APP_SSO_URL}
RUN npm run build

FROM nginx:alpine
COPY deploy/images/gojira-app/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /src/build /usr/share/nginx/html
EXPOSE 3001
