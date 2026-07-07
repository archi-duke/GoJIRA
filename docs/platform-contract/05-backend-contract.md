# 05. 백엔드 플랫폼 계약 (언어중립)

> 플랫폼에 합류하는 **모든 백엔드**(Go, 파이썬, …)가 구현해야 하는 HTTP/헤더/ENV 규약.
> 코드 공유를 전제하지 않는다 — RAGaaS(파이썬)는 이 문서만 보고 얇은 미들웨어 어댑터를 구현한다.
> Go 참조 구현: `Platform-API/auth/auth.go` (GoJIRA-API/auth/auth.go 와 동일 로직).

## 1. 신원 확정 알고리즘 (요청 미들웨어)

모든 보호 API 요청에 대해 아래 순서로 **행위 사용자(userID)** 를 확정한다. 핸들러는 이 미들웨어가 확정한
신원만 사용하며, body/쿼리의 user 필드를 신뢰하지 않는다.

```
handle(request):
  1. request.method == OPTIONS            → 통과 (preflight)
  2. path 에 공개 구간 포함                → 통과 (신원 없음)
     공개 구간(참조 구현): "/launcher/", "/webhook/"  — 서비스별로 자체 정의 가능
  3. X-Service-Token 헤더 존재:
       constant-time 비교로 env SERVICE_TOKEN 과 일치?
         일치   → userID = X-User-Id 헤더 값 (act-as, 없으면 빈 신원). introspect 생략. 통과
         불일치 → 401
  4. env USE_SSO == "true":
       Authorization: Bearer <token> 추출 → introspect(token) (§2)
         성공 → userID = loginid. 통과
         실패 → 401
  5. (dev 폴백, USE_SSO != "true"):
       userID = X-User-Id 헤더 → 없으면 X-Dev-User 헤더 → 없으면 env DEV_USER
```

- SERVICE_TOKEN 미설정(빈 값)이면 3번은 **항상 거부** — "설정 안 하면 열리는" 폴백 금지.
- 토큰 비교는 constant-time (Go: `subtle.ConstantTimeCompare`, 파이썬: `hmac.compare_digest`).

## 2. 토큰 검증 — introspect (outbound 호출)

**introspect 는 플랫폼이 호스팅하는 API 가 아니다.** 각 백엔드가 사내 SSO(MobilAve)로 **나가는** 검증 호출이다.

| 항목 | 값 |
|------|----|
| 요청 | `GET {SSO_INTROSPECT_URL}/Account/introspect?token=<url-encoded token>` |
| 타임아웃 | 3초 |
| TLS | 기본 검증. `SSO_INSECURE_TLS=true` 일 때만 검증 우회 (사내 사설 인증서 대비) |
| 응답 (200) | `{ "loginid": "<사용자ID>", "active": "<문자열>" }` — **둘 다 문자열** |
| 유효 판정 | HTTP 200 **AND** `loginid != ""` **AND** `active != "false"` |

파이썬 참조:

