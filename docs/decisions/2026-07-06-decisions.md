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
  Descriptor JSON 을 쿼리 계약으로 수신 — web-ui 이관 후에도 wire format 유지. **좌석/에디션/만료는 레포에 정보 없음 — 미결,
  조달 주체 별도 확인 필요.**
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
