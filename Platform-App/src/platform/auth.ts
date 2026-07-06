// 셸 인증 런타임 — 플랫폼 통합 계약 docs/platform-contract/02 의 구현.
//
// - MF exposed 모듈 'platformApp/auth' 로 노출 (vite.config.ts exposes)
// - 같은 페이지에 로드된 remote 를 위해 window.__platformAuth 로도 게시 —
//   remote 어댑터는 이 게시본을 1순위로 소비한다 (별도 네트워크 fetch 불필요, 결정적)
// - 기존 window.fetch monkey-patch 를 대체한다. 전송계층을 오버라이드하지 않고
//   명시적 authFetch / getToken 으로 제공 (axios 앱은 getToken 을 인터셉터에 연결)
import config from '../config';

export interface SessionInfo {
  user: string | null;
}

type SessionListener = (s: SessionInfo) => void;

let currentUser: string | null = null;
let currentAuthority: Record<string, number> | null = null;
const listeners = new Set<SessionListener>();

/** 현재 세션 토큰 (SSO_SESSION 쿠키). 없으면 null. 저장해 두지 말고 요청 시마다 호출할 것 */
export function getToken(): string | null {
  if (typeof document === 'undefined') return null;
  const m = document.cookie.match(/(^| )SSO_SESSION=([^;]+)/);
  return m ? decodeURIComponent(m[2]) : null;
}

/** introspect 로 확정된 사용자. 셸이 인증 확정 후에만 remote 를 마운트하므로 remote 에선 null 아님 */
export function getUser(): { loginid: string } | null {
  return currentUser ? { loginid: currentUser } : null;
}

/** GET /Account/me 의 authority 스냅샷 */
export function getAuthority(): Record<string, number> | null {
  return currentAuthority;
}

// 플랫폼 API 판정 — 인증 헤더는 플랫폼 API 에만 붙인다 (외부/SSO/런처 호출엔 미부착)
const apiBases = [config.API_BASE, config.GOJIRA_API].filter(Boolean) as string[];
function isApi(url: string): boolean {
  return url.includes('/api/v2') || apiBases.some((b) => url.startsWith(b));
}

/**
 * fetch 호환 래퍼 — 플랫폼 API 요청에 인증 헤더를 붙인다.
 * SSO: Authorization Bearer / dev(토큰 없음): X-User-Id (백엔드 dev 폴백, 계약 05 §1-5).
 * 플랫폼 API 가 아닌 URL 은 그대로 통과하므로 모든 호출부를 안전하게 대체 가능.
 */
export function authFetch(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
  const url =
    typeof input === 'string' ? input : input instanceof URL ? input.href : (input && input.url) || '';
  if (isApi(url)) {
    init = init || {};
    const headers = new Headers(init.headers || {});
    const token = getToken();
    if (token) {
      if (!headers.has('Authorization')) headers.set('Authorization', 'Bearer ' + token);
    } else if (currentUser && !headers.has('X-User-Id')) {
      headers.set('X-User-Id', currentUser);
    }
    init.headers = headers;
  }
  return fetch(input, init);
}

/** 세션 변경(사용자 전환·만료·로그아웃) 구독. 반환값 = 구독 해제 */
export function onSessionChange(cb: SessionListener): () => void {
  listeners.add(cb);
  return () => listeners.delete(cb);
}

/** 401 등 세션 무효를 감지한 remote 가 호출 — 셸이 재로그인 흐름을 수행 */
export function requireLogin(): void {
  if (config.USE_SSO && typeof window !== 'undefined') {
    document.cookie = 'SSO_SESSION=; path=/; max-age=0';
    window.location.href = `${config.SSO_URL}/Account/ADFSLogin?client=${window.location.origin}`;
  } else {
    console.warn('[platformApp/auth] requireLogin: dev 모드(USE_SSO=false) — 재로그인 흐름 없음');
  }
}

/**
 * 셸 내부 전용 (계약 외 — remote 는 호출 금지).
 * App.jsx 가 user/authority 확정·변경 시 호출해 모듈 상태를 동기화한다.
 */
export function __setSession(user: string | null, authority: Record<string, number> | null): void {
  const changed = user !== currentUser;
  currentUser = user;
  currentAuthority = authority;
  if (changed) listeners.forEach((cb) => cb({ user: currentUser }));
}

const api = { getToken, getUser, getAuthority, authFetch, onSessionChange, requireLogin };

// in-page 게시 — 같은 페이지의 remote 어댑터가 소비 (계약 02 §5)
if (typeof window !== 'undefined') {
  (window as unknown as { __platformAuth?: typeof api }).__platformAuth = api;
}

export default api;
