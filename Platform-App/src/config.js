// 런타임 config — 컨테이너 startup 시 nginx entrypoint 가 생성하는
// /env.js (window._env_) 를 1순위로 읽고, dev 모드에서는 import.meta.env.REACT_APP_*
// (Vite .env — envPrefix 'REACT_APP_' 유지) 로 fallback. 빌드된 번들은 host-agnostic
// 이므로 폐쇄망에서 .env 만 수정해도 재배포 없이 URL 갱신됨.
const e = (typeof window !== 'undefined' && window._env_) || {};
const pick = (key, fallback) =>
  e[key] || (import.meta.env && import.meta.env[key]) || fallback;

const config = {
  // 공통 플랫폼 API (인증, 계정, 공지)
  API_BASE: pick('REACT_APP_PLATFORM_API', 'http://localhost:9000/api/v2'),
  // GoJIRA 앱 API (이슈, 프로젝트)
  GOJIRA_API: pick('REACT_APP_GOJIRA_API', 'http://localhost:9001/api/v2'),
  // GoJIRA-App base URL — Module Federation remote 용.
  // Promise-based dynamic remote (craco.config.js) 에서도 동일 값을 사용.
  GOJIRA_APP: pick('REACT_APP_GOJIRA_APP_REMOTE', 'http://localhost:3001'),
  // SSO 설정
  SSO_URL: pick('REACT_APP_SSO_URL', 'https://10.227.37.90:443'),
  // .env(REACT_APP_USE_SSO=true) 로 활성화. env 값은 문자열이므로 'true' 비교.
  USE_SSO: pick('REACT_APP_USE_SSO', 'false') === 'true',
  // dev 폴백 신원 — 런타임 키로 주입 가능 (백엔드 env DEV_USER 와 짝, 계약 05 §6)
  DEV_USER: pick('REACT_APP_DEV_USER', 'duke.kimm'),
  // 로컬 런처 (사용자 PC) — 로고/타이틀 클릭 → /chat 으로 에이전트 spawn.
  LAUNCHER_BASE: pick('REACT_APP_LAUNCHER_BASE', 'http://127.0.0.1:5599'),
};

export default config;
