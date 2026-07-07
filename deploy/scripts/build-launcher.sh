#!/usr/bin/env bash
# 런처 단일 exe 빌드 — 스킬(.claude)을 embed 하고 OS별 바이너리 + 자동업데이트
# 매니페스트(version.txt + .sha256)를 deploy/launcher-dist/ 로 산출한다.
#
# 산출물(이미 docker-compose 가 platform-api 에 bind mount → /api/v2/launcher/* 로 서빙):
#   deploy/launcher-dist/version.txt                                  (런처 const version)
#   deploy/launcher-dist/windows-amd64/gojira-launcher.exe(+.sha256)
#   deploy/launcher-dist/linux-amd64/gojira-launcher(+.sha256)
#   deploy/launcher-dist/darwin-arm64/gojira-launcher(+.sha256)
#
# 폐쇄망 절차: 외부망에서 이 스크립트 1회 실행 → launcher-dist 를 배포물에 포함 →
# 사내에서 docker compose up. 사용자는 /api/v2/launcher/download/{os} 로 받아
# `gojira-launcher install` 1회 실행하면 스킬 배포 + 자동시작 + 데몬기동까지 끝.
#
# 사용:
#   ./build-launcher.sh                 # 3개 OS 전부
#   ./build-launcher.sh --os windows    # 특정 OS 만 (windows|linux|darwin)
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/_lib.sh"

ROOT_DIR="$(cd "${DEPLOY_DIR}/.." && pwd)"
LAUNCHER_SRC="${ROOT_DIR}/gojira-launcher"
DIST_DIR="${DEPLOY_DIR}/launcher-dist"
GO_IMAGE="golang:1.26-alpine"

# OS 키 → "GOOS GOARCH subdir 파일명"
# 연관배열(declare -A) 대신 case — macOS 기본 bash 3.2 호환(연관배열은 bash 4+).
target_spec() {
  case "$1" in
    windows) echo "windows amd64 windows-amd64 gojira-launcher.exe" ;;
    linux)   echo "linux amd64 linux-amd64 gojira-launcher" ;;
    darwin)  echo "darwin arm64 darwin-arm64 gojira-launcher" ;;
    *)       return 1 ;;
  esac
}

SELECT=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    --os) shift; while [[ $# -gt 0 && "$1" != --* ]]; do SELECT+=("$1"); shift; done ;;
    -h|--help) sed -n '2,20p' "$0"; exit 0 ;;
    *) die "알 수 없는 옵션: $1 (--help 참조)" ;;
  esac
