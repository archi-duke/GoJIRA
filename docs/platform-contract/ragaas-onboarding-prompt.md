# RAGaaS → Platform-App 셸 합류 작업 지시서

> **대상**: RAGaaS 프론트엔드/백엔드 담당자. 코딩 에이전트(Claude Code 등)에게 그대로 전달 가능한 형태로 작성했습니다.
> **전제**: 이 문서만으로는 부족합니다 — GoJIRA 레포의 `docs/platform-contract/` 아래 7개 문서(README, 01~06, openapi-auth.yaml)를
> **반드시 함께 전달**하세요. 이 프롬프트는 그 계약을 RAGaaS 관점에서 실행 순서로 재구성한 것이지, 계약 자체를 대체하지 않습니다.
> 계약 문서와 이 프롬프트가 어긋나면 **계약 문서가 우선**입니다.

## 배경

GoJIRA 팀이 Platform-App 을 다제품 MFE(Micro-Frontend) 셸로 삼는 플랫폼 전략을 확정했습니다. RAGaaS 는 이 셸에 합류하는
**두 번째 제품(remote)** 입니다. 인증·계정·공지·디자인시스템(Kendo)처럼 여러 제품이 공유해야 하는 관심사는 셸(Platform-App)과
공용 백엔드(Platform-API)가 소유하고, RAGaaS 는 그 계약을 소비하는 쪽입니다.

## 0. RAGaaS 현재 상태 실사 결과 (2026-07-06, GoJIRA 팀이 확인한 사실 — 추정 아님)

| 항목 | 값 |
|---|---|
| 프론트 빌드 | Vite 5 + TypeScript |
| React | **18.3.1** ← 플랫폼 셸은 19.x, 버전 불일치 |
| 라우팅 | react-router-dom 7 |
| HTTP 클라이언트 | axios 1.13 (fetch 아님 — 아래 §2.3 인증 어댑터에서 중요) |
| Kendo | KendoReact **5.8.0**(exact pin), kendo-theme-bootstrap 5.8.0. 실사용 표면은 **Grid 단일** (`GraphDataTable.tsx`, `ChunksModal.tsx`) + `kendo-data-query` Sort/Filter Descriptor |
| Kendo 라이선스 | perpetual, **업데이트 구독 만료 2024-04-05** → 이 시점 이전 릴리스만 사용 가능. 5.8.0(2022 릴리스)은 커버됨. **5.8.0 이후 버전으로 올리지 말 것**(구독 갱신 전까지) |
| 백엔드 계약 | `graph_viewer.py` 가 Kendo Grid 의 Sort/Filter Descriptor **JSON 을 쿼리 파라미터로 그대로 수신**하는 계약이 이미 있음 — 이관 시에도 이 wire format 은 유지해야 함 |

## 1. 선행 조건 (이거 안 하면 나머지가 다 막힘)

### React 18.3.1 → 19.x 상향

플랫폼 셸(Platform-App)과 GoJIRA-App 이 이미 React 19.x 이고, Module Federation 의 `react`/`react-dom` shared
singleton 은 **같은 메이저 버전**이어야 런타임에 하나로 합쳐집니다. 다르면 MF 가 두 개의 React 인스턴스를 따로 로드하거나
아예 에러를 냅니다. **다른 모든 작업(§2)보다 먼저** 처리하세요.

- react-router-dom 7, axios 는 React 19 와 호환 문제 없음 (별도 조치 불요).
- Kendo 5.8.0 은 공식 지원이 React 18 까지지만, GoJIRA 쪽에서 React 19 셸에 KendoReact 5.8 Grid 를 얹어 실측 검증한 결과
  **렌더/소팅/필터/페이징 전부 정상 동작**했습니다(class 컴포넌트 기반이라 React 19 가 제거한 API 에 안 걸림). 다만 이건
  "우리 쪽 검증"이지 Kendo 의 공식 보증이 아니니, RAGaaS 쪽에서도 업그레이드 후 Grid 화면을 한 번 직접 확인하세요.

