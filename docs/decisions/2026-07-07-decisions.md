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

## D7. 스코프별 타이틀 아이콘 동작 + 노출 플랫폼 메뉴 설정

- **요구**: 앱마다 타이틀 아이콘 클릭 동작과 노출할 플랫폼 메뉴를 다르게. platform: home(초기화면)
  + [account,notice]. gojira: agent(로컬 에이전트 채팅) + [account,notice,repository]. ragaas:
  home(그 스코프 기본 화면 복귀) + [account,notice].
- **결정**: registry 항목에 `titlebarAction`('agent'|'home')과 `platformMenus`(키 배열) 필드 추가.
  플랫폼 스코프는 registry 항목이 아니므로 `PLATFORM_SCOPE_CONFIG` 상수로 별도 지정.
  `visiblePlatformMenus` 계산을 `activeScopeConfig.platformMenus` (= scopeApp ?? PLATFORM_SCOPE_CONFIG)
  로 필터링해 스코프별 메뉴 집합이 기존 authCode 권한 필터와 이중 적용으로 자연히 도출되게 단일화.
- **검증**(실브라우저): 3 스코프 모두 메뉴 목록/타이틀 툴팁/아이콘 클릭 동작이 기대값과 일치.

## D8. 사용자·공지 Platform 영역 이관 + 앱별 DB 분리

> 요구: "notice 를 띄우는 역할은 platform app 이 담당(공용화), 계정 관리도 동일하게 platform 영역.
> 데이터만 app 별로 구분." + "DB 이름이 gojira 인데 platform 이 관리하는 거면 platform 이름으로."

- **신원 모델**: 전역 신원(knoxId/name/email 공유) + 앱별 권한(사용자 확인 질문에서 이 옵션 선택).
  같은 사람이 gojira 에선 Master, ragaas 에선 무권한일 수 있음.
- **기존 데이터 귀속**: `platform` (공용/메타 관리자 — 두 번째 확인 질문에서 선택). GoJIRA 는 이미
  실사용 중이었으므로 연속성을 위해 platform 그룹을 `_gojira` 접미사로 복제하고 기존 사용자의
  authGroupIds 에 대응 그룹을 추가 — 기존 GoJIRA 사용자 전원이 종전과 같은 권한을 유지한다.
  RAGaaS 는 authGroup 0건으로 시작(진짜 빈 상태).
- **DB 분리**: 기존엔 Platform-API/GoJIRA-API 가 기본값으로 같은 `gojira` DB 를 공유(교차 직접
  접근까지 존재 — 아래 참조). `PLATFORM_MONGO_DB` 기본값을 `platform` 으로, GoJIRA-API 쪽은
  `gojira` 유지. Mongo 계정(`gojira_app`, 종전엔 `gojira` DB 에만 dbOwner)에 `platform` dbOwner
  role grant 필요(root 계정으로 1회) — `deploy/scripts/migrate-platform-db.js` 가 grant +
  6개 컬렉션(users/authGroups/orgUnits/userProjects/userConnections/notices) 이관 +
  `app:'platform'` 태그 + gojira 연속성 복제까지 멱등 수행.
- **역방향 결합 잔존**: Platform-API 의 DocRepo(`db/docrepo.go`)가 GoJIRA 소유 `projects` 컬렉션의
  `documents` 필드를 직접 Mongo 로 갱신한다 — 과거부터 있던 예외라 이번엔 유지하되, 별도
  `gojiraDB` 핸들(`PLATFORM_GOJIRA_MONGO_DB`)로 명시적으로 분리해 `platform` DB 분리와 충돌하지
  않게 함. 신규 코드는 이 패턴을 따르지 말 것.
- **더 큰 발견**: GoJIRA-API 가 users/notices/authGroups/orgUnits/userProjects/userConnections
  전부를 Platform-API HTTP API 경유 없이 **자기 Mongo 드라이버로 직접** 읽고 쓰는 코드
  (`handlers/account_mgmt.go`, `handlers/notice.go`, `db/account.go`, `db/notice.go`,
  `db/account_seed.go`)를 갖고 있었다. 조사 결과 이 핸들러들은 **main.go 라우터에 전혀
  등록되지 않은 죽은 코드**였음을 확인 — 실서비스 영향 없이 안전하게 전부 삭제.
  (다른 방식: HTTP 전환 옵션도 고려했으나 죽은 코드로 판명되어 불필요해짐.)
