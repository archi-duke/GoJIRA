# GoJIRA

JIRA Datacenter 연동 프로젝트/이슈 관리 **플랫폼**.
이슈/프로젝트 관리 + 계정·공지·문서 + AI 에이전트(OpenCode/Claude) 기반 설계 자동화 + JIRA 양방향 동기화를 제공하며, 사내 폐쇄망 Docker 배포를 지원한다.

## 구성

```
GoJIRA/
├── Platform-API/      # 계정·조직·공지·알림·Git(Gitea)·LLM 게이트웨이·런처 배포 (Go/Chi + MongoDB) :9000
├── GoJIRA-API/        # 이슈·프로젝트·링크·코멘트·JIRA 동기화·문서생성·관계분석 (Go/Chi + MongoDB) :9001
├── JIRA-API/          # JIRA Datacenter REST v2 Mock 서버 (Python/FastAPI) :8000
├── Platform-App/      # 계정·공지·문서 SPA (React) :3000
├── GoJIRA-App/        # 이슈·프로젝트·간트·채팅 SPA (React, Module Federation remote) :3001
├── gojira-launcher/   # 웹→로컬 소환 경량 데몬 (Go 단일 exe, localhost:5599)
│                      #   + 스킬/opencode 자산 embed 배포 + ClickOnce 식 자동 업데이트
├── gojira-shared/     # Go 워크스페이스 공용 패키지 (secret/crypto 등, module gojira.local/shared)
├── gojira-cli/        # Go CLI — 프로젝트 등 관리 보조 도구
├── deploy/            # 폐쇄망 Docker 배포 (docker-compose + 빌드/저장/적재 스크립트 + 자체빌드 캐시)
├── docs/              # 포트 매핑(ports.md)·배포 토폴로지·아키텍처/스키마 문서
├── specs/             # 이슈/프로젝트/캐시 스키마 정의 (JSON/MD)
├── e2e/               # E2E 테스트 케이스/테스트
├── issues/            # 샘플 JIRA 이슈 데이터
├── .claude/           # 에이전트 스킬 (.claude/skills) 등 Claude 자산
└── .opencode/         # OpenCode 에이전트 페르소나(.opencode/agents) + opencode.json
```

## 아키텍처 / 포트

서비스 토폴로지와 포트의 단일 출처는 **`docs/ports.md`** 다. 요약:

| 포트 | 컴포넌트 | 종류 | 책임 |
|---|---|---|---|
| 3000 | Platform-App | Frontend (React) | 계정/공지/문서 SPA |
| 3001 | GoJIRA-App | Frontend (React) | 이슈/프로젝트/간트/채팅 SPA |
| 3030 | Pumlex | 외부 (Docker) | PlantUML 인라인 편집 + `/render-with-layout` |
| 3300 | Gitea | 외부 인프라 | Git 서버 (DocRepo / 프로젝트 repo) |
| 4096 | OpenCode | 사용자 PC / 별도 호스트 | LLM 호출 게이트웨이 (채팅/스킬) |
| 5599 | gojira-launcher | 사용자 PC (localhost) | 웹→로컬 에이전트 소환 데몬 |
| 8000 | JIRA Mock | 외부 | JIRA Datacenter 에뮬레이터 |
| 9000 | Platform-API | Backend (Go) | 계정/문서/Webhook/LLM 게이트웨이/런처 배포 |
| 9001 | GoJIRA-API | Backend (Go) | 이슈/프로젝트/JIRA 동기화 |
| 27017 | MongoDB | DB | 공용 DB (`gojira`) |

> 호출 흐름: 브라우저(3000/3001) → Backend(9000/9001) → MongoDB(27017)·Gitea(3300)·JIRA Mock(8000)·외부 LLM. 웹의 "로컬 수정" 버튼은 사용자 PC 의 `gojira-launcher`(5599)를 통해 VS Code + OpenCode/Claude 를 소환한다.
> 사내 폐쇄망 배포 시 호스트 외부 노출 포트는 별도 매핑(예: 9000→44350, 9001→44360, 3000→44300, 3001→44301). `docs/ports.md` 참조.
>
> ※ `docs/ports.md` 에 기재된 Agent-Service(9002, Python OpenCode 브리지)는 **로컬 런처 + OpenCode 직접 소환 모델로 전환된 레거시**로, 현재 `deploy/docker-compose.yml` 스택에는 포함되지 않는다.

