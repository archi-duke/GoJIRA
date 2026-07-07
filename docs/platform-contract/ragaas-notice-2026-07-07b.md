# [통지] C1~C3 합동 검증 통과 — 온보딩 Phase 1 완료 (2026-07-07)

> **대상**: RAGaaS 팀. `ragaas-status-reply-2026-07-07.md` 회신에 대한 결과 통지.
> **요지**: ① **C1~C3 검증 통과 — RAGaaS 셸 합류 완료.** ② API 폴백 계약 제안 **수용** (05 §5 반영).
> ③ 셸에 브랜디드 스코프가 추가됨 — `/ragaas` 진입 시 RAGaaS 브랜드 화면 (§3, manifest 는 선택).

## 1. C1~C3 검증 결과 (실브라우저, 게이트웨이 경유)

| 항목 | 결과 |
|---|---|
| C1 마운트/onNavigate | ✅ 셸 RAGaaS 탭 → Knowledge Bases 렌더. onNavigate 의 내부 라우트 키는 셸이 무시하도록 수정(계약 01 §2 명문화 — 초기 placeholder 문제는 셸 결함이었음) |
| C2 인증 관통 | ✅ `/ragaas/api/v2/knowledge-bases/` 200 (dev 폴백 신원). SSO 모드 관통은 SSO 전환 시 재검증 예정 |
| C3 탭 전환/세션 유지 | ✅ Project ↔ RAGaaS 왕복, 사용자/세션 유지, 콘솔 에러 0 |

standalone `/ragaas/` 는 base 반영으로 정상 (참고: 게이트웨이가 `/ragaas/` HTML 진입을 `/ragaas` 로
302 수렴시키므로 사용자는 항상 셸 크롬을 받습니다 — 그쪽 base 수정은 직접 접속/위생 차원에서 유효).

## 2. API 폴백 계약 제안 — 수용

05 §5 에 규칙으로 반영했습니다: **remote 의 자기 API 폴백은 빌드 base 기준 상대경로**
(`import.meta.env.BASE_URL + 'api/v2/'`). 셸 env.js 에 제품 키를 싣는 대안은 결합 증가로 기각 —
제안하신 방향 그대로입니다. 다음 제품 온보딩 지시서에도 포함하겠습니다.

## 3. 신규(선택): 브랜디드 스코프 + `./manifest`

셸이 진입 URL 로 스코프를 정합니다: `/` = 플랫폼 통합 뷰, **`/ragaas` = RAGaaS 브랜드(아이콘+이름)
+ RAGaaS 가 선언한 타이틀바 메뉴만** (로그인 사용자 표시/인증/공지는 동일 제공). 메뉴/브랜드는
remote 의 선택 expose `./manifest` 로 선언합니다:

```js
// exposes: { './manifest': './src/manifest' } — 순수 데이터, 초경량 청크
export default {
  name: 'RAGaaS',
  icon: 'icons/ragaas-logo.svg',           // remote base 상대 (public/ 자산)
  menus: [
    { key: 'kb',   label: 'Knowledge Bases' },
    { key: 'jobs', label: 'Ingest Jobs' },  // 예시 — 키는 자유, page prop 으로 전달됨
  ],
};
```

- **안 하면**: 현행 유지 (`/ragaas` 진입 시 브랜드명 'RAGaaS' 폴백, 메뉴 없음, 바디 전체).
- **하면**: 타이틀바 메뉴를 제품이 소유. 클릭된 메뉴 키는 `page` prop(RemoteAppProps 추가)으로
  들어오고, `pages` 배열로 내부 페이지↔메뉴 활성 매핑을 선언할 수 있습니다. 계약 01 §2 참조.

## 4. 남은 것

- WS 인증 규약(계약 05 v1.1) 초안 — 플랫폼 쪽 작성 예정, 공유하겠습니다.
- Phase 2 (web-ui tarball 합류) — 그쪽 일정대로 착수하시면 됩니다.
