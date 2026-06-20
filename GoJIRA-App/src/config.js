// 런타임 config — 컨테이너 startup 시 nginx entrypoint 가 생성하는
// /env.js (window._env_) 를 1순위로 읽고, dev 모드에서는 process.env.REACT_APP_*
// (CRA 의 .env / shell env) 로 fallback. 빌드된 번들은 host-agnostic 이므로
// 폐쇄망에서 .env 만 수정해도 재배포 없이 URL 갱신됨.
const e = (typeof window !== 'undefined' && window._env_) || {};
const pick = (key, fallback) =>
  e[key] || (typeof process !== 'undefined' && process.env && process.env[key]) || fallback;

const config = {
  // 공통 플랫폼 API (인증, 계정, 공지)
  PLATFORM_API: pick('REACT_APP_PLATFORM_API', 'http://localhost:9000/api/v2'),
  // GoJIRA 앱 API (이슈, 프로젝트)
  API_BASE: pick('REACT_APP_GOJIRA_API', 'http://localhost:9001/api/v2'),
  // pumlex (plantumlEx) 서버 — 인라인 메타 편집 + /render-with-layout (내부에서 plantuml-server:8080 호출)
  PUMLEX_SERVER: pick('REACT_APP_PUMLEX_SERVER', 'http://localhost:3030'),
  // 로컬 런처 (gojira-launcher) — 사용자 PC 의 127.0.0.1 데몬. "수정" 버튼이
  // VS Code + CLI(opencode|claude) 를 소환 (local-agent-mvp §2).
  LAUNCHER_BASE: pick('REACT_APP_LAUNCHER_BASE', 'http://127.0.0.1:5599'),
  SSO_URL: pick('REACT_APP_SSO_URL', 'https://10.227.37.90:443'),
  // .env(REACT_APP_USE_SSO=true) 로 활성화. env 값은 문자열이므로 'true' 비교.
  USE_SSO: pick('REACT_APP_USE_SSO', 'false') === 'true',
  DEV_USER: 'duke.kimm',
};

export default config;
