### 런처 배포 이미지 — gojira/launcher-dist
#
# 역할: gojira-launcher 를 OS 3종(windows/linux/darwin)으로 cross-compile 하고
#       에이전트 자산(.claude/skills · opencode.json · .opencode/agents)을 embed 한 뒤,
#       version.txt + 각 바이너리 .sha256 사이드카까지 묶어 /launcher-dist 에 담는다.
#       기동 시 그 내용을 공유 볼륨으로 복사(init job)하고 종료한다 → platform-api 가 서빙.
#
# 왜 별도 이미지인가:
#   - 소스 복사 빌드(폐쇄망 자체 빌드) 모델에 통합 — `docker compose build` 가 런처까지 빌드.
#     호스트 Go 툴체인/별도 build-launcher.sh 불필요.
#   - launcher-dist 폴더 수동 반입(gitignore 함정) 제거 — 런처가 이미지로 docker save/load.
#   - platform-api 와 독립 릴리스 — 런처만 rebuild + up 가능.
#
# Builder base = gojira/build-cache-go (폐쇄망 자체 빌드용 캐시 — 외부 호출 없이 go build).
# build context: 레포 루트.

FROM gojira/build-cache-go:latest AS builder
WORKDIR /src

# 런처 소스 전체.
COPY gojira-launcher/ gojira-launcher/

# embed 자산 — 레포 루트가 단일 진실. 소스 복사본의 잔재를 지우고 루트 기준으로 재구성.
# (gojira-launcher/skills_embed.go 의 //go:embed all:assets 가 이 트리를 읽는다)
RUN rm -rf gojira-launcher/assets/.claude gojira-launcher/assets/.opencode gojira-launcher/assets/opencode.json
COPY .claude/skills/      gojira-launcher/assets/.claude/skills/
COPY opencode.json        gojira-launcher/assets/opencode.json
COPY .opencode/agents/    gojira-launcher/assets/.opencode/agents/
# clone 배포용 프로젝트 메모리(웹발 이슈 변경 저널 계약) — build-launcher.sh 와 동일 동기화.
COPY gojira-launcher/assets-extra/CLAUDE.md gojira-launcher/assets/.claude/CLAUDE.md

# cross-compile 3종 + manifest. gojira-launcher 는 독립 모듈 → GOWORK=off.
# 산출 레이아웃은 Platform-API/handlers/launcher.go 가 읽는 형태와 일치해야 한다:
#   /dist/<os-subdir>/gojira-launcher[.exe](+.sha256), /dist/version.txt
WORKDIR /src/gojira-launcher
RUN set -eu; \
    ver="$(grep -E '^const version = ' handlers.go | sed -E 's/.*"([^"]+)".*/\1/')"; \
    [ -n "$ver" ] || { echo 'handlers.go 에서 const version 을 찾지 못함' >&2; exit 1; }; \
    for spec in "windows amd64 windows-amd64" "linux amd64 linux-amd64" "darwin arm64 darwin-arm64"; do \
      set -- $spec; goos="$1"; goarch="$2"; sub="$3"; \
      ext=""; if [ "$goos" = windows ]; then ext=".exe"; fi; \
      fn="gojira-launcher$ext"; \
      mkdir -p "/dist/$sub"; \
      GOWORK=off CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
        go build -trimpath -ldflags="-s -w" -o "/dist/$sub/$fn" .; \
      sha256sum "/dist/$sub/$fn" | awk '{print $1}' > "/dist/$sub/$fn.sha256"; \
    done; \
    printf '%s\n' "$ver" > /dist/version.txt; \
    echo "[launcher-dist] v${ver} 빌드 완료:"; ls -R /dist

### init carrier — 공유 볼륨을 채우고 종료(restart: "no").
FROM alpine:3.20
COPY --from=builder /dist /launcher-dist
# 매 기동마다 공유 볼륨(/shared)을 최신 산출물로 덮어쓴다 → up -d 시 항상 최신.
CMD ["sh","-c","cp -r /launcher-dist/. /shared/ && echo '[launcher-dist] 공유 볼륨 채움:' && ls -R /shared"]
