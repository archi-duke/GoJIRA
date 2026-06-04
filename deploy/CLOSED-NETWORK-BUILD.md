# 폐쇄망 자체 빌드 운영 가이드

소스 변경마다 외부망에서 도커 이미지 645 MB 를 옮기는 대신, 폐쇄망에서 직접 빌드하는 모델.
의존성 캐시 이미지 3종 (`gojira/build-cache-{go,node,py}`) 만 한 번 적재해두면
코드 변경 → 빌드 → 배포가 사내망 안에서 1-3분 안에 끝납니다. (런처 포함 전체 7개 서비스 오프라인 빌드.)

| 캐시 이미지 | 무엇을 담나 | 어떤 서비스가 쓰나 |
|---|---|---|
| `gojira/build-cache-go` | Go 모듈 캐시 | platform-api, gojira-api, launcher-dist |
| `gojira/build-cache-node` | GoJIRA-App·Platform-App·**pumlex** node_modules | gojira-app, platform-app, pumlex-server |
| `gojira/build-cache-py` | jira-api Python wheel | jira-api |

> pumlex-server(`npm install`)·jira-api(`pip wheel`) 도 빌드 시 외부 레지스트리를 타므로,
> 위 캐시에 의존성을 미리 담아 오프라인 빌드되도록 통합했다.

## 사전 조건

| 항목 | 비고 |
|---|---|
| Docker (compose v2 포함) | 사내망 빌드 서버 |
| 소스 코드 | 사내 Gitea clone 또는 tar |
| **베이스 이미지 6종** | 첫 셋업 시 외부망에서 반입 (`alpine:3.20` 등) |
| **build-cache 이미지 3종** | 첫 셋업 시 외부망에서 반입 (`gojira/build-cache-{go,node,py}`) |
| (선택) `deploy/.env` | 사내 인프라값 반영 (Mongo/Gitea 자격, host IP) |

## 최초 셋업 (한 번만)

### 1) 외부망 PC 에서 베이스 이미지 + build-cache 한 번에 패키징

```bash
# 외부망 PC — GoJIRA 소스 clone 후
xargs -n1 docker pull < <(grep -v '^gojira/\|^#\|^$' deploy/scripts/images.txt)
bash deploy/scripts/build-cache-update.sh    # → gojira/build-cache-{go,node,py}:latest 생성

docker save \
  plantuml/plantuml-server:jetty \
  nginx:alpine \
  python:3.12-slim \
  alpine:3.20 \
  golang:1.26-alpine \
  node:lts-alpine \
  gojira/build-cache-go:latest \
  gojira/build-cache-node:latest \
  gojira/build-cache-py:latest \
  -o base-and-cache.tar
# 약 1.5 GB
```

> **호스트 arch 주의**: 사내 빌드 서버가 amd64 면 외부망 PC 도 amd64 에서 pull/build 하거나
> `DOCKER_DEFAULT_PLATFORM=linux/amd64` 명시. arm64 캐시를 amd64 서버에 반입하면
> `DeadlineExceeded` (registry 재조회) 또는 platform mismatch 로 빌드 실패.

### 2) 폐쇄망 빌드 서버에 적재

```bash
docker load -i base-and-cache.tar
docker images | grep -E "alpine|node|golang|python|nginx|plantuml|build-cache"
# → 9 줄 (베이스 6종 + 캐시 3종) 확인
```

이로써 폐쇄망에서 외부 호출 0회로 모든 서비스(런처 포함)를 빌드할 수 있습니다.

> 참고: `deploy/vendor/pumlex/**/package.json` 3개는 루트 `.gitignore` 의 광범위한
> `package.json` 무시 대상이지만 `!` 부정 규칙으로 명시 추적된다(build-cache-node 가
> 읽어야 함). 과거엔 누락되어 수동 복사가 필요했으나 이제 fresh clone 에 항상 포함된다.

## 운영 사이클

### 일반 코드 수정 (대부분의 경우)

```bash
# 사내망 빌드 서버
cd /opt/GoJIRA

git pull                                                  # 사내 Gitea 에서 최신
docker compose -f deploy/docker-compose.yml build \
  platform-api gojira-api gojira-app                      # 변경된 서비스만
docker compose -f deploy/docker-compose.yml up -d \
  --force-recreate platform-api gojira-api gojira-app
```

