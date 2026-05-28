# GoJIRA 사내 폐쇄망 배포 절차

운영자용 step-by-step 가이드. 패키지 구성/설계 원칙은 `README.md` 참조.

## 전체 흐름

```
[외부망 (Ubuntu/macOS)]            [전송]            [폐쇄망 (사내 서버)]
   ① 사전 준비                                         ⑤ 이미지 load
   ② .env 작성                                         ⑥ 기동
   ③ docker compose build                              ⑦ 검증
   ④ docker save → tar      ──── USB / 승인 채널 ──→   ⑧ 운영
```

전제: MongoDB / Gitea / OpenCode 는 사내 별도 인프라로 이미 운영 중.

---

## ① 외부망 — 사전 준비

### 1-1. 환경 요구사항

| 항목 | 버전 / 비고 |
|---|---|
| Docker | 24+ (compose v2 plugin 포함) |
| 디스크 여유 | ~5GB (빌드 + 이미지 tar) |
| 네트워크 | Docker Hub + npm registry + Go modules 접근 가능 |

### 1-2. 레포 클론 + 배포 디렉토리 진입

```bash
git clone https://github.com/archi-duke/GoJIRA.git
cd GoJIRA/deploy
```

---

## ② 외부망 — `.env` 작성

```bash
cp .env.example .env
```

`?required` 마크가 붙은 변수는 반드시 채워야 `docker compose` 가 fail-fast 하지 않는다.

### 2-1. 채워야 할 필수 값

**atomic 입력만 작성한다 — 조합된 URL (MONGO_URI / GOJIRA_API_URL / REACT_APP_* 등) 은 compose 가 자동 생성.**

| 카테고리 | 변수 | 출처 / 결정 주체 |
|---|---|---|
| MongoDB | `MONGODB_SERVER` | 사내 MongoDB host:port (한 줄). 3 백엔드 공용 |
| | `MONGO_USER` / `MONGO_PASS` | 인증. 특수문자 URL encoding (`@` → `%40`) 불요 — compose 가 raw 로 조합 후 코드가 처리 |
| | `MONGO_AUTH_SOURCE` | default `admin` |
| Gitea | `GITEA_SERVER` | 사내 Gitea host:port |
| | `PLATFORM_GITHUB_TOKEN` | Gitea PAT (scope: `repo` + `admin:org`) |
| | `PLATFORM_GITHUB_ORG` | Gitea organization 이름 |
| 호스트 파일시스템 | `GOJIRA_REPOS_DIR` | Platform-API 의 bare clone + worktree 저장 경로 (호스트 측). 폐쇄망은 절대경로 권장 — 예: `/var/lib/gojira/repos` |
| | `UID` / `GID` | 컨테이너가 호스트 사용자 권한으로 실행되도록 매핑. 보통 `id -u` / `id -g` 값. bind mount fs 권한 일치 목적 |
| GoJIRA 서비스<br>(사용자 access) | `*_SERVER` + `*_PORT` 쌍 6종 | 브라우저가 접근하는 host + port (Platform/GoJIRA API + Platform/GoJIRA App + PlantUML + pumlex). React 에 **런타임** 주입 (env.js) — 재빌드 불요 |
| 컨테이너 노출 port<br>(선택) | `*_HOST_PORT` | nginx reverse proxy 환경에서 컨테이너 외부 노출 port 가 사용자 access port 와 다를 때만 |

### 2-2. 선택 값

| 변수 | 사용 시점 |
|---|---|
| `GOJIRA_ENC_KEY` | 기존 환경의 `ENC:` 값들을 그대로 재사용할 때 — 동일 키 필수 |
| `OPENAI_API_KEY` / `PLATFORM_LLM_TICKET` | LLM provider 에 따라 (외부=OpenAI / 사내=Internal) |
| `SMTP_*` | 메일 알림 사용 시 |
| `GOJIRA_JIRA_USER` / `GOJIRA_JIRA_PASS` | 실제 JIRA 연동 시 (mock 사용하면 불요) |

### 2-3. React 런타임 config (재빌드 불요)

`REACT_APP_*` 는 더 이상 빌드 시점에 inline 되지 않는다. nginx 컨테이너 startup 시 entrypoint 가 environment → `/env.js` (`window._env_`) 로 직렬화하고 React 의 `src/config.js` 가 1순위로 읽는다. 따라서:

- 외부망에서 한 번 빌드된 이미지는 host-agnostic — 어떤 사내망에도 반입 가능
- 폐쇄망에서 host/port 변경 시 `.env` 수정 → `docker compose up -d` 만으로 즉시 반영
- Module Federation remote URL (`REACT_APP_GOJIRA_APP_REMOTE`) 도 Promise-based dynamic remote 로 runtime 결정 — 동일하게 재빌드 불요

---

## ③ 외부망 — 빌드

> **권장:** 아래 한 줄로 ③ 전체를 수행. 베이스 이미지 목록은 `deploy/scripts/images.txt` 가 단일 출처.
>
> ```bash
> ./scripts/build-images.sh
> ```
>
> 옵션: `--no-pull` (베이스 이미지 pull 생략), `--service platform-app gojira-app` (React 만 재빌드).

### 3-1. (수동) 베이스 이미지 미리 받기

폐쇄망에선 Docker Hub 접근 불가이므로 베이스 이미지도 함께 반입해야 한다.

```bash
# scripts/images.txt 의 베이스 이미지 라인을 한 줄씩 pull
xargs -n1 docker pull < <(grep -v '^gojira/\|^#\|^$' scripts/images.txt)
```

### 3-2. (수동) 서비스 이미지 빌드

```bash
docker compose build
```

첫 빌드는 ~10분 (npm install 이 가장 오래). 재빌드는 캐시로 빠름.

### 3-3. 결과 확인

```bash
docker images | grep -E "gojira|plantuml"
# gojira/platform-api    latest
# gojira/gojira-api      latest
# gojira/jira-api        latest
# gojira/platform-app    latest
# gojira/gojira-app      latest
# gojira/pumlex-server   latest
# plantuml/plantuml-server  jetty
```

---

## ④ 외부망 — 패키징

> **권장:** 한 줄로 모든 이미지를 tar 로 추출 (누락 이미지 사전 감지 포함).
>
> ```bash
> ./scripts/save-images.sh                 # → deploy/gojira-images.tar
> IMAGE_TAG=v1.2.3 ./scripts/save-images.sh /tmp/out.tar   # 태그/경로 지정
> ```

(수동 등가 명령)

```bash
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

폐쇄망으로 옮길 파일:

| 파일 | 비고 |
|---|---|
| `gojira-images.tar` | ~440MB (layer 공유로 압축됨, 원본 합계 ~1.9GB) |
| `deploy/.env` | 시크릿 포함 — **보안 채널** |
| `deploy/docker-compose.yml` | |
| `deploy/config/llm.json` | volume mount 대상 |
| `deploy/DEPLOY.md` / `deploy/README.md` | 운영자 가이드 |

압축 (선택):

```bash
cd ..
tar czf gojira-deploy-$(date +%F).tgz gojira-images.tar deploy/
```

→ USB / 사내 승인 file transfer 채널로 반입.

---

## ⑤ 폐쇄망 — 이미지 load

> **권장:** `images.txt` 의 전체 이미지가 정말 적재됐는지 검증까지 한 번에.
>
> ```bash
> tar xzf gojira-deploy-YYYY-MM-DD.tgz && cd deploy
> ./scripts/load-images.sh                   # ../gojira-images.tar 자동
> ./scripts/load-images.sh /path/to/x.tar    # 경로 지정
> ```

(수동 등가)

```bash
docker load -i ../gojira-images.tar
docker images | grep -E "gojira|plantuml"   # 7개 확인
```

---

## ⑥ 폐쇄망 — 기동

### 6-0. 호스트 측 repos 디렉토리 준비

`.env` 의 `GOJIRA_REPOS_DIR` 경로를 호스트에 미리 만들고 `UID`/`GID` 사용자가 소유하도록 설정:

```bash
source .env
mkdir -p ${GOJIRA_REPOS_DIR}
sudo chown -R ${UID}:${GID} ${GOJIRA_REPOS_DIR}
```

> bind mount 는 호스트 권한이 그대로 컨테이너에 보임. 컨테이너 user (= `${UID}:${GID}`) 가 write 못 하면 Platform-API 의 `git clone` / worktree 생성이 실패한다.

### 6-1. `.env` 사후 점검

> **권장:** required 변수 / 디렉토리 권한 / TCP 도달 / compose config / 이미지 적재까지 단일 명령으로.
>
> ```bash
> ./scripts/preflight.sh                     # 전체
> ./scripts/preflight.sh --skip-net          # 네트워크 검사 생략
> ./scripts/preflight.sh --skip-images       # 이미지 검사 생략 (build 전)
> ```
>
> 실패 항목은 비-0 종료 코드와 함께 조치 명령을 함께 출력.

(수동 등가)

```bash
# compose 가 조합한 최종 URL 미리보기 (시크릿도 함께 출력되니 주의)
docker compose config | grep -E "MONGO_URI|GITHUB_URL|REACT_APP_" | sort -u