```python
import httpx, urllib.parse

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

- 캐싱: 참조 구현은 무캐시(매 요청 introspect). 캐시를 두려면 TTL ≤ 60s 권장 — 세션 폐기 지연 한도.

## 3. 헤더 규약 (서비스 간 + 클라이언트)

| 헤더 | 방향 | 의미 |
|------|------|------|
| `Authorization: Bearer <sessionToken>` | 브라우저→백엔드 | SSO 세션 토큰. introspect 로 신원 도출 |
| `X-Service-Token: <공유비밀>` | 백엔드↔백엔드, 런처/CLI→백엔드 | 서비스 인증. env `SERVICE_TOKEN` 과 일치해야 함 |
| `X-User-Id: <loginid>` | 서비스토큰과 동반 / dev | act-as 행위 사용자. **서비스토큰 검증 통과 시에만 신뢰** (dev 모드 제외) |
| `X-Dev-User: <loginid>` | dev 전용 | dev 폴백 2순위 |

서비스 간 호출(예: RAGaaS-API → Platform-API) 시 붙일 것:

```
Content-Type: application/json
X-User-Id: <행위 사용자, 있으면>
X-Service-Token: <env SERVICE_TOKEN, 설정돼 있으면>
```

참조 구현: GoJIRA-API/handlers/platform.go:26-50 (`callPlatform`).

## 4. 게이트웨이 라우팅 규약 (nginx 단일 진입점) — 경로 체계 v2 (2026-07-06)

**제품 하나 = 프리픽스 하나.** 프리픽스 아래에서 `/` 는 프론트, `/api/…` 는 백엔드로 분기한다
(nginx prefix location 최장일치 — `/gojira/api/` 가 `/gojira/` 보다 우선).

| 경로 | 종류 | upstream (프리픽스 strip 후 수신 경로) |
|---|---|---|
| `/` (fallback) | 셸 SPA | platform-app:3000 |
| `/platform/api/…` | 공용 Platform-API | platform-api:9000 → `/api/…` |
| `/<제품>/` | remote 프론트 (remoteEntry.js 포함) | 예: `/gojira/` → gojira-app:3001 → `/…` |
| `/<제품>/api/…` | 제품 백엔드 API | 예: `/gojira/api/v2/X` → gojira-api:9001 → `/api/v2/X` |
| (부속 서비스) | 자유 | `/pumlex/` → pumlex-server:3030 |

- 게이트웨이는 **`/<제품>` 세그먼트만 strip** 한다 — 백엔드는 `/api/v2/…` 그대로 수신 (기존과 동일).
- 신규 제품은 `/<제품>/api/` + `/<제품>/` 두 블록을 `deploy/images/gateway/locations.conf` 에 추가한다
  (예: `/ragaas/api/`, `/ragaas/`). 타 스택 컨테이너는 런타임 DNS(resolver+변수 proxy_pass)로 해석.
- API 블록엔 WebSocket/SSE 대응 필수: `proxy_http_version 1.1`, `Upgrade/Connection` 헤더, SSE 는 `proxy_buffering off`.
- `client_max_body_size 100m` (게이트웨이 전역).
- 예: 브라우저 `GET /platform/api/v2/Account/me` → upstream `platform-api:9000/api/v2/Account/me`.
- **백엔드 API 베이스 경로는 `/api/v2` 를 표준**으로 한다. 프론트 설정 조립 규칙 = `{gateway}/<제품>/api/v2`
  (공용은 `{gateway}/platform/api/v2`).
- **주의(셸 딥링크와의 구분)**: 셸 탭 딥링크는 슬래시 없는 `/<제품>`(예: `/ragaas`) — 프리픽스 location 에
  안 걸려 셸로 간다. 슬래시 있는 `/<제품>/…` 는 셸이 로드하는 remote 자산·env.js·API 경로.
- **standalone HTML 진입은 없다** (2026-07-07 D4): `location = /<제품>/` exact match 가 셸 딥링크
  `/<제품>` 으로 302 — remote 는 바디 전용(계약 01 §2)이라 단독 화면에 크롬이 없기 때문.
  자산/remoteEntry/API 는 exact match 에 안 걸려 그대로 통과. 제품 단독 실행은 dev 서버 전용.
- **구 경로(v1) alias**: `/platform-api/`·`/gojira-api/`·`/gojira-app/`·`/ragaas-api/`·`/ragaas-app/` 는
  구버전 런처(≤0.1.15) 호환을 위한 deprecated alias — 런처 fleet 업데이트 후 제거 예정. 신규 구현은 v2 만 사용.

## 5. 런타임 env 주입 (재빌드 없는 폐쇄망 배포)

프론트 컨테이너는 **빌드 산출물에 호스트/스킴을 인라인하지 않는다.** 기동 시 entrypoint 가 env.js 를 생성한다:

```sh
# entrypoint.sh 개요 — 컨테이너 env → 웹루트 /env.js
cat > /usr/share/nginx/html/env.js <<EOF
window._env_ = {
  REACT_APP_PLATFORM_API: "$(esc "$REACT_APP_PLATFORM_API")",
  ...앱이 소비하는 키 전부...
};
EOF
exec nginx -g 'daemon off;'
```

- `index.html` 은 번들보다 먼저 `<script src="env.js">` 를 로드한다.
- 앱 코드의 조회 우선순위: `window._env_[key]` → (dev) 빌드도구 env → 하드코딩 dev 폴백.
- `esc()` 로 `\` 와 `"` 를 escape.
- **규칙: 앱이 소비하는 모든 런타임 키는 entrypoint 직렬화 목록에 반드시 포함** — 현재 platform-app entrypoint 가
  `REACT_APP_USE_SSO` 를 누락한 버그가 이 규칙 위반 사례 (P2 수정 대상).
