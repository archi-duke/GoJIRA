# 02. 인증 제공 계약 — `platformApp/auth`

> **원칙: remote 는 자체 로그인하지 않는다.** SSO 리다이렉트·코드 교환·토큰 보관·introspect 는 전부 셸 소유.
> remote 는 셸이 노출하는 `platformApp/auth` 모듈만 소비한다.

## 1. 제공 방식 결정: props 가 아니라 **MF exposed 모듈** — 근거

셸 소유자로서 다음 근거로 **exposed 모듈 방식**을 확정한다:

1. **axios 호환이 결정 요인.** RAGaaS 는 axios(XHR) 기반이다. props 는 React 컴포넌트 트리 안에서만 흐르는데,
   axios 인스턴스/인터셉터는 모듈 스코프에서 만들어진다. `import { getToken } from 'platformApp/auth'` 는
   모듈 스코프에서 바로 쓸 수 있지만, props 는 컨텍스트→전역 재수출 같은 우회가 필요해진다.
   (GoJIRA 의 기존 `window.fetch` monkey-patch 가 XHR 을 못 잡는 문제와 같은 뿌리 — 전송계층에 숨기지 말고
   명시적 API 로 제공한다.)
2. **React 밖 코드 경로가 실재한다.** SSE/WebSocket 연결, 파일 다운로드, 런처 호출 등 컴포넌트 밖에서
   토큰이 필요한 지점이 이미 GoJIRA 에 있다.
3. **초기화 순서를 셸이 보장할 수 있다.** `platformApp/auth` 는 셸 부트스트랩(host-init) 시점에 준비 완료된다.
   셸은 **인증이 확정된 뒤에만 remote 를 마운트**하므로, remote 입장에서 이 모듈은 언제나 "로그인 끝난 상태"다.
4. props 는 보조 채널로 유지한다 — `user`/`authority` 를 마운트 props 로도 내려서([01](01-mf-host-contract.md) §2)
   React 렌더링에 자연스럽게 쓰게 한다. **진실의 원천은 auth 모듈**이고 props 는 그 스냅샷이다.

## 2. 모듈 API

```ts
// import * as auth from 'platformApp/auth'
export interface PlatformAuth {
  /** 현재 세션 토큰. 없으면 null (dev 모드 등). 동기 — 인터셉터에서 바로 호출 가능 */
  getToken(): string | null;

  /** introspect 로 확정된 사용자. 셸이 마운트 전에 확정하므로 remote 에선 null 아님 */
  getUser(): { loginid: string } | null;

  /** GET /Account/me 의 authority 맵 스냅샷. 예: { USR: 15, GITRP: 4 } */
  getAuthority(): Record<string, number> | null;

  /** fetch 호환 래퍼 — 인증 헤더(아래 §3 규칙)를 붙여 호출. fetch 사용 앱은 이것만 쓰면 됨 */
  authFetch(input: RequestInfo | URL, init?: RequestInit): Promise<Response>;

  /** 세션 변경(사용자 전환·만료·로그아웃) 구독. 반환값은 구독 해제 함수 */
  onSessionChange(cb: (s: { user: string | null }) => void): () => void;

  /** 401 등으로 세션 무효를 감지한 remote 가 호출 — 셸이 재로그인 흐름(SSO 리다이렉트)을 수행 */
  requireLogin(): void;
}
```

### 의미 규칙

- `getToken()` 은 **동기**다. 셸이 토큰을 갱신하면 이후 호출부터 새 값을 돌려준다. remote 는 토큰을 변수에
  **저장해 두지 말고 요청 시마다 호출**한다.
- remote 는 토큰을 자체 저장(쿠키/localStorage)하거나 SSO 서버를 직접 호출하지 않는다.
- 401 응답을 받으면 재시도하지 말고 `requireLogin()` 을 호출하고 흐름을 중단한다.

## 3. 인증 헤더 규칙 (authFetch 가 하는 일 — axios 앱은 직접 구현)

| 조건 | 붙이는 헤더 |
|------|-------------|
| `getToken()` 이 토큰 반환 (SSO 모드) | `Authorization: Bearer <token>` (이미 있으면 덮어쓰지 않음) |
| 토큰 없음 (dev 모드, USE_SSO=false) | `X-User-Id: <getUser().loginid>` — 백엔드 dev 폴백용 ([05](05-backend-contract.md) §2) |
| 대상이 플랫폼 API 가 아닌 외부 호출 | 아무것도 붙이지 않음 (토큰 유출 방지 — 호출측이 authFetch 대신 raw fetch 사용) |

### axios 소비 패턴 (RAGaaS 참조 구현)

```ts
import axios from 'axios';
import { getToken, getUser, requireLogin } from 'platformApp/auth';

export const api = axios.create({ baseURL: window._env_.REACT_APP_RAGAAS_API });

api.interceptors.request.use((cfg) => {
  const t = getToken();
  if (t) cfg.headers.Authorization = `Bearer ${t}`;
  else if (getUser()) cfg.headers['X-User-Id'] = getUser()!.loginid;  // dev 폴백
  return cfg;
});

api.interceptors.response.use(undefined, (err) => {
  if (err.response?.status === 401) requireLogin();
  return Promise.reject(err);
});
```

## 4. 셸 내부 동작 (remote 는 몰라도 되지만 기록)

- 토큰 보관: `SSO_SESSION` 쿠키 (path=/, max-age 8h). 셸만 읽고 쓴다.
- 로그인 흐름: 미인증 → `{SSO_URL}/Account/ADFSLogin?client=<origin>` 리다이렉트 → 콜백 `?auth=1&code=` →
  `/Account/exchange?code=` 로 sessionToken 교환 → 쿠키 저장 → `/Account/introspect?token=` 으로 loginid 확정.
- **loginid 는 클라이언트에 저장하지 않는다** (자가발급 신원 = 사칭 가능). 매 세션 introspect 로 도출.
- 신원 확정(`user`)과 권한 로드(`authority`) 전에는 remote 를 마운트하지 않는다.

## 5. Standalone 폴백 (remote 의무)

remote 단독 실행 시 `platformApp/auth` 가 없다. remote 는 **어댑터 한 겹**으로 이를 흡수한다:

```ts
// src/platform/auth.ts — remote 쪽 유일한 소비 지점 (앱 코드는 이 파일만 import)
import type { PlatformAuth } from './types';

let impl: PlatformAuth;
try {
  impl = await import('platformApp/auth');          // 셸 안에서 실행 중
} catch {
  impl = createStandaloneAuth();                     // 단독 실행 — 아래 규칙
}
export const { getToken, getUser, authFetch, requireLogin, onSessionChange, getAuthority } = impl;
```

standalone 구현 규칙 (셸과 동일 의미 보장):
- `USE_SSO=true` 환경: `SSO_SESSION` 쿠키 → introspect 로 신원 확정 (셸 §4 와 동일 절차의 축소판).
- dev 환경: `window._env_`/설정의 dev 사용자로 `getUser()` 를 채우고 `getToken()` 은 null.

## 6. 폐기되는 패턴 (P2 에서 제거)

| 기존 | 위치 | 대체 |
|------|------|------|
| `window.fetch` monkey-patch | Platform-App/src/App.js:20-39 | 셸 내부 authFetch 구현으로 흡수 |
| `window.fetch` monkey-patch + `window.__gojiraUser` | GoJIRA-App/src/GoJIRAApp.js:26-53 | `platformApp/auth` 소비로 대체 |
| `window.__platformFetchWrapped` / `__gojiraFetchWrapped` 가드 | 상동 | 불필요 (래핑 자체가 사라짐) |
