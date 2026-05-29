### Platform-API 빌드
#
# Builder base = gojira/build-cache-go — go.work / go.mod 골격 + /go/pkg/mod 캐시가
# 미리 들어있어 외부 호출 없이 go build 가 가능 (폐쇄망 자체 빌드용).
# 외부망에서도 동일 동작 — 캐시에 있는 모듈을 그대로 사용한다.
#
# build context: 레포 루트.

FROM gojira/build-cache-go:latest AS builder
WORKDIR /src
# 캐시 이미지의 go.work / 모듈 골격 복사 — 의존성 download 가 끝난 상태.
RUN cp -r /cache/. /src/
# 소스 전체 (덮어쓰기)
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
