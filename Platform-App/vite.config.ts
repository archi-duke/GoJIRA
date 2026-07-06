import { fileURLToPath } from 'node:url';
import { createRequire } from 'node:module';
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { federation } from '@module-federation/vite';

// @platform/web-ui — 레포 내 소스 패키지 (계약 04, 1단계: alias 소비)
const webUiSrc = fileURLToPath(new URL('../packages/web-ui/src', import.meta.url));
// web-ui 소스가 import 하는 서드파티는 앱 밖 경로라 bare 해석이 안 됨 → 앱 node_modules 로 고정.
// (react/react-dom 은 MF shared 변환이 처리하므로 alias 금지)
const require = createRequire(import.meta.url);
const webUiDeps = {
  html2canvas: require.resolve('html2canvas'),
  diff: require.resolve('diff'),
};

// Platform-App — MFE 셸(호스트) (플랫폼 통합 계약 docs/platform-contract/01)
// remote URL 은 빌드에 inline 하지 않는다 — 여기 entry 는 dev 폴백일 뿐이고,
// 부트스트랩(src/apps/registry.js)이 window._env_.REACT_APP_GOJIRA_APP_REMOTE 로
// registerRemotes(force) 하여 런타임에 결정한다. 폐쇄망에서 env.js 만 바꾸면 반영.
export default defineConfig({
  plugins: [
    react(),
    federation({
      name: 'platformApp',
      filename: 'remoteEntry.js',
      dts: false, // JS 앱 — 타입 산출물(@mf-types) 생성 안 함
      // 셸 런타임 제공 모듈 (계약 02·03) — remote 는 어댑터 한 겹으로 소비
      exposes: {
        './auth': './src/platform/auth',
      },
      remotes: {
        gojiraApp: {
          type: 'module',
          name: 'gojiraApp',
          entry: 'http://localhost:3001/remoteEntry.js',
        },
      },
      shared: {
        react: { singleton: true },
        'react-dom': { singleton: true },
      },
    }),
  ],
  resolve: {
    alias: { '@platform/web-ui': webUiSrc, ...webUiDeps },
  },
  // CRA 의 REACT_APP_* env 관례 유지 (dev .env 호환)
  envPrefix: ['REACT_APP_', 'VITE_'],
  server: {
    port: 3000,
    strictPort: true,
  },
  build: {
    target: 'chrome89', // MF 2.0 (top-level await) 요구
    outDir: 'build',    // CRA 산출 경로 유지 — deploy Dockerfile 무변경
  },
});
