# 01. MF 호스트 계약 — Module Federation 규약

> 셸(Platform-App)과 remote(제품 앱) 간 Module Federation 통합 규약.
> 빌드도구: **Vite + @module-federation/vite** (MF 2.0 런타임). remote 도 Vite 로 빌드한다.

## 1. 이름 규약

| 항목 | 규약 | 예 |
|------|------|----|
| 호스트 컨테이너 이름 | `platformApp` (고정) | — |
| remote 컨테이너 이름 | `<제품명 camelCase>App` | `gojiraApp`, `ragaasApp` |
| remote 엔트리 파일 | `remoteEntry.js` (앱 웹루트 직하) | `http://…/ragaas/remoteEntry.js` |
| 게이트웨이 경로 | `/<제품명>/` — 프론트, `/<제품명>/api/…` — 백엔드 ([05](05-backend-contract.md) §4) | `/gojira/`, `/ragaas/` |
| 런타임 env 키 | `REACT_APP_<제품명 대문자>_APP_REMOTE` | `REACT_APP_RAGAAS_APP_REMOTE` |

> env 키의 `REACT_APP_` 접두사는 CRA 잔재지만 **런타임 env(window._env_) 조회는 접두사 무관**하므로
> 기존 배포와의 연속성을 위해 유지한다. 계약의 본질은 키 이름 전체이지 접두사가 아니다.

## 2. Remote 가 셸에 노출해야 하는 것 (exposes)

```js
// remote 의 module federation 설정
exposes: {
  './App': './src/App',        // 필수 — 마운트 진입점
}
```

- **`./App` 은 필수**이며 default export 로 React 컴포넌트를 내보낸다.
- 추가 expose 는 자유이나 셸이 의존하는 것은 `./App` 뿐이다.
- (이행기) GoJIRA-App 의 기존 `./GoJIRAApp` 은 Vite 이전 시 `./App` 으로 표준화한다.

### 마운트 컴포넌트 계약 (props)

```ts
interface RemoteAppProps {
  /** introspect 로 확정된 사용자 loginid. 셸이 인증 완료 후에만 remote 를 마운트하므로 항상 존재 */
  user: string;
  /** 권한 맵 — GET /Account/me 응답의 authority. 예: { USR: 15, GITRP: 4 } */
  authority: Record<string, number> | null;
  /** 셸 탭 재클릭 등으로 초기 화면 복귀를 요구할 때 증가하는 카운터 */
  resetKey?: number;
  /** 셸이 remote 에 배정한 URL 프리픽스 (라우팅 표준 도입 시). 예: '/ragaas' */
  basePath?: string;
  /** remote 내부 페이지 전환을 셸에 알림 — 셸 탭 활성화 판정·현재 과제 추적용.
   *  ctx.projectCode: 현재 선택된 과제 코드 (없으면 ''). remote 는 페이지 전환마다 +
   *  마운트 직후(복원 상태 동기화) 호출한다 */
  onNavigate?: (page: string, ctx?: { projectCode?: string }) => void;
}
```

- **상태 공유는 이 props 와 콜백이 전부다.** sessionStorage 를 셸↔remote 채널로 쓰지 않는다
  (기존 `gojira_page`/`gojira_project` magic string 채널은 2026-07-06 D12 로 제거 — 셸은 더 이상 읽지 않는다).
- remote 내부 전용 UI 상태(sessionStorage)는 자유이나 키에 `<제품명>_` 접두사를 붙여 네임스페이스를 분리한다.
  remote 의 새로고침 복원(자기 세션키 사용)은 내부 구현으로 허용 — 복원 후 마운트 보고(onNavigate)로 셸과 동기화.
- **셸 URL**: 셸이 탭 수준 딥링크(`/account` 등, history API)를 소유한다. remote 내부 페이지는 URL 에 반영하지
  않는다 — remote 별 딥링크가 필요해지면 `basePath` 계약으로 확장.

## 3. 셸이 remote 에 노출하는 것 (호스트 expose — MF 2.0 양방향)

| 모듈 | 내용 | 계약 문서 |
|------|------|-----------|
| `platformApp/auth` | 인증 런타임 (getToken/authFetch/…) | [02-auth](02-auth-contract.md) |
| `platformApp/notice` | NoticeCenter 위젯 | [03-notice](03-notice-contract.md) |

