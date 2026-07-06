# GoJIRA 포트 매핑

> 본 시스템에서 사용하는 모든 포트의 단일 출처.
> 각 서비스는 startup 시 자기 포트 + 의존 peer 포트를 로그로 출력한다.

## 환경별 호스팅 위치

| 환경 | MongoDB / Gitea | OpenCode | Backend / Frontend |
|---|---|---|---|
| **로컬 개발 (Mac)** | Docker — 같은 호스트 | 같은 호스트 | 같은 호스트 |
| **Mac + Parallels Ubuntu 게스트** | Mac (host IP, 예: `10.211.55.2`) | Mac (host IP, 예: `10.211.55.2`) | 게스트 안 |
| **사내망 (Windows + WSL)** | **별도 사내 서버** | **WSL 안 (사용자 PC)** | WSL 안 (사용자 PC) — 가정, 추후 확정 |

OpenCode 는 어디서든 띄울 수 있다 (단일 호스트 / 게스트→호스트 / 별도 서버 / 사내 폐쇄망 컨테이너). Agent-Service 가 `OPENCODE_SERVER_URL` 환경변수로 위치를 지정한다. 외부 노출 시 `opencode serve --hostname 0.0.0.0 --port 4096` 으로 띄워야 LAN/게스트에서 도달 가능.

사내망에서는 MongoDB / Gitea 의 호스트명이 `localhost` 가 아니므로 환경변수 (`*_URL`) 에 실제 사내 호스트를 명시한다. cross-service `_URL` 변수 표는 본 문서 아래 참조.

## 전체 포트 표

| 포트 | 컴포넌트 | 종류 | 책임 |
|---|---|---|---|
| 3000 | Platform-App | Frontend (React) | 계정/공지/문서 SPA |
| 3001 | GoJIRA-App | Frontend (React) | 이슈/프로젝트 SPA, 채팅 |
| 3030 | Pumlex | 외부 (Docker) | PlantUML 인라인 편집 + `/render-with-layout` |
| 3300 | Gitea | 외부 인프라 (사내망: 별도 서버 / 로컬: Docker) | Git 서버 (호스트 3300 → 컨테이너 3000) |
| 4096 | OpenCode | 로컬: 사용자 PC / 사내망: WSL 안 | LLM 호출 게이트웨이 (채팅/스킬) |
| 8000 | JIRA Mock | 외부 | JIRA Datacenter 에뮬레이터 |
| 9000 | Platform-API | Backend (Go) | 계정/문서/Webhook/LLM 게이트웨이 |
| 9001 | GoJIRA-API | Backend (Go) | 이슈/프로젝트/Agent 프록시 |
| 9002 | Agent-Service | Backend (Python) | OpenCode 브리지 (SSE 번역) |
| 27017 | MongoDB | DB (사내망: 별도 서버 / 로컬: shared-infra 스택) | 공용 DB (`gojira` database — RAGaaS 와 인스턴스 공유 시 `shared-mongo`, DB 는 분리) |

> 위 표는 **컨테이너 내부 listen 포트**. Docker compose 배포 시 호스트 외부에는 아래의 매핑된 포트로 노출 — 사내 컨벤션이며 firewall/policy 등록 이력 때문에 변경 금지.

### 사내 운영 호스트 매핑 (deploy/docker-compose.yml)

| 컨테이너 포트 | 호스트 매핑 포트 | `.env` 변수 |
|---|---|---|
| 9000 | **44350** | `PLATFORM_API_PORT` (or `PLATFORM_API_HOST_PORT`) |
| 9001 | **44360** | `GOJIRA_API_PORT` (or `GOJIRA_API_HOST_PORT`) |
| 9002 | **44370** | `AGENT_API_PORT` (or `AGENT_API_HOST_PORT`) |
| 3000 | 44300 | `PLATFORM_APP_PORT` (or `PLATFORM_APP_HOST_PORT`) |
| 3001 | 44301 | `GOJIRA_APP_PORT` (or `GOJIRA_APP_HOST_PORT`) |
| 8000 | 8000 | `JIRA_API_HOST_PORT` |
| 4096 | 4096 | OpenCode (host native, 컨테이너 외부) |

