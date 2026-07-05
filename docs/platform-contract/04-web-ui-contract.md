# 04. 디자인시스템 계약 — `@platform/web-ui`

> **원칙: Kendo(Telerik KendoReact)의 유일한 소유자는 @platform/web-ui 패키지다.**
> 제품 앱은 `@progress/kendo-*` 를 **직접 의존하지 않는다** — web-ui 가 감싼 컴포넌트만 사용한다.

## 0. 현황 명시 (2026-07-06)

**Kendo 는 아직 GoJIRA 어디에도 없다** (두 앱 package.json·src 전체에 `@progress/kendo-*` 0건).
이 계약은 "이미 확산된 것을 수습"이 아니라 **확산 전에 단일 소유 지점을 세우는 예방 계약**이다.
RAGaaS 를 포함해 앞으로 Kendo 를 쓰는 첫 코드는 반드시 web-ui 안에서 시작한다.

## 1. 패키지 소유 범위

| 범위 | 내용 |
|------|------|
| Kendo 의존 | `@progress/kendo-react-*`, `@progress/kendo-licensing`, 테마 패키지 — **web-ui 의 dependencies 로만 존재** |
| 라이선스 활성화 | `kendo-ui-license` 활성화 코드/키 관리 — web-ui 초기화 함수 한 곳 |
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

## 5. 강제 장치

- 제품 앱 package.json 에 `@progress/kendo-*` 가 나타나면 CI 에서 실패시킨다 (lint/스크립트 체크 — P1 때 추가).
- PR 리뷰 체크리스트에 "공통 UI 를 앱 로컬에 새로 만들지 않았는가" 항목 포함.
