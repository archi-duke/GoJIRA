const config = {
  // 공통 플랫폼 API (인증, 계정, 공지)
  PLATFORM_API: process.env.REACT_APP_PLATFORM_API || 'http://localhost:9000/api/v2',
  // GoJIRA 앱 API (이슈, 프로젝트)
  API_BASE: process.env.REACT_APP_API_BASE || 'http://localhost:9001/api/v2',
  // Agent Service (Python, 스트리밍)
  AGENT_API: process.env.REACT_APP_AGENT_API || 'http://localhost:9002/api/v2',
  PLANTUML_SERVER: process.env.REACT_APP_PLANTUML_SERVER || 'http://localhost:8080/svg/',
  // pumlex (plantumlEx) 서버 — 인라인 메타 편집 + /render-with-layout
  PUMLEX_SERVER: process.env.REACT_APP_PUMLEX_SERVER || 'http://localhost:3030',
  SSO_URL: process.env.REACT_APP_SSO_URL || 'https://10.227.37.90:443',
  USE_SSO: false,
  DEV_USER: 'duke.kimm',
};

export default config;