- **규칙(2026-07-07, RAGaaS 제안 수용): remote 의 자기 API 폴백은 빌드 base 기준 상대경로** —
  예: `import.meta.env.BASE_URL + 'api/v2/'` → `/ragaas/api/v2/`. 셸 탭에서 remote 가 읽는
  `window._env_` 는 **셸의 env.js** 라 제품 API 키가 없다 — 루트 절대 폴백(`/api/v2/`)은 셸로
  새므로 금지. base 상대 폴백은 게이트웨이 단일 origin 전제에서 셸/standalone/직접/dev 진입을
  모두 관통한다. (셸 env.js 에 제품별 키를 싣는 대안은 셸-remote 결합 증가로 기각.)

## 5.5 Account/Notice 앱 스코핑 (D8, 2026-07-07)

**계정 관리와 공지는 여전히 Platform-API 단독 소유·구현**이다(RAGaaS 등 신규 제품이 각자
Account/Notice 를 재구현하지 않는다 — 04/03 계약 그대로). 다만 데이터는 앱별로 분리된다.

- **신원은 전역, 권한은 앱별** — 한 사람(knoxId)이 여러 앱에서 같은 신원으로 존재하되, 앱마다
  다른 authGroup(따라서 다른 authority)에 속할 수 있다. `User` 컬렉션은 전역 그대로, `AuthGroup`
  과 `Notice` 에 `app` 필드(`'platform'|'gojira'|'ragaas'…`, 미지정/레거시는 `'platform'`)가 추가됐다.
- **쿼리 파라미터 `app`** — `GET /Account/me`, `/Account/users`, `/Account/authGroups`,
  `POST /Account/authGroups`, `GET|POST /Notice`, `GET /Notice/active` 가 `?app=<scope>` 를
  받는다. 생략 시 `'platform'`. `POST` 는 body 의 `app` 을 우선하고 없으면 쿼리로 채운다.
- **신규 앱 부트스트랩 폴백** — 어떤 앱에 authGroup 이 하나도 없으면(막 온보딩된 제품),
  `Account/me` 의 권한 계산이 그 앱 대신 `'platform'` 권한으로 계산된다 — 플랫폼 마스터가
  그 앱의 Account 화면에 처음 들어가 첫 authGroup 을 만들 수 있게 하기 위함. authGroup 목록/편집
  API 자체는 폴백하지 않는다(플랫폼 그룹을 실수로 그 앱 소속처럼 편집하는 사고 방지).
  같은 이유로 그 앱에 소속 사용자가 아직 없으면 `Account/users` 가 플랫폼 로스터를 온보딩
  후보로 보여준다 — 첫 authGroup 이 생기고 누군가 실제로 그 그룹에 배정되는 순간부터
  정상적으로 그 앱 스코프만 보인다.
- **RAGaaS(외부 제품) 쪽 구현**: 이 API 들을 직접 만들 필요 없음 — 셸의 Account/Notice 관리
  화면이 이미 `app='ragaas'` 로 호출한다. RAGaaS 코드가 할 일은 없음(문의 시 참고용).

Go 참조 구현: `Platform-API/db/account.go`(`appFilter`, `resolveAuthorityScope`, `FindAllUsers`),
`Platform-API/db/notice.go`.

## 5.6 MongoDB 데이터베이스 분리 (D8)

Platform-API 소유 데이터(users/authGroups/orgUnits/userProjects/userConnections/notices)는
**`platform` DB** 로 분리됐다 (이전엔 GoJIRA-API 와 물리적으로 같은 `gojira` DB 를 공유했다).

