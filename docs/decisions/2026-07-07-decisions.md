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

## 참고 — RAGaaS 온보딩 상태 (회신 2026-07-06 수신)

- Phase 1 (A1~A12) 전부 완료, React 19.2.7 상향 포함. frontend shared-net 조인 완료(`/ragaas/` 관통).
- 셸 탭은 GoJIRA 쪽 env(`RAGAAS_APP_REMOTE`)가 이미 반영돼 있어 활성 상태 — C1~C3 합동 검증 대기.
- 잔여: RAGaaS base `/ragaas/` 반영 → 합동 검증 → Phase 2(web-ui tarball, 경로 회신함: `frontend/platform-web-ui-0.1.0.tgz`).

## 관찰 (조치 보류)

- 셸 콘솔에 `http://localhost:3001/remoteEntry.js` ERR_CONNECTION_REFUSED 1건 — Platform-App
  vite.config 의 dev 초기 remote 선언을 MF 런타임이 선요청. registry 의 force 재등록이 덮어써
  기능 무해(실 remote 는 env 값으로 로드됨). 거슬리면 셸 config 의 정적 remote 선언 제거 검토.