### 게이트웨이 단일 진입점 (deploy/docker-compose.yml)

> **변경(리버스 프록시 도입):** deploy compose 에서는 위 백엔드/프론트 컨테이너를
> 호스트에 직접 노출하지 않는다(`expose` 만, 내부 네트워크 전용). 대신 `gateway`
> (nginx) 컨테이너 **하나만** `GATEWAY_PORT` 로 노출하고, 경로 기반으로 프록시한다.
> 방화벽에는 `GATEWAY_PORT` 하나만 등록하면 된다.

| gateway 경로 | 대상 컨테이너 |
|---|---|
| `/` | Platform-App (3000) |
| `/gojira-app/` | GoJIRA-App (3001, Module Federation remote) |
| `/platform-api/` | Platform-API (9000) |
| `/gojira-api/` | GoJIRA-API (9001) |
| `/pumlex/` | pumlex (3030) |

`URL_SCHEME=https` 면 gateway 가 인증서(`TLS_CERT_DIR`)로 TLS 종단하고 모든 URL 이
https/wss 로 조합된다. `http`(기본)면 평문. 백엔드는 항상 컨테이너 내부 HTTP.

스킬 / 외부 클라이언트(런처 주입)가 사용하는 URL 도 gateway 경로 기준:
- `${URL_SCHEME}://<gojira-host>:<GATEWAY_PORT>/platform-api/api/v2` — Platform-API
- `${URL_SCHEME}://<gojira-host>:<GATEWAY_PORT>/gojira-api/api/v2` — GoJIRA-API

자세한 배포 토폴로지: `docs/deployment-topology.md`

### RAGaaS 와 같은 호스트에 배포할 때 (shared-infra, docs/shared-infra-handoff.md)

MongoDB 는 `shared-infra` 스택의 `shared-mongo` 로 일원화 (호스트 27017 은 shared-infra 가
바인딩, GoJIRA 백엔드는 `shared-net` 컨테이너 DNS `shared-mongo:27017` 로 접근 — 호스트
포트 경유 아님). platform-api / gojira-api 가 `shared-net` 에 조인하며, RAGaaS API 는
`http://ragaas-backend:8000` (컨테이너 DNS) 으로 호출한다.

호스트 포트 충돌은 **8000 하나** — GoJIRA JIRA Mock 과 RAGaaS Backend API:

| 호스트 포트 | RAGaaS | GoJIRA | 조정 |
|---|---|---|---|
| 8000 | Backend API | JIRA Mock (`JIRA_API_HOST_PORT`) | 공존 시 GoJIRA 쪽 `.env` 에 `JIRA_API_HOST_PORT=44380` 권장 (44xxx 관례) |
| 3030 | Fuseki | ~~Pumlex~~ | 충돌 없음 — gateway 도입 후 pumlex 는 호스트 비노출(`expose` 전용), RAGaaS 가 3030 사용 가능 |
| 27017 | shared-mongo | shared-mongo | shared-infra 로 일원화되어 해소 |

**shared-infra 호스트 점유 포트 (2차 통합, 2026-07-05)** — GoJIRA 가 새 포트를 쓸 때
아래와 겹치지 않게 shared-infra README 포트표를 먼저 확인:

| 호스트 포트 | shared-infra 서비스 | 비고 |
|---|---|---|
| 27017 | shared-mongo | 공용 (gojira/ragaas DB 분리) |
| 3030 | shared-fuseki | RAGaaS |
| 19530, 9091 | shared-milvus | RAGaaS |
| **9000, 9001** | shared-minio | ⚠️ Windows dev 에서 platform-api(9000)/gojira-api(9001) exe 와 **localhost 포트가 겹침** — WSL2 포트포워딩 때문에 dev exe 미기동 시 localhost:9000/9001 이 MinIO 로 연결돼 혼동 가능(프론트는 S3 XML 403 을 받음). **방침(2026-07-06 확정): GoJIRA 는 9000/9001 을 유지하고, 포트 조정은 RAGaaS(shared-infra MinIO) 쪽에서 수행한다.** 그때까지의 임시 우회: dev exe 를 `PLATFORM_SERVER_PORT=:9100` / `GOJIRA_SERVER_PORT=:9101`(+`GOJIRA_PLATFORM_API=http://localhost:9100/api/v2`) 로 띄우고 두 앱 `public/env.js`(git-ignored) 에 `REACT_APP_PLATFORM_API/GOJIRA_API` 를 9100/9101 로 주입 — 커밋 코드/기본값은 9000/9001 불변 |
| 7474, 7687 | shared-neo4j | RAGaaS |
| 6379 | shared-redis | RAGaaS (GoJIRA 도 사용 가능) |
| (예정) 3300 | shared-gitea | GoJIRA — Mac Gitea 데이터 이관과 동시 활성화 (아래 참조) |