→ **1-3분 안에 반영 완료**. 외부망 의존 0회.

### 의존성 추가 (드물게)

의존성 변경 시 — `go.mod`/`go.sum`(build-cache-go), `package-lock.json`·pumlex `package.json`(build-cache-node), `JIRA-API/requirements.txt`(build-cache-py).

```bash
# 외부망에서 캐시 갱신
bash deploy/scripts/build-cache-update.sh

# 새 build-cache.tar 만 사내망에 반입 (645 MB → 사내망 이미지는 그대로)
docker load -i build-cache.tar
```

코드 변경분은 평소처럼 git push 로.

### launcher (로컬 런처) 변경 시

런처는 **`launcher-dist` 이미지**로 빌드·배포된다 — 별도 cross-compile 손작업이 필요 없다.
`docker compose build` 가 OS 3종(windows/linux/darwin) cross-compile + 에이전트 자산 embed +
`version.txt`/`.sha256` 생성까지 한 번에 수행하고, 기동 시 init job 이 공유 볼륨을 채우면
platform-api 가 `/api/v2/launcher/download/{os}` · `/launcher/version` 으로 서빙한다.

```bash
# 사내망 빌드 서버 — 런처 소스(gojira-launcher/) 또는 embed 자산(.claude/skills,
# opencode.json, .opencode/agents) 또는 버전(handlers.go const version) 변경 시
git pull
docker compose -f deploy/docker-compose.yml build launcher-dist
docker compose -f deploy/docker-compose.yml up -d launcher-dist platform-api
```

→ init job 이 재실행되어 공유 볼륨을 새 바이너리로 덮어쓴다. platform-api 본체 재빌드 불필요.
자동 업데이트(`/launcher/version`)도 즉시 새 버전을 인식한다.

> 런처 바이너리는 **클라이언트 OS용**(서버 arch 와 무관)이므로, 빌드 호스트가 arm64 든 amd64 든
> 동일한 산출물이 나온다. `deploy/launcher-dist/` 폴더 수동 반입은 더 이상 필요 없다.

#### 런처 배포 이미지 빌드 위치 — 권장 **"platform-api 가 도는 그 서버"**

PR #40 이후 `launcher-dist` 가 compose 에 통합돼 같은 `docker compose build` 한 번으로
끝난다. **빌드 위치를 platform-api 와 분리하지 말 것**:

| 빌드 위치 | 권장도 | 이유 |
|---|---|---|
| 사내 배포 서버 (= platform-api 호스트) | ✅ **권장** | 단일 진실. 산출물 ↔ 서빙 위치 동일. compose 한 줄로 처리 |
| 별도 빌드 전용 머신 | △ | sha256/embed drift 위험. 산출물 tar 반입 손작업 발생 |
| 개발자 노트북 (Apple Silicon) | ❌ | embed 한 `.claude/skills` 가 사내 git 과 어긋날 수 있고, build-cache arch 도 mismatch 가능 |

런처만 hotfix 시:
```bash
docker compose build launcher-dist && docker compose up -d
```

`handlers.go` 의 `const version` 을 bump 한 경우만 클라이언트가 자동 업데이트 받는다 —
스킬/embed 만 바뀌었는데 version 그대로면 manifest 가 같아 자동 업데이트 트리거 안 됨.

## 어떤 변경이 어떤 경로?

| 변경 유형 | 누가 | 어디서 | 외부망 필요? |
|---|---|---|---|
| 코드 수정 (Go) | 개발자 | 사내망 git push → build 서버 build & up -d | ❌ |
| 코드 수정 (React) | 개발자 | 사내망 git push → build 서버 build & up -d | ❌ |
| `.env` 변경 | 운영자 | `docker compose up -d` (env 자동 재주입) | ❌ |
| 의존성 추가 | 개발자 | 외부망 → `build-cache-update.sh` → tar 반입 | ✅ (월 1-2회) |
| 베이스 이미지 갱신 (Go 1.27, node 22 등) | 운영자 | 외부망 → 베이스 이미지 + build-cache 둘 다 재생성 | ✅ (분기 1회 정도) |

## Dockerfile 의 build-cache 사용 검증