- 이 모듈들은 **셸 부트스트랩(host-init) 시점에 로드 완료**되어 있다. remote 는 자신의 어떤 코드 경로에서든
  즉시 import 할 수 있다 (lazy 초기화 대기 불필요).
- remote 는 standalone 모드에서 이 모듈이 없을 때의 폴백을 갖춰야 한다 ([02](02-auth-contract.md) §5).

## 4. shared 정책 (singleton 목록과 버전 협상)

```js
shared: {
  react:              { singleton: true },
  'react-dom':        { singleton: true },
  '@platform/web-ui': { singleton: true },   // 도입 시점부터
}
```

| 규칙 | 내용 |
|------|------|
| singleton 목록 | `react`, `react-dom`, `@platform/web-ui` — **이 3개 외에는 singleton 금지** (각 앱이 자기 버전 번들) |
| 버전 정책 | React 메이저는 플랫폼이 공지 (현재 **19.x**). remote 는 같은 메이저의 semver range 로 의존 선언 |
| strictVersion | 사용하지 않음 — MF 2.0 런타임 협상에 맡기고, 협상 실패는 콘솔 경고로 노출 |
| requiredVersion | package.json 의 range 를 그대로 사용 (수동 고정 문자열 금지 — 기존 `'^19.2.4'` 하드코딩이 유발한 이중 관리 제거) |

> **왜 3개만인가**: singleton 은 "버전이 어긋나면 앱이 안 뜨는" 계약이다. 런타임이 반드시 하나여야 하는 것
> (React 인스턴스, 모달 레지스트리를 가진 web-ui)만 올리고, 나머지(react-markdown, mermaid 등)는
> 각 remote 가 자기 번들에 갖는다. 중복 다운로드 비용 < 버전 결합 비용.

## 5. Remote URL 결정 (런타임, 재빌드 없이)

셸은 remote 를 **빌드타임에 고정하지 않는다**. 등록·URL 결정은 전부 런타임:

1. 셸 registry(`src/apps/registry`) 에 제품 엔트리 추가 — key/label/authCode/authLevel/remote 이름 (코드 변경, 셸 재빌드 1회).
2. remote URL 은 `window._env_.REACT_APP_<제품>_APP_REMOTE` 에서 읽는다. 미설정 시 dev 폴백(localhost 포트).
3. MF 런타임(`@module-federation/vite` 의 runtime API — `registerRemotes`/dynamic remote)으로 해당 URL 의
   `remoteEntry.js` 를 로드한다. 현재 CRA 구현의 promise-based dynamic remote (Platform-App/craco.config.js:13)와
   의미상 동일 — Vite 이전 시 MF 2.0 runtime API 로 대체한다.

### 자산 경로 (publicPath)

- remote 의 chunk/이미지 등 부속 자산은 **remoteEntry.js 가 로드된 origin+경로 기준으로 상대 해석**되어야 한다
  (webpack `publicPath: 'auto'` 상당 — MF 2.0 `getPublicPath` 지원). **절대경로(`/static/…`) 인라인 금지** —
  게이트웨이의 `/<app>/` 프리픽스 아래에서 깨진다.
- 게이트웨이는 remote 정적 자산에 CORS 헤더를 붙여 셸 origin 에서의 로드를 허용한다 (동일 origin 게이트웨이 경유가 기본이라
  통상 불필요하나, dev 크로스포트(3000→3001) 대비 nginx 설정 유지).

## 6. 로드 실패 폴백 (셸 책임)

- 셸은 remote import 를 ErrorBoundary + Suspense 로 감싼다. remote 다운 시 해당 탭만 폴백 UI("연결할 수 없습니다" + 재시도)를
  보여주고 **셸과 다른 remote 는 계속 동작**해야 한다.
- 재시도는 MF 런타임의 remote 재로드로 구현한다 (기존 구현은 ErrorBoundary state 만 리셋해 실패 컨테이너가 캐시에 남는
  결함이 있음 — P1 에서 수정).

## 7. Standalone 모드 (remote 의무)

- remote 는 셸 없이 자기 포트에서 단독 실행 가능해야 한다 (개발·장애 격리 목적).
- standalone 에서는: 인증 폴백([02](02-auth-contract.md) §5), 자체 최소 크롬(타이틀/네비) 허용, `platformApp/*` import 실패를
  안전하게 처리.
