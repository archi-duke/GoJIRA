#!/usr/bin/env bash
# @platform/web-ui 를 npm tarball 로 패키징 — 외부 레포 제품(RAGaaS 등) 반입용 (계약 04 §2, 온보딩 B7).
#
# GoJIRA 모노레포 내부 앱은 Vite alias 로 소스를 직접 소비하지만(D11), 별도 레포는 alias 가
# 불가능하므로 tarball 을 npm install 한다. tarball 은 소스(src/) 그대로를 담고, 소비측 Vite 가
# .jsx 를 컴파일한다 (esbuild 가 확장자 기준으로 node_modules 안에서도 JSX 변환).
#
# 소비측(RAGaaS) 설치:
#   npm install ./platform-web-ui-<ver>.tgz
#   → import DataGrid from '@platform/web-ui/components/DataGrid' 등 서브패스 그대로 사용.
#   Kendo 의존은 tarball 의 dependencies 로 함께 설치됨 (react/react-dom 은 peer — 소비측 것 사용).
#   폐쇄망이면 Kendo 패키지가 소비측 npm 캐시/미러에 있어야 한다 (RAGaaS 는 이미 동일 버전 사용 중).
#
# 사용:
#   ./pack-web-ui.sh            # → deploy/dist/platform-web-ui-<ver>.tgz
#   ./pack-web-ui.sh /tmp/out   # 출력 디렉토리 지정
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
OUT_DIR="${1:-${REPO_ROOT}/deploy/dist}"

mkdir -p "${OUT_DIR}"
cd "${REPO_ROOT}/packages/web-ui"

echo "[ pack-web-ui ] npm pack → ${OUT_DIR}"
TARBALL="$(npm pack --pack-destination "${OUT_DIR}" | tail -1)"

echo "[ pack-web-ui ] 완료: ${OUT_DIR}/${TARBALL}"
echo "[ pack-web-ui ] 소비측 설치: npm install ./${TARBALL}"