- **앱 스코핑 API**: `AuthGroup`/`Notice` 에 `app` 필드, 관련 API 에 `?app=` 쿼리 파라미터
  (계약 05 §5.5). **신규 앱 부트스트랩 폴백**: 그 앱에 authGroup 이 0건이면 권한 계산만
  platform 권한으로 폴백(플랫폼 마스터가 그 앱 최초 진입 가능) — 목록/편집 API 는 폴백 안 함
  (플랫폼 그룹 오조작 방지). 그 앱에 소속 사용자가 0명이면 `Account/users` 가 플랫폼 로스터를
  온보딩 후보로 보여준다(첫 배정 전까지만) — 안 그러면 authGroup 을 막 만들어도 붙일 기존
  사용자를 화면에서 찾을 방법이 없어 잠기는 2차 문제 발견·해결.
- **프론트**: App.jsx 의 authority 조회에 `app=<scope>` 추가(스코프 전환마다 재계산 — 같은 사용자도
  스코프마다 권한이 다르므로). AccountManager/NoticeManager/NoticePopup 에 `app` prop 추가.
  NoticePopup 의 "3일간 보지 않기" localStorage 키도 앱별로 분리.
- **검증**(실브라우저 + API): platform/gojira 스코프 Account 3명(마이그레이션 데이터), ragaas
  스코프는 0명 → 부트스트랩 후 마스터 권한 확인(authority 전부 15) → Account 메뉴 노출 →
  Users 탭에 플랫폼 로스터 3명 표시. GoJIRA-API 삭제 후 `/gojira/api/v2/Project` 정상 200
  (프로젝트 기능 무영향 확인).

## D9. platform 스코프 = 전체 조회 (메타 관리 콘솔) + 원본 정리

> 사용자 결정: "계정/공지 데이터는 app 태그로 앱별 구분, 각 앱은 자기 것만 필터, platform 은
> 전체 조회(메타 관리 콘솔)". D8 에서 만든 부트스트랩 폴백 2단은 이 모델에서 불필요해졌다.

- **모델 전환**: 기존엔 platform 스코프도 `app='platform'` 데이터만 봤다(사실상 다른 앱과 동급
  스코프). 이제 platform 은 **전체 조회** — `appFilter`/`FindAllUsers` 에 특수값 `app='all'`
  을 추가해 필터 없이 전체 문서를 반환한다. 각 앱(gojira/ragaas 등) 스코프는 기존대로 자기
  `app` 값만 필터.
- **D8 부트스트랩 폴백 2단 제거**: `resolveAuthorityScope`(권한 계산을 platform 권한으로
  폴백)와 `FindAllUsers` 끝의 "플랫폼 로스터 온보딩 후보" 폴백을 완전히 삭제했다. platform
  전체 조회 모델에서는 플랫폼 관리자가 platform 화면에서 신규 앱의 authGroup 을 직접 만들고
  기존 사용자를 바로 배정할 수 있어(전체 사용자/그룹이 다 보이므로) 폴백이 더 이상 필요 없다.
- **UI**: AccountManager/NoticeManager 가 `app==='platform'` 이면 목록 조회를 `app=all` 로
  호출. Users 탭은 권한그룹을 id→name 매핑하되 매핑에 없는 id(다른 스코프에서 로드 안 된 경우)
  는 표시하지 않음(raw id 노출 방지) — platform 스코프에서만 전체 그룹이 로드되므로 `이름(앱)`
  형식으로 표시하고, "소속 앱" 컬럼(authGroupIds → app distinct)도 추가. Auth Groups 탭은
  name 을 첫 컬럼으로 옮기고 platform 스코프에서만 ID/앱 컬럼을 추가, 생성 다이얼로그에
  앱 선택 select(`APPS` 상수: platform/gojira/ragaas, 기본 platform, 편집 시 disabled)를
  추가했다. NoticeManager 도 동일하게 platform 전용 앱 컬럼 + 작성 다이얼로그 앱 선택.
  App.jsx 의 마스터 사용자 전환 목록도 platform 스코프에서는 `app=all` 로 전체 사용자를 로드.
- **DB 정리**: D8 이관(`migrate-platform-db.js`) 이후 `gojira` DB 에 남아있던 원본 7개
  컬렉션(users/authGroups/orgUnits/notices/userProjects/userConnections/notifications)을
  drop 하는 `deploy/scripts/cleanup-gojira-platform-collections.js` 를 추가했다. 안전 가드로
  `platform` DB 쪽 문서 수가 `gojira` DB 쪽 이상일 때만 drop — 이관이 의심스러우면 경고만
  출력하고 건너뛴다.
