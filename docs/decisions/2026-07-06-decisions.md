# 의사결정 기록 — 2026-07-06

> 세션 주제: 프론트엔드 표준화 & 플랫폼 통합 계약(P0). RAGaaS 가 두 번째 remote 로 합류하는
> 플랫폼 전략의 일부. 계약 문서: `docs/platform-contract/`.

## D1. 인증 제공 방식 = MF exposed 모듈 `platformApp/auth` (props 아님)

- **결정**: 셸이 인증 런타임을 MF 모듈로 노출(`getToken/getUser/getAuthority/authFetch/onSessionChange/requireLogin`),
  props(user/authority)는 보조 스냅샷 채널로 유지.
- **근거**: ① RAGaaS 는 axios — 인터셉터는 모듈 스코프라 props 로는 우회가 필요. ② SSE/런처 등 React 밖 코드 경로 실재.
  ③ 셸이 host-init 시점 준비 + 인증 확정 후 remote 마운트로 초기화 순서 보장. (02-auth-contract.md §1)
- **파급**: window.fetch monkey-patch 2곳 폐기 예정(P2). remote 는 표준 어댑터 한 겹으로 standalone 폴백.

## D2. Kendo 소유권 = @platform/web-ui 단독 (예방 계약)

- **결정**: Kendo React 의존·라이선스·테마는 @platform/web-ui 패키지만 가진다. 제품 앱 직접 의존은 CI 실패.
- **사실 확인**: 2026-07-06 현재 GoJIRA 두 앱에 Kendo 의존 0건 — "수습"이 아니라 확산 전 예방 계약.
- **형태**: 레포 내 workspace 패키지(packages/web-ui)로 시작, MF singleton 공유. 복붙 공통코드(AlertModal 등 17종,
  기능 슈퍼셋 판 기준)를 흡수. (04-web-ui-contract.md)

## D3. 셸↔remote 상태 공유 = 마운트 props + onNavigate 콜백 (sessionStorage magic string 폐기)

- **결정**: `RemoteAppProps { user, authority, resetKey, basePath?, onNavigate? }` 가 유일한 상태 계약.
  `gojira_page`/`gojira_project` sessionStorage 채널은 P2 에서 제거. remote 내부 UI 상태 키는 `<app>_` 접두사.
- **근거**: 타입 없음·키 충돌·3번째 앱 합류 시 암묵 계약 확산 위험. (01-mf-host-contract.md §2)

## D4. MF 표준 = Vite + @module-federation/vite, singleton 은 react/react-dom/@platform/web-ui 3개만

- **결정**: raw webpack MF plugin + `requiredVersion '^19.2.4'` 수동 고정 → MF 2.0 런타임(버전 협상/폴백)으로.
  requiredVersion 은 package.json range 재사용, strictVersion 미사용. remote expose 는 `./App` 표준
  (GoJIRA 의 `./GoJIRAApp` 은 이전 시 개명).
- **근거**: 빌드도구 무관 상호운용(RAGaaS 도 Vite), singleton 은 "어긋나면 앱이 안 뜨는" 결합이므로 최소화.

## D5. 게이트웨이/런타임 env 규약을 계약으로 명문화

- **결정**: `/<app>-app/`·`/<svc>-api/` 프리픽스 + strip, 백엔드 베이스 `/api/v2` 표준, env.js(window._env_) 주입 방식을
  언어중립 계약으로 고정. introspect 는 "SSO 서버로의 outbound GET, `{loginid, active}` 문자열 스키마, 3s 타임아웃,
  200∧loginid≠""∧active≠"false"" 로 명세 — 파이썬 참조 스니펫 제공. (05-backend-contract.md)
- **정정된 오해**: introspect 는 Platform-API 가 호스팅하는 API 가 아님 (조사로 확인).

## D6. 조사 중 발견된 계약 위반 3건 → P2 수정 목록 등재

1. platform-app entrypoint.sh 가 `REACT_APP_USE_SSO` 를 env.js 에 미직렬화 (컨테이너에서 SSO 토글 불가).
2. 프론트 DEV_USER 가 config.js 하드코딩 (`duke.kimm`) — 런타임 키로 승격 필요.
3. `Platform-App/src/App.js:23` 이 미정의 `config.PLATFORM_API` 참조 (조용히 무시되는 잠재 버그).

## D7. 실행 단계 (계획)

