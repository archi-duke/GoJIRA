/**
 * pex-core 동적 로더.
 *
 * 마이크로프론트엔드 환경 (컨테이너 :3000 + remote GoJIRA-App :3001) 에서
 * 컨테이너의 index.html 은 GoJIRA-App 의 public/index.html 을 사용하지 않으므로,
 * pex-core (geom/meta/inline) 가 page 에 자동 로드되지 않는다.
 * → PexBlock 마운트 시점에 GoJIRA-App 자기 origin 에서 동적으로 inject.
 *
 * pumlex vendor 파일 자체에는 손대지 않는다 (public/pex-core/*.js 그대로).
 */

let readyPromise = null;

function alreadyExposed() {
  return (
    typeof window !== 'undefined' &&
    window.PexGeom &&
    window.PexMeta &&
    window.PexInline
  );
}

/**
 * GoJIRA-App 자기 자산 base URL 결정.
 * - webpack public path (CRA 빌드 / MFE remote) 우선
 * - dev 컨테이너 (3000) → standalone GoJIRA-App (3001) 로 fallback
 * - 그 외엔 same-origin (relative)
 */
function pexAssetBase() {
  // eslint-disable-next-line no-undef, camelcase
  const wpPath = typeof __webpack_public_path__ === 'string' ? __webpack_public_path__ : '';
  if (wpPath && wpPath !== '/' && /^https?:\/\//.test(wpPath)) {
    return wpPath.replace(/\/$/, '');
  }
  if (typeof window !== 'undefined') {
    const { protocol, hostname, port } = window.location;
    if (port === '3000') return `${protocol}//${hostname}:3001`;
  }
  return '';
}

function loadScriptOrdered(srcs) {
  return srcs.reduce(
    (p, src) =>
      p.then(() => {
        if (document.querySelector(`script[data-pex="${src}"]`)) return undefined;
        return new Promise((resolve, reject) => {
          const s = document.createElement('script');
          s.src = src;
          s.async = false; // 순서 보장 (geom → meta → inline)
          s.dataset.pex = src;
          s.onload = () => resolve();
          s.onerror = () => reject(new Error(`pex-core script 로드 실패: ${src}`));
          document.head.appendChild(s);
        });
      }),
    Promise.resolve(),
  );
}

export function loadPexCore() {
  if (readyPromise) return readyPromise;
  if (typeof window === 'undefined') return Promise.resolve();
  if (alreadyExposed()) {
    readyPromise = Promise.resolve();
    return readyPromise;
  }

  readyPromise = (async () => {
    const base = pexAssetBase();
    const saved = {
      module: window.module,
      exports: window.exports,
      require: window.require,
    };
    // pex-core 는 UMD. webpack/dev-server runtime 이 module/exports/require 를
    // 노출하면 Node 분기로 빠지므로, 로드 동안 임시로 가린다.
    window.module = undefined;
    window.exports = undefined;
    window.require = undefined;
    try {
      await loadScriptOrdered([
        `${base}/pex-core/geom.js`,
        `${base}/pex-core/meta.js`,
        `${base}/pex-core/inline.js`,
      ]);
    } finally {
      window.module = saved.module;
      window.exports = saved.exports;
      window.require = saved.require;
    }
    if (!alreadyExposed()) {
      throw new Error('pex-core 로드 후에도 window.PexInline 이 노출되지 않았습니다.');
    }
  })();

  // 실패 시 다음 호출에서 재시도 가능하도록 promise reset
  readyPromise.catch(() => {
    readyPromise = null;
  });

  return readyPromise;
}
