# GoJIRA 포트 매핑

> 본 시스템에서 사용하는 모든 포트의 단일 출처.
> 각 서비스는 startup 시 자기 포트 + 의존 peer 포트를 로그로 출력한다.

## 환경별 호스팅 위치

| 환경 | MongoDB / Gitea | OpenCode | Backend / Frontend |
|---|---|---|---|
| **로컬 개발 (Mac)** | Docker — 같은 호스트 | 같은 호스트 | 같은 호스트 |
| **사내망 (Windows + WSL)** | **별도 사내 서버** | **WSL 안 (사용자 PC)** | WSL 안 (사용자 PC) — 가정, 추후 확정 |

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
| 27017 | MongoDB | DB (사내망: 별도 서버 / 로컬: Docker) | 공용 DB (`gojira` database) |

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
| `PLATFORM_GOJIRA_API` | Platform-API → GoJIRA-API 베이스 URL |
| `GOJIRA_API_URL` | Agent-Service → GoJIRA-API 베이스 URL |
| `OPENCODE_SERVER_URL` | Agent-Service → OpenCode |
| `GITHUB_URL` | Platform-API → Gitea / GitHub |

## 새 환경에서 셋업 시 점검 순서

1. **인프라 가동 확인**: MongoDB(27017), Gitea(3300), OpenCode(4096), Pumlex(3030)
2. **Backend 기동**: Platform-API → GoJIRA-API → Agent-Service (의존 순)
3. **Frontend 기동**: Platform-App / GoJIRA-App
4. **각 서비스 startup 로그**에서 자기 포트 + peer 포트 출력 확인
5. 누락된 환경변수가 있으면 startup 시 fail-fast (log.Fatal / ValidationError)
