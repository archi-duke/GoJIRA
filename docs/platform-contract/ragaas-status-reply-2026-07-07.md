# [회신] standalone base 수정 완료 — C1~C3 진행 가능 (2026-07-07)

> **대상**: GoJIRA/플랫폼 팀. `ragaas-notice-2026-07-07.md` §2 지시 반영 회신.
> **요지**: `base: '/ragaas/'` 반영·재배포·§2 검증 커맨드 통과. **C1~C3 합동 검증 진행 가능합니다.**
> 추가로 셸 모드 API 폴백 결함 1건을 함께 발견·수정했습니다 (아래 §2 — 계약 관점 공유).

## 1. §2 반영 결과 (RAGaaS PR #32, merge 완료)

| 항목 | 반영 |
|---|---|
| Vite `base` | `command === 'build' ? '/ragaas/' : '/'` — 지시 그대로 |
| BrowserRouter basename | `BASE_URL` 기반. 단 **URL 이 실제 `/ragaas` 프리픽스일 때만 적용** — 직접(게이트웨이 없이) 접속·dev(:3002 루트)는 basename 없이 동작 유지 |
| SPA fallback | 기존 `try_files $uri $uri/ /index.html` 있음 — 딥링크 새로고침 200 확인 |
| (자체 추가) nginx `/ragaas/` strip rewrite | 게이트웨이 없이 컨테이너 직접 접속 시 재기준된 절대참조(`/ragaas/assets/...`)가 살도록 프리픽스를 벗겨 재진입. 게이트웨이 경유(이미 strip됨)에는 타지 않음 |

검증 (§2 커맨드):

```
GET {gw}/ragaas/            → index 참조 전수 /ragaas/ 프리픽스 확인
GET {gw}/ragaas/env.js      → 200 application/javascript
GET {gw}/ragaas/assets/mf-entry-bootstrap-*.js → 200 application/javascript
GET {gw}/ragaas/graph-viewer → 200 (딥링크 새로고침)
GET {gw}/ragaas/remoteEntry.js / /ragaas/api/v2/... → 200 / 200 (셸 모드 회귀 없음)
```

## 2. 함께 수정한 결함 — 셸 모드 API 폴백 (계약 관점 공유)

셸 탭에서 remote 의 `window._env_` 는 **셸의 env.js** 라서 `REACT_APP_RAGAAS_API` 가 없고,
RAGaaS 의 빌드 폴백 `/api/v2/` 가 게이트웨이 루트 → 셸로 라우팅되는 상태였습니다 (C2 에서
터졌을 결함). 폴백을 `import.meta.env.BASE_URL` 기준(`/ragaas/api/v2/`)으로 재기준해 해결 —
게이트웨이 단일 origin 전제에서 셸/standalone/직접/dev 4개 진입 모두 관통합니다.

**계약 제안(선택)**: 05 §5 에 "remote 의 API 폴백은 빌드 base 기준 상대경로" 를 관례로 명시하면
다음 제품 온보딩 때 같은 함정을 피할 수 있습니다. (셸 env.js 에 제품별 API 키를 싣는 대안보다
셸-remote 결합이 낮아 이쪽을 권합니다.)

## 3. 다음 단계

- **C1~C3 합동 검증 준비 완료** — 셸 탭에서 마운트/onNavigate, 인증 관통, 탭 전환 확인만 남음.
- 문의 답변 4건 모두 확인했습니다. tarball 수령 확인(`frontend/platform-web-ui-0.1.0.tgz`),
  Phase 2 는 합동 검증 후 착수. WS 인증 v1.1 초안 기다리겠습니다.
