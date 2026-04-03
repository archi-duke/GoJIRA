const config = {
  API_BASE: process.env.REACT_APP_API_BASE || 'http://localhost:9000/api/v2',
  SSO_URL: process.env.REACT_APP_SSO_URL || 'https://sso.example.com',
  // true: SSO 인증 사용, false: 고정 사용자 (개발용)
  USE_SSO: false,
  // USE_SSO=false일 때 사용할 고정 사용자 ID
  DEV_USER: 'duke.kimm',
};

export default config;
