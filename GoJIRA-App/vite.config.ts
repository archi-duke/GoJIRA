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

// GoJIRA-App — MF remote (플랫폼 통합 계약 docs/platform-contract/01 §2)
// - expose './App' 표준 (구 './GoJIRAApp' 개명)
// - shared singleton: react/react-dom (requiredVersion 은 package.json range 자동)
// - 런타임 env(window._env_/env.js) 패턴은 그대로 — 빌드 산출물은 host-agnostic
export default defineConfig({
  plugins: [
    react(),
    federation({
      name: 'gojiraApp',
      filename: 'remoteEntry.js',
      dts: false, // JS 앱 — 타입 산출물(@mf-types) 생성 안 함

      exposes: {
        './App': './src/GoJIRAApp',
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
    port: 3001,
    strictPort: true,
    // dev 에서 셸(:3000)이 remoteEntry/chunk 를 크로스오리진 로드
    cors: true,
    origin: 'http://localhost:3001',
  },
  build: {
    target: 'chrome89', // MF 2.0 (top-level await) 요구
    outDir: 'build',    // CRA 산출 경로 유지 — deploy Dockerfile 무변경
  },
});
