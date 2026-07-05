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

## 4. 게이트웨이 라우팅 규약 (nginx 단일 진입점)

경로 프리픽스로 라우팅하고 **프리픽스를 strip** 해서 upstream 에 전달한다 (`proxy_pass http://svc:port/` 끝 슬래시).

| 경로 프리픽스 | 종류 | 예 (현재) |
|---|---|---|
| `/` (fallback) | 셸 SPA | platform-app:3000 |
| `/<제품>-app/` | remote 프론트 (remoteEntry.js 포함) | `/gojira-app/` → gojira-app:3001 |
| `/<서비스>-api/` | 백엔드 API | `/platform-api/` → platform-api:9000, `/gojira-api/` → gojira-api:9001 |
| (부속 서비스) | 자유 | `/pumlex/` → pumlex-server:3030 |

- 신규 제품은 `/<제품>-app/` + `/<제품>-api/` 두 블록을 `deploy/images/gateway/locations.conf` 에 추가한다
  (예: `/ragaas-app/`, `/ragaas-api/`).
- API 블록엔 WebSocket/SSE 대응 필수: `proxy_http_version 1.1`, `Upgrade/Connection` 헤더, SSE 는 `proxy_buffering off`.
- `client_max_body_size 100m` (게이트웨이 전역).
- 예: 브라우저 `GET /platform-api/api/v2/Account/me` → upstream `platform-api:9000/api/v2/Account/me`.
- **백엔드 API 베이스 경로는 `/api/v2` 를 표준**으로 한다 (프리픽스 strip 후 기준). RAGaaS-API 도 `/api/v2/...` 로 노출 권장 —
  프론트 설정 조립 규칙(`{gateway}/{svc}-api/api/v2`)이 단순해진다.

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

## 6. ENV 키 표

### 백엔드 (모든 언어 공통 의미)

| 키 | 기본값 | 의미 |
|----|--------|------|
| `USE_SSO` | `false` | `"true"` 면 Bearer+introspect 강제, 아니면 dev 폴백 (§1-4,5) |
| `SSO_INTROSPECT_URL` | (없음) | introspect 베이스 URL. SSO 모드에서 필수 |
| `SSO_INSECURE_TLS` | `false` | introspect TLS 검증 우회 토글 |
| `SERVICE_TOKEN` | (없음) | 서비스간 공유비밀. 미설정 시 서비스토큰 경로 전면 거부 |
| `DEV_USER` | 서비스별 | dev 폴백 최종 신원 |

### 프론트 (런타임 env.js 키 — 셸 + remote)

| 키 | 소비처 | 의미 |
|----|--------|------|
| `REACT_APP_USE_SSO` | 셸(+standalone remote) | SSO 로그인 흐름 활성화 |
| `REACT_APP_SSO_URL` | 셸 | SSO 서버 (ADFSLogin/exchange/introspect) |
| `REACT_APP_PLATFORM_API` | 전체 | Platform-API 베이스 (`{gateway}/platform-api/api/v2`) |
| `REACT_APP_GOJIRA_API` | GoJIRA | GoJIRA-API 베이스 |
| `REACT_APP_<제품>_APP_REMOTE` | 셸 | 각 remote 의 origin (remoteEntry.js 로드 기준) |
| `REACT_APP_LAUNCHER_BASE` | 전체 | 로컬 런처 (`http://127.0.0.1:5599`) |

신규 제품 추가 키 예: `REACT_APP_RAGAAS_API`, `REACT_APP_RAGAAS_APP_REMOTE`.

## 7. 알려진 계약 위반 (P2 수정 대상, 2026-07-06 조사)

1. `deploy/images/platform-app/entrypoint.sh` — `REACT_APP_USE_SSO` 를 env.js 에 직렬화하지 않음 → 컨테이너에서 SSO 토글 불가.
2. 프론트 `DEV_USER` 가 config.js 에 하드코딩(`duke.kimm`) — 백엔드는 env 인데 프론트만 코드 고정. 런타임 키로 승격 필요.
3. `Platform-App/src/App.js:23` 이 정의되지 않은 `config.PLATFORM_API` 를 참조 (Platform config.js 는 `API_BASE` 로만 노출) —
   monkey-patch 제거와 함께 소멸 예정.