- **P0(본 세션)**: 계약 문서 docs/platform-contract/ 작성 → PR #70 → **머지됨(v1.0 확정)**. RAGaaS 팀 소비 시작 가능.
- **P1**: CRA/CRACO→Vite+TS 이전(두 앱), MF 런타임 현대화, env.js/게이트웨이 파이프라인 갱신. 회귀 없음 조건.
- **P2**: monkey-patch 제거→auth 모듈, 복붙→web-ui, magic string→props 계약, 라우팅 표준화, D6 버그 수정.

## D8. Kendo 플랫폼 기준 버전 = RAGaaS 실사 값 (KendoReact 5.8.0 / theme-bootstrap / TELERIK_LICENSE 빌드타임)

- **결정**: @platform/web-ui 의 Kendo 기준을 RAGaaS frontend 실사(2026-07-06)에서 출발 — KendoReact 5.8.0(exact pin),
  테마 kendo-theme-bootstrap 5.8.0(사전컴파일 all.css), 라이선스는 빌드타임 `TELERIK_LICENSE` env/`telerik-license.txt`
  (RAGaaS Dockerfile build-arg 패턴을 플랫폼 표준으로). 버전 상향은 web-ui 단독 결정·일괄 적용.
- **실사 요점**: 실사용 표면은 Grid 단일(2개 파일) + data-query descriptor. RAGaaS 백엔드가 Kendo Grid Sort/Filter
  Descriptor JSON 을 쿼리 계약으로 수신 — web-ui 이관 후에도 wire format 유지.
- **라이선스 확보 확인(같은 날 해소)**: Duke 가 Telerik 계정에서 키 파일 확보(telerik-license.txt, JWT).
  perpetual·비trial·KENDOUIREACT 포함(Complete 번들). **업데이트 구독 만료 2024-04-05** → 그 이전 릴리스만 사용 가능,
  **5.8.0 은 커버** (exact pin 의 라이선스적 근거). 2024-04-05 이후 릴리스로 상향 시 구독 갱신 선행 — D8 의
  "버전 상향은 web-ui 단독 결정" 에 이 제약이 걸림. 키는 개발 머신 `~/.telerik/telerik-license.txt` 에 설치(비커밋).
- web-ui 1차 구현 범위 = Grid 래퍼 + 테마 엔트리 + 라이선스 빌드 규약. (04-web-ui-contract.md §5)

## D9. React 버전 충돌 해소 방향 = RAGaaS 가 19 로 상향 (플랫폼 표준 고수)

- **사실**: RAGaaS react 18.3.1 vs 플랫폼 셸 19.x — singleton 공유 불가 조합.
- **결정**: 플랫폼 표준(19.x)을 유지하고 RAGaaS 가 합류 전 React 19 로 상향한다. 온보딩 A2 의 선행 조건으로 명시.
- **근거**: 셸/GoJIRA 가 이미 19 기반이고, 18→19 는 전방 이동(3개 앱 중 1개만 이동)이며 singleton 다운그레이드는 퇴행.

## D10. 인증 런타임 바인딩 채널 = MF expose + in-page 게시(window.__platformAuth) 이중화 (P2-A 구현)

- **결정**: 셸 auth 모듈(Platform-App/src/platform/auth.ts)은 MF `platformApp/auth` 로 expose 하면서, 로드 시
  `window.__platformAuth` 에도 게시한다. remote 어댑터는 **게시본을 1순위**로 소비(호출 시점 조회) — remote 는 항상
  셸 페이지 안에서 실행되므로 결정적이고, MF cross-runtime 인스턴스 바인딩 문제를 회피한다. (02-auth-contract.md §5)
- **근거**: MF 2.0 에서 remote→host 역방향 import 는 remote 측 런타임 인스턴스에 host entry 등록이 필요해 취약.
  게시본은 monkey-patch 와 달리 primitive 오버라이드가 아닌 명시적 단일 게시 지점. 게시 주체는 셸 하나, remote 쓰기 금지.
- **동반 수리**: fetch monkey-patch 2곳 제거(16파일 87개 호출부 authFetch 치환), D6 버그 3건 해소
  (platform-app entrypoint USE_SSO/DEV_USER 직렬화, DEV_USER 런타임 키 REACT_APP_DEV_USER 승격, 미정의 config 참조 소멸).

## D11. @platform/web-ui 1단계 = 소스 패키지 + Vite alias 소비 (workspace/MF singleton 은 Kendo 도입 시 승격)

