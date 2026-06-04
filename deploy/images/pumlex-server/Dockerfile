### pumlex (plantumlEx) 서버 — Express PlantUML 렌더러
#
# GoJIRA-App 의 PexBlock 이 런타임에 `${PUMLEX_SERVER}/render-with-layout` 를 호출한다.
# 이 서버는 내부적으로 PLANTUML_URL 로 PlantUML 서버를 호출해 SVG 를 받은 뒤
# pex-core layout delta 를 적용해 돌려준다. (compose 의 plantuml-server 재사용)
#
# 소스는 deploy/vendor/pumlex/ 에 vendoring 되어 있다. 단, node_modules 는 vendoring
# 되어 있지 않으므로(package.json 만 존재), 폐쇄망 오프라인 빌드를 위해
# Builder base = gojira/build-cache-node 를 쓴다 — 그 안 /cache/pumlex 에
# vendor 소스 + 설치된 node_modules(+workspace 심링크)가 들어있어 npm registry
# 접근 없이 빌드된다. build context 는 레포 루트 (docker-compose.yml 의 x-build-context).

FROM gojira/build-cache-node:latest AS builder
# /cache/pumlex = vendor 소스 + 설치된 node_modules. 추가 작업 없음 — 그대로 복사.

FROM node:lts-alpine
WORKDIR /app
COPY --from=builder /cache/pumlex/ ./
RUN mkdir -p packages/pex-server/data/diagrams \
 && chown -R node:node /app

USER node
# server.js 는 PORT / PLANTUML_URL 을 환경변수로 읽는다 (compose 에서 주입).
ENV PORT=3030
EXPOSE 3030
WORKDIR /app/packages/pex-server
CMD ["node", "server.js"]