## Platform-API (Go) — :9000

계정/조직/권한, 공지·알림(SSE/WS), Gitea 사용자 연동, 프로젝트 아카이브, LLM 게이트웨이, 런처 바이너리 배포·버전 매니페스트를 담당.

| Method | Endpoint | 설명 |
|--------|----------|------|
| POST | `/webhook/github` | Gitea/GitHub 웹훅 수신 |
| GET/POST/PUT/DELETE | `/api/v{v}/Account/users[...]` | 사용자 CRUD + 권한 |
| GET/POST/PUT/DELETE | `/api/v{v}/Account/authGroups`·`/orgUnits`·`/connections` | 권한그룹/조직/연동 |
| GET/POST/PUT/DELETE | `/api/v{v}/Notice[...]` · `/Notification/*` | 공지·알림 (SSE `/Notice/stream`, WS `/Notice/ws`) |
| GET | `/api/v{v}/Git/users` | Gitea 사용자 조회 |
| POST | `/api/v{v}/Project/{code}/archive` | 프로젝트 아카이브 |
| POST | `/api/v{v}/LLM/call` | LLM 호출 게이트웨이 |
| GET | `/api/v{v}/launcher/manifest`·`/launcher/version`·`/launcher/download/{os}` | 런처 배포/자동업데이트 |

### 실행
```bash
cd Platform-API && go run .        # 기본 :9000 (PLATFORM_SERVER_PORT 로 override)
```

## GoJIRA-API (Go) — :9001

이슈/프로젝트 도메인. 이슈를 MongoDB 에 캐싱하고 변경분만 JIRA 에 동기화하며, 이슈 YAML ingest, 문서 생성, 관계 분석을 제공.

| Method | Endpoint | 설명 |
|--------|----------|------|
| GET | `/api/v{v}/Check` | 서버 상태 확인 |
| POST/GET/PUT/DELETE | `/api/v{v}/Issue` | 이슈 CRUD |
| POST/DELETE | `/api/v{v}/Issue/ingest` · `/Project/{code}/issues/ingest` | 이슈 YAML → Mongo ingest |
| POST/DELETE | `/api/v{v}/Issue/label`·`/Issue/link`·`/Issue/comment` | 라벨/링크/코멘트 |
| GET | `/api/v{v}/Issue/fetch` | JIRA → MongoDB Pull |
| POST | `/api/v{v}/Issue/docgen` | 이슈 기반 문서 생성 |
| PUT | `/api/v{v}/Issue/field`·`/Issue/relations` | 필드 단건 수정 / 관계 분석 |
| POST/GET/PUT | `/api/v{v}/Project` | 프로젝트 CRUD (Gitea repo 생성 포함) |
| POST/GET | `/api/v{v}/Project/{code}/reset`·`/dump`·`/usecase-artifacts`·`/alerts` | 리셋/덤프/산출물/알림 |
| POST | `/api/v{v}/Jira/Account` | JIRA 계정 등록 |

### 실행
```bash
cd GoJIRA-API && go run .          # 기본 :9001 (GOJIRA_SERVER_PORT 로 override)
```

> Platform-API·GoJIRA-API·gojira-shared 는 Go 워크스페이스(`go.work`) 멤버다. `gojira-launcher`·`gojira-cli` 는 독립 모듈.

## JIRA-API (Mock 서버) — :8000

JIRA Datacenter REST API v2 를 모사하는 FastAPI 서버. `issues/` 의 JSON 을 DB 로 사용한다.

| Method | Endpoint | 설명 |
|--------|----------|------|
| GET/POST | `/rest/api/2/issue` | 이슈 목록/생성 |
| GET/PUT/DELETE | `/rest/api/2/issue/{key}` | 이슈 단건 조회/수정/삭제 |
| GET | `/rest/api/2/search` | JQL 검색 |
| POST/DELETE | `/rest/api/2/issueLink[/{id}]` | 이슈 링크 생성/삭제 |

