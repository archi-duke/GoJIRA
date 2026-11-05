# 로컬 런처 게이트웨이화 — 단일 진입점 + 기능 경로 + 로컬 서비스 배포

> 기록 일자: 2026-11-05
> 상태: **설계 (구현 대기)**
> 대체 범위: [`Long-term Considerations/local-agent-mvp.md`](./Long-term%20Considerations/local-agent-mvp.md) §2 "로컬 런처 서비스" 의
> 역할 정의("소환 + 상태 회신 전용")를 이 문서가 대체한다. MVP 문서의 나머지(에이전트 로컬 실행,
> `docs/{user}` push→PR 흐름)는 유효.
> 계기: RAGaaS 의 사내 DRM(pptx/docx) 문서를 사용자 PC 에서 텍스트로 변환해 등록해야 하는 요구
> (서버에 보안 도구 설치 불가) — 이를 런처에 직접 넣지 않고, 런처를 "로컬 서비스 플랫폼"으로 정리하는
> 계기로 삼는다.
> 반영 이력: 2026-11-05 이슈 #116(런처 관련 검토 필요 사항 — 사내 GHES 실측·서명 지점 검토) 반영 —
> 런처 매니페스트 서명, 루트/카탈로그 키 분리, Actions 빌드 이전, 2단계 이행(§10.6, §10.7, §14 0단계).

## 0. 한 줄 요약

런처(`http://127.0.0.1:5599`)는 **로컬 서비스의 배포·기동·라우팅만** 담당하는 단일 진입점이 된다.
기능은 별도 로컬 서비스(exe)가 제공하고, 웹은 서비스 이름이 아니라 **기능 경로**
(`POST /v1/docs/pptx/text`, `POST /v1/workspace/{code}/sync` …)로 호출한다. 새 기능은 서비스 패키지를
카탈로그에 등록하는 것만으로 추가되며 **런처 수정·재배포가 필요 없다.**

## 1. 배경 — 현재 런처의 책임 혼재

README 는 "소환 + 상태 회신만" 이라 정의하지만, 실제 19개 엔드포인트는 성격이 다른 5묶음이다.

| 묶음 | 엔드포인트·모듈 | 성격 |
|---|---|---|
| ① 에이전트 소환 | `/edit`, `/chat`, `/sessions`, 스킬·opencode 자산 배포 | 원래 목적 |
| ② Git 작업본 관리 | `/open`, `/prewarm`, `/status`, `/sync`, `/push`, `/pull`, `/remove-clone`, gitcreds | GoJIRA 도메인 |
| ③ 로컬 파일 CRUD | `/files`, `/file`, `/write`, `/delete`, `/rename` | 웹 "내 작업본" 의 로컬 백엔드 |
| ④ GoJIRA 업무 로직 | `/issues`, `/issue-journal` | GoJIRA 도메인 |
| ⑤ 데몬 운영 | `/health`(토큰), `/shutdown`, install/uninstall, 자동 업데이트 | 범용 기반 |

- ①~④ 는 "GoJIRA 프로젝트 로컬 clone" 이라는 단일 목적으로 묶이지만 책임은 여럿이며,
  ⑤(범용 기반)가 GoJIRA 전용 설정(스킬·git·repoRoot)과 `config.go`/`install.go` 에서 엉켜 있다.
- 여기에 RAGaaS DRM 변환(제품 무관 기능)을 더하면 "단일 제품 전용" 경계까지 깨진다.

## 2. 목표 / 비목표

**목표**
1. 브라우저가 붙는 로컬 진입점을 `http://127.0.0.1:5599` 하나로 단일화.
2. 런처 책임을 **배포(설치·업데이트) + 소환(기동·감시) + 라우팅** 으로 한정.
3. 기능은 독립 로컬 서비스로 분리하고, 서비스 추가 시 런처 무수정.
4. 기존 GoJIRA 웹을 깨지 않는 점진 이행(호환 별칭).

**비목표**
- HTTPS 로컬 인증서(결정: HTTP 유지, §3).
- 서비스 샌드박싱(OS 수준 권한 격리) — 권한 선언은 심사·표시용.
- 서비스 간 직접 호출 버스 — 필요 시 별도 설계.

## 3. 결정 사항