- **부작용**: 신규 앱(예: 새로 온보딩되는 제품)은 authGroup 이 배정되기 전까지 그 앱 스코프의
  Account/Notice 관리 메뉴가 정상적으로 "안 보이는" 상태가 된다 — 이건 이제 폴백이 아니라
  정상 흐름이다. 신규 앱 온보딩은 항상 **platform 스코프 관리 화면**에서 그 앱의 authGroup 을
  만들고 사용자를 배정하는 방식으로 수행한다.

## D10. 웹 이슈 뷰 이원화 설계 확정 — 공식본(main/Mongo) + 내 작업본(로컬 YAML)

- **발단**: NBBALL Relation Map 연결선 실종 — 원인은 issueLinks 128건이 잘못된 shape 로 Mongo 에
  ingest 된 데이터 오염 (repair-nbball-issuelinks.js 로 복구, GoJIRA-API 코드는 무혐의 — 외부 도구
  usecase-analyze2 가 직접 쓴 것으로 추정). 이어서 "웹 편집 모드 부활" 논의 → 동시편집 문제 지적
  → 사용자가 모델을 정리: "main 발행본만 공식 공유 버전, 편집 중에는 자기 로컬 작업본을 본다.
  완료 시 등록→PR" (문서와 동일 모델을 이슈에도).
- **결정**: 상세 설계는 docs/Long-term Considerations/web-local-view-model.md (§3.3 개정).
  Phase 0(ingest shape 검증, 즉시) → 1(런처 /issues + 웹 공식본/작업본 토글) → 2(dual-write 의
  즉시 ingest 를 PR 머지 웹훅 시점으로 이동) → 3(로컬뷰 한정 편집 브릿지 부활).
- **기각**: 읽기 전체 런처 경유(공유 뷰가 각자 로컬 상태에 의존 + 런처 없으면 화면 불능),
  웹 Mongo 직접쓰기 원복(YAML=진실 붕괴).
- **상태**: 설계 확정·구현 보류 (착수 시 열린 결정 4건 — 특히 SATAM 의 usecase_artifacts 가
  Mongo 전용이라 로컬뷰 범위 확인 필수 — 먼저 정리).

## 참고 — RAGaaS 온보딩 상태 (회신 2026-07-06 수신)

- Phase 1 (A1~A12) 전부 완료, React 19.2.7 상향 포함. frontend shared-net 조인 완료(`/ragaas/` 관통).
- 셸 탭은 GoJIRA 쪽 env(`RAGAAS_APP_REMOTE`)가 이미 반영돼 있어 활성 상태 — C1~C3 합동 검증 대기.
- 잔여: RAGaaS base `/ragaas/` 반영 → 합동 검증 → Phase 2(web-ui tarball, 경로 회신함: `frontend/platform-web-ui-0.1.0.tgz`).

## 관찰 (조치 보류)

- 셸 콘솔에 `http://localhost:3001/remoteEntry.js` ERR_CONNECTION_REFUSED 1건 — Platform-App
  vite.config 의 dev 초기 remote 선언을 MF 런타임이 선요청. registry 의 force 재등록이 덮어써
  기능 무해(실 remote 는 env 값으로 로드됨). 거슬리면 셸 config 의 정적 remote 선언 제거 검토.

## D11. 이슈 쓰기 시점 이동 (D10 Phase 2) — base-from-client 프로토콜 + 원자 카운터

- **설계 보정**: D10 Phase 2 요약("CLI 의 즉시 ingest 호출 제거")과 실제 코드가 달랐다 —
  dual-write 의 실체는 GoJIRA-API /Issue 핸들러가 **Mongo 를 병합 base 로 읽고 캐시로 쓰는**
  구조. Mongo 쓰기만 제거하면 연속 로컬 편집이 깨진다(2번째 편집이 공식본 base 로 병합돼
  1번째 유실). → **base-from-client**: 로컬 쓰기 요청이 자기 로컬 YAML(진실)을 동봉
  (`baseYaml`/`inwardYaml`·`outwardYaml`/`baseFiles`/`localOnly`), 서버는 병합·링크·키할당
  "두뇌"만 수행하고 Mongo 무접촉으로 localWrites 반환. 동봉 없는 구버전 요청은 기존 경로
  (전환기 호환). 공식본(Mongo)은 main push 웹훅의 reconcile ingest 로만 갱신.
