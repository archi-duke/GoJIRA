// 런타임 config — 컨테이너 startup 시 nginx entrypoint 가 생성하는
// /env.js (window._env_) 를 1순위로 읽고, dev 모드에서는 process.env.REACT_APP_*
// (CRA 의 .env / shell env) 로 fallback. 빌드된 번들은 host-agnostic 이므로
// 폐쇄망에서 .env 만 수정해도 재배포 없이 URL 갱신됨.
const e = (typeof window !== 'undefined' && window._env_) || {};
const pick = (key, fallback) =>
  e[key] || (typeof process !== 'undefined' && process.env && process.env[key]) || fallback;

const config = {
  // 공통 플랫폼 API (인증, 계정, 공지)
  API_BASE: pick('REACT_APP_PLATFORM_API', 'http://localhost:9000/api/v2'),
  // GoJIRA 앱 API (이슈, 프로젝트)
  GOJIRA_API: pick('REACT_APP_GOJIRA_API', 'http://localhost:9001/api/v2'),
  // Agent Service (Python, 스트리밍)
  AGENT_API: pick('REACT_APP_AGENT_API', 'http://localhost:9002/api/v2'),
  // GoJIRA-App base URL — Module Federation remote 와 채팅 popup 용.
  // Promise-based dynamic remote (craco.config.js) 에서도 동일 값을 사용.
  GOJIRA_APP: pick('REACT_APP_GOJIRA_APP_REMOTE', 'http://localhost:3001'),
  // SSO 설정
  SSO_URL: pick('REACT_APP_SSO_URL', 'https://10.227.37.90:443'),
  USE_SSO: false,
  DEV_USER: 'duke.kimm',
};

export default config;