## 2. Phase 1 — 셸 합류 핵심 (우선순위 순)

### 2.1 Vite MF remote 설정

`@module-federation/vite` 사용. GoJIRA-App 참조 구현(`GoJIRA-App/vite.config.ts`, 이 레포에 있음)을 그대로 참고하세요.
핵심:

```js
federation({
  name: 'ragaasApp',
  filename: 'remoteEntry.js',
  exposes: { './App': './src/App' },   // 필수 — 마운트 진입점, 반드시 './App' 이름 사용
  shared: {
    react: { singleton: true },
    'react-dom': { singleton: true },
    // '@platform/web-ui': { singleton: true },  // §3 이관 이후에 추가
  },
})
```

- singleton 은 `react`/`react-dom`(추후 `@platform/web-ui`) **이 3개만**. 나머지 라이브러리는 각자 번들에 포함(공유 안 함).
- `requiredVersion` 을 문자열로 수동 고정하지 마세요 — package.json 의 range 를 그대로 쓰면 MF 런타임이 알아서 협상합니다.
- build target 은 `chrome89` 이상 권장 (MF 2.0 이 top-level await 사용).

### 2.2 마운트 컴포넌트 계약

`./App` 이 export 하는 컴포넌트는 이 props 를 받습니다:

```ts
interface RemoteAppProps {
  user: string;                 // 항상 존재 (셸이 인증 확정 후에만 마운트)
  authority: Record<string, number> | null;
  resetKey?: number;             // 셸 탭 재클릭 시 증가 — 내부 초기 화면 복귀용
  basePath?: string;
  onNavigate?: (page: string, ctx?: { projectCode?: string }) => void;
}
```

`onNavigate` 는 내부 페이지 전환 때마다(그리고 마운트 직후 1회, 복원 상태 동기화용) 호출하세요. 셸은 이 값으로 탭 활성
판정만 하고, RAGaaS 내부 라우팅(react-router)에는 관여하지 않습니다.

### 2.3 인증 어댑터 — **가장 중요한 부분**

**자체 로그인/토큰 저장/SSO 리다이렉트 절대 금지.** 셸이 인증을 끝낸 뒤에만 RAGaaS 를 마운트합니다.

셸(Platform-App)은 로드 시 `window.__platformAuth` 에 인증 런타임을 게시합니다(같은 페이지 안에서 실행되므로 항상
존재). RAGaaS 는 자기 레포 안에 어댑터 파일 **한 개**를 두고, 앱 코드 전체가 그 파일만 import 하게 하세요:

```ts
// src/platform/auth.ts — RAGaaS 쪽 유일한 소비 지점
const shell = () => (typeof window !== 'undefined' && (window as any).__platformAuth) || null;

export const getToken     = () => shell()?.getToken?.() ?? localGetToken();
export const getUser      = () => shell()?.getUser?.() ?? localGetUser();
export const getAuthority = () => shell()?.getAuthority?.() ?? null;
export const requireLogin = () => shell()?.requireLogin?.() ?? localRequireLogin();
```

axios 인터셉터에서 이걸 사용:

```ts
import axios from 'axios';
import { getToken, getUser, requireLogin } from './platform/auth';

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

`localGetToken`/`localGetUser`/`localRequireLogin` 은 **standalone(셸 없이 단독 실행) 폴백** 구현입니다 — 아래 §2.4.
참조 구현(fetch 버전이지만 패턴은 동일): GoJIRA 레포 `GoJIRA-App/src/platform/auth.js`.

### 2.4 Standalone 폴백

RAGaaS 를 셸 없이 단독 포트로 띄웠을 때 `window.__platformAuth` 가 없습니다. 이 경우:
- `USE_SSO=true` 환경: `SSO_SESSION` 쿠키 → introspect 로 신원 확정(§2.6 과 유사 절차의 프론트 축소판).
- dev 환경: 설정의 dev 사용자로 `getUser()` 를 채우고 `getToken()` 은 null → 인터셉터가 `X-User-Id` 로 폴백.

### 2.5 sessionStorage 네임스페이스

내부 UI 상태(필터/정렬/탭 등)를 sessionStorage 에 쓸 때 키에 `ragaas_` 접두사를 붙이세요. 셸/GoJIRA 와 키 충돌 방지용입니다.
셸과의 상태 공유는 sessionStorage 가 아니라 §2.2 의 props/onNavigate 로만 합니다.

### 2.6 백엔드 신원 확정 미들웨어 (Python)

`introspect` 는 Platform-API 가 제공하는 API 가 아니라, **각 백엔드가 사내 SSO 로 나가는 outbound 호출**입니다.

```python
import httpx, os

