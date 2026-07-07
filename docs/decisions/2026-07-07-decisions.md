# 2026-07-07 의사결정 로그

## D1. 제품 프론트 빌드에 Vite `base = '/<제품>/'` (빌드 전용)

- **문제**: 경로 v2 반영 후 standalone 진입(`/gojira/`, `/ragaas/`)이 흰 화면. 빌드된 index.html 이
  자산을 루트 절대경로(`/assets/…`, `/env.js`)로 참조 → 게이트웨이 최장일치가 셸(`/`)로 라우팅 →
  404 → JS 부트스트랩 실패. 셸 탭 모드는 remoteEntry 기준 상대 해석이라 무증상 — standalone 만 깨짐.
- **결정**: 제품 프론트는 `defineConfig(({ command }) => ({ base: command === 'build' ? '/<제품>/' : '/' }))`.
  dev 서버(루트 서빙)와 셸 remote 로드(publicPath 자동)는 영향 없음. Vite 가 index.html 의 public
  자산 참조(env.js, pex-core 등)까지 전부 재기준하므로 컨테이너 entrypoint/nginx 는 무변경.
- **적용**: GoJIRA-App 반영·재빌드·검증 완료(브라우저 렌더 + 콘솔 에러 0). RAGaaS 에는
  `docs/platform-contract/ragaas-notice-2026-07-07.md` 로 동일 수정 요청 (+ BrowserRouter
  basename, SPA fallback 주의점). 계약 05 §5 자산 경로 절에 반영할 것.
- **셸은 해당 없음**: Platform-App 은 루트(`/`) 서빙이라 base `/` 유지.

## D2. WebSocket 인증 규약 — 계약 05 v1.1 로 추가 예정

- RAGaaS 회신의 논의 요청 수용. Starlette HTTP 미들웨어가 WS 를 안 가로채 신원 미들웨어 밖인 문제.
- 방향: 첫 메시지 토큰 방식 유력(쿼리파라미터는 액세스 로그 노출 우려). 초안은 플랫폼 쪽 작성.
- 그전까지 게이트웨이/셸 경유 전제로 현행 수용 (블로커 아님).

## D3. 셸 onNavigate — 모르는 page 키는 무시 (현재 탭 유지)

- **문제**: 셸에서 RAGaaS 탭 클릭 시 "페이지를 찾을 수 없습니다"(셸 placeholder). RAGaaS 가 계약대로
  마운트 직후 onNavigate(내부 라우트 키)를 호출하는데, 셸 handleRemoteNavigate 가 무조건 setPage →
  registry 에 없는 키라 컨텐츠 분기가 entry 를 못 찾음. GoJIRA 는 subPages(docs/issues…)가 등록돼
  있어 우연히 무증상이었다.
- **결정**: 계약 01 §2 는 page 키 집합을 강제하지 않으므로, 셸은 아는 키만 setPage 하고 모르는 키는
  무시(현재 탭 유지). ctx.projectCode 는 항상 반영. 계약 01 §2 에 명문화.
- **대안 기각**: registry 에 제품별 subPages 등록 — 셸이 remote 내부 라우트를 알아야 해서 결합 증가.

## D4. Remote = 바디 전용 표준 + standalone HTML 진입 폐지 (셸로 302 수렴)

- **문제**: `/gojira/`·`/ragaas/` standalone 진입 시 반쪽 화면 — GoJIRA 는 구식 자체 타이틀바(플랫폼
  메뉴 없음), RAGaaS 는 크롬 전무. 플랫폼 메뉴/사용자 표시가 P2 에서 셸로 이관됐기 때문.
- **결정**: remote `./App` 은 바디 전용(이미 그 형태), 크롬은 셸 단독 소유로 계약 01 §2 에 확정.
  게이트웨이 `location = /<제품>/` (exact) 가 셸 딥링크 `/<제품>` 으로 302 — 자산/remoteEntry/API 는
  exact match 에 안 걸려 통과. 사용자 진입점 = 셸 하나. 제품 단독 실행은 dev 서버 전용.
- **효과**: 제품 코드 변경 0 (GoJIRA 자체 타이틀바는 dev 전용으로 잔존 — 유저 전환기 유용).
  RAGaaS base 수정 지시는 필수→선택 다운그레이드 (통지문 §0 갱신).
- **기각**: standalone 에 플랫폼 크롬 이식 — 제품별 중복 구현, 크롬 변경마다 전 제품 재배포.

## D5. 브랜디드 스코프 + remote './manifest' (제품이 타이틀바 메뉴를 소유)

- **요구**: `/` = 플랫폼 브랜드(임시 NETRIX) + 전체 메뉴, `/gojira` = GoJIRA 아이콘/이름 + GoJIRA
  메뉴(Project)만, `/ragaas` = RAGaaS 브랜드 + 그 메뉴만(현재 없음).