각 서비스의 **builder stage** 는 반드시 `gojira/build-cache-*` 를 base 로 써야 한다
(외부 레지스트리에서 의존성을 받지 않도록). **runtime stage** 가 `nginx:alpine`,
`alpine:3.20`, `python:3.12-slim`, `node:lts-alpine` 를 FROM 하는 것은 정상 —
이들은 base-and-cache.tar 의 베이스로 제공된다.

```bash
grep -nE "^FROM " deploy/images/*/Dockerfile
# 기대값:
#   build-cache-go/node/py  : FROM golang/node/python                 (캐시 이미지 자신)
#   platform-api/gojira-api/launcher-dist : builder=gojira/build-cache-go,   runtime=alpine:3.20
#   gojira-app/platform-app               : builder=gojira/build-cache-node, runtime=nginx:alpine
#   pumlex-server                         : builder=gojira/build-cache-node, runtime=node:lts-alpine
#   jira-api                              : builder=gojira/build-cache-py,   runtime=python:3.12-slim
# ⚠️ 서비스 builder stage 에 build-cache 가 아닌 golang/node/python 이 직접 보이면
#    그 서비스는 폐쇄망 오프라인 빌드가 깨진다(외부 레지스트리 fetch 시도).
```

## 함정

1. **build-cache 이미지가 사내망에 없으면 build 실패** — 처음 셋업 시 `docker load -i build-cache.tar` 누락 확인.
2. **베이스 이미지(`alpine:3.20` 등)도 없으면 build 실패** — `target X: failed to solve: DeadlineExceeded: alpine:3.20`. 베이스 6종도 함께 반입 필요(최초 셋업 §1 참조).
3. **proxy 설정이 사내 미가용 proxy 를 가리키면 모든 빌드가 timeout** — `proxyconnect tcp: dial X.X.X.X:8080: i/o timeout`. 진단:
   ```bash
   env | grep -i proxy
   cat ~/.docker/config.json 2>/dev/null
   sudo cat /etc/docker/daemon.json 2>/dev/null
   ```
   임시 해결: `unset HTTP_PROXY HTTPS_PROXY http_proxy https_proxy && ./scripts/build-images.sh --no-pull`. 영구 해결은 daemon 설정에서 proxy 제거.
4. **build-cache arch 가 호스트와 다르면 buildx 가 registry 재조회 시도 → DeadlineExceeded** — 빌드 서버가 amd64 면 build-cache 도 amd64. arm64 PC 에서 build-cache 만들 때는 `DOCKER_DEFAULT_PLATFORM=linux/amd64` 명시 또는 `./scripts/build-images.sh --platform native` 로 호스트 arch 사용.
5. **의존성 변경 후 캐시 갱신 안 하면 외부 fetch 시도 → 실패.** 갱신 트리거: `go.mod`/`go.sum`(→ build-cache-go), `package-lock.json`·pumlex `package.json`(→ build-cache-node), `JIRA-API/requirements.txt`(→ build-cache-py). 변경 시 외부망에서 `build-cache-update.sh` 재실행.
6. **사내 Gitea 의 push 권한** — 빌드 서버가 git pull 할 수 있어야 함. 또는 운영자가 수동으로 git pull 후 build 트리거.
7. **vendor/pumlex 의 package.json** — 루트 `.gitignore` 의 `package.json` 무시 대상이나 `!` 부정 규칙으로 추적된다(`!deploy/vendor/pumlex/**/package.json`). 새 vendor 의존성 파일이 광범위 무시 규칙에 걸리지 않는지 확인.
8. **`.env` 의 MongoDB password 에 `@` 가 있으면 URL parse fail** — `MONGO_PASS` 의 특수문자는 URL-encoding 필요(`@` → `%40`). 코드가 raw 로 처리한다는 옛 문서 설명은 부정확.
9. **런처 버전 bump 누락** — 스킬/embed 만 변경하고 `handlers.go:const version` 그대로 두면 manifest version 이 안 바뀌어 클라이언트 자동 업데이트 트리거 안 됨. 패치마다 bump 또는 클라이언트에서 수동 재다운로드.

## 비교

| 모델 | 외부망 ↔ 폐쇄망 트래픽 | 반영 시간 | 의존성 추가 |
|---|---|---|---|
| 기존 (전체 이미지 반입) | 변경마다 645 MB | 수 십분 | 동일 |
| **폐쇄망 자체 빌드** | 의존성 변경 시만 ~700 MB | 1-3분 | 약간 더 작업 |
