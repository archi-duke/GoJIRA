### pumlex (plantumlEx) 서버 — Express PlantUML 렌더러
#
# GoJIRA-App 의 PexBlock 이 런타임에 `${PUMLEX_SERVER}/render-with-layout` 를 호출한다.
# 이 서버는 내부적으로 PLANTUML_URL 로 PlantUML 서버를 호출해 SVG 를 받은 뒤
# pex-core layout delta 를 적용해 돌려준다. (compose 의 plantuml-server 재사용)
#
# 소스는 deploy/vendor/pumlex/ 에 vendoring 되어 있어 외부망 접근 없이 빌드된다.
# 빌드 context 는 레포 루트 (docker-compose.yml 의 x-build-context: ..).

FROM node:lts-alpine
WORKDIR /app

# workspace 전체 복사 후 설치 (pex-server → @archi-duke/pex-core 워크스페이스 링크).
# 외부망 build 시점에 npm registry 에서 express/cheerio/plantuml-encoder 설치.
COPY deploy/vendor/pumlex/ ./
RUN npm install --omit=dev --no-audit --no-fund \
 && mkdir -p packages/pex-server/data/diagrams \
 && chown -R node:node /app

USER node
# server.js 는 PORT / PLANTUML_URL 을 환경변수로 읽는다 (compose 에서 주입).
ENV PORT=3030
EXPOSE 3030
WORKDIR /app/packages/pex-server
CMD ["node", "server.js"]
