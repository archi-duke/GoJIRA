# [회신] RAGaaS 온보딩 진행 상황 (2026-07-06)

> **대상**: GoJIRA/플랫폼 팀. `ragaas-update-notice-2026-07-06.md` 의 "진행 상황 공유 요청"에 대한 회신.
> **요지**: **React 19 상향 포함 Phase 1 (A1~A12) 전부 완료. 변경 3(compose)도 반영·검증 완료.
> 셸 탭 활성화(env 한 줄) 바로 진행해 주셔도 됩니다.**

## 체크리스트 현황 (06-onboarding-checklist 기준)

| 항목 | 상태 | 비고 |
|---|---|---|
| A1. Vite + TS + @module-federation/vite | ✅ | @module-federation/vite 1.16.12 |
| A2. MF remote 설정 | ✅ | name `ragaasApp`, `remoteEntry.js`, exposes `./App`, shared react/react-dom singleton (requiredVersion 자동) |
| A2 선행. **React 19 상향** | ✅ | **18.3.1 → 19.2.7**. Kendo 5.8.0 고정 유지(.npmrc legacy-peer-deps) |
| A3. 마운트 컴포넌트 (`./App` + RemoteAppProps) | ✅ | 셸 모드 MemoryRouter / standalone BrowserRouter. onNavigate 마운트 직후 1회 + 전환마다 호출 |
| A4. 인증 소비 (`window.__platformAuth` 어댑터 + axios 인터셉터) | ✅ | Bearer / X-User-Id 폴백, 401→requireLogin(). 자체 로그인 없음 |
| A5. standalone 폴백 | ✅ | USE_SSO 쿠키 introspect / dev 사용자 |
| A6. @platform/web-ui | ⏳ Phase 2 | tarball 준비 완료 통지 확인 — Phase 1 합류 검증 후 진행 예정 |
| A7. 공지 | — | 이번 범위 제외 (지시서 §4 대로) |
| A8. 자산 경로 (publicPath 상대 해석) | ✅ | 게이트웨이 `/ragaas/` 아래에서 remoteEntry/chunk 로드 확인 (아래 검증) |
| A9. 런타임 env (window._env_ + entrypoint→env.js) | ✅ | 재빌드 없이 env 교체 반영 검증 (C6) |
| A10. 백엔드 신원 미들웨어 | ✅ | introspect/X-Service-Token(hmac)/dev 폴백 — 단위 검증 13/13 PASS. backend·ingest 양쪽 적용, 서비스간 호출에 X-Service-Token 부착 |
| A11. /api/v2 베이스 | ✅ | backend·ingest 모두 /api/v2 (구 /api 는 404) |
| A12. sessionStorage `ragaas_` 접두사 | ✅ | 웹스토리지 키 전수 적용 |

## 갱신 통지 3건 반영 결과

1. **경로 v2**: 코드에 게이트웨이 URL 하드코딩 없음(전부 런타임 env) — 주석/예시만 v2 로 갱신. 백엔드 수신 경로 불변 확인.
2. **web-ui tarball**: 확인. Phase 2 는 셸 합류 검증 후 착수 — tarball 전달 경로만 알려주시면 됩니다.
3. **frontend shared-net 조인**: 반영 완료 + **게이트웨이 관통 검증 완료**:

```
http://localhost:44300/ragaas/                       200  (frontend index)
http://localhost:44300/ragaas/remoteEntry.js          200  (MF 엔트리)
http://localhost:44300/ragaas/api/v2/knowledge-bases/ 200  (backend 관통, dev 폴백)
http://localhost:44300/ragaas/ingest-api/jobs/probe   404  (ingest 도달 — 없는 job 프로브라 404 = 정상)
```

## 완료 판정 (C 항목)

- C4 (dev X-User-Id 폴백) ✅ / C5 (standalone) ✅ / C6 (재빌드 없이 env 교체) ✅
- C1~C3 은 셸 탭 활성화 후 합동 검증 대기 — **RAGaaS 쪽 준비 완료. env 한 줄 켜주시면 됩니다.**

## 논의 요청 1건 (블로커 아님)

- **WebSocket 인증**: Starlette HTTP 미들웨어가 WS(`/api/v2/ws/{kb_id}`, 문서 인제스션 진행상황 push)를 가로채지
  않아 신원 미들웨어 보호 범위 밖입니다. 게이트웨이/셸 경유 전제로 당장은 수용 가능하나, 계약 05 에 WS 인증
  규약(예: 첫 메시지 토큰 또는 쿼리파라미터 + introspect)을 추가할지 논의를 제안합니다.

## 참고 (RAGaaS 내부 사정)

- dev 포트: 3002 (셸 registry 의 dev 폴백 URL 에 반영 필요 시 참고)
- Kendo 라이선스 실키는 RAGaaS 레포/로컬에 없음 — 현재 빌드는 미등록 모드. 04 계약대로 web-ui 이관 시
  플랫폼 소유 라이선스로 일원화되면 해소될 것으로 이해하고 있습니다 (다르면 알려주세요).
