# GoJIRA 사내 폐쇄망 배포 패키지

GoJIRA 의 6개 서비스 + PlantUML 렌더링 서버를 단일 `docker-compose` 로 구동한다. MongoDB / Gitea 는 사내 별도 인프라 사용 가정.

## 패키지 구성

```
deploy/
├── docker-compose.yml          # 모든 컨테이너 정의
├── .env.example                # 설정 템플릿 (→ .env 로 복사 후 채움)
├── README.md                   # 본 문서
├── DEPLOY.md                   # 운영자용 step-by-step 절차
├── config/
│   └── llm.json                # LLM 게이트웨이 정의 (외부망 default 그대로. 사내 swap 가능)
├── scripts/                    # 폐쇄망 배포 자동화 스크립트
│   ├── images.txt              # 베이스 + 서비스 이미지 단일 출처 (모든 스크립트가 참조)
│   ├── build-images.sh         # 외부망: pull baseline + docker compose build
│   ├── save-images.sh          # 외부망: 전체 이미지 → gojira-images.tar
│   ├── load-images.sh          # 폐쇄망: tar → docker, 적재 검증
│   └── preflight.sh            # 폐쇄망: .env/권한/TCP/compose/이미지 사전 검증
└── images/
    ├── platform-api/Dockerfile      # multi-stage: go 1.26 → alpine + git
    ├── gojira-api/Dockerfile        # multi-stage: go 1.26 → alpine
    ├── jira-api/Dockerfile          # python:3.12-slim + wheelhouse
    ├── platform-app/Dockerfile      # node:lts build → nginx:alpine
    │   └── nginx.conf
    ├── gojira-app/Dockerfile        # 동일
    │   └── nginx.conf
    └── pumlex-server/Dockerfile     # node:lts-alpine (Express PlantUML 렌더러)

vendor/
└── pumlex/                          # pumlex 서브셋 vendoring (pex-core + pex-server)
```

> **pumlex-server**: GoJIRA-App 의 인라인 PlantUML 편집(PexBlock)이 런타임에 호출하는
> 렌더러. 소스는 외부 repo (`pumlex`) 의존을 없애려고 `deploy/vendor/pumlex/` 로
> vendoring 했다. 내부적으로 `plantuml-server:8080` 을 호출하므로 PlantUML 컨테이너에 의존.

## 폐쇄망 배포 단축 명령 (TL;DR)

```bash
# ── 외부망 ──
cd deploy && cp .env.example .env && vi .env   # atomic 입력 채우기
./scripts/build-images.sh                       # pull base + compose build
./scripts/save-images.sh                        # → gojira-images.tar

# ── 폐쇄망 (deploy/ 와 tar 반입 후) ──
cd deploy
./scripts/load-images.sh                        # docker load + 검증
./scripts/preflight.sh                          # .env/권한/TCP/compose/이미지 검증
docker compose up -d
```

## 한 곳에서 설정 관리 (atomic 입력)

**모든 환경변수가 `deploy/.env` 한 파일에 모이고, atomic 값만 사용자가 입력한다.**

- 외부 인프라 (Mongo, Gitea) → `MONGODB_SERVER` / `GITEA_SERVER` 같은 `host:port` 한 줄
- 나머지 서비스 (OpenCode, GoJIRA 6 서비스) → `*_SERVER` + `*_PORT` 두 줄 분리
- 조합된 URL (`MONGO_URI`, `GOJIRA_API_URL`, `REACT_APP_*` 등) 은 **compose 가 자동 생성**
- 사용자가 한 서버 / port 변경 시 한 줄만 수정 → 모든 사용처에 자동 반영
- 각 서비스의 개별 `.env` 파일은 **사용하지 않는다**

## 외부망 빌드 → 폐쇄망 반입 흐름

### 1. 외부망 — 빌드 + 이미지 추출

```bash
cd deploy
cp .env.example .env
# .env 의 atomic 입력 (host/port/user/pass/token) 만 사내 값으로 채움.
# 조합된 URL (MONGO_URI / GOJIRA_API_URL / REACT_APP_*) 은 자동 생성.
#   - MONGODB_SERVER, MONGO_USER, MONGO_PASS  (host:port 한 줄 + 인증)
#   - GITEA_SERVER, PLATFORM_GITHUB_TOKEN, PLATFORM_GITHUB_ORG
#   - *_SERVER + *_PORT (사내 GoJIRA 서비스 6종 — 브라우저 access endpoint)
#   - 시크릿 (선택): GOJIRA_ENC_KEY / OPENAI_API_KEY / SMTP_* / JIRA_USER 등

# 베이스 이미지 미리 받기 (폐쇄망에선 pull 불가)
docker pull plantuml/plantuml-server:jetty
docker pull nginx:alpine
docker pull python:3.12-slim
docker pull alpine:3.20
docker pull golang:1.26-alpine
docker pull node:lts-alpine

# 서비스 이미지 빌드
docker compose build

# 모든 이미지 (베이스 + 빌드 결과) 를 tar 로 추출
docker save \
  plantuml/plantuml-server:jetty \
  gojira/platform-api:latest \
  gojira/gojira-api:latest \
  gojira/jira-api:latest \
  gojira/platform-app:latest \
  gojira/gojira-app:latest \
  gojira/pumlex-server:latest \
  -o gojira-images.tar
```