- **결정**:
  - remote 는 선택적으로 `./manifest` expose: `{ name, icon(base 상대), menus:[{key,label,pages?,authCode?,authLevel?}] }`.
    셸이 기동 시 로드, 실패/미노출 → registry 폴백(단일 탭). 제품 메뉴 변경 = 제품 재배포만.
  - 셸 스코프 = URL 1st 세그먼트: 제품 세그먼트면 그 제품 브랜드+메뉴만, 아니면 플랫폼.
    RemoteAppProps 에 `page?: string` 추가 — 메뉴 클릭 키를 remote 에 전달.
  - **URL 의미 변경**: 제품 세그먼트 URL = 스코프 진입점. 플랫폼 스코프의 제품 탭 전환은
    URL 불변(구 D12 의 탭 딥링크 의미를 대체). `/account` 등 내장 딥링크는 유지.
  - 플랫폼 브랜드명은 REACT_APP_PLATFORM_NAME(env, 기본 NETRIX) — 확정 시 .env 만 교체.
- **적용**: 셸(App.jsx 스코프/manifest 로드/브랜딩) + GoJIRA-App manifest('Project') + 계약 01 §2.
  RAGaaS 는 무수정 폴백 동작, manifest 추가는 그쪽 선택.
- **보강(같은 날)**: 제품 스코프에서 **마스터(USR≥15)** 에게는 구분선 뒤에 셸 내장 플랫폼 메뉴
  (Account/Notice/Repository, 개별 authCode 필터 중첩)도 노출 — 클릭 시 같은 스코프 안에서
  해당 내장 화면 렌더(브랜드 유지, URL 불변). 비마스터는 제품 메뉴만.

## D6. 플랫폼 스코프(/) 순수화 — 제품 미연결, 제품 링크 목록만

- **요구**: `/` 접속 시 플랫폼 메뉴(Account/Notice/Repository)만 보이고, 어떤 제품(GoJIRA/RAGaaS)도
  연결하지 않는다. 단, 플랫폼 권한 없는 일반 사용자가 막히지 않도록 제품 링크 카드 목록 제공.
- **결정**:
  - pageFromPath 기본값 `'map'` → `'home'`. 플랫폼 스코프 타이틀바는 `visiblePlatformMenus`
    (Account/Notice/Repository, 권한 필터)만 — 제품 탭 제거.
  - 바디 기본(`page==='home'`)은 제품 카드 목록(`<a href="/<product>">`, 전체 페이지 네비게이션) —
    remote 를 렌더하지 않으므로 연결 안 함. 아이콘은 셸 로컬 자산(`localIcon`)이나 글자 배지.
  - manifest 로딩 effect 를 `scope` 로 게이트(이미 D5 작업 중 시도) — **했음에도 GoJIRA remoteEntry.js
    가 루트에서 계속 fetch 되는 걸 발견**: 원인은 `Platform-App/vite.config.ts` federation 설정의
    **빌드타임 정적 `remotes: { gojiraApp: {...} }` 선언** — MF 런타임이 host bootstrap(hostInit)
    시점에 정적 remotes 를 스코프 무관하게 즉시 fetch. **제거**가 진짜 수정. registry.js 의 런타임
    지연 등록(`ensureRegistered`, 스코프 진입 시 1회)만으로 충분 — dev 폴백은 여전히
    `remoteBaseFromEnv` 기본값(`localhost:3001`)이 담당.
  - registry.js: `component`(즉시 생성 lazy) → `remote.getComponent()`(호출 시점에만
    registerRemotes+React.lazy, 메모이즈) 로 리팩터. `loadAppManifest` 도 같은
    `ensureRegistered()` 경유.
- **검증**(실브라우저, request 트레이스): `/` 방문 시 remoteEntry.js 요청은 셸 자신뿐(GoJIRA/RAGaaS
  0건). `/gojira` 방문 시 GoJIRA remoteEntry 만, RAGaaS 는 0건 (역도 마찬가지) — 스코프별 완전
  격리 확인.
- **교훈**: MF 2.0 vite 플러그인의 빌드타임 `remotes` 선언은 "dev 폴백 힌트"가 아니라 host
  bootstrap 이 실제로 소비하는 즉시-연결 목록이다. 런타임 동적 등록과 병행 선언하면 정적 쪽이
  이긴다 — 순수 런타임 제어가 목적이면 정적 `remotes` 를 아예 비워야 한다.

## 참고 — RAGaaS 온보딩 상태 (회신 2026-07-06 수신)

- Phase 1 (A1~A12) 전부 완료, React 19.2.7 상향 포함. frontend shared-net 조인 완료(`/ragaas/` 관통).
- 셸 탭은 GoJIRA 쪽 env(`RAGAAS_APP_REMOTE`)가 이미 반영돼 있어 활성 상태 — C1~C3 합동 검증 대기.
- 잔여: RAGaaS base `/ragaas/` 반영 → 합동 검증 → Phase 2(web-ui tarball, 경로 회신함: `frontend/platform-web-ui-0.1.0.tgz`).

## 관찰 (조치 보류)

- 셸 콘솔에 `http://localhost:3001/remoteEntry.js` ERR_CONNECTION_REFUSED 1건 — Platform-App
  vite.config 의 dev 초기 remote 선언을 MF 런타임이 선요청. registry 의 force 재등록이 덮어써
  기능 무해(실 remote 는 env 값으로 로드됨). 거슬리면 셸 config 의 정적 remote 선언 제거 검토.
