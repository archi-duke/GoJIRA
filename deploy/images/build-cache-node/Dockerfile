### Node 의존성 캐시 이미지 — 폐쇄망 자체 빌드용
#
# 외부망에서 한 번만 빌드해 docker save 로 tar 만들고 폐쇄망에 docker load.
# 이후 서비스 Dockerfile(gojira-app, platform-app) 들이 builder stage 의
# base 를 `node:lts-alpine` → `gojira/build-cache-node:latest` 로 바꾸면
# 외부 호출 없이 npm run build 가능.
#
# 갱신 시점: 어느 앱이라도 package-lock.json 변경 시.
#
# build context: 레포 루트.
#
# 캐시 위치 — 각 앱의 /cache/<App>/node_modules. 서비스 빌드 시 그대로 복사 사용.

FROM node:lts-alpine
RUN apk add --no-cache python3 make g++   # 일부 패키지의 native build 대비
WORKDIR /cache

COPY GoJIRA-App/package.json GoJIRA-App/package-lock.json* GoJIRA-App/
COPY Platform-App/package.json Platform-App/package-lock.json* Platform-App/

RUN cd GoJIRA-App && npm ci --no-audit --no-fund
RUN cd Platform-App && npm ci --no-audit --no-fund

# @platform/web-ui — Kendo 의존을 단독 소유하는 소스 패키지 (계약 04).
# .npmrc(legacy-peer-deps)가 react 자동설치를 막는다 — react 가 web-ui/node_modules
# 에 생기면 앱과 이중 인스턴스가 되므로 반드시 함께 복사.
# Kendo 라이선스: 외부망 캐시 빌드 시 TELERIK_LICENSE build-arg(선택) — 미지정이어도
# 5.x 는 경고만 하고 설치는 진행된다.
ARG TELERIK_LICENSE
ENV TELERIK_LICENSE=${TELERIK_LICENSE}
COPY packages/web-ui/package.json packages/web-ui/package-lock.json* packages/web-ui/.npmrc packages/web-ui/
RUN cd packages/web-ui && npm ci --no-audit --no-fund

# pumlex (vendored workspace: pex-core + pex-server) — npm 레지스트리 접근을
# 빌드 시점에서 제거하기 위해 소스 전체 + 설치된 node_modules 를 캐시에 담는다.
# workspace 심링크(@archi-duke/pex-core → ../packages/pex-core) 때문에 node_modules
# 만 떼면 깨지기 쉬워, 전체 트리를 통째로 캐시하고 pumlex-server 가 그대로 복사한다.
# pumlex 는 vendoring 정적 소스라 소스 변경 시에만 갱신(사실상 의존성 변경 빈도와 동일).
# lock 파일이 없어 npm install(ci 아님). 캐시 위치: /cache/pumlex.
COPY deploy/vendor/pumlex/ pumlex/
RUN cd pumlex && npm install --omit=dev --no-audit --no-fund
