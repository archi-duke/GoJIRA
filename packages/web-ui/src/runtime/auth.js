// web-ui 내부용 인증 어댑터 (계약 02 §5 와 동일 패턴).
// 셸이 게시한 window.__platformAuth 를 1순위로 소비 — 셸/임베디드 remote 에선 항상 존재.
// standalone(셸 없음)에선 로컬 폴백: SSO_SESSION 쿠키 → Bearer, 없으면
// window.__gojiraUser(standalone 앱이 세팅) → X-User-Id dev 폴백.
import { PLATFORM_API, GOJIRA_API } from './env';

const shell = () => (typeof window !== 'undefined' && window.__platformAuth) || null;

const apiBases = [PLATFORM_API, GOJIRA_API].filter(Boolean);
const isApi = (url) => url.includes('/api/v2') || apiBases.some((b) => url.startsWith(b));

function localGetToken() {
  if (typeof document === 'undefined') return null;
  const m = document.cookie.match(/(^| )SSO_SESSION=([^;]+)/);
  return m ? decodeURIComponent(m[2]) : null;
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
      const user = typeof window !== 'undefined' ? window.__gojiraUser : null;
      if (user && !headers.has('X-User-Id')) headers.set('X-User-Id', user);
    }
    init.headers = headers;
  }
  return fetch(input, init);
}

export const getToken = () => (shell()?.getToken ?? localGetToken)();
export const authFetch = (input, init) => (shell()?.authFetch ?? localAuthFetch)(input, init);