### 실행
```bash
cd JIRA-API
pip install -r requirements.txt
python -m uvicorn main:app --host 0.0.0.0 --port 8000 --reload
```

## 프론트엔드 (React)

- **Platform-App** (:3000) — 계정/공지/문서 SPA. Module Federation **host**(GoJIRA-App 를 remote 로 소비).
- **GoJIRA-App** (:3001) — 이슈/프로젝트 SPA. 간트차트(ProjectMap), 이슈 Relation Map, SATAM, AI 채팅, PlantUML 인라인 편집(PexBlock)을 제공. Module Federation **remote**(`remoteEntry.js` 노출).

런타임 설정(`REACT_APP_*`)은 빌드가 아니라 컨테이너 startup 시 `env.js` 로 주입되므로 호스트/포트 변경 시 재빌드가 불필요하다.

### 실행
```bash
cd GoJIRA-App && npm install && npm start    # :3001
cd Platform-App && npm install && npm start  # :3000
```

## gojira-launcher (로컬 에이전트 소환)

웹의 "로컬 수정" 요청을 사용자 PC 에서 처리하는 경량 localhost 데몬(`127.0.0.1:5599`). 단일 exe 안에 에이전트 스킬(`.claude/skills`)·OpenCode 자산(`opencode.json`, `.opencode/agents`)을 embed 해 설치 시 배포하고, 기동 시 + 주기적으로 버전을 확인해 자기 자신을 자동 업데이트한다(ClickOnce 식).

```bash
gojira-launcher install     # 설치 + 스킬 배포 + 자동시작 등록 + 데몬 기동
```
설정: `~/.gojira/launcher.json` (백엔드 URL·Gitea·repoRoot·자동업데이트). 자세한 내용은 `gojira-launcher/README.md`.

## 폐쇄망 배포 (deploy/)

MongoDB·Gitea 는 사내 별도 인프라를 사용하고, 나머지 서비스는 `deploy/docker-compose.yml` 로 기동한다. 모든 환경값은 `deploy/.env` 한 곳에서 주입한다.

```bash
# 외부망: 이미지 빌드 → 단일 tar (기본 linux/amd64 강제)
bash deploy/scripts/build-images.sh
bash deploy/scripts/save-images.sh           # → deploy/gojira-images.tar

# 사내망: 적재 → 점검 → 기동
docker load -i deploy/gojira-images.tar
bash deploy/scripts/preflight.sh
docker compose -f deploy/docker-compose.yml up -d
```

코드 한 줄 수정마다 전체 이미지를 옮기는 대신, 의존성 캐시 이미지(`gojira/build-cache-go`·`build-cache-node`)만 한 번 반입하고 사내망에서 직접 `docker compose build` 하는 **자체 빌드 모델**도 지원한다 — `deploy/CLOSED-NETWORK-BUILD.md`. 런처 빌드는 `deploy/scripts/build-launcher.sh`.

## 로컬 개발 전체 실행 순서

```bash
# 1. 인프라 — MongoDB / Gitea (로컬은 Docker 권장)
# 2. Backend (의존 순서)
cd Platform-API && go run . &
cd GoJIRA-API   && go run . &
cd JIRA-API     && python -m uvicorn main:app --port 8000 &
# 3. Frontend
cd Platform-App && npm start &
cd GoJIRA-App   && npm start
# 4. (선택) AI 에이전트 — 사용자 PC 에서 OpenCode(4096) + gojira-launcher install
```
각 백엔드는 startup 시 자기 포트와 의존 peer 포트를 로그로 출력한다. 누락 환경변수는 fail-fast.

## 기술 스택

| 구성 | 기술 |
|------|------|
| Backend | Go (Chi Router), Go workspace(`go.work`), MongoDB Driver v2 |
| JIRA Mock | Python, FastAPI |
| Frontend | React, Module Federation, SVG |
| DB | MongoDB |
| Git/문서 저장 | Gitea (DocRepo + 프로젝트 repo) |
| AI 에이전트 | OpenCode / Claude, 스킬(`.claude/skills`), 로컬 런처(Go 단일 exe) |
| 다이어그램 | PlantUML + Pumlex(layout) |
| 배포 | Docker / docker-compose, 폐쇄망 자체 빌드(build-cache) |
```
