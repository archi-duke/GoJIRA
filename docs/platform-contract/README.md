# 플랫폼 통합 계약 (Platform Integration Contract)

> **대상 독자**: Platform-App(셸)에 remote 로 합류하는 모든 제품 팀 — 1호 GoJIRA-App, 2호 RAGaaS.
> **지위**: 이 폴더의 문서가 셸↔제품 간 **유일한 통합 계약**이다. 코드가 문서와 다르면 문서가 우선하며, 코드를 고친다.
> **버전**: v1.0-draft (2026-07-06) — Duke 승인(머지) 시 v1.0 확정.

## 배경

Platform-App/Platform-API 를 분리한 이유는 **공통 기능(인증·공지·계정·디자인)을 플랫폼 표준으로 중앙 관리**하기 위함이다.
Platform-App 은 여러 제품을 담는 **MFE 셸(shell)** 이고, 각 제품 앱은 Module Federation **remote** 로 합류한다.

- 1호 remote: GoJIRA-App (React, Go 백엔드)
- 2호 remote: RAGaaS (React 프론트, **파이썬 백엔드**) — 이 계약을 그대로 구현해서 붙는다.

백엔드가 언어별(Go/파이썬)로 갈리므로 **백엔드 계약은 언어중립 스펙**(HTTP/헤더/ENV 규약)으로만 정의한다. 코드 공유를 전제하지 않는다.

## 확정 결정 (이 계약의 전제 — 변경 시 전 제품 합의 필요)

| # | 결정 | 근거 문서 |
|---|------|-----------|
| 1 | 공통 프론트 툴체인 = **Vite + TypeScript** (CRA/CRACO 폐기) | [01-mf-host](01-mf-host-contract.md) |
| 2 | MF 런타임 = **@module-federation/vite** (MF 2.0 런타임 — 버전 협상/폴백) | [01-mf-host](01-mf-host-contract.md) |
| 3 | Kendo React 는 **@platform/web-ui 가 유일한 소유자** — 앱이 Kendo 를 직접 의존 금지 | [04-web-ui](04-web-ui-contract.md) |
| 4 | 인증은 **셸이 런타임 제공** (remote 자체 로그인 금지) — 제공 방식은 **MF exposed 모듈** `platformApp/auth` | [02-auth](02-auth-contract.md) |
| 5 | 공지는 **셸이 NoticeCenter 를 MFE 위젯으로 제공**, remote 는 마운트만 | [03-notice](03-notice-contract.md) |
| 6 | 폐쇄망 운영 전제: **런타임 env 주입(env.js / window._env_)** 으로 재빌드 없이 호스트/스킴 교체 | [05-backend](05-backend-contract.md) §5 |

## 문서 맵

| 문서 | 내용 | RAGaaS 필수 여부 |
|------|------|------------------|
| [01-mf-host-contract.md](01-mf-host-contract.md) | MF 호스트/remote 규약 — 이름·expose·shared singleton·remote URL 결정 | **필수** |
| [02-auth-contract.md](02-auth-contract.md) | 셸 인증 제공 계약 — `platformApp/auth` API, 소비 패턴(fetch/axios), standalone 폴백 | **필수** |
| [03-notice-contract.md](03-notice-contract.md) | 공지 위젯 계약 — NoticeCenter/NoticePopup 소유권과 마운트 규약 | 필수(소극적 — "직접 만들지 마라") |
| [04-web-ui-contract.md](04-web-ui-contract.md) | 디자인시스템 @platform/web-ui — Kendo 소유권, MF 공유, 테마 스코프 | **필수** |
| [05-backend-contract.md](05-backend-contract.md) | 언어중립 백엔드 계약 — introspect·X-Service-Token·헤더·게이트웨이 경로·env 주입 | **필수** (파이썬 어댑터 구현 대상) |
| [06-onboarding-checklist.md](06-onboarding-checklist.md) | 신규 제품 합류 절차 체크리스트 | **필수** (실행 순서) |
| [openapi-auth.yaml](openapi-auth.yaml) | 인증 관련 HTTP 계약의 OpenAPI 표현 (introspect·exchange·/Account/me) | 참조 |

## 용어

| 용어 | 정의 |
|------|------|
| **셸(shell) / 호스트** | Platform-App. 타이틀바·네비게이션·인증·공지를 소유하고 remote 를 로드하는 컨테이너 |
| **remote / 제품 앱** | MF 로 셸에 로드되는 제품 프론트엔드 (GoJIRA-App, RAGaaS-App) |
| **standalone 모드** | remote 를 셸 없이 자기 포트로 단독 실행하는 개발/비상 모드. 계약상 지원 의무 |
| **게이트웨이** | deploy 의 nginx 단일 진입점. 경로 프리픽스로 각 앱/API 라우팅 ([05](05-backend-contract.md) §4) |
| **런타임 env** | 컨테이너 기동 시 entrypoint 가 생성하는 `env.js` → `window._env_` ([05](05-backend-contract.md) §5) |

## 현재 코드와의 괴리 (알려진 것)

이 계약은 **목표 상태**를 기술한다. 2026-07-06 기준 GoJIRA 코드는 다음이 계약과 다르며, P1(Vite 이전)·P2(부채 상환) 단계에서 계약에 수렴시킨다:

- 인증이 `window.fetch` monkey-patch (계약: `platformApp/auth` 모듈) — Platform-App/src/App.js:20, GoJIRA-App/src/GoJIRAApp.js:26
- 공통 컴포넌트가 두 앱에 복붙 (계약: @platform/web-ui) — AlertModal·CommonBar·NoticePopup 등 17종
- 셸↔remote 상태가 sessionStorage magic string (계약: mount props + 콜백) — `gojira_page`, `gojira_project`
- remote expose 가 `./GoJIRAApp` (계약: `./App` 표준화)
- 툴체인 CRA/CRACO (계약: Vite+TS)
- deploy 버그: platform-app entrypoint 가 `REACT_APP_USE_SSO` 를 env.js 에 누락