| # | 결정 | 근거 |
|---|---|---|
| D1 | 진입점 **`http://127.0.0.1:5599`** (현행 포트 유지, HTTP) | 브라우저는 https 페이지 → `http://127.0.0.1` 호출을 mixed content 로 막지 않음(localhost 는 potentially trustworthy). 로컬 루트 인증서 배포의 보안·운영 부담 회피 |
| D2 | 런처 책임 = **배포 + 소환 + 라우팅** | §1 책임 혼재 해소 |
| D3 | **기능 기반 경로** 라우팅 (`/v1/docs/pptx/*` → 서비스) | 웹이 서비스 구성을 몰라도 됨. 같은 이름공간 안에서 기능별로 다른 서비스가 담당 가능(예: `/v1/docs/hwp` 는 별도 서비스) |
| D4 | 외부 도구 spawn(VS Code·에이전트 CLI·터미널)은 **서비스 책임** | 경로 검증(clone 컨테인먼트)과 spawn 이 같은 곳에 있어야 안전. 런처는 "서비스" 만 기동 |
| D5 | 브라우저 보안(Origin·토큰·CORS·PNA)은 **런처 단독** | 서비스는 런처가 주입한 내부 비밀값만 검증 → 보안 코드 중복 제거 |
| D6 | 서비스 목록은 **Platform-API 가 서빙하는 서명된 카탈로그** | 런처는 한 곳만 조회. 각 제품은 PR 로 항목 등록 |
| D7 | 기존 런처 경로(`/edit`, `/sync` …)는 이행 기간 동안 **호환 별칭**으로 유지 | GoJIRA 웹 일괄 수정 회피 |
| D8 | 서비스 경로는 **`/v1` 버전 접두사** 필수 (`/v1/docs/pptx/text`). `/launcher/*` 는 무버전 | 호환 깨는 변경 시 `/v2` 를 병행 제공하며 점진 전환 |
| D9 | **서비스 공개 원칙** — 카탈로그의 모든 서비스는 모든 사용자에게 노출·설치 가능 (제품별 대상 제한 없음) | 로컬 도구는 제품 횡단 자산. 노출 제어 복잡도 제거 |
| D10 | **서비스 간 호출 허용** — 런처 경유(§7.4) | 예: workspace 가 문서 변환 사용. 런처가 단일 경로표·감사 지점 유지 |
| D11 | 카탈로그 **파이프라인 자동 서명** (main 머지 시) | 배포 빈도가 높아 수동 서명은 병목. 통제는 PR 리뷰 + 보호 브랜치로 확보(§10.6) |
| D12 | 서명 파이프라인은 **사내 GitHub Actions** (environment secret + 서명 전용 self-hosted runner) | 레포 운영처와 동일. 머지 조건·키 접근을 GitHub 가 강제, 사내망 안에서 빌드→서명→게시 완결(번들 반입 불요) |
| D13 | **런처 매니페스트도 서명** + 키 2종 분리: **루트 키**(런처 매니페스트, 승인자 필수) / **카탈로그 키**(서비스 카탈로그, 자동) | 런처는 공개키를 품은 신뢰 루트 — 런처 업데이트가 무서명이면 카탈로그 서명 전체가 우회됨(#116). 빈도·피해 범위에 맞춰 통제 수준 분리 |
| D14 | 서명 대상 바이너리는 **Actions 에서 빌드·서명**, 배포 서버는 산출물만 서빙(재빌드 금지) | 서버에서 다시 빌드하면 서명 대상과 실제 배포물이 달라져 서명이 무의미(#116) |
| D15 | 서명 검증은 **경고 → 필수 2단계** 도입, 구 매니페스트 형식 유지 | ≤0.1.38 런처는 서명을 모름. 0.1.37 이하 구제 경로 보존(#116) |

## 4. 아키텍처

```
[브라우저: Platform-App 셸 / GoJIRA / RAGaaS]
        │  http://127.0.0.1:5599   (Origin 허용목록 + X-Gojira-Launcher-Token)
        ▼
┌─────────────── gojira-launcher (단일 진입점) ───────────────┐
│ /launcher/*   런처 자체 API (health·capabilities·services)  │
│ Router        경로 prefix → 서비스 (최장 일치)               │
│ Supervisor    기동·헬스체크·재시작·idle 종료                 │
│ Installer     카탈로그 조회·서명검증·다운로드·sha256·롤백    │
└────────┬──────────────────────┬─────────────────────────────┘
         │ 127.0.0.1:<동적포트>  │ 127.0.0.1:<동적포트>   (X-Svc-Secret)
         ▼                      ▼
  gojira-workspace        ragaas-converter        … (추가 서비스)
  /v1/workspace/* /v1/agents/*  /v1/docs/pptx/* /v1/docs/docx/*
  /v1/issues/*
         ▲
         │ 카탈로그·패키지
  Platform-API  /api/v2/launcher/catalog, /api/v2/launcher/services/...
```

## 5. 경로 이름공간

최상위 세그먼트는 소유 도메인이 정해진 **예약 이름공간**이다. 새 최상위 이름공간은 플랫폼 승인 사항.

| 이름공간 | 소유 | 예시 |
|---|---|---|
| `/launcher/*` | 런처 (서비스 등록 불가) | `GET /launcher/health`, `GET /launcher/capabilities` |
| `/v1/workspace/*` | gojira-workspace | `GET /v1/workspace/{code}/status`, `POST /v1/workspace/{code}/sync`, `GET /v1/workspace/{code}/files`, `GET·PUT·DELETE /v1/workspace/{code}/file?path=`, `POST /v1/workspace/{code}/rename`, `POST /v1/workspace/{code}/push`, `POST /v1/workspace/{code}/pull`, `POST /v1/workspace/{code}/open`, `POST /v1/workspace/{code}/prewarm`, `DELETE /v1/workspace/{code}` |
| `/v1/agents/*` | gojira-workspace (향후 분리 가능) | `POST /v1/agents/chat`, `POST /v1/agents/edit`, `GET /v1/agents/sessions` |
| `/v1/issues/*` | gojira-workspace | `GET /v1/issues/{code}`, `POST /v1/issues/{code}/journal` |
| `/v1/docs/*` | 문서 처리 서비스들 (형식별) | `POST /v1/docs/pptx/text`, `POST /v1/docs/docx/text`, (향후) `POST /v1/docs/hwp/text` |

- 경로 규칙: `/v{버전}/{이름공간}/{리소스|형식}/{동작}` (D8). 예약 이름공간 판정은 버전 다음 세그먼트 기준.
  이하 표의 `/launcher/*` 외 경로는 모두 `/v1` 접두. 사용자 식별(`user`)은 바디가 아니라 런처가 주입하는
  헤더(§7.3)로 전달하는 것을 원칙으로 한다(현행 `{code, user}` 바디는 호환 별칭에서만 허용).

## 6. 런처 자체 API (`/launcher/*`)

| 메서드/경로 | 토큰 | 응답 / 동작 |
|---|---|---|
| `GET /launcher/health` | 불필요 | `{version, ready, token}` — token 은 Origin 이 허용목록과 일치할 때만 포함(현행 핸드셰이크 유지) |
| `GET /launcher/capabilities` | 필요 | `{routes:[{prefix, methods, service, state}]}` — `state`: `available` \| `installable` \| `unsupported`(OS·요건 불충족) |
| `GET /launcher/services` | 필요 | 설치·실행 상태 `{name, version, state, latest, requires:{ok, missing[]}}` |
| `POST /launcher/services/{name}/install` | 필요 | optional 서비스 설치(SSE 진행률). 이미 설치면 no-op |
| `POST /launcher/services/{name}/restart` | 필요 | 재기동 |
| `POST /launcher/shutdown` | 필요 | 현행 `/shutdown` 과 동일(재설치 시 포트 인계) |

현행 `GET /health`, `POST /shutdown` 은 별칭으로 유지(§11).

## 7. 라우팅

### 7.1 경로 선언
각 서비스는 패키지 안 `service.json` 에 담당 경로를 선언한다(§9.1). 런처는 **설치된 서비스의 선언을
모아 라우팅 표**를 만든다.

```jsonc
"routes": [
  { "prefix": "/v1/docs/pptx", "methods": ["POST"], "timeoutSec": 180, "maxBodyMB": 200 },
  { "prefix": "/v1/docs/docx", "methods": ["POST"], "timeoutSec": 180, "maxBodyMB": 200 }
]
```

### 7.2 매칭·충돌
- **최장 prefix 일치**, 세그먼트 경계 기준(`/v1/docs/pptx` 는 `/v1/docs/pptxx` 와 불일치).
- 메서드 불일치 → `405`. 매칭 없음 → `404 {code:"no_route"}`.
- 카탈로그에는 있으나 미설치 → `409 {code:"service_not_installed", service, installable:true}` →
  웹이 설치 안내 후 `/launcher/services/{name}/install` 호출.
- 요건 불충족(예: Office 미설치) → `424 {code:"requirement_missing", missing:[...]}`.
- 충돌은 **카탈로그 등록 CI 에서 차단**(동일/중첩 prefix + 메서드 교집합). 런타임에 충돌이 발견되면
  먼저 등록된 서비스를 유지하고 로그.

### 7.3 중계 규칙
- 요청 경로·쿼리는 **그대로** 서비스에 전달(prefix 제거 안 함).
- 런처가 **제거**하는 헤더: `X-Gojira-Launcher-Token`, `Cookie`, `Origin`.
- 런처가 **주입**하는 헤더: `X-Svc-Secret`(서비스별 비밀값), `X-Request-Id`, `X-Launcher-User`
  (런처 설정의 사용자 식별), `X-Gojira-Session`(SSO 세션, 현행 Phase 4 C 유지 — 서비스가 백엔드 호출 시 사용).
- 요청·응답 바디 **스트리밍**(메모리 적재 금지), SSE(`text/event-stream`) 플러시 통과.
- 경로별 `timeoutSec`, `maxBodyMB` 적용(초과 시 `413`/`504`).
- `on-demand` 서비스가 꺼져 있으면 기동→헬스 통과까지 요청을 대기(최대 30s) 후 중계.

### 7.4 서비스 간 호출 (D10)
- 서비스는 다른 서비스를 **직접 호출하지 않고 런처 경유**로 호출한다: `http://127.0.0.1:5599/v1/...` +
  `X-Svc-Caller: <name>` + `X-Svc-Secret: <자기 비밀값>`.
- 런처는 호출자 비밀값으로 신원을 확인(브라우저 Origin/토큰 검사 대신)하고, 일반 요청과 같은 경로표로 중계한다.
  대상 서비스에는 `X-Svc-Caller` 가 전달되어 호출 출처를 구분할 수 있다.
- 호출 가능한 대상은 `service.json.permissions.call`(경로 prefix 목록)로 선언 — 카탈로그 심사 대상이며 런처가 강제한다.
- 순환 호출 방지: 런처가 `X-Svc-Hop` 을 증가시키고 3 초과 시 `508` 거부.

## 8. 보안

| 계층 | 내용 |
|---|---|
| 바인드 | 런처·서비스 모두 `127.0.0.1` 전용 |
| 브라우저 | Origin 허용목록(현행 `webOrigin(s)`/`gateway` 파생 유지), `/launcher/health` 토큰 핸드셰이크, CORS preflight |
| PNA | 크롬 Private Network Access 대비 preflight 응답에 `Access-Control-Allow-Private-Network: true` |
| 런처↔서비스 | 기동 시 서비스별 랜덤 `SVC_SECRET` 주입 → 서비스는 `X-Svc-Secret` 불일치 요청 거부(브라우저 직접 호출 차단) |
| 공급망 | **런처 매니페스트(루트 키) + 서비스 카탈로그(카탈로그 키)** ed25519 서명, 공개키는 런처 임베드(키별 2슬롯) + 바이너리·패키지 sha256. 서명 불일치·만료·구버전이면 무시(기존 설치 유지). 무결성(sha256)뿐 아니라 **출처**까지 검증(§10.6) |
| 서명 runner | GHES runner group 을 **특정 레포의 특정 워크플로 파일**(`launcher-release.yml`, `catalog-sign.yml`)로 제한 + ephemeral — environment secret 만으로는 같은 runner 를 쓰는 다른 워크플로의 잔여물 접근을 못 막음(#116) |
| 권한 선언 | `service.json.permissions`(spawn 대상, fs 범위, 네트워크 대상) — 카탈로그 심사 근거 + `/launcher/services` 에 표시. OS 수준 강제는 비목표 |
| 입력 검증 | 경로 컨테인먼트·명령 화이트리스트는 **각 서비스 책임**(현행 런처의 safeFilePath 등은 workspace 로 이관) |

## 9. 서비스 계약

### 9.1 `service.json` (패키지 내 자기 기술)

```jsonc
{
  "name": "ragaas-converter",
  "version": "1.0.0",
  "entry": "ragaas-converter.exe",
  "os": ["windows"],
  "minLauncher": "0.2.0",
  "run": { "mode": "on-demand", "idleTimeoutMin": 10 },   // always | on-demand
  "health": "/health",
  "requires": ["powershell", "office-com"],               // 런처 사전 점검 키(§9.3)
  "permissions": { "spawn": ["powershell"], "fs": ["temp"], "net": [], "call": [] },
  "routes": [ /* §7.1 */ ]
}
```

### 9.2 런타임 계약
- 환경변수: `SVC_PORT`(런처가 할당), `SVC_SECRET`, `SVC_DATA_DIR`, `SVC_LOG_DIR`, `LAUNCHER_VERSION`.
- `127.0.0.1:$SVC_PORT` 바인드, `X-Svc-Secret` 검증.
- `GET /health` → `{version, ready, busy}` — `busy=true` 면 런처가 업데이트를 미룬다(예: 에이전트 세션 진행 중).
- `POST /shutdown` 또는 종료 신호 → 진행 중 요청 마무리 후 종료(최대 10s).
- 로그는 `SVC_LOG_DIR` 에 기록(런처가 회전·보관).
- 템플릿: `launcher-service-template`(Go, 위 계약 구현 + 빌드/게시 스크립트) 을 제공.

### 9.3 요건 점검 키
런처가 기동 전·`/launcher/capabilities` 계산 시 점검: `git`, `code`, `opencode|claude`, `powershell`,
`office-com`(Word/PowerPoint COM 등록 여부) 등. 키 추가는 런처 업데이트 사항이므로 최소한으로 유지.

## 10. 배포

### 10.1 패키지·게시
- 패키지: `{name}-{version}-{os}-{arch}.zip` = 실행 파일 + `service.json` (+ 필요 자산).
  Go 외 언어도 단일 실행물이면 허용(PyInstaller 등).
- 제품 레포 Actions → `publish-service.sh` → 패키지 저장소에 zip + `.sha256` (§10.6 패키지 저장소).
- 카탈로그 원본: `deploy/launcher-dist/catalog.json` — 각 제품이 PR 로 항목 추가, 플랫폼 담당이
  `permissions`·경로 충돌을 **PR 리뷰**로 검토. main 머지 시 사내 GitHub Actions 가 **자동 서명**(`catalog.json.sig`, §10.6).

### 10.2 Platform-API (기존 `/api/v2/launcher/*` 확장)
| 경로 | 내용 |
|---|---|
| `GET /api/v2/launcher/version`, `/download/{os}` | 런처 자기 업데이트. 응답에 `sig`(루트 키 서명) 필드 **추가만** 하고 기존 필드·형식 유지 → ≤0.1.38 런처 무영향(§10.7) |
| `GET /api/v2/launcher/manifest` | **(기존, 변경 없음)** 웹 설치 모달용 — OS별 호스팅 여부·크기. 서명과 무관 |
| `GET /api/v2/launcher/release-manifest`, `…/release-manifest.sig` | (신규) 서명 대상 런처 매니페스트 원문 + 서명 — 신버전 런처가 사용. 원문은 **파일 바이트 그대로** 서빙(서버가 JSON 을 재조립하면 서명이 깨진다) |
| `GET /api/v2/launcher/catalog` | `catalog.json` + 서명 |
| `GET /api/v2/launcher/services/{name}/{version}/{file}` | 서비스 패키지 다운로드 |

`/launcher/` 는 신원 미들웨어 공개 구간(계약 05 §1)이므로 인증 없이 서빙 — 무결성은 서명으로 보장.

### 10.3 카탈로그 형식

```jsonc
{
  "issuedAt": "2026-11-05T00:00:00Z",
  "services": [
    { "name": "gojira-workspace", "version": "1.0.0", "policy": "required",
      "packages": { "windows-amd64": { "file": "gojira-workspace-1.0.0-windows-amd64.zip", "sha256": "…" },
                    "darwin-arm64":  { "file": "…", "sha256": "…" } } },
    { "name": "ragaas-converter", "version": "1.0.0", "policy": "optional",
      "packages": { "windows-amd64": { "file": "…", "sha256": "…" } } }
  ]
}
```
- `policy`: `required`(자동 설치) | `optional`(첫 사용 시 사용자 동의 후 설치).
- 대상 제한 없음(D9): 모든 서비스가 모든 사용자의 `/launcher/capabilities` 에 노출된다. `required` 는 런처가
  공통으로 필요하다고 판단한 서비스에만 쓴다(남용 금지 — 기본은 `optional`).
- `routes` 는 카탈로그에도 사본을 둔다(미설치 상태에서 `409 service_not_installed` 판정용).

### 10.4 로컬 배치

```
~/.gojira/
  launcher.json, launcher-token
  services/<name>/<version>/…      버전별 디렉터리 (덮어쓰기 없음 → Windows 실행 파일 잠금 회피)
  services/<name>/current          현재 버전 포인터
  data/<name>/                     서비스 데이터 (업데이트와 무관하게 유지)
  logs/<name>/
```
- gojira-workspace 의 clone 루트(`repoRoot`, 기본 `~/GoJIRA-projects`)는 현행 위치 유지 — 서비스 설정으로 전달.

### 10.5 수명주기
- **설치**: 카탈로그 조회(기동 시 + `update.intervalMinutes`) → 서명 검증 → `required` 자동 설치,
  `optional` 은 웹 요청 시 설치.
- **기동**: `always` 는 런처 기동 시, `on-demand` 는 첫 요청 시. 실패 시 지수 백오프 재시작(최대 5회 후 `failed`).
- **업데이트**: 새 버전 다운로드·검증 → `busy=false` 대기 → 정지 → `current` 교체 → 기동·헬스 →
  실패 시 **자동 롤백**. 직전 1개 버전 보관.
- **idle 종료**: `on-demand` 는 `idleTimeoutMin` 동안 요청 없으면 종료.
- **런처 자체 업데이트**: ClickOnce 식(staged swap) 유지 + **루트 키 서명 검증 추가**(§10.6). 라우팅·설치 로직 변경 시에만 필요.

### 10.6 서명 체계 — 사내 GitHub Actions (D11~D14)

현행 런처 업데이트는 sha256 과 실행 파일을 **같은 서버**에서 받는다 — 전송 중 손상은 막아도 서버·배포 경로가
오염되면 막지 못한다(무결성은 확인, 출처는 미확인. #116). 이를 **두 종류의 서명 매니페스트**로 막는다.
폐쇄망이라 keyless(Sigstore Fulcio/Rekor) 불가 → **키 기반 ed25519**(Go 표준 `crypto/ed25519`, 외부 의존성 없음).

#### 키 2종

| 키 | 서명 대상 | 빈도 | 워크플로 / environment | 승인 | 유출 시 |
|---|---|---|---|---|---|
| **루트 키** | 런처 매니페스트(버전 + OS별 sha256·size) | 드묾(런처 릴리스) | `launcher-release.yml` / `launcher-release` | **Required reviewers 필수** | 원격 복구 불가(런처가 신뢰 루트) → 최고 수준 통제, next 슬롯으로만 교체 |
| **카탈로그 키** | 서비스 카탈로그 | 잦음 | `catalog-sign.yml` / `catalog-signing` | 머지 승인만(자동 서명) | 루트 키로 서명된 런처 긴급 릴리스로 폐기·교체 가능 |

- 런처는 키별로 공개키 2슬롯(current, next)을 임베드한다. 카탈로그 공개키 교체는 런처 릴리스(루트 키 서명)로 이뤄진다.
- 개별 바이너리가 아니라 **매니페스트 한 장**에 서명 — 서명 1개로 전체를 묶고, 키 교체·다운그레이드 방지 정책을 매니페스트에서 처리.
- 카탈로그는 "리뷰를 통과해 main 에 들어간 것만 서명" 통제를 **GitHub(보호 브랜치 + Environment)가 강제**한다.

#### 흐름

```
[제품 레포 Actions]   서비스 빌드 → zip + .sha256 → 패키지 저장소 게시
                      → 플랫폼 레포(GoJIRA)에 catalog.json 항목 변경 PR 자동 생성
[플랫폼 레포 PR]      catalog-validate 워크플로 (키 없음)
                        스키마 · 경로 충돌 · 패키지 존재/sha256 일치 · version 단조 증가 · permissions diff 요약 코멘트
                      CODEOWNERS(플랫폼 담당) 승인 필수  ← 사람의 통제 지점
[main 머지]           catalog-sign 워크플로 (environment: catalog-signing)
                        1. 검증 재실행 (머지 결과 기준)
                        2. issuedAt / expiresAt / commit / run id 기록
                        3. ed25519 서명 → catalog.json.sig
                        4. deploy/launcher-dist/ 로 게시 → Platform-API 서빙
[schedule]            catalog-sign 주 1회 재서명 (expiresAt 갱신, 내용 불변)

[런처 릴리스 태그]    launcher-release 워크플로 (environment: launcher-release, 승인자 승인 후 실행)
  launcher-v*          1. OS별 크로스 컴파일 + 스킬 임베드 (Actions 에서 빌드 — §10.7)
                       2. 런처 매니페스트 {version, platforms:{os:{sha256,size}}, issuedAt} 생성
                       3. 루트 키로 서명 → release-manifest.json.sig, version.txt·*.sha256(구형식)도 함께 생성
                       4. 산출물을 deploy/launcher-dist/ 로 게시 → Platform-API 서빙
```

#### 사내 GitHub 설정

| 설정 | 값 | 목적 |
|---|---|---|
| 보호 브랜치 `main` | PR 필수, CODEOWNERS 승인 1+, `catalog-validate` 통과 필수, force-push·삭제 금지, 관리자 포함 | 리뷰 안 된 카탈로그가 main 에 못 들어감 |
| CODEOWNERS | `deploy/launcher-dist/catalog.json` · `.github/workflows/catalog-*.yml` · `.github/workflows/launcher-release.yml` · `deploy/scripts/build-catalog.sh` · `gojira-launcher/` (공개키·검증 코드) → 플랫폼 담당 | 카탈로그뿐 아니라 **서명 워크플로·스크립트 변경도** 승인 대상 (워크플로를 고쳐 키를 빼돌리는 경로 차단) |
| Environment `catalog-signing` | Deployment branches: `main` 만. Required reviewers 없음 | 카탈로그 키는 main 에서 도는 job 만 접근. 잦은 배포 병목 회피 |
| Environment `launcher-release` | Deployment branches/tags: `main`, `launcher-v*` 만. **Required reviewers: 플랫폼 담당 1+** | 루트 키 사용 직전 사람 승인 |
| Environment secret | `CATALOG_SIGNING_KEY`(catalog-signing), `ROOT_SIGNING_KEY`(launcher-release) — ed25519 개인키, base64 | 레포·조직 secret 금지, **environment secret** 으로만 보관 |
| self-hosted runner | **현재 0개(GHES 실측) → 선행 구축**(§14 0단계). 라벨 `signer`, ephemeral(job 마다 초기화) | GitHub 호스팅 runner 없음 |
| runner group | 서명 runner 는 전용 group, **허용 레포 = GoJIRA, 허용 워크플로 = `launcher-release.yml`, `catalog-sign.yml`** | 같은 runner 를 쓰는 다른 워크플로가 키·잔여물에 접근하는 경로 차단 |
| Actions 권한 | 워크플로 기본 `permissions: contents: read`, 포크 PR 워크플로에 secret 미전달(기본값 유지) | 최소 권한 |
| 게시 경로 | 서명 runner 가 Platform-API 호스트의 `deploy/launcher-dist/` 에 쓰기 (bind mount 또는 전용 배포 계정 rsync) | 서명과 게시를 한 job 에서 끝냄 |

#### 워크플로 골격

```yaml
# .github/workflows/catalog-sign.yml
name: catalog-sign
on:
  push:
    branches: [main]
    paths: ["deploy/launcher-dist/catalog.json"]
  schedule:
    - cron: "0 3 * * 1"          # 주 1회 재서명 (expiresAt 갱신)
  workflow_dispatch: {}            # 수동 재실행 — environment 가 main 으로 제한하므로 다른 브랜치에서는 키 접근 불가
permissions:
  contents: read
concurrency:
  group: catalog-sign
  cancel-in-progress: false        # 서명·게시가 겹치지 않게 직렬화
jobs:
  sign:
    runs-on: [self-hosted, signer]
    environment: catalog-signing
    steps:
      - uses: actions/checkout@v4
      - name: Validate
        run: deploy/scripts/build-catalog.sh validate
      - name: Sign
        env:
          CATALOG_SIGNING_KEY: ${{ secrets.CATALOG_SIGNING_KEY }}
          RUN_ID: ${{ github.run_id }}
          COMMIT: ${{ github.sha }}
        run: deploy/scripts/build-catalog.sh sign      # issuedAt/expiresAt/commit/runId 기록 후 서명
      - name: Publish
        run: deploy/scripts/build-catalog.sh publish   # catalog.json + .sig → launcher-dist
```

`catalog-validate.yml` 은 `pull_request` 트리거, 서명 runner 가 아닌 일반 self-hosted runner, **environment·secret 없음**.
`launcher-release.yml` 은 `launcher-v*` 태그 push 트리거, `environment: launcher-release`(승인 대기 후 실행), 같은 `signer` runner.

#### 키·카탈로그 규칙

| 항목 | 규칙 |
|---|---|
| 서명 범위 | 런처 매니페스트(루트 키), 서비스 카탈로그(카탈로그 키). 바이너리·패키지는 각 매니페스트 내 sha256 으로 간접 보증 |
| 재전송·다운그레이드 방지 | `issuedAt` 포함 — 런처는 마지막으로 수용한 것보다 **오래된 매니페스트·카탈로그 거부**. 런처 매니페스트는 자기 버전보다 낮은 version 거부 |
| 만료 (카탈로그) | `expiresAt` 기본 30일. 만료 카탈로그는 신규 설치·업데이트에 쓰지 않음(기존 설치 유지). 주 1회 스케줄 재서명으로 정상 운영 중엔 만료되지 않음 |
| 키 생성 | 키별로 오프라인 PC 에서 1회 생성 → 개인키는 해당 environment secret 등록 후 원본 파기(백업은 봉인 보관 1부, 루트 키는 별도 보관), 공개키는 런처 소스에 커밋 |
| 키 교체 | 키별 공개키 **2개 슬롯**(current, next) 임베드 → next 를 미리 배포 후 secret 교체. 다음 런처 릴리스에서 구키 제거 |
| 유출 대응 | **카탈로그 키**: secret 을 next 키로 교체·재서명 + 유출 키를 폐기 목록에 넣은 런처 긴급 릴리스(루트 키 서명). **루트 키**: next 슬롯 키로 서명한 런처를 즉시 릴리스해 current 를 폐기 — next 까지 유출되면 원격 복구 불가, 런처 수동 재설치 공지 |
| 감사 | 매니페스트·카탈로그 메타 `{issuedAt, commit, runId}` + Actions 실행 로그 + Environment 배포·승인 이력 |

#### 패키지 저장소

- 제품 레포 Actions 는 패키지(zip+sha256)만 게시하고 **서명 키에 접근하지 않는다**.
- 저장 위치: 사내 GitHub Releases(제품 레포) 또는 Platform-API `deploy/launcher-dist/services/` — 구현 시 택1(§15).
  카탈로그의 `file` 은 Platform-API 경로로 고정하고, Releases 를 쓰면 `publish` 단계가 내려받아 배치한다
  (런처는 Platform-API 한 곳만 바라본다).

### 10.7 빌드 위치 이전과 구버전 호환 (D14, D15)

**빌드 위치** — 현재는 배포 서버의 `docker compose build` 가 `launcher-dist` 이미지에서 런처를 크로스 컴파일해 공유 볼륨에
채운다(`deploy/docker-compose.yml` `launcher-dist` init job). 서버에서 다시 빌드하면 서명한 바이너리와 배포 바이너리가 달라지므로:

| 현재 | 변경 |
|---|---|
| 서버 `docker compose build` → `deploy/images/launcher-dist/Dockerfile` 이 컴파일 | **Actions(`launcher-release`)가 빌드·서명** |
| init job 이 컴파일 결과를 볼륨에 채움 | init job 은 **서명된 산출물을 볼륨에 배치만**(또는 bind mount 로 대체·제거). 컴파일 단계 삭제 |
| `build-launcher.sh`(외부망 수동) | Actions 워크플로 내부에서 호출하는 빌드 스크립트로 축소 |

서비스 패키지도 동일 원칙 — 제품 레포 Actions 빌드 산출물만 게시, 배포 서버 재빌드 금지.

**구버전 호환** — 0.1.38 이하 런처는 서명 검증을 모른다.

| 단계 | 런처 버전 | 동작 |
|---|---|---|
| A. 경고 모드 | 0.1.39(가칭) | 루트 공개키·검증 코드 탑재. 서명 없음·불일치 시 **경고 로그만** 남기고 기존처럼 sha256 검증 후 교체 |
| B. 필수 모드 | 0.1.40(가칭)~ | 서명 검증 실패 시 **교체 거부**. A 가 충분히 퍼진 뒤 배포(Platform-API 접속 로그의 런처 버전 분포로 판단) |

- `/api/v2/launcher/version` 은 기존 필드(`version`, `platforms.{os}.sha256/size/ready`)를 그대로 두고 `sig` 만 **추가** —
  0.1.37 이하가 쓰는 매니페스트 구제 경로를 유지한다. 서명 원문은 신규 `/api/v2/launcher/release-manifest` 로 제공한다
  (설계 초안은 `/api/v2/launcher/manifest` 를 지목했으나 그 경로는 이미 웹 설치 모달이 쓰고 있어 분리 — 구현 PR #118).
- ≤0.1.38 런처가 A 버전으로 올라오는 첫 업데이트 자체는 sha256 만으로 이뤄진다(불가피). 이 1회 구간은 Actions 빌드 + 서명
  산출물만 서버에 올리는 것으로 위험을 줄인다.

**Authenticode(별개 과제)** — ed25519 서명은 런처 자기 업데이트·서비스 설치의 출처 검증용이다. 최초 설치 시 Windows
SmartScreen "알 수 없는 게시자" 경고 제거는 Authenticode 코드서명 인증서가 별도로 필요(§15).

## 11. 기존 기능 이관 매핑

| 현행 (런처 내장) | 이관 후 | 호환 별칭 (이행 기간) |
|---|---|---|
| `GET /health` | 런처 `/launcher/health` | `GET /health` → 동일 |
| `POST /shutdown` | 런처 `/launcher/shutdown` | 유지 |
| 자기 업데이트, install/uninstall | 런처 | — |
| 스킬·opencode 자산 임베드/배포 | gojira-workspace | — |
| `POST /open` | `POST /v1/workspace/{code}/open` | 바디 `{code,user}` → 경로 변환 |
| `POST /edit` | `POST /v1/agents/edit` | 〃 |
| `POST /chat` | `POST /v1/agents/chat` | 〃 |
| `GET /sessions` | `GET /v1/agents/sessions` | 〃 |
| `POST /prewarm` | `POST /v1/workspace/{code}/prewarm` | 〃 |
| `GET /status` | `GET /v1/workspace/{code}/status` | 〃 |
| `POST /sync`, `/push`, `/pull` | `POST /v1/workspace/{code}/{sync\|push\|pull}` | 〃 |
| `GET /files`, `GET /file` | `GET /v1/workspace/{code}/files`, `GET /v1/workspace/{code}/file` | 〃 |
| `POST /write`, `/delete`, `/rename` | `PUT·DELETE /v1/workspace/{code}/file`, `POST /v1/workspace/{code}/rename` | 〃 |
| `POST /remove-clone` | `DELETE /v1/workspace/{code}` | 〃 |
| `GET /issues`, `POST /issue-journal` | `GET /v1/issues/{code}`, `POST /v1/issues/{code}/journal` | 〃 |

별칭은 런처 라우터의 정적 재작성 표로 구현(서비스 비의존). 웹이 신규 경로로 전환되면 제거(§13 4단계).

## 12. 초기 서비스 명세

### 12.1 gojira-workspace
| 항목 | 값 |
|---|---|
| 정책 / 실행 | required / always |
| OS | windows, darwin, linux |
| 경로 | `/v1/workspace/*`, `/v1/agents/*`, `/v1/issues/*` |
| 요건 | `git`, `code`, `opencode\|claude` (부족해도 기동 — 해당 기능만 `424`) |
| 데이터 | `repoRoot` clone, 이슈 저널(`~/.gojira/data/gojira-workspace/journal/`) |
| 비고 | 현행 런처 코드(repo.go, spawn*.go, gitcreds.go, handlers.go, issues.go, journal.go, skills_embed.go)를 이관. 에이전트 세션 진행 중이면 `busy=true` |

### 12.2 ragaas-converter
| 항목 | 값 |
|---|---|
| 정책 / 실행 | optional / on-demand, idle 10분 |
| OS | windows 전용 |
| 경로 | `POST /v1/docs/pptx/text`, `POST /v1/docs/docx/text` |
| 요건 | `powershell`, `office-com` |
| 입력 | `application/octet-stream` 원본 바이트, `X-File-Name` 헤더(확장자 판정) |
| 출력 | `{text, format, pages\|slides, drm:boolean, chars}` |
| 동작 | 임시 디렉터리에 원 확장자로 저장 → 임베드 PowerShell 스크립트가 Office COM(Word/PowerPoint, 보이지 않게·읽기 전용)으로 열어 텍스트 추출(PPT: 슬라이드 도형·표·노트) → 임시 파일 즉시 삭제 → 응답. 프로세스 타임아웃 시 해당 Office 프로세스 정리 |
| 배경 | 사내 DRM(NASCA) 문서는 파일 선두가 `<## NASCA DRM FILE - VER1.00 ##>` 로 시작. DRM 에이전트가 붙은 로컬 Office 만 복호화 가능하므로 서버 처리 불가 → 사용자 PC 에서 변환 후 RAGaaS 텍스트 업로드 API 로 등록 |
| 사전 검증 | **임시 경로 사본도 DRM 에이전트가 복호화하는지** 실 PC 확인 필요(§14) |

RAGaaS 측 변경(이 문서 범위 밖, RAGaaS 레포에서 진행): 업로드 UI 의 DRM 헤더 감지 → `/v1/docs/{fmt}/text`
호출 → `upload-text` 등록, 서버 파일 업로드 경로의 DRM 파일 거부.

## 13. 웹 클라이언트 (`packages/web-ui` `runtime/launcher.js`)

```js
const local = await connectLauncher();             // /launcher/health 핸드셰이크 → 토큰 보관, capabilities 캐시
local.has('/v1/docs/pptx');                            // available 여부
local.state('/v1/docs/pptx');                          // available | installable | unsupported | null(런처 없음)
await local.install('ragaas-converter', onProgress);
const res = await local.post('/v1/docs/pptx/text', file, { headers: { 'X-File-Name': file.name } });
```
- 기존 `requestEdit` 등은 내부적으로 신규 경로를 호출하도록 교체(시그니처 유지) → 소비 앱 무수정.
- 서비스 이름은 설치 안내 UX 에만 쓰고, 호출은 경로만 사용.

## 14. 이행 단계

0. **서명 기반** (#116, 다른 단계의 선행)
   - GHES self-hosted runner 구축(현재 0개): 일반 runner + 서명 전용 `signer` runner(전용 group, 워크플로 제한, ephemeral).
   - 루트 키·카탈로그 키 생성, `launcher-release`·`catalog-signing` environment 구성, 보호 브랜치·CODEOWNERS.
   - `launcher-release.yml` 로 런처 빌드를 Actions 로 이전, `launcher-dist` init job 의 컴파일 제거(§10.7).
   - 런처 A(경고 모드) 배포 → 버전 분포 확인 → B(필수 모드) 배포. `/launcher/version` 구형식 유지.
1. **런처 코어** — `/launcher/*`, Router(호환 별칭 포함), Supervisor, Installer, 카탈로그 서명 검증.
   기존 ①~④ 는 런처 내부 "내장 서비스" 로 라우터 뒤에 둬서 동작 불변.
   Platform-API `catalog`·`services` 서빙 추가. 사내 GitHub 에 `catalog-validate`/`catalog-sign` 워크플로,
   `catalog-signing` environment, 서명 전용 runner, 보호 브랜치·CODEOWNERS 구성.
2. **ragaas-converter** — 첫 외부 서비스로 구조 검증. 선행: DRM 임시 경로 복호화 수동 검증
   (PowerShell 단독 스크립트). RAGaaS 업로드 연동 동시 진행.
3. **gojira-workspace 분리** — ①~④·스킬 배포를 서비스로 이관, 런처 내장 서비스 제거(별칭은 유지).
4. **정리** — `launcher.js` 신규 경로 전환, 별칭 제거, README·계약 05 §6 갱신.

## 15. 미해결

> 2026-11-05 확정: 경로 버전 접두사(D8), 서비스 공개 원칙(D9), 서비스 간 호출 허용(D10),
> 카탈로그 자동 서명(D11), 사내 GitHub Actions 서명(D12), 런처 매니페스트 서명·키 분리(D13),
> Actions 빌드 이전(D14), 2단계 검증 도입(D15), converter 는 Windows 전용 — macOS/Linux 는 `unsupported` 표시(§12.2).

1. **패키지 저장소** — 사내 GitHub Releases vs Platform-API 배포 디렉터리 직접 게시.
2. **게시 경로 구현** — 서명 runner → Platform-API 호스트 `deploy/launcher-dist/` 쓰기 방식(같은 호스트 bind mount vs 배포 계정 rsync).
3. **Authenticode 코드서명** — 최초 설치 SmartScreen 경고 제거용. 사내 코드서명 인증서 보유 여부 확인 필요.
4. **A→B 전환 기준** — 경고 모드 런처 보급률 임계값(예: 접속 런처의 95%)과 관측 기간.
