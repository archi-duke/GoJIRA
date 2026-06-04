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

### Runtime — ca-certificates·tzdata 가 gojira/build-cache-alpine 에 선설치돼 있다
# (폐쇄망에서 apk add 가 CDN 을 타 깨지므로). adduser 는 busybox 내장 — 네트워크 불필요.
FROM gojira/build-cache-alpine:latest
RUN adduser -D -u 10001 app
WORKDIR /app
COPY --from=builder /out/gojira-api /app/gojira-api
# 프롬프트 템플릿 디렉토리 포함
COPY GoJIRA-API/prompts /app/prompts
USER app
ENV GOJIRA_PROMPTS_DIR=/app/prompts
EXPOSE 9001
CMD ["/app/gojira-api"]