- `PLATFORM_MONGO_DB` 기본값 `platform` (구 기본값 `gojira`). `PLATFORM_GOJIRA_MONGO_DB`
  (기본 `gojira`) 는 Platform-API 의 DocRepo 기능이 GoJIRA 소유 `projects` 컬렉션의
  `documents` 필드를 직접 갱신하는 **역방향 결합 전용** 핸들 — 신규 코드는 쓰지 말 것
  (제품 데이터는 그 제품 API 경유가 원칙, 이건 과거부터 있던 예외).
- Mongo 계정(`gojira_app`)에 `platform` DB `dbOwner` role grant 필요 — 최초 1회
  `deploy/scripts/migrate-platform-db.js` 실행(root 계정)이 grant + 기존 컬렉션 이관 +
  기존 데이터 `app:'platform'` 태그 + GoJIRA 연속성(authGroup `_gojira` 복제, 사용자 링크)까지
  수행한다. 신규 반입 환경에서도 최초 1회 실행 필요.
- GoJIRA-API 는 무변경 — 자체 `GOJIRA_MONGO_DB`(기본 `gojira`) 그대로. GoJIRA-API 가 과거
  Account/Notice 컬렉션에 직접 Mongo 접근하던 죽은 코드(`db/account.go` 등, 라우터 미등록 확인)는
  제거했다 — 그 기능은 Platform-API HTTP API 로만 제공된다.

## 6. ENV 키 표

### 백엔드 (모든 언어 공통 의미)

| 키 | 기본값 | 의미 |
|----|--------|------|
| `USE_SSO` | `false` | `"true"` 면 Bearer+introspect 강제, 아니면 dev 폴백 (§1-4,5) |
| `SSO_INTROSPECT_URL` | (없음) | introspect 베이스 URL. SSO 모드에서 필수 |
| `SSO_INSECURE_TLS` | `false` | introspect TLS 검증 우회 토글 |
| `SERVICE_TOKEN` | (없음) | 서비스간 공유비밀. 미설정 시 서비스토큰 경로 전면 거부 |
| `DEV_USER` | 서비스별 | dev 폴백 최종 신원 |
| `PLATFORM_MONGO_DB` | `platform` | Platform-API 전용 DB명 (D8, 구 기본값 `gojira`) |
| `PLATFORM_GOJIRA_MONGO_DB` | `gojira` | Platform-API 의 DocRepo `projects` 역방향 결합용 (§5.6) |
| `GOJIRA_MONGO_DB` | `gojira` | GoJIRA-API 전용 DB명 (무변경) |

### 프론트 (런타임 env.js 키 — 셸 + remote)

| 키 | 소비처 | 의미 |
|----|--------|------|
| `REACT_APP_USE_SSO` | 셸(+standalone remote) | SSO 로그인 흐름 활성화 |
| `REACT_APP_SSO_URL` | 셸 | SSO 서버 (ADFSLogin/exchange/introspect) |
| `REACT_APP_PLATFORM_API` | 전체 | Platform-API 베이스 (`{gateway}/platform/api/v2`) |
| `REACT_APP_GOJIRA_API` | GoJIRA | GoJIRA-API 베이스 |
| `REACT_APP_<제품>_APP_REMOTE` | 셸 | 각 remote 의 origin (remoteEntry.js 로드 기준) |
| `REACT_APP_LAUNCHER_BASE` | 전체 | 로컬 런처 (`http://127.0.0.1:5599`) |

신규 제품 추가 키 예: `REACT_APP_RAGAAS_API`, `REACT_APP_RAGAAS_APP_REMOTE`.

## 7. 알려진 계약 위반 (P2 수정 대상, 2026-07-06 조사)

1. `deploy/images/platform-app/entrypoint.sh` — `REACT_APP_USE_SSO` 를 env.js 에 직렬화하지 않음 → 컨테이너에서 SSO 토글 불가.
2. 프론트 `DEV_USER` 가 config.js 에 하드코딩(`duke.kimm`) — 백엔드는 env 인데 프론트만 코드 고정. 런타임 키로 승격 필요.
3. `Platform-App/src/App.js:23` 이 정의되지 않은 `config.PLATFORM_API` 를 참조 (Platform config.js 는 `API_BASE` 로만 노출) —
   monkey-patch 제거와 함께 소멸 예정.