- **결정**: packages/web-ui 를 소스 패키지로 신설하고 앱이 `resolve.alias` 로 직접 컴파일 소비. npm workspaces 는
  도입하지 않음 — 폐쇄망 build-cache-node(/cache/<App>/node_modules 레이아웃)와 lockfile 을 건드리지 않기 위함.
  Dockerfile 은 `COPY packages/` 한 줄 추가. MF singleton 공유(계약 04 §3)는 Kendo 도입(라이선스/테마 단일화가
  실제로 필요해지는 시점)에 진짜 패키지로 승격하며 수행. (04-web-ui-contract.md §2)
- **판 선정**: AlertModal(GoJIRA 슈퍼셋 — dangerous/confirmLabel/validate), CommonBar(GoJIRA — alert-pr/alert-diff
  아이콘이 라이브), launcher(GoJIRA 15함수 슈퍼셋), NoticePopup/sseManager(Platform — WS subscribe + user별 HIDE_KEY).
- **의도적 동작 통일 2건**: ① GoJIRA standalone 공지도 EventSource 대신 WS(sseManager) 사용(ADR-4 근거 확장)
  ② 공지 "3일 보지 않기" 키가 user별(`gojira_notice_hide_until_<user>`)로 통일 — 기존 standalone 사용자 1회 리셋.
- **부수 효과**: web-ui runtime/env.js 가 캐노니컬 env 키의 단일 소비 지점 — 앱별 config 키 분기(API_BASE 의미 상이)가
  공유 모듈로 번지던 원인 제거. web-ui 는 앱 config.js import 금지.

## D12. 셸↔remote 상태 채널 전환 구현 + 셸 URL 딥링크는 탭 수준 (P2-C)

- **구현**: D3 결정 이행 — 셸이 sessionStorage(`gojira_page`/`gojira_project`)를 읽던 암묵 계약 제거.
  remote 는 `onNavigate(page, { projectCode })` 로 페이지 전환마다 + **마운트 직후(복원 동기화)** 보고,
  셸은 탭 활성 판정과 에이전트 소환용 현재 과제 추적(activeProjectCode state)에만 사용.
- **remote 세션키 유지**: `gojira_*` 키는 remote 내부 새로고침 복원 전용으로 존치 (`<app>_` 접두사 규칙 충족,
  셸 미참조). 제거가 아니라 "채널로 쓰지 않음"이 계약의 본질.
- **URL 딥링크 범위 = 탭 수준**: `/account`·`/notice`·`/repository`(맵=`/`) 를 history API(pushState/popstate)로 동기화.
  라우터 라이브러리 미도입 — 탭 4개에 라우터는 과함(ADR-1 유지). remote 내부 페이지 URL 은 remote 별 딥링크 수요가
  생기면 `basePath` 계약으로 확장. 배포는 기존 nginx `try_files /index.html` SPA 폴백으로 즉시 호환.

## D13. Kendo 도입 1호 = web-ui DataGrid → 셸 AccountManager Users (소유권 계약 첫 이행)

- **구현**: packages/web-ui 가 KendoReact **5.8.0**(D8 기준, 라이선스 커버 범위) 세트를 자체 node_modules 로 소유.
  `.npmrc` legacy-peer-deps 로 react 자동설치 차단(앱 MF shared react 와 이중 인스턴스 방지 — hooks 깨짐 예방).
  `DataGrid` 래퍼(소팅/필터, kendo-data-query process) 를 노출하고 셸 AccountManager Users 목록이 1호 소비.
  테마(kendo-theme-bootstrap all.css)는 래퍼가 import — 이후 소비 확장 시 셸 1회 로드 규칙(§4)로 승격.
- **React 19 호환 실측**: KendoReact 5.8 공식 지원은 React 18 까지이나, 실브라우저 판정(렌더·소팅 토글·
  필터·액션 셀) **에러 0 으로 통과** — 5.x 가 class 컴포넌트 기반이라 React 19 제거 API(findDOMNode 등)에
  걸리지 않는 것으로 확인. 단 공식 지원 밖이므로 업그레이드 시 재판정 필요.
- **deploy**: build-cache-node 가 packages/web-ui npm ci 추가(+선택 TELERIK_LICENSE build-arg 훅),
  서비스 Dockerfile 은 fresh clone 시 캐시에서 node_modules 보충. **build-cache-node 재생성 필요.**