GoJIRA 에 Redis/그래프DB 등 서드파티 인프라가 필요해지면 **앱 compose 에 추가하지 말고**
shared-infra 의 기존 서비스(shared-redis 등)를 공용하거나 shared-infra 에 추가한다
(분리 기준: 서드파티 기성 이미지(상태 보유) = shared-infra / 자체 빌드 앱 = 각 프로젝트).

## 서비스별 의존 매트릭스

| 서비스 | LISTEN | 호출하는 곳 |
|---|---|---|
| Platform-App | 3000 (브라우저) | → Platform-API(9000), GoJIRA-API(9001) |
| GoJIRA-App | 3001 (브라우저) | → GoJIRA-API(9001), Agent-Service(9002), Pumlex(3030) |
| Platform-API | 9000 | → MongoDB(27017), Gitea(3300), OpenAI/외부 LLM |
| GoJIRA-API | 9001 | → MongoDB(27017), Platform-API(9000) /LLM/call /DocRepo, JIRA Mock(8000) |
| Agent-Service | 9002 | → MongoDB(27017), OpenCode(4096), Platform-API(9000), GoJIRA-API(9001) |
| OpenCode | 4096 | → 외부 LLM 게이트웨이 (사용자 PC 정책) |

## 호출 흐름 요약

```
[Browser]
  3000 ─→ Platform-App SPA
  3001 ─→ GoJIRA-App SPA
           │
           │ HTTP/SSE
           ▼
[Backend services]
  9000 Platform-API  ──→ MongoDB(27017), Gitea(3300), 외부 LLM
  9001 GoJIRA-API    ──→ Platform-API(9000), JIRA(8000), MongoDB
  9002 Agent-Service ──→ OpenCode(4096), MongoDB

[Local agent runtime]
  4096 OpenCode 서버 (사용자 PC)
```

## 환경변수와 매핑

각 서비스의 포트는 환경변수로 override 가능 (선택 항목, default 적용).

| 서비스 | 변수 | default |
|---|---|---|
| Platform-API | `PLATFORM_SERVER_PORT` | `:9000` |
| GoJIRA-API | `GOJIRA_SERVER_PORT` | `:9001` |
| Agent-Service | `AGENT_SERVICE_PORT` | `9002` |
| Platform-App / GoJIRA-App | `PORT` (CRA) | 3000 / 3001 |
| OpenCode | (사용자 OpenCode 정책) | 4096 |

cross-service URL 변수 (필수, 환경별로 명시):

| 변수 | 의미 |
|---|---|
| `GOJIRA_PLATFORM_API` | GoJIRA-API → Platform-API 베이스 URL |
| `GOJIRA_API_URL` | Platform-API → GoJIRA-API 베이스 URL |
| `GOJIRA_API_URL` | Agent-Service → GoJIRA-API 베이스 URL |
| `OPENCODE_SERVER_URL` | Agent-Service → OpenCode |
| `GITHUB_URL` | Platform-API → Gitea / GitHub |

## 새 환경에서 셋업 시 점검 순서

1. **인프라 가동 확인**: MongoDB(27017), Gitea(3300), OpenCode(4096), Pumlex(3030)
2. **Backend 기동**: Platform-API → GoJIRA-API → Agent-Service (의존 순)
3. **Frontend 기동**: Platform-App / GoJIRA-App
4. **각 서비스 startup 로그**에서 자기 포트 + peer 포트 출력 확인
5. 누락된 환경변수가 있으면 startup 시 fail-fast (log.Fatal / ValidationError)
