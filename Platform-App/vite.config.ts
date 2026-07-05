import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { federation } from '@module-federation/vite';

// Platform-App — MFE 셸(호스트) (플랫폼 통합 계약 docs/platform-contract/01)
// remote URL 은 빌드에 inline 하지 않는다 — 여기 entry 는 dev 폴백일 뿐이고,
// 부트스트랩(src/apps/registry.js)이 window._env_.REACT_APP_GOJIRA_APP_REMOTE 로
// registerRemotes(force) 하여 런타임에 결정한다. 폐쇄망에서 env.js 만 바꾸면 반영.
export default defineConfig({
  plugins: [
    react(),
    federation({
      name: 'platformApp',
      dts: false, // JS 앱 — 타입 산출물(@mf-types) 생성 안 함
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
