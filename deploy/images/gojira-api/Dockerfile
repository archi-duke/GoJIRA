### GoJIRA-API 빌드
#
# Builder base = gojira/build-cache-go (의존성 캐시 포함). 폐쇄망 외부 호출 0회.
# build context: 레포 루트.

FROM gojira/build-cache-go:latest AS builder
WORKDIR /src
RUN cp -r /cache/. /src/
COPY GoJIRA-API/ GoJIRA-API/
COPY gojira-shared/ gojira-shared/
COPY Platform-API/ Platform-API/
RUN cd GoJIRA-API && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/gojira-api .

### Runtime — exec 안 함 → scratch 가능. 다만 ca-certificates 위해 alpine 사용
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 app
WORKDIR /app
COPY --from=builder /out/gojira-api /app/gojira-api
# 프롬프트 템플릿 디렉토리 포함
COPY GoJIRA-API/prompts /app/prompts
USER app
ENV GOJIRA_PROMPTS_DIR=/app/prompts
EXPOSE 9001
CMD ["/app/gojira-api"]
