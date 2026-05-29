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
