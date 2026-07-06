// 플랫폼 공통 런타임 env 로더 (계약 05 §5·§6).
// window._env_ (컨테이너 env.js) → 빌드도구 dev .env → 하드코딩 폴백.
// web-ui 모듈은 앱별 config.js 대신 이 캐노니컬 키만 사용한다 —
// 앱마다 config 키 이름이 갈라져(API_BASE 의미 상이) 복제본이 분기하던 부채의 원인 제거.
const e = (typeof window !== 'undefined' && window._env_) || {};
const pick = (key, fallback) =>
  e[key] || (import.meta.env && import.meta.env[key]) || fallback;

export const PLATFORM_API = pick('REACT_APP_PLATFORM_API', 'http://localhost:9000/api/v2');
export const GOJIRA_API = pick('REACT_APP_GOJIRA_API', 'http://localhost:9001/api/v2');
export const LAUNCHER_BASE = pick('REACT_APP_LAUNCHER_BASE', 'http://127.0.0.1:5599');
export const SSO_URL = pick('REACT_APP_SSO_URL', 'https://10.227.37.90:443');
export const USE_SSO = pick('REACT_APP_USE_SSO', 'false') === 'true';
export const DEV_USER = pick('REACT_APP_DEV_USER', 'duke.kimm');
