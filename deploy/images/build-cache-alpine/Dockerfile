### Alpine 런타임 패키지 캐시 — 폐쇄망 자체 빌드용
#
# platform-api / gojira-api 의 runtime stage 는 git·ca-certificates·tzdata 가 필요한데,
# `FROM alpine:3.20` + `apk add` 는 빌드 시 Alpine CDN(dl-cdn.alpinelinux.org)을 타서
# 폐쇄망에서 깨진다(IO ERROR / unable to select packages). 이 패키지들을 미리 설치한
# 런타임 base 를 외부망에서 한 번 만들어 두면, 서비스 runtime stage 가 apk fetch 없이
# `FROM gojira/build-cache-alpine` 만 하면 된다.
#
# 갱신 시점: 런타임 패키지 추가/변경 시 (드묾).
# build context: 레포 루트(무관 — COPY 없음).

FROM alpine:3.20
RUN apk add --no-cache git ca-certificates tzdata
