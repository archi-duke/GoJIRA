# [통지] 셸 탭 활성화 완료 + standalone 자산 경로 이슈 1건 (2026-07-07)

> **대상**: RAGaaS 팀. `ragaas-status-reply-2026-07-06.md` 회신 잘 받았습니다.
> **요지**: ① 셸 탭 **활성화 완료** — C1~C3 합동 검증 가능. ② 단, **standalone `/ragaas/` 진입이
> 흰 화면**입니다 — 원인·수정법 아래 (GoJIRA-App 도 같은 문제라 오늘 같은 방식으로 수정했습니다).
> ③ 문의 4건(tarball 경로 / WS 인증 / Kendo 라이선스 / dev 포트) 답변 포함.

## 1. 셸 탭 활성화 완료

플랫폼 게이트웨이 스택의 env 에 `REACT_APP_RAGAAS_APP_REMOTE={scheme}://{host}:{port}/ragaas` 반영
완료 — 셸(`/`)에 RAGaaS 탭이 노출되고 remoteEntry 를 로드합니다. **C1~C3 합동 검증 진행 가능합니다.**

## 2. 이슈: standalone `/ragaas/` 흰 화면 — Vite `base` 필요

### 증상 / 원인

`http://{gateway}/ragaas/` 직접 진입 시 HTML(200)은 오지만 화면이 비어 있습니다.
빌드된 `index.html` 이 자산을 **루트 절대경로**로 참조하기 때문입니다:

```html
<script src="/env.js"></script>
<script type="module" src="/assets/mf-entry-bootstrap-….js"></script>
```

브라우저가 이어서 요청하는 `/assets/…`, `/env.js` 에는 `/ragaas` 프리픽스가 없어서, 게이트웨이
최장일치 라우팅이 이를 **셸(`/` → platform-app)** 로 보냅니다. 셸 컨테이너에는 그 파일이 없으니
JS 부트스트랩이 실패 → 흰 화면.

A8 에서 검증하신 것(remoteEntry/청크)은 **셸 모드**입니다 — remoteEntry 는 자기 로드 URL 기준으로
청크를 상대 해석하므로 멀쩡합니다. 깨지는 것은 **index.html 로 시작하는 standalone 진입**뿐입니다.
(C5 standalone 검증은 dev 서버 루트에서 하셔서 통과했을 것으로 추정.)

### 수정 (GoJIRA-App 에 오늘 적용한 것과 동일 패턴)

`vite.config` 에 **빌드 전용** `base` 를 추가:

```ts
export default defineConfig(({ command }) => ({
  // standalone 진입(게이트웨이 /ragaas/)용 — dev(:3002 루트)와 셸 remote 로드는 영향 없음
  base: command === 'build' ? '/ragaas/' : '/',
  ...
}));
```

- Vite 가 빌드 시 index.html 의 절대 참조(`/env.js`, `/assets/…`, public 자산)를 전부
  `/ragaas/…` 로 재기준합니다. 게이트웨이는 `/ragaas` 만 strip 하므로 컨테이너 nginx 에는
  종전과 같은 경로로 도달 — **entrypoint 의 env.js 생성 등 컨테이너 내부는 무변경**.
- remoteEntry 의 publicPath 는 자동(로드 URL 기준)이라 셸 탭 모드는 영향 없음 (재확인 권장).
- dev 서버는 `command === 'serve'` 라 base `/` 유지 — 로컬 개발 흐름 무변경.

### 함께 확인할 것 2가지

1. **BrowserRouter basename** — standalone 모드에서 BrowserRouter 를 쓰신다면(A3), 게이트웨이
   경유 시 브라우저 URL 이 `/ragaas/…` 이므로 `basename={import.meta.env.BASE_URL}` (또는
   `'/ragaas'`) 지정이 필요합니다. 안 하면 라우트 매칭이 어긋납니다.
2. **컨테이너 nginx SPA fallback** — `/ragaas/some/route` 딥링크는 strip 후 `/some/route` 로
   도달하므로 `try_files $uri /index.html` 류 fallback 이 있어야 새로고침이 살아납니다.

### 검증 방법

```bash
curl -s http://{gateway}/ragaas/ | grep -o 'src="[^"]*"'   # 전부 /ragaas/ 프리픽스인지
# 그중 하나를 직접 GET → 200 이고 Content-Type 이 javascript 인지 (셸 HTML 이 오면 실패)
```

## 3. 문의 답변

| 문의 | 답변 |
|---|---|
| **web-ui tarball 전달 경로** | 이미 복사해 두었습니다: RAGaaS 레포 루트 기준 `frontend/platform-web-ui-0.1.0.tgz`. 설치: `npm i ./platform-web-ui-0.1.0.tgz` (`.npmrc` 에 `legacy-peer-deps=true` 유지 — react 이중 설치 방지) |
| **WebSocket 인증** | 제안 수용합니다. 계약 05 에 WS 인증 규약(첫 메시지 토큰 방식 유력 — 쿼리파라미터는 액세스 로그 노출 우려)을 **v1.1 로 추가 예정**, 초안은 플랫폼 쪽에서 작성해 공유하겠습니다. 그전까지는 게이트웨이/셸 경유 전제로 현행 수용. |
| **Kendo 라이선스** | 이해하신 대로입니다. web-ui 이관(Phase 2) 시 플랫폼 소유 라이선스로 일원화 — 실키는 어느 레포에도 커밋하지 않고 **빌드 시점 주입**(CI/빌드 머신의 `TELERIK_LICENSE`)입니다. 미등록 모드 워터마크는 Phase 2 에서 자연 해소. |
| **dev 포트 3002** | 셸 registry 는 RAGaaS 를 env 게이트로만 등록합니다(dev 폴백 없음 — 미기동 환경에서 죽은 탭 방지). dev 합동 검증 시 Platform-App 쪽 `.env` 에 `REACT_APP_RAGAAS_APP_REMOTE=http://localhost:3002` 한 줄이면 됩니다. |

## 4. 다음 단계

1. RAGaaS: `base: '/ragaas/'` 반영 + basename/SPA fallback 확인 → 재빌드·재배포 → §2 검증 커맨드 통과
2. 합동: 셸 탭에서 C1(마운트/onNavigate) · C2(인증 관통) · C3(탭 전환/세션 유지) 검증
3. 이후 Phase 2 (web-ui tarball 합류) 착수
