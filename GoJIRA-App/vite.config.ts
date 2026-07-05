import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { federation } from '@module-federation/vite';

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