### 2. 폐쇄망 반입

```bash
# tar + deploy/ 디렉토리 통째로 사내망 서버로 복사
# (USB / approved file transfer)

# 사내망 서버에서
docker load -i gojira-images.tar
docker images | grep gojira     # 6개 이미지 확인
```

### 3. 폐쇄망 — 기동

```bash
cd deploy
# .env 가 이미 외부망에서 채워진 상태로 함께 반입됨.
# 사내 host 가 외부망 빌드 시점과 다를 경우 다음 작업 필요:
#   - REACT_APP_* 가 다르면 → 사내에서 docker compose build platform-app gojira-app
#   - 그 외 변수만 다르면 → .env 수정 후 docker compose up -d (재빌드 불요)
docker compose up -d

# 헬스 체크
curl -sS http://localhost:9000/                    # Platform-API
curl -sS http://localhost:9001/                    # GoJIRA-API
curl -sS http://localhost:8000/rest/api/2/serverInfo  # JIRA Mock
curl -sS http://localhost:3000/                    # Platform-App
curl -sS http://localhost:3001/                    # GoJIRA-App
curl -sS http://localhost:3030/                    # pumlex (plantumlEx 렌더러)
```

## React 런타임 config (재빌드 불요)

`REACT_APP_*` 는 nginx 컨테이너 entrypoint 가 컨테이너 startup 시 environment → `/usr/share/nginx/html/env.js` (`window._env_`) 로 직렬화한다. React (`src/config.js`) 가 1순위로 이를 읽고, 없으면 `import.meta.env.REACT_APP_*` (Vite dev .env) 로 fallback.

- 빌드된 이미지는 host-agnostic — 어떤 사내망에도 그대로 반입 가능
- 폐쇄망에서 host/도메인 변경 → `.env` 수정 → `docker compose up -d` (재빌드 불요)
- Module Federation remote URL 도 런타임 결정 — Platform-App 부트스트랩이 `window._env_.REACT_APP_GOJIRA_APP_REMOTE` 로 `registerRemotes` (MF 2.0). 동일하게 재빌드 불요
- (Vite 이전 2026-07-06) 프론트 빌드는 Vite 6 — package-lock 이 바뀌었으므로 **build-cache-node 이미지 재생성 필요** (deploy/images/build-cache-node)

## LLM provider 변경

`config/llm.json` 이 volume mount 로 Platform-API 에 주입된다. 외부망에선 OpenAI 가 active. 사내망에서:

1. `deploy/config/llm.json` 편집 — 현재 active 구성을 `_disabled_OpenAI` 로 이름 변경
2. `_disabled_Internal` 의 내용을 root level 로 끌어올림
3. `.env` 에 `PLATFORM_LLM_TICKET`, `PLATFORM_LLM_USER_ID` 설정
4. `docker compose restart platform-api`

## 트러블슈팅

| 증상 | 원인 / 대처 |
|---|---|
| `docker compose up` 시 `required` 메시지 | `.env` 에 해당 변수 누락 — `?required` 마크된 항목 다시 확인 |
| 백엔드 startup 시 `MongoDB connect: ...` | `MONGO_URI` 의 host/auth 확인. URL encoding (`@` → `%40`) |
| Frontend 가 `localhost:9000` 호출 | env.js 미생성 / 빈 값. `docker compose exec platform-app cat /usr/share/nginx/html/env.js` 확인, `.env` 보완 후 `docker compose up -d platform-app gojira-app` |
| Module Federation `remoteEntry.js` 실패 | `REACT_APP_GOJIRA_APP_REMOTE` 가 브라우저에서 접근 가능한 URL 인지 확인 |
| PlantUML 블록이 `렌더링 오류` / `fetch failed` | pumlex-server 가 plantuml-server 도달 못함. `docker compose logs pumlex-server` 확인. compose 내부 `PLANTUML_URL=http://plantuml-server:8080` 고정이므로 보통 plantuml 컨테이너 미기동이 원인 |
| PlantUML 블록이 빈 화면 / CORS | 브라우저가 `REACT_APP_PUMLEX_SERVER` (`PUMLEX_SERVER:PUMLEX_PORT`) 에 도달 가능한지 확인. env.js 값 점검: `docker compose exec gojira-app cat /usr/share/nginx/html/env.js` |
| LLM 호출 시 `unauthorized` | `OPENAI_API_KEY` (외부) 또는 `PLATFORM_LLM_TICKET` (사내) 확인. `GOJIRA_ENC_KEY` 가 ENC: 값들을 암호화한 키와 동일한지 |

## 관련 문서

- `deploy/DEPLOY.md` — **운영자용 step-by-step 배포 절차** (외부망 빌드 → 폐쇄망 기동 → 검증 → 운영)
- `docs/env_info.md` — 각 서비스의 환경변수 카탈로그 (개발/운영 공통)
- `docs/ports.md` — 포트 매핑 표
- `docs/architecture/deployment.md` — 배포 토폴로지
