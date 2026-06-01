# 폐쇄망 자체 빌드 운영 가이드

소스 변경마다 외부망에서 도커 이미지 645 MB 를 옮기는 대신, 폐쇄망에서 직접 빌드하는 모델.
의존성 캐시 이미지 2종 (`gojira/build-cache-go`, `gojira/build-cache-node`) 만 한 번 적재해두면
코드 변경 → 빌드 → 배포가 사내망 안에서 1-3분 안에 끝납니다.

## 사전 조건 (이미 충족된 것)

- [x] Docker (폐쇄망 빌드 서버)
- [x] 베이스 이미지 6종 적재 (`gojira-images.tar` 의 첫 빌드에 포함)
- [x] 소스 코드 (사내 Gitea 또는 tar)

## 추가로 필요한 것 (한 번만)

### 1) 외부망에서 의존성 캐시 이미지 생성

```bash
# 외부망 (인터넷 가능한 PC) — GoJIRA 소스 위에서
bash deploy/scripts/build-cache-update.sh
# → build-cache.tar (약 600~800 MB) 생성
```

### 2) 폐쇄망에 적재

```bash
# 사내망 빌드 서버
docker load -i build-cache.tar
# → Loaded image: gojira/build-cache-go:latest
# → Loaded image: gojira/build-cache-node:latest
```

이게 끝. 이후 폐쇄망에서 외부 호출 없이 모든 서비스를 빌드할 수 있습니다.

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

`go get <pkg>` 또는 `npm install <pkg>` 후 `go.mod` / `package-lock.json` 변경됨.

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

## 어떤 변경이 어떤 경로?

| 변경 유형 | 누가 | 어디서 | 외부망 필요? |
|---|---|---|---|
| 코드 수정 (Go) | 개발자 | 사내망 git push → build 서버 build & up -d | ❌ |
| 코드 수정 (React) | 개발자 | 사내망 git push → build 서버 build & up -d | ❌ |
| `.env` 변경 | 운영자 | `docker compose up -d` (env 자동 재주입) | ❌ |
| 의존성 추가 | 개발자 | 외부망 → `build-cache-update.sh` → tar 반입 | ✅ (월 1-2회) |
| 베이스 이미지 갱신 (Go 1.27, node 22 등) | 운영자 | 외부망 → 베이스 이미지 + build-cache 둘 다 재생성 | ✅ (분기 1회 정도) |

## Dockerfile 의 build-cache 사용 검증

빌드 시 `golang:1.26-alpine` 또는 `node:lts-alpine` 을 직접 base 로 안 쓰는지 확인:

```bash
grep -E "^FROM golang|^FROM node" deploy/images/*/Dockerfile
# 출력은 build-cache 이미지 Dockerfile (deploy/images/build-cache-*) 만 남고
# 서비스 Dockerfile 들은 FROM gojira/build-cache-* 만 보여야 함.
```

## 함정

1. **build-cache 이미지가 사내망에 없으면 build 실패** — 처음 셋업 시 `docker load -i build-cache.tar` 누락 확인.
2. **go.mod / package-lock.json 변경 후 캐시 갱신 안 하면** `go build` 또는 `npm run build` 가 외부 fetch 시도 → 실패. 의존성 추가 후 반드시 build-cache 갱신.
3. **사내 Gitea 의 push 권한** — 빌드 서버가 git pull 할 수 있어야 함. 또는 운영자가 수동으로 git pull 후 build 트리거.

## 비교

| 모델 | 외부망 ↔ 폐쇄망 트래픽 | 반영 시간 | 의존성 추가 |
|---|---|---|---|
| 기존 (전체 이미지 반입) | 변경마다 645 MB | 수 십분 | 동일 |
| **폐쇄망 자체 빌드** | 의존성 변경 시만 ~700 MB | 1-3분 | 약간 더 작업 |