# 사내 인프라 도달 확인 (.env 의 값으로 직접 호출)
source .env
nc -zv ${MONGODB_SERVER%%:*} ${MONGODB_SERVER##*:}      # Mongo TCP 도달
curl -sS http://${GITEA_SERVER}/                        # Gitea 응답
```

### 6-2. 기동

```bash
docker compose up -d
docker compose ps
```

7개 컨테이너가 `Up` 상태여야 한다.

### 6-3. (재빌드 불요)

이전 버전과 달리 React 도 런타임 config 로 전환됐다. `.env` 의 어떤 값을 바꾸든 (`*_SERVER`, `*_PORT`, Mongo, Gitea, OpenCode, 시크릿 등) `docker compose up -d` 만으로 반영된다.

```bash
# .env 수정 후
docker compose up -d
```

> 단, 코드 변경 (Go/Python/React 소스) 이 있는 경우만 외부망에서 재빌드 → tar 반입 절차가 필요.

---

## ⑦ 폐쇄망 — 검증

### 7-1. 헬스 체크 (서버 내부)

`.env` 의 `*_PORT` (또는 `*_HOST_PORT` 가 설정됐으면 그 값) 으로 호출. 아래는 `.env` 의 값을 읽어 자동 호출:

```bash
source .env
curl -sS http://localhost:${PLATFORM_API_HOST_PORT:-${PLATFORM_API_PORT}}/                  # Platform-API → 404 OK
curl -sS http://localhost:${GOJIRA_API_HOST_PORT:-${GOJIRA_API_PORT}}/                      # GoJIRA-API   → 404 OK
curl -sS http://localhost:${JIRA_API_HOST_PORT:-8000}/rest/api/2/serverInfo                 # JIRA Mock    → JSON
curl -sS http://localhost:${PLATFORM_APP_HOST_PORT:-${PLATFORM_APP_PORT}}/                  # Platform-App → HTML
curl -sS http://localhost:${GOJIRA_APP_HOST_PORT:-${GOJIRA_APP_PORT}}/                      # GoJIRA-App   → HTML
curl -sS http://localhost:${PUMLEX_HOST_PORT:-${PUMLEX_PORT}}/                              # pumlex       → HTML
```

### 7-2. startup 로그 확인

```bash
docker compose logs platform-api  | head -30
docker compose logs gojira-api    | head -30
```

각 서비스 startup 로그에서 `MongoDB connected`, `Gitea : <url>` 가 정상 출력되면 OK.

### 7-3. 브라우저 통합 확인

```
http://${PLATFORM_APP_SERVER}:${PLATFORM_APP_PORT}   # Platform-App (계정/공지/문서)
http://${GOJIRA_APP_SERVER}:${GOJIRA_APP_PORT}       # GoJIRA-App (이슈/프로젝트/채팅)
```
(.env 에 채운 값이 그대로 사용자 access URL)

체크리스트:
- [ ] Platform-App 로그인 / 프로젝트 목록 표시
- [ ] GoJIRA-App 이슈 목록 / 다이어그램 (PlantUML) 렌더링
- [ ] Module Federation: Platform-App 화면에 GoJIRA-App 의 `remoteEntry.js` 로드 (DevTools Network 탭)

---

## ⑧ 운영

### 일상 작업

| 작업 | 명령 |
|---|---|
| 전체 재시작 | `docker compose restart` |
| 단일 서비스 재시작 | `docker compose restart <service-name>` |
| 로그 실시간 | `docker compose logs -f <service-name>` |
| 상태 확인 | `docker compose ps` |

### 설정 변경 반영

| 변경 대상 | 절차 |
|---|---|
| 백엔드용 변수 (Mongo/Gitea/OpenCode/시크릿) | `.env` 수정 → `docker compose up -d` |
| 사용자 access URL (`*_SERVER` / `*_PORT`) | `.env` 수정 → `docker compose up -d` (런타임 config 라 재빌드 불요) |
| LLM provider swap | `config/llm.json` 편집 (active provider 와 `_disabled_*` swap) → `docker compose restart platform-api` |
| llm.json 만 수정 | volume mount 이므로 컨테이너 재시작만 (`docker compose restart platform-api`) |

### 백업 / 복구

`GOJIRA_REPOS_DIR` 은 이제 호스트 디렉토리 (bind mount) 이므로 일반 tar 로 직접 백업:

```bash
source .env
tar czf repos-$(date +%F).tgz -C "$(dirname ${GOJIRA_REPOS_DIR})" "$(basename ${GOJIRA_REPOS_DIR})"
```

복구는 호스트에서 tar 풀기 + 권한 재설정:

```bash
tar xzf repos-YYYY-MM-DD.tgz -C "$(dirname ${GOJIRA_REPOS_DIR})"
sudo chown -R ${UID}:${GID} ${GOJIRA_REPOS_DIR}
```

### 완전 제거

```bash
docker compose down       # 컨테이너만 제거 (bind mount 호스트 디렉토리는 보존)
# repos 도 같이 지우려면 명시적으로:
# rm -rf ${GOJIRA_REPOS_DIR}
```

### 업데이트 (새 이미지 반입)

외부망에서 새 빌드 → ④ 동일하게 tar 생성 → 폐쇄망에 반입 → 아래 실행:

```bash
docker load -i gojira-images.tar       # 새 이미지 덮어쓰기
docker compose up -d                   # 변경된 이미지만 재생성
docker image prune                     # 옛 이미지 정리
```

---

## 트러블슈팅

| 증상 | 원인 / 대처 |
|---|---|
| `docker compose up` 시 `WARN: required` 에러 | `.env` 누락 — `?required` 항목 확인 |
| Platform-API 가 `permission denied` 로 worktree 생성 실패 | `GOJIRA_REPOS_DIR` 디렉토리 소유자가 `${UID}:${GID}` 와 다름. `sudo chown -R ${UID}:${GID} ${GOJIRA_REPOS_DIR}` |
| Platform-API startup 시 `mkdir /data/repos: permission denied` | 위와 동일 — bind mount 호스트 디렉토리 권한 확인 |
| 백엔드 startup 시 `MongoDB connect timeout` | `MONGODB_SERVER` / `MONGO_USER` / `MONGO_PASS` 확인. 비밀번호에 `@` / `:` / `/` 들어가면 URL encoding 필요 (`@` → `%40`) |
| 브라우저 콘솔 `ERR_CONNECTION_REFUSED localhost` | env.js 가 생성되지 않았거나 비어있음. `docker compose exec platform-app cat /usr/share/nginx/html/env.js` 로 확인. `.env` 의 `*_SERVER`/`*_PORT` 값 검증 후 `docker compose up -d platform-app gojira-app` |
| Module Federation `remoteEntry.js` 실패 | `GOJIRA_APP_SERVER` + `GOJIRA_APP_PORT` 가 브라우저 접근 가능한 endpoint 인지 확인 |
| LLM 호출 시 `unauthorized` | `OPENAI_API_KEY` / `PLATFORM_LLM_TICKET` 확인. `GOJIRA_ENC_KEY` 가 `ENC:` 값 암호화 시 사용한 키와 동일한지 |
| `ENC:` 값 복호화 fail | 두 환경의 `GOJIRA_ENC_KEY` 가 다름. 양쪽 모두 미설정이면 코드 default key 가 동일하게 적용됨 |
| PlantUML 한글/다이어그램 깨짐 | `docker compose restart plantuml-server` (JVM 폰트 캐시 이슈) |
| Gitea push 시 403 | `PLATFORM_GITHUB_TOKEN` scope 에 `repo` + `admin:org` 포함 여부 확인 |

---

## 관련 문서

- `deploy/README.md` — 패키지 구성 및 설계 원칙
- `docs/env_info.md` — 환경변수 카탈로그 (개발/운영 공통)
- `docs/ports.md` — 포트 매핑 표
- `docs/architecture/deployment.md` — 배포 토폴로지