async def introspect(token: str) -> str | None:
    base = os.environ["SSO_INTROSPECT_URL"].rstrip("/")
    r = await client.get(f"{base}/Account/introspect",
                         params={"token": token}, timeout=3.0)
    if r.status_code != 200:
        return None
    d = r.json()
    if not d.get("loginid") or d.get("active") == "false":
        return None
    return d["loginid"]
```

미들웨어 전체 알고리즘 (요청마다):

```
1. OPTIONS                     → 통과
2. 공개 경로(예: /webhook/)     → 통과
3. X-Service-Token 헤더 있음:
     hmac.compare_digest(받은 값, env SERVICE_TOKEN) 일치?
       일치   → userID = X-User-Id 헤더(act-as). introspect 생략. 통과
       불일치 → 401
4. env USE_SSO == "true":
     Authorization: Bearer <token> → introspect(token)
       성공 → userID = loginid. 통과
       실패 → 401
5. (dev 폴백): userID = X-User-Id → 없으면 X-Dev-User → 없으면 env DEV_USER
```

- `SERVICE_TOKEN` 미설정이면 3번은 **항상 거부**.
- 토큰 비교는 `hmac.compare_digest` (timing attack 방지).
- 핸들러는 이 미들웨어가 확정한 신원만 신뢰 — body/쿼리의 user 필드 신뢰 금지.

서비스간 호출(RAGaaS-API ↔ Platform-API) 시 붙일 헤더:
```
X-User-Id: <행위 사용자, 있으면>
X-Service-Token: <env SERVICE_TOKEN>
```

### 2.7 백엔드 API 베이스 경로

`/api/v2/...` 로 노출하세요 (게이트웨이가 `/ragaas/api/` 프리픽스를 strip 하고 넘겨줍니다 — 예:
`GET /ragaas/api/v2/graph` → 백엔드는 `/api/v2/graph` 로 받음). 이 규약을 따르면 프론트 설정 조립 규칙
(`{gateway}/ragaas/api/v2`) 이 GoJIRA/Platform 과 동일해져 셸 쪽 작업이 단순해집니다.

### 2.8 런타임 env

`window._env_` 를 1순위로 읽는 config 로더를 쓰세요(Vite 라면 `import.meta.env` 를 dev 폴백으로). 컨테이너
entrypoint.sh 가 컨테이너 기동 시 env → `/env.js` 를 생성해 재빌드 없이 host/scheme 을 바꿀 수 있어야 합니다
(폐쇄망 배포 요건). 신규 키 예: `REACT_APP_RAGAAS_API`, `REACT_APP_RAGAAS_APP_REMOTE`.

## 3. Phase 2 — Kendo Grid 를 @platform/web-ui 로 이관 (Phase 1 합류 검증 후, 서두르지 않아도 됨)

**~~크로스레포 이슈~~ → 해소됨 (2026-07-06, GoJIRA 팀 B7 완료)**:

`@platform/web-ui` tarball 반입 경로가 준비됐습니다 — GoJIRA 레포 `deploy/scripts/pack-web-ui.sh` 실행 →
`deploy/dist/platform-web-ui-<ver>.tgz` 생성. **외부 Vite 프로젝트에서 tarball 설치 → 서브패스 import
(`@platform/web-ui/components/DataGrid` 등) → 빌드까지 검증 완료**했습니다. 소비:

```bash
npm install ./platform-web-ui-0.1.0.tgz
# Kendo 의존은 tarball 의 dependencies 로 함께 설치됨 (react/react-dom 은 peer — RAGaaS 것 사용)
```

이관 작업 시:
- RAGaaS 의 `@progress/kendo-*` 직접 의존을 제거하고 `@platform/web-ui` 의 `DataGrid` 래퍼로 교체
  (참조: GoJIRA 레포 `packages/web-ui/src/components/DataGrid.jsx` — kendo-data-query 기반 소팅/필터/페이징 래퍼)
- MF shared 에 `'@platform/web-ui': { singleton: true }` 추가
- **`GraphDataTable.tsx`/`ChunksModal.tsx` 가 백엔드로 보내는 Sort/Filter Descriptor JSON 포맷은 그대로 유지** —
  `graph_viewer.py` 의 쿼리 계약을 깨면 안 됩니다.
- 그 전까지는 RAGaaS 자체 Kendo 5.8.0 의존을 유지해도 됩니다. **단, 라이선스 만료(2024-04-05) 때문에 5.8.0 이후
  버전으로는 절대 올리지 마세요** — 구독 갱신은 별도 논의 사항입니다.

## 4. 공지(NoticeCenter) — 필요해지면

자체 공지 UI 는 만들지 마세요. 필요하면 셸이 노출하는 `platformApp/notice` 의 `NoticeCenter` 위젯을 마운트만 하면
됩니다(계약 03). 급하지 않으면 이번 온보딩 범위에서 제외해도 무방합니다.

## 5. GoJIRA(플랫폼) 쪽 준비 상태 — 2026-07-06 완료분

- ✅ 셸 registry 에 RAGaaS 엔트리 등록 (env `REACT_APP_RAGAAS_APP_REMOTE` 설정 시 탭 자동 노출, `/ragaas` 딥링크 포함)
- ✅ 게이트웨이 `/ragaas/`(→ragaas-frontend:80), `/ragaas/api/`(→ragaas-backend:8000) — **`/ragaas/api/` 는
  이미 라이브 관통 확인됨** (RAGaaS API 루트 응답 수신)
- ✅ `@platform/web-ui` tarball 반입 경로 (§3)
- ⏳ SERVICE_TOKEN 공유 — SSO 모드 전환 시점에 양쪽 .env 동일 값 배포

**RAGaaS 쪽에 필요한 compose 수정 1건 (검증 중 발견)**: `frontend` 서비스가 shared-net 에 미조인이라
게이트웨이가 `/ragaas/` 을 프록시하지 못합니다(현재 502). backend 처럼 추가해주세요:

```yaml
  frontend:
    networks:
      - default
      - shared-net
```

## 6. 완료 판정 체크리스트

- [ ] C1. 게이트웨이 경유로 셸 접속 → RAGaaS 탭 표시(권한 필터 동작) → 탭 클릭 시 remote 로드
- [ ] C2. RAGaaS remote 다운 상태에서 셸/GoJIRA 는 정상 동작, RAGaaS 탭만 폴백 UI
- [ ] C3. SSO 모드: RAGaaS 의 API 호출에 Bearer 자동 부착, 백엔드 introspect 검증 통과
- [ ] C4. dev 모드: 토큰 없이 X-User-Id 폴백 동작
- [ ] C5. RAGaaS 단독 실행(standalone) 정상
- [ ] C6. 재빌드 없이 env.js 값 변경만으로 게이트웨이 호스트/스킴 교체 반영

## 7. 요청 시 명확히 할 것

작업 중 계약과 다른 판단이 필요하면(예: RAGaaS 쪽 사정으로 다른 인증 소비 방식이 낫다고 판단되는 경우) **먼저
GoJIRA 팀과 논의**하세요 — 이 계약은 양쪽이 같은 규약을 공유해야 의미가 있어서, RAGaaS 단독으로 바꾸면 셸 쪽도
따라서 바뀌어야 합니다.