- **검증 중 발견 결함 2건 (Fable 검증 단계에서)**:
  1. **키 할당 중복**: NextIssueNum/NextSubkey 가 issues 컬렉션 max 스캔 → localOnly 생성은
     Mongo 에 안 쓰므로 연속 생성 시 같은 키 중복 발급(batch-create 전멸급). 삭제 후 키
     재사용 잠재 버그도 동근원. → `db/counters.go` 원자 `$inc` 카운터(첫 발급은 max 스캔
     seed, 경합은 duplicate key 감지 후 재시도)로 교체.
  2. **reconcile 삭제 사고 (실사고)**: 수동 resync E2E 중 NBBALL main 의 오염 YAML 49건이
     Phase 0 검증에서 거부되자, reconcile 이 "ingest 집합에 없음"으로 보고 정상 Mongo 캐시
     49건을 삭제. 로컬 복구본 50건 재-ingest 로 즉시 복원. → **검증 실패 = 상태 불명**:
     실패 파일의 키는 제거 보호, 키 미상(파싱 불가) 실패가 있으면 제거 단계 전체 생략
     (`reconcileSkipped` 응답 필드).
- **batch-create 함정**: 생성 후 PUT(추가 필드)이 baseYaml 없이 가면 legacy 경로가 Mongo 를
  찾다 404(localOnly 생성이라 없음) → 필드 조용히 유실. 생성 응답의 localWrites[0].content 를
  baseYaml 로 재사용해 해결.
- **분업**: 계획/사양 Fable 5 → 코딩 Sonnet 5 에이전트 2개 병렬(서버/클라이언트) → 검증
  Fable 5 (빌드·테스트·44300 E2E — localOnly 중복키·Mongo 무접촉·reconcile 보호 전부 실측).
- **잔여**: NBBALL main 오염 YAML 49건은 로컬 복구본 등록(PR 머지) 시 웹훅 resync 로 공식본
  갱신 — 웹훅 경로의 자연 E2E 겸용. 이 PC 런처 0.1.17→0.1.18 교체 필요(콜드스타트 ingest
  제거는 0.1.18부터, 구버전은 전환기 무해).

## D12. 웹 편집 브릿지 부활 (D10 Phase 3) — 게이트 재개방 + 저널 방식(a)

- **조사 선행**: 읽기전용화 커밋(9d32ebb, 2026-05-22)은 편집 UI 를 게이트로만 잠갔고 핸들러를
  삭제하지 않았음을 확인 — "예전 편집기"는 git 역사가 아니라 현재 working tree 에 원형 보존.
  부활 작업의 대부분은 게이트 재개방(세터 복원 + onEditChange 재전달)이었고, 진짜 신규는
  ① 로컬 이슈 로드 시 raw YAML 보존(_baseYaml/_localPath — parseLocalIssues 가 버리고 있었음),
  ② 쓰기 라우팅을 legacy(Mongo 직접)에서 Phase 2 base-from-client 프로토콜로 교체,
  ③ 에이전트 변경 전달.
- **아키텍처**: 웹 편집도 CLI/스킬과 동일한 base-from-client — 서버 "작성 두뇌" 재사용, 웹은
  localWrites 를 launcher /write·/delete 로 자기 clone 에만 반영. 편집은 로컬뷰(내작업) 한정,
  공식본은 계속 읽기 전용. 생성은 클라이언트 subkey 계산을 제거하고 서버 카운터 할당(D11)로 통일.
- **에이전트 전달 = 저널(a) 채택, 위치 변경**: 원안(클론 내 .gojira/pending-changes.yaml)은
  프로젝트 레포에 .gitignore 가 없어 PR 오염 → **~/.gojira/journal/<code>.yaml (런처 영역)**
  로 확정. 런처 POST /issue-journal(append, code별 mutex), spawn env GOJIRA_ISSUE_JOURNAL 주입,
  에이전트 계약은 스킬 배포 라인으로: `.claude/skills/_shared/issue-journal.md`(처리 지침) +
  clone 배포 `.claude/CLAUDE.md`(assets-extra — build-launcher.sh 와 launcher-dist Dockerfile
  양쪽 sync 필요, 후자 누락을 배포 검증에서 발견·수정). (b) 능동통지 버튼은 후속.
- **검증 중 발견·수정**: IssueList select 가 status/priority 를 항상 `{name}` 객체로 강제
  (구 Mongo/JIRA 캐시 shape) → 로컬 YAML 스칼라 관례 파괴. shape 보존형으로 수정
  (IssueDetailDialog 는 이미 보존형). E2E 실측: 수정(스칼라 유지)/생성(파일+카운터키)/삭제
  (파일 제거)/저널 3건/Mongo 무접촉/공식뷰 게이트 전부 확인.
- **분업**: 조사(Explore·Sonnet) → 사양(Fable) → 코딩 Sonnet 2병렬(런처/웹) → 검증·수정 Fable.
