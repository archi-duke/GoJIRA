# 04. 디자인시스템 계약 — `@platform/web-ui`

> **원칙: Kendo(Telerik KendoReact)의 유일한 소유자는 @platform/web-ui 패키지다.**
> 제품 앱은 `@progress/kendo-*` 를 **직접 의존하지 않는다** — web-ui 가 감싼 컴포넌트만 사용한다.

## 0. 현황 명시 (2026-07-06)

**Kendo 는 아직 GoJIRA 어디에도 없다** (두 앱 package.json·src 전체에 `@progress/kendo-*` 0건).
이 계약은 "이미 확산된 것을 수습"이 아니라 **확산 전에 단일 소유 지점을 세우는 예방 계약**이다.

단, **RAGaaS 는 이미 Kendo 를 사용 중**이다 (D:\works\RAGaaS\frontend 실사, 아래 §6) — 플랫폼 기준 버전은
RAGaaS 실사용 버전에서 출발하고, RAGaaS 의 Kendo 의존은 합류 시 web-ui 로 이관한다.

## 1. 패키지 소유 범위

| 범위 | 내용 |
|------|------|
| Kendo 의존 | `@progress/kendo-react-*`, `@progress/kendo-licensing`, 테마 패키지 — **web-ui 의 dependencies 로만 존재** |
| 라이선스 활성화 | **빌드타임 주입** — `TELERIK_LICENSE` env(신방식) 또는 `telerik-license.txt`(키 파일, 커밋 금지·.gitignore). Docker 빌드는 `ARG TELERIK_LICENSE` build-arg. RAGaaS Dockerfile 패턴을 플랫폼 표준으로 채택 (§6) |
| 테마 | Kendo 테마 + 플랫폼 디자인 토큰(CSS 변수 `--pf-*`) — web-ui 가 단일 CSS 엔트리 제공 |
| 공통 컴포넌트 | 현재 두 앱에 복붙된 것들을 흡수: AlertModal(+modalAlert/modalConfirm/modalPrompt), CommonBar, NoticePopup, DiffAnalysis, exportUtils 등 |
| 플랫폼 런타임 유틸 | config/env 로더(window._env_ 조회), sseManager, launcher 클라이언트 공통 부분 — `@platform/web-ui/runtime` 서브패스 |

> 복붙본 흡수 시 **기능 슈퍼셋 판을 기준**으로 한다 (예: AlertModal 은 GoJIRA 판 — `dangerous/confirmLabel/validate`
> 옵션 포함. launcher 는 GoJIRA 판 15개 함수가 슈퍼셋).

## 2. 배포 형태 (폐쇄망 전제)

- 레포 내 **workspace 패키지** (`packages/web-ui`) 로 시작한다 — npm 레지스트리 불필요, 폐쇄망 오프라인 빌드와 호환.
- RAGaaS 등 외부 레포 제품은: ① MF shared singleton 으로 런타임 공유(아래 §3)를 기본으로 하고,
  ② 빌드타임 타입/authoring 이 필요하면 tarball(`npm pack`) 반입 또는 사내 레지스트리 등록. (Kendo 라이선스 특성상
  소스 vendoring 금지 — 패키지 단위로만 이동.)

## 3. MF 공유 규약

```js
shared: { '@platform/web-ui': { singleton: true } }
```

- **singleton 필수** — 모달 레지스트리(전역 modalAlert)·라이선스 활성화·테마 주입이 런타임에 하나여야 한다.
- 버전 정책: web-ui 는 semver 를 지킨다. breaking(컴포넌트 API 변경)은 메이저 범프 + 전 제품 공지.
- remote 는 web-ui 를 dependencies 에 선언하되(타입·standalone 대비), 셸 안에서는 MF 협상으로 셸 인스턴스를 공유받는다.

## 4. 테마/CSS 스코프

- Kendo 테마 CSS 는 **셸이 1회 로드**한다 (web-ui 의 `@platform/web-ui/theme.css` 엔트리). remote 는 테마를 다시 import 하지 않는다
  (standalone 실행 시에만 자기 엔트리에서 로드 — 중복 로드 가드는 web-ui 초기화 함수가 담당).
- 플랫폼 토큰은 CSS 변수 `--pf-*` 로 `:root` 에 정의 — remote 커스텀 스타일도 이 변수를 참조해 테마 일관성 유지.
- remote 자체 CSS 는 제품 접두사(`gj-`, `rg-` 등) 또는 CSS Modules 로 격리 — 전역 태그 셀렉터 금지 (셸/타 remote 오염 방지).

## 5. 기준 버전 (2026-07-06 RAGaaS frontend 실사 — 플랫폼 표준의 출발점)

| 항목 | 값 | 비고 |
|------|----|------|
| KendoReact 컴포넌트군 | **5.8.0** (exact pin — grid/intl/theme는 caret 없음) | 실사용 표면은 **Grid 단일** (GraphDataTable, ChunksModal) + kendo-data-query descriptor |
| 테마 | **@progress/kendo-theme-bootstrap 5.8.0** — 사전컴파일 `dist/all.css`, 커스텀 SCSS 없음 | kendo-theme-default 도 설치돼 있으나 미사용 → web-ui 이관 시 제거 |
| 라이선스 | @progress/kendo-licensing (lock 1.10.0), **빌드타임 `TELERIK_LICENSE` env / `telerik-license.txt`** — 키는 레포 미커밋, CI 파이프라인 부재(Dockerfile build-arg 훅만 준비) | **좌석/에디션/만료 정보가 레포에 없음 — 조달 주체·갱신 책임은 별도 확인 필요** (미결) |
| 백엔드 결합 | RAGaaS backend 가 Kendo Grid Sort/Filter **Descriptor JSON 을 쿼리 파라미터 계약으로 수신** (graph_viewer.py) | web-ui 로 Grid 를 이관해도 이 wire format 은 유지해야 함 |
| RAGaaS 툴체인 | Vite 5 + TS, react-router-dom 7, axios 1.13 | 계약(01·02)과 이미 정합 |
| **React 버전 충돌** | RAGaaS **react 18.3.1** vs 플랫폼 셸 **19.x** | singleton 공유 불가 조합 — **RAGaaS 는 합류 전 React 19 로 상향** ([01](01-mf-host-contract.md) §4 버전 정책). 온보딩 A2 선행 조건 |

- 버전 상향(Kendo 5.8.0 → 이후 라인)은 web-ui 가 단독 결정·일괄 적용한다 — 제품 앱이 개별적으로 올릴 수 없음(그게 이 계약의 목적).
- web-ui 최초 구현 범위도 이 실사에 맞춘다: **Grid 래퍼 + 테마 엔트리 + 라이선스 빌드 규약**이 1차, 나머지 컴포넌트는 수요 발생 시.

## 6. 강제 장치

- 제품 앱 package.json 에 `@progress/kendo-*` 가 나타나면 CI 에서 실패시킨다 (lint/스크립트 체크 — P1 때 추가).
- PR 리뷰 체크리스트에 "공통 UI 를 앱 로컬에 새로 만들지 않았는가" 항목 포함.
