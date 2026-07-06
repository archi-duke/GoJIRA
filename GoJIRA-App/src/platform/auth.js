// 플랫폼 인증 어댑터 — 앱 코드의 유일한 인증 소비 지점 (계약 docs/platform-contract/02 §5).
//
// 셸(Platform-App) 안에서 실행 중이면: 셸 auth 모듈이 로드 시 게시한 window.__platformAuth
// 를 그대로 사용 (같은 페이지 — 셸이 remote 마운트 전에 반드시 로드해 둠).
// standalone(단독 :3001)이면: 아래 로컬 구현 — 구 fetch monkey-patch 와 동일 의미
// (SSO_SESSION 토큰 → Bearer, 없으면 window.__gojiraUser → X-User-Id dev 폴백).
import config from '../config';

function getShellAuth() {
  return (typeof window !== 'undefined' && window.__platformAuth) || null;
}

// ── standalone 로컬 구현 ──
const apiBases = [config.PLATFORM_API, config.API_BASE].filter(Boolean);
const isApi = (url) => url.includes('/api/v2') || apiBases.some((b) => url.startsWith(b));

function localGetToken() {
  if (typeof document === 'undefined') return null;
  const m = document.cookie.match(/(^| )SSO_SESSION=([^;]+)/);
  return m ? decodeURIComponent(m[2]) : null;
}

function localGetUser() {
  const u = typeof window !== 'undefined' ? window.__gojiraUser : null;
  return u ? { loginid: u } : null;
}

function localAuthFetch(input, init) {
  const url = typeof input === 'string' ? input : (input && input.url) || '';
  if (isApi(url)) {
    init = init || {};
    const headers = new Headers(init.headers || {});
    const token = localGetToken();
    if (token) {
      if (!headers.has('Authorization')) headers.set('Authorization', 'Bearer ' + token);
    } else {
      const user = localGetUser();
      if (user && !headers.has('X-User-Id')) headers.set('X-User-Id', user.loginid);
    }
    init.headers = headers;
  }
  return fetch(input, init);
}

function localRequireLogin() {
  if (config.USE_SSO && typeof window !== 'undefined') {
    document.cookie = 'SSO_SESSION=; path=/; max-age=0';
    window.location.href = `${config.SSO_URL}/Account/ADFSLogin?client=${window.location.origin}`;
  }
}

// ── 공개 API — 호출 시점에 셸 게시본 우선 (모듈 로드 순서 무관하게 안전) ──
export const getToken = () => (getShellAuth()?.getToken ?? localGetToken)();
export const getUser = () => (getShellAuth()?.getUser ?? localGetUser)();
export const getAuthority = () => getShellAuth()?.getAuthority?.() ?? null;
export const authFetch = (input, init) =>
  (getShellAuth()?.authFetch ?? localAuthFetch)(input, init);
export const requireLogin = () => (getShellAuth()?.requireLogin ?? localRequireLogin)();
export const onSessionChange = (cb) =>
  getShellAuth()?.onSessionChange?.(cb) ?? (() => {});
