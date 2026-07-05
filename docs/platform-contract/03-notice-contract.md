# 03. 공지 위젯 계약 — `platformApp/notice`

> **원칙: 공지는 셸 소유.** remote 는 공지 UI 를 만들지 않고, 필요하면 셸이 노출하는 위젯을 마운트만 한다.

## 1. 소유권

| 역할 | 소유 |
|------|------|
| 공지 데이터 (CRUD, Platform-API `/Notice/*`) | 플랫폼 (Platform-API + 셸 NoticeManager 관리화면) |
| 전역 공지 팝업 (긴급/일반 공지 표시, "오늘 하루 보지 않기") | **셸이 전역 렌더** — remote 는 아무것도 하지 않아도 표시됨 |
| 공지 목록/센터 위젯 | 셸이 `platformApp/notice` 로 노출 — remote 가 자기 화면 안에 넣고 싶을 때만 마운트 |

## 2. 모듈 API

```ts
// import { NoticeCenter } from 'platformApp/notice'
export interface NoticeCenterProps {
  /** 표시 개수 제한 (기본 전체) */
  limit?: number;
  /** 컴팩트(목록만) / full(읽음 처리 포함) — 기본 'list' */
  variant?: 'list' | 'full';
}
export const NoticeCenter: React.ComponentType<NoticeCenterProps>;
```

- 데이터 로딩·읽음 처리·실시간(긴급 공지 SSE/WS) 은 위젯 내부에서 해결한다. remote 는 props 외 아무것도 주입하지 않는다.
- 인증은 위젯이 `platformApp/auth` 를 내부 사용 — remote 가 토큰을 넘길 필요 없다.

## 3. Standalone 모드

- remote 단독 실행 시 전역 공지 팝업이 없다. 필요하면 remote 가 자체 최소 구현을 갖는 것을 허용하되
  (현 GoJIRA-App NoticePopup 이 이 케이스), **셸 안에서는 반드시 비활성**이어야 한다 — 이중 팝업 금지.
  판별: `platformApp/notice` import 성공 여부 (셸 내 실행 = 성공).

## 4. 현 코드와의 괴리 (P2 정리 대상)

- NoticePopup 이 두 앱에 복붙되어 갈라짐 — Platform 판은 `gojira_notice_hide_until_<user>` (user별),
  GoJIRA 판은 `gojira_notice_hide_until` (고정 키). **같은 개념에 다른 키** → 계약상 user별 키로 통일.
- 셸 안에서 GoJIRA remote 가 로드될 때 GoJIRA-App 자체 NoticePopup 은 렌더하지 않아야 하나, 현재는
  standalone App.js 경로에만 있어 실질 문제는 없음 — Vite 이전 시 §3 판별 규칙으로 명시화.
