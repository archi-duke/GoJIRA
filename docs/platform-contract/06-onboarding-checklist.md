# 06. 신규 제품 온보딩 체크리스트

> 새 제품(예: RAGaaS)이 플랫폼에 합류할 때 양쪽이 수행하는 작업 목록. 순서대로.
> `<app>` = 제품명 소문자 (예: `ragaas`), `<App>` = camelCase 컨테이너명 (예: `ragaasApp`).

## A. 제품 팀 (remote 쪽)

- [ ] **A1. 툴체인**: Vite + TypeScript + `@module-federation/vite` 로 빌드 구성 ([01](01-mf-host-contract.md))
- [ ] **A2. MF remote 설정**: `name: '<App>'`, `filename: 'remoteEntry.js'`, `exposes: { './App': ... }`,
      shared singleton = react / react-dom / @platform/web-ui ([01](01-mf-host-contract.md) §2, §4).
      **선행 조건: react/react-dom 을 플랫폼 메이저(현재 19.x)로 정렬** — RAGaaS 는 18.3.1 → 19 상향 필요 (D9)
- [ ] **A3. 마운트 컴포넌트**: `./App` 이 `RemoteAppProps { user, authority, resetKey?, basePath?, onNavigate? }` 를 받는
      default export React 컴포넌트 ([01](01-mf-host-contract.md) §2)
- [ ] **A4. 인증 소비**: `platformApp/auth` 어댑터 작성 (axios 는 인터셉터 패턴, [02](02-auth-contract.md) §3) —
      자체 로그인/토큰 저장 금지, 401 → `requireLogin()`
- [ ] **A5. standalone 폴백**: 셸 없이 단독 실행 시 auth 폴백 동작 ([02](02-auth-contract.md) §5)
- [ ] **A6. 디자인시스템**: UI 는 `@platform/web-ui` 컴포넌트 사용. `@progress/kendo-*` 직접 의존 금지 ([04](04-web-ui-contract.md))
- [ ] **A7. 공지**: 자체 공지 UI 금지. 필요 시 `platformApp/notice` 의 NoticeCenter 마운트만 ([03](03-notice-contract.md))
- [ ] **A8. 자산 경로**: publicPath 상대 해석 확인 — `/<app>-app/` 프리픽스 아래에서 chunk/이미지 로드 검증 ([01](01-mf-host-contract.md) §5)
- [ ] **A9. 런타임 env**: 앱이 소비하는 키를 `window._env_` 우선으로 읽는 config 로더 적용, 컨테이너 entrypoint 가
      해당 키 전부를 env.js 로 직렬화 ([05](05-backend-contract.md) §5)
- [ ] **A10. 백엔드 미들웨어**: 신원 확정 알고리즘 구현 — introspect / X-Service-Token / dev 폴백 ([05](05-backend-contract.md) §1~3).
      파이썬이면 §2 참조 스니펫 사용
- [ ] **A11. 백엔드 API 베이스**: `/api/v2` 표준 경로로 노출 ([05](05-backend-contract.md) §4)
- [ ] **A12. sessionStorage 네임스페이스**: 내부 UI 상태 키에 `<app>_` 접두사 ([01](01-mf-host-contract.md) §2)

## B. 플랫폼 팀 (셸/배포 쪽)

- [ ] **B1. registry 등록**: `Platform-App/src/apps/registry` 에 엔트리 추가 —
      `{ key, label, component: lazy(() => import('<App>/App')), authCode, authLevel, subPages }`
- [ ] **B2. remote URL env**: `REACT_APP_<APP>_APP_REMOTE` 키를 셸 config 로더 + platform-app entrypoint.sh 직렬화 목록에 추가
- [ ] **B3. 게이트웨이**: `deploy/images/gateway/locations.conf` 에 `/<app>-app/`, `/<app>-api/` location 블록 추가
      (WS/SSE 대응 포함, [05](05-backend-contract.md) §4)
- [ ] **B4. compose**: `deploy/docker-compose.yml` 에 `<app>-app`, `<app>-api` 서비스 추가 + env 주입
      (SSO 4종 + SERVICE_TOKEN 은 기존 anchor 재사용)
- [ ] **B5. SERVICE_TOKEN 공유**: 신규 백엔드 컨테이너에 동일 `SERVICE_TOKEN` env 배포
- [ ] **B6. 권한 코드**: 제품 메뉴 접근 제어용 authCode 신설 시 Platform-API authority 체계에 등록
- [ ] **B7. web-ui 제공**: 외부 레포 제품이면 @platform/web-ui tarball 반입 경로 안내 ([04](04-web-ui-contract.md) §2)

## C. 합류 검증 (완료 판정)

- [ ] **C1.** 게이트웨이 경유로 셸 접속 → 신규 탭 표시(권한 필터 동작) → 탭 클릭 시 remote 로드
- [ ] **C2.** remote 다운 상태에서 셸/타 제품 정상 동작 + 해당 탭만 폴백 UI
- [ ] **C3.** SSO 모드: remote 의 API 호출에 Bearer 자동 부착, 백엔드 introspect 검증 통과
- [ ] **C4.** dev 모드: 토큰 없이 X-User-Id 폴백 동작
- [ ] **C5.** remote 단독 실행(standalone) 정상
- [ ] **C6.** 재빌드 없이 env.js 값 변경만으로 게이트웨이 호스트/스킴 교체 반영 (폐쇄망 요건)
