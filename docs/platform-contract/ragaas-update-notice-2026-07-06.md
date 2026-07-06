# [갱신 통지] RAGaaS 온보딩 — 변경 3건 (2026-07-06)

> **대상**: 앞서 `ragaas-onboarding-prompt.md` 로 작업 지시를 받은 RAGaaS 담당자/에이전트.
> **전제**: 기존 지시서 내용은 그대로 유효합니다. 이 통지는 그 이후 바뀐 것 **3건만** 담습니다.
> 함께 전달되는 **갱신판 `ragaas-onboarding-prompt.md` 와 계약 문서(01~06)** 가 이 변경들이 반영된 최신본이니,
> 이전에 받아둔 사본이 있다면 폐기하고 이번 것으로 교체하세요.

---

## 변경 1 — 게이트웨이 경로 체계가 단순해졌습니다 (아직 URL 을 코드에 넣지 않았다면 영향 최소)

`-app`/`-api` 접미사가 제거되고 **제품 하나 = 프리픽스 하나**가 됐습니다:

| 이전 안내 (v1 — 폐기) | 현재 (v2 — 이걸 사용) |
|---|---|
| `{게이트웨이}/ragaas-app/` | **`{게이트웨이}/ragaas/`** (프론트/remoteEntry.js) |
| `{게이트웨이}/ragaas-api/api/v2/…` | **`{게이트웨이}/ragaas/api/v2/…`** (백엔드 API) |
| `{게이트웨이}/platform-api/api/v2` | **`{게이트웨이}/platform/api/v2`** (공용 Platform-API) |

- 프론트의 자기 백엔드 baseURL 예: `REACT_APP_RAGAAS_API = {게이트웨이}/ragaas/api/v2`
- **백엔드가 받는 경로는 변화 없음** — 게이트웨이가 `/ragaas` 세그먼트만 떼고 넘기므로 FastAPI 는
  기존처럼 `/api/…` 로 수신합니다. frontend 내부 nginx 의 `/api/` 프록시 등 내부 경로도 무관.
- 백엔드→Platform-API 호출은 게이트웨이를 거치지 않고 컨테이너 직결(`http://platform-api:9000/api/v2`)이라 무관.
- 참고: 슬래시 없는 `/ragaas` 는 셸의 RAGaaS 탭 딥링크, 슬래시 있는 `/ragaas/…` 부터가 RAGaaS 영역입니다.

## 변경 2 — @platform/web-ui tarball 준비 완료 (Phase 2 블로커 해소)

이전 지시서에서 "GoJIRA 팀 준비 대기"였던 항목이 해소됐습니다:

```bash
# GoJIRA 팀이 전달하는 tarball 설치
npm install ./platform-web-ui-0.1.0.tgz
# 이후: import DataGrid from '@platform/web-ui/components/DataGrid' 등 서브패스 그대로 사용
# Kendo 의존은 tarball 이 함께 설치. react/react-dom 은 peer — RAGaaS 것 사용 (React 19 상향 선행)
```

외부 Vite 프로젝트에서 설치→import→빌드까지 GoJIRA 쪽에서 검증을 마쳤습니다. Phase 2(Kendo→web-ui 이관)는
Phase 1 합류 검증 후 진행하면 됩니다 — 서두를 필요 없음.

## 변경 3 — [즉시 액션] compose 수정 1건: frontend 를 shared-net 에 조인

게이트웨이가 `/ragaas/` 를 `ragaas-frontend:80` 으로 프록시하는데, 현재 frontend 컨테이너가 shared-net 에
없어 도달하지 못합니다(502). backend 는 이미 조인되어 `/ragaas/api/` 관통이 확인됐습니다. docker-compose.yml 의
frontend 서비스에 backend 와 동일하게 추가 후 `docker compose up -d frontend`:

```yaml
  frontend:
    networks:
      - default
      - shared-net
```

---

## 변경 없음 (안심 목록)

- 인증 어댑터(`window.__platformAuth`), 마운트 props 계약, MF remote 설정, 백엔드 신원 미들웨어 — **기존 지시서 그대로**
- React 19 상향이 선행 조건인 것도 그대로
- Kendo 5.8.0 고정(라이선스 만료로 상향 금지)도 그대로

## 진행 상황 공유 요청

기존 지시서의 체크리스트 기준으로 현재 어디까지 왔는지(특히 React 19 상향 여부)를 GoJIRA 팀에 알려주시면,
셸 쪽 RAGaaS 탭 활성화(env 한 줄) 시점을 맞추겠습니다. GoJIRA 쪽 수용 준비(셸 registry·게이트웨이·tarball)는
모두 완료된 상태라, RAGaaS 가 `./App` expose + 위 compose 수정만 마치면 바로 셸에서 탭이 켜집니다.
