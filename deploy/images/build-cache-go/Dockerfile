### Go 의존성 캐시 이미지 — 폐쇄망 자체 빌드용
#
# 외부망에서 한 번만 빌드해 docker save 로 tar 만들고 폐쇄망에 docker load.
# 이후 서비스 Dockerfile(platform-api/gojira-api/...) 들이 builder stage 의
# base 를 `golang:1.26-alpine` → `gojira/build-cache-go:latest` 로 바꾸면
# 외부 호출 없이 go build 가능.
#
# 갱신 시점: 어느 모듈이라도 go.mod / go.sum 변경 시. 코드만 바뀌면 갱신 불필요.
#
# build context: 레포 루트.

FROM golang:1.26-alpine
RUN apk add --no-cache git ca-certificates
WORKDIR /cache

# go.work 멤버 (Platform-API, GoJIRA-API, gojira-shared) — 한 묶음
COPY go.work go.work.sum* ./
COPY Platform-API/go.mod Platform-API/go.sum Platform-API/
COPY GoJIRA-API/go.mod GoJIRA-API/go.sum GoJIRA-API/
COPY gojira-shared/go.mod gojira-shared/

# 독립 모듈 (go.work 미포함) — gojira-launcher, gojira-cli
COPY gojira-launcher/go.mod gojira-launcher/go.sum* gojira-launcher/
COPY gojira-cli/go.mod gojira-cli/go.sum* gojira-cli/

# 의존성 fetch — workspace 모듈은 go.work 기준 한 번에, 독립 모듈은 GOWORK=off 로 개별
RUN cd Platform-API && go mod download && \
    cd ../GoJIRA-API && go mod download
RUN cd gojira-launcher && GOWORK=off go mod download
RUN cd gojira-cli && GOWORK=off go mod download

# 캐시 위치 확인용 (디버깅 — 정리 가능)
RUN ls -la /go/pkg/mod | head -3 || true
