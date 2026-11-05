#!/usr/bin/env bash
# publish-dist.sh — 서명된 산출물을 Platform-API 가 서빙하는 배포 디렉터리로 게시한다 (설계 §10.6).
#
# 배치 전제: signer runner 가 Platform-API 와 **같은 호스트**(GoJIRA 운영 서버)에서 돈다.
#   → rsync·배포 계정 없이 로컬 복사로 끝난다. runner 를 컨테이너로 띄웠다면
#     호스트의 배포 디렉터리를 bind mount 하고 그 경로를 LAUNCHER_DIST_PUBLISH_DIR 로 준다.
#
# 환경변수:
#   LAUNCHER_DIST_PUBLISH_DIR   게시 대상 (미설정이면 게시를 건너뛰고 경고만 — 로컬 빌드 보호)
#
# 사용:
#   publish-dist.sh launcher    런처 산출물 (바이너리·sha256·version.txt·release-manifest+sig)
#   publish-dist.sh catalog     카탈로그 (catalog.json + .sig)
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/_lib.sh"

SRC="${DEPLOY_DIR}/launcher-dist"
what="${1:-}"
[ "$what" = launcher ] || [ "$what" = catalog ] || die "사용: publish-dist.sh launcher|catalog"

DEST="${LAUNCHER_DIST_PUBLISH_DIR:-}"
if [ -z "$DEST" ]; then
  warn "LAUNCHER_DIST_PUBLISH_DIR 미설정 — 게시를 건너뜁니다 (로컬 빌드로 간주)"
  exit 0
fi
[ -d "$DEST" ] || die "게시 대상 디렉터리가 없습니다: $DEST"

# 같은 경로를 가리키면(운영 서버에서 레포를 직접 체크아웃해 쓰는 구성) 복사 불필요.
if [ "$(cd "$SRC" && pwd -P)" = "$(cd "$DEST" && pwd -P)" ]; then
  ok "게시 대상이 빌드 위치와 동일 — 복사 생략 ($DEST)"
  exit 0
fi

copy_one() {
  local rel="$1"
  [ -e "${SRC}/${rel}" ] || return 0
  mkdir -p "$(dirname "${DEST}/${rel}")"
  cp -f "${SRC}/${rel}" "${DEST}/${rel}"
  log "  → ${rel}"
}

case "$what" in
  launcher)
    [ -f "${SRC}/release-manifest.json.sig" ] || die "서명(.sig) 없음 — 서명 단계를 먼저 실행"
    for sub in windows-amd64 linux-amd64 darwin-arm64; do
      for f in gojira-launcher gojira-launcher.exe; do
        copy_one "${sub}/${f}"
        copy_one "${sub}/${f}.sha256"
      done
    done
    copy_one version.txt
    copy_one release-manifest.json
    copy_one release-manifest.json.sig
    ok "런처 산출물 게시 완료 → $DEST"
    ;;
  catalog)
    [ -f "${SRC}/catalog.json.sig" ] || die "서명(.sig) 없음 — 서명 단계를 먼저 실행"
    copy_one catalog.json
    copy_one catalog.json.sig
    ok "카탈로그 게시 완료 → $DEST"
    ;;
esac