- **D11 승격 트리거 재정의**: "Kendo 도입 시" → **"둘 이상의 앱이 Kendo(web-ui singleton 필요 표면)를 소비하는
  시점"** 으로 정밀화 — 현재 소비는 셸 1곳이라 alias 모델 유지가 여전히 유효.

## D14. RAGaaS 팀에 온보딩 작업지시 프롬프트 전달 완료

- **사실**: `docs/platform-contract/ragaas-onboarding-prompt.md` 작성 후 RAGaaS 팀에 실제 전달·작업 지시함
  (Duke 확인, 계약 01~06 재구성 + RAGaaS 실사 요약 + 선행조건/Phase1·2/체크리스트).
- **크로스레포 함정 사전 고지**: `@platform/web-ui` 1단계(alias 소비, D11)는 같은 레포 안에서만 가능 — RAGaaS 는
  별도 레포이므로 그대로 못 씀. RAGaaS 착수 전 **GoJIRA 팀이 tarball(npm pack)/레지스트리 반입 경로를 먼저 마련**해야
  함을 프롬프트에 명시(B7 선행). Phase 2(Kendo 이관)는 이게 준비되기 전까지 RAGaaS 쪽 착수 불가 — RAGaaS 는
  당분간 자체 Kendo 5.8.0 의존 유지(라이선스 만료로 인한 버전 상향 금지는 별개로 유효, D8).
- **How to apply**: 다음 세션에서 RAGaaS 진행 상황 문의가 오면, tarball 반입 경로(B7)가 GoJIRA 쪽에서 준비됐는지부터
  확인 — 안 됐다면 Phase 2 는 여전히 블록 상태. → **같은 날 D15 로 해소됨.**

## D15. RAGaaS 온보딩 GoJIRA측 준비 완료 (B1·B2·B3·B7) — B4 는 불필요로 확정

- **B7 (블로커 해소)**: web-ui package.json 에 `exports` 맵(서브패스→소스 파일)·`files` 추가,
  `deploy/scripts/pack-web-ui.sh` → `deploy/dist/platform-web-ui-<ver>.tgz`. **외부 Vite 프로젝트를 실제로 만들어
  tarball 설치→서브패스 import→빌드까지 검증** (node_modules 안 .jsx 를 Vite/esbuild 가 확장자 기준으로 변환 확인).
  react/react-dom 은 peer, Kendo 는 dependencies 로 동반 설치.
- **B1/B2**: 셸 registry 에 RAGaaS 엔트리 — **env 게이트** (`REACT_APP_RAGAAS_APP_REMOTE` 미설정 시 탭 미노출,
  미배포 환경에서 죽은 탭 방지). makeRemote() 공통화, TAB_PATHS 를 registry 파생으로 바꿔 `/ragaas` 딥링크 자동 지원.
  RemoteErrorBoundary 폴백 문구도 제품 label 기반으로 일반화. entrypoint/compose(x-react-env, `RAGAAS_APP_REMOTE`
  변수) 직렬화 추가.
- **B3**: 게이트웨이 `/ragaas-app/`·`/ragaas-api/` — 타 스택 컨테이너라 **런타임 DNS**(resolver 127.0.0.11 +
  변수 proxy_pass) 로 해석해 미배포 시에도 nginx 기동 무영향. gateway 가 shared-net 조인.
  **실검증: `/ragaas-api/` 는 라이브 ragaas-backend 까지 관통**(RAGaaS API 루트 응답 수신 — 404 는 그쪽 /api/v2
  미표준화 때문, A11 항목). `/ragaas-app/` 은 ragaas-frontend 의 shared-net 미조인으로 502 → **RAGaaS 측 compose
  수정 전달사항** (frontend 에 networks [default, shared-net] 추가).
- **B4 불필요 확정**: RAGaaS 컨테이너는 자기 레포 compose 로 배포 + shared-net 연결이 원칙 — GoJIRA compose 에
  타 제품 서비스를 넣지 않는다.
- **셸 검증**: env 미설정 → 탭 없음(회귀 무), env 주입 → RAGaaS 탭 표시·클릭 시 `/ragaas` URL + 폴백 UI
  ("RAGaaS에 연결할 수 없습니다"), 딥링크 직행 정상, 기존 라우트 200 유지.
- **"platform app/api 분리" 질의 결론**: 이미 독립 서비스로 분리되어 있음 — 추가 분리(레포 분할 등)는 불필요 판단,
  web-ui 공유만 더 복잡해짐.
