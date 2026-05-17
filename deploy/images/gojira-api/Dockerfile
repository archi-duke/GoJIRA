### GoJIRA-API 빌드

FROM golang:1.26-alpine AS builder
RUN apk add --no-cache git
WORKDIR /src
COPY go.work go.work.sum* ./
COPY Platform-API/go.mod Platform-API/go.sum Platform-API/
COPY GoJIRA-API/go.mod GoJIRA-API/go.sum GoJIRA-API/
COPY gojira-shared/go.mod gojira-shared/
RUN cd GoJIRA-API && go mod download
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
