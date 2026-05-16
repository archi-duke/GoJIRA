### Platform-API 빌드 (go.work 포함 — repo root 가 build context)
# build context: 레포 루트 (deploy/ 의 부모). docker-compose.yml 의 context: .. 와 일치.

FROM golang:1.26-alpine AS builder
RUN apk add --no-cache git
WORKDIR /src
# 모듈 캐시 친화 — go.* 와 모든 사용 모듈의 go.mod 만 먼저
COPY go.work go.work.sum* ./
COPY Platform-API/go.mod Platform-API/go.sum Platform-API/
COPY GoJIRA-API/go.mod GoJIRA-API/go.sum GoJIRA-API/
COPY gojira-shared/go.mod gojira-shared/
RUN cd Platform-API && go mod download
# 소스 전체
COPY Platform-API/ Platform-API/
COPY gojira-shared/ gojira-shared/
COPY GoJIRA-API/ GoJIRA-API/
RUN cd Platform-API && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/platform-api .

### Runtime — git binary 필요 (Gitea 로컬 clone)
FROM alpine:3.20
RUN apk add --no-cache git ca-certificates tzdata && adduser -D -u 10001 app
WORKDIR /app
COPY --from=builder /out/platform-api /app/platform-api
# llm.json 은 volume mount 로 외부 주입 (deploy/config/llm.json → /app/config/llm.json)
# 그러나 fallback 으로 이미지에도 default 포함
COPY Platform-API/config/llm.json /app/config/llm.json
# 로컬 repo workspace
RUN mkdir -p /data/repos && chown -R app:app /data /app
USER app
ENV PLATFORM_REPO_DIR=/data/repos \
    PLATFORM_LLM_CONFIG=/app/config/llm.json \
    GOJIRA_FIX_REVIEW=0
EXPOSE 9000
CMD ["/app/platform-api"]