done
[[ ${#SELECT[@]} -eq 0 ]] && SELECT=(windows linux darwin)

require_docker
[[ -d "$LAUNCHER_SRC" ]] || die "런처 소스 없음: $LAUNCHER_SRC"
[[ -d "${ROOT_DIR}/.claude" ]] || die "스킬 소스 없음: ${ROOT_DIR}/.claude"

# 런처 버전 추출 (handlers.go 의 const version = "x.y.z").
VERSION="$(grep -E '^const version = ' "${LAUNCHER_SRC}/handlers.go" | sed -E 's/.*"([^"]+)".*/\1/')"
[[ -n "$VERSION" ]] || die "handlers.go 에서 const version 을 찾지 못함"
log "런처 버전 v${VERSION}"

# 에이전트 자산 embed 준비 — 필요한 것만 골라 assets/ 로 동기화(단일 진실 = 루트).
# ⚠️ .claude / .opencode 디렉터리 전체를 복사하면 안 된다 — 로컬 런타임 상태
#    (.claude/projects, .opencode/node_modules 수만개, settings.local.json, 자격증명 등)가
#    exe 에 박혀 수십 MB 로 부풀고 시크릿 유출 위험. 에이전트가 실제 읽는 것만 embed:
#      .claude/skills/ · .claude/CLAUDE.md(assets-extra 원본) · opencode.json · .opencode/agents/
log "에이전트 자산 동기화 → gojira-launcher/assets/ (skills + CLAUDE.md + opencode.json + agents 만)"
rm -rf "${LAUNCHER_SRC}/assets/.claude" "${LAUNCHER_SRC}/assets/.opencode" "${LAUNCHER_SRC}/assets/opencode.json"
mkdir -p "${LAUNCHER_SRC}/assets/.claude"
cp -r "${ROOT_DIR}/.claude/skills" "${LAUNCHER_SRC}/assets/.claude/skills"
# 프로젝트 메모리(CLAUDE.md) — 원본은 assets-extra/ (레포 루트 .claude/CLAUDE.md 는
# GoJIRA 개발 세션용이 아니므로 절대 복사하지 않는다 — clone 전용 메모리는 별도 관리).
# Claude Code 가 clone 의 .claude/CLAUDE.md 를 프로젝트 메모리로 자동 로드해,
# 웹발 이슈 변경 저널(GOJIRA_ISSUE_JOURNAL) 검토를 세션 시작 시 인지시킨다 (D10 Phase 3).
if [[ -e "${LAUNCHER_SRC}/assets-extra/CLAUDE.md" ]]; then
  cp "${LAUNCHER_SRC}/assets-extra/CLAUDE.md" "${LAUNCHER_SRC}/assets/.claude/CLAUDE.md"
else
  warn "  assets-extra/CLAUDE.md 없음 — 스킵"
fi
if [[ -e "${ROOT_DIR}/opencode.json" ]]; then
  cp "${ROOT_DIR}/opencode.json" "${LAUNCHER_SRC}/assets/opencode.json"
else
  warn "  opencode.json 없음 — 스킵"
fi
if [[ -d "${ROOT_DIR}/.opencode/agents" ]]; then
  mkdir -p "${LAUNCHER_SRC}/assets/.opencode"
  cp -r "${ROOT_DIR}/.opencode/agents" "${LAUNCHER_SRC}/assets/.opencode/agents"
else
  warn "  .opencode/agents 없음 — 스킵"
fi
# 혹시 모를 OS 메타파일 제거(.DS_Store 등) — embed 청결 유지.
find "${LAUNCHER_SRC}/assets" -name '.DS_Store' -delete 2>/dev/null || true
ASSET_FILES="$(find "${LAUNCHER_SRC}/assets" -type f ! -name README.txt | wc -l | tr -d ' ')"
log "  임베드 대상 ${ASSET_FILES}개 파일 (.claude/skills + .claude/CLAUDE.md + opencode.json + .opencode/agents)"

mkdir -p "$DIST_DIR"

# sha256 헬퍼 (sha256sum 또는 shasum -a 256).
sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}';
  else shasum -a 256 "$1" | awk '{print $1}'; fi
}

for key in "${SELECT[@]}"; do
  spec="$(target_spec "$key")" || die "알 수 없는 OS: $key (windows|linux|darwin)"
  read -r goos goarch subdir fname <<<"$spec"

  outdir="${DIST_DIR}/${subdir}"
  mkdir -p "$outdir"
  log "빌드 ${key} (${goos}/${goarch}) → ${subdir}/${fname}"

  # GOWORK=off — gojira-launcher 는 독립 모듈(go.work 워크스페이스 밖).
  docker run --rm \
    -v "${LAUNCHER_SRC}:/src" -w /src \
    -e GOOS="$goos" -e GOARCH="$goarch" -e CGO_ENABLED=0 -e GOWORK=off \
    "$GO_IMAGE" go build -trimpath -ldflags="-s -w" -o "/src/dist-out/${fname}" .

  # docker 가 /src/dist-out 에 쓴 결과를 dist 로 이동.
  mv "${LAUNCHER_SRC}/dist-out/${fname}" "${outdir}/${fname}"
  sha256_of "${outdir}/${fname}" > "${outdir}/${fname}.sha256"
  ok "  ${subdir}/${fname}  ($(du -h "${outdir}/${fname}" | awk '{print $1}'), sha256=$(cut -c1-12 "${outdir}/${fname}.sha256")...)"
done
rm -rf "${LAUNCHER_SRC}/dist-out"

# 자동 업데이트 매니페스트 버전.
printf '%s\n' "$VERSION" > "${DIST_DIR}/version.txt"
ok "version.txt = v${VERSION}"

ok "런처 빌드 완료 → ${DIST_DIR}"
log "다음: docker compose up -d platform-api (launcher-dist bind mount 즉시 반영)"
