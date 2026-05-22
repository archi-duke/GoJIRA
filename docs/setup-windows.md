# Windows 사내망 셋업 가이드

> ⚠️ **방향 전환 예고 (2026-05-22)**: 본 문서는 **현행 server-side 모델**(Agent-Service +
> WSL OpenCode 브리지 + Windows 네이티브 백엔드) 기준의 **동작하는 운영 가이드**입니다.
> 단, 폐쇄망 server-side 운영 한계(개인 SSO 토큰 격리·docker↔host 경로 불일치·브리지 복잡도)로
> **"에이전트 로컬 실행 + 웹→로컬 런처(B)"** 모델로 전환이 결정되었습니다. 전환 후에는 아래
> §3(WSL OpenCode 서버), §5.3(Agent-Service .env) 등이 **로컬 런처 + CLI 선택(opencode|claude)**
> 모델로 교체됩니다. 전환 설계는 [`Long-term Considerations/local-agent-mvp.md`](./Long-term%20Considerations/local-agent-mvp.md)
> 및 [`architecture-pivot-vscode.md`](./Long-term%20Considerations/architecture-pivot-vscode.md) 참조.
> **구현 전까지는 본 가이드가 유효합니다.**

> 본 문서는 사내망에서 GoJIRA 를 운영하기 위한 Windows 환경 셋업 가이드입니다.
> 테스트 후 Windows Server 로 옮길 예정이므로, 사용자 PC 와 운영 서버 양쪽에 적용 가능한 절차로 작성되어 있습니다.

## 1. 환경 구성

| 컴포넌트 | 위치 | 비고 |
|---|---|---|
| **MongoDB** | 사내 별도 서버 | IT 가 관리하는 공용 인프라. URL 만 .env 에 명시 |
| **Gitea** | 사내 별도 서버 | 동일 |
| **OpenCode** (`:4096`) | 사용자 PC 의 WSL | `opencode serve` — LLM 호출 + 스킬 디스패치 런타임 |
| **Platform-API** (`:9000`) | **Windows 네이티브** | Go binary 직접 실행 |
| **GoJIRA-API** (`:9001`) | **Windows 네이티브** | Go binary 직접 실행 |
| **Agent-Service** (`:9002`) | **Windows 네이티브** | Python FastAPI |
| **Platform-App** (`:3000`) | **Windows 네이티브** | React dev server (테스트) / build 산출물 (운영) |
| **GoJIRA-App** (`:3001`) | **Windows 네이티브** | React dev server (테스트) / build 산출물 (운영) |
| **사내 LLM (gpt-oss-120b)** | 사내 LLM 게이트웨이 | X-Dep-Ticket 인증, `llm.json` 에 설정 |
| **SSO (ADFS)** | 미사용 (테스트 단계) | 추후 운영 단계 도입 |

전체 포트 + 의존 매트릭스는 `docs/ports.md` 참조.

## 2. 사전 설치 (Windows)

| 도구 | 버전 | 용도 |
|---|---|---|
| Go | 1.21+ | Platform-API / GoJIRA-API 빌드·실행 |
| Python | 3.9+ | Agent-Service 실행 |
| Node.js | 18+ | Platform-App / GoJIRA-App 빌드·dev server |
| Git for Windows | 최신 | Git Bash + git 명령. 스킬 bash 스크립트 실행 |
| jq | 최신 | 스킬 JSON 처리 (`winget install jqlang.jq` 또는 `choco install jq`) |
| curl | Windows 10+ 기본 포함 | (Git Bash 의 curl 도 OK) |

설치 후 검증 (PowerShell):

```powershell
where.exe go      # Go 경로 출력
where.exe python  # Python
where.exe node
where.exe git
where.exe jq
where.exe bash    # Git Bash 의 bash
```

5개 모두 경로 출력되면 OK. 빠진 게 있으면 해당 도구 PATH 추가.

## 3. WSL 사전 — OpenCode 설치

OpenCode 는 WSL 안에서 실행합니다. WSL2 설치 + Linux 배포판(예: Ubuntu) 후:

```bash
# WSL 안에서
curl -fsSL https://opencode.ai/install | bash
# 설치 결과: ~/.opencode/bin/opencode
echo 'export PATH="$HOME/.opencode/bin:$PATH"' >> ~/.bashrc
source ~/.bashrc
opencode --version
```

서버 가동:

```bash
opencode serve --port 4096
```

### 3.1 Windows ↔ WSL 통신

WSL2 의 `localhost` 는 Windows 와 양방향 forwarding 됨 (기본):
- Windows 의 Agent-Service 가 `http://localhost:4096` 로 호출 → WSL 안 OpenCode 도달
- WSL 안 OpenCode 가 `localhost` 로 Windows 서비스 접근은 제한적 — WSL 입장에선 Windows 호스트를 별도 IP 로 봄

OpenCode 가 Windows 의 Platform-API / GoJIRA-API 를 호출할 일은 거의 없음 (스킬이 curl 로 호출하지만 그건 호스트 PC PowerShell 에서 OpenCode 가 spawn 한 shell 에서 실행 — 후술).

만약 `localhost` forwarding 이 작동 안 하면 `%USERPROFILE%\.wslconfig` 에:

```ini
[wsl2]
localhostForwarding=true
```

추가 후 PowerShell 에서 `wsl --shutdown` → WSL 재진입.

### 3.2 OpenCode 가 Windows 의 bash/curl/jq 를 사용해야 하는 케이스

OpenCode 가 스킬을 실행할 때 `Bash` tool 로 명령을 spawn 합니다. 사용자가 WSL 안에서 OpenCode 를 실행하면 spawn 된 shell 도 WSL 안 — bash/curl/jq/python3 가 WSL 안에서 모두 동작해야 합니다:

```bash
sudo apt update && sudo apt install -y curl jq python3 git
# WSL 안에서 검증
bash -c "curl --version | head -1 && jq --version && python3 --version"
```

## 4. 리포지토리 클론 (Windows)

PowerShell 또는 Git Bash 에서:

```bash
cd C:\Works   # 또는 운영 표준 경로
git clone <gitea-url>/<org>/GoJIRA.git
cd GoJIRA
```

## 5. 환경변수 (`.env`) 설정

각 Go/Python 서비스는 binary 옆의 `.env` 를 자동 탐색합니다 (`PLATFORM_ENV_FILE` / `GOJIRA_ENV_FILE` 환경변수로 override 가능). cwd 와 binary 위치가 어긋나도 동작 — `gojira-shared/envload` 가 처리.

### 5.1 Platform-API/.env

`Platform-API/.env.example` 참고. 사내 호스트로 채우기:

```ini
MONGO_URI=mongodb://<사내-mongo-호스트>:27017
PLATFORM_GITHUB_TOKEN=ENC:<사내-gitea-pat>
PLATFORM_GITHUB_ORG=<사내-gitea-org>
GITHUB_URL=http://<사내-gitea-호스트>:3300/
GOJIRA_API_URL=http://localhost:9001/api/v2

# 사내 LLM 게이트웨이 인증 (gpt-oss)
PLATFORM_LLM_TICKET=ENC:<X-Dep-Ticket-base64>
PLATFORM_LLM_USER_ID=<knox-id>

# 시크릿 마스터 키 (ENC: 복호화에 사용)
GOJIRA_ENC_KEY=<32바이트-hex-or-base64>
```

`ENC:` 접두사 값은 `Platform-API/cmd/encrypt` 로 평문 → 암호문 변환 후 입력:

```powershell
cd Platform-API
go run ./cmd/encrypt "원본값" $env:GOJIRA_ENC_KEY
# 출력: ENC:base64...
```

### 5.2 GoJIRA-API/.env

```ini
MONGO_URI=mongodb://<사내-mongo-호스트>:27017
GOJIRA_PLATFORM_API=http://localhost:9000/api/v2
GOJIRA_JIRA_URL=http://<사내-jira-호스트>:8000

# JIRA 자격 (실제 JIRA DC 연동 시. mock 사용 시 생략 가능)
# GOJIRA_JIRA_USER=ENC:...
# GOJIRA_JIRA_PASS=ENC:...
```

### 5.3 Agent-Service/.env

```ini
MONGO_URI=mongodb://<사내-mongo-호스트>:27017
GOJIRA_API_URL=http://localhost:9001/api/v2
GOJIRA_PLATFORM_API=http://localhost:9000/api/v2

# WSL 의 OpenCode — Windows 에서 localhost forwarding 으로 접근
OPENCODE_SERVER_URL=http://localhost:4096
# OpenCode 가 password 보호되어 있으면 명시:
# OPENCODE_SERVER_USERNAME=opencode
# OPENCODE_SERVER_PASSWORD=<password>
```

## 6. LLM 설정 — `Platform-API/config/llm.json`

`llm.json` 은 현재 외부 OpenAI (`gpt-4o-mini`) 가 active, 사내 gpt-oss 가 `_disabled_Internal` 에 보존되어 있습니다. **사내망 테스트 전에 top-level 활성 필드를 `_disabled_Internal` 의 값으로 swap** 해야 합니다.

수동 swap 예 (운영자가 직접 편집):

```jsonc
{
  // 활성 필드를 사내 gpt-oss 값으로 교체
  "endpoint": "http://apigw.samsungds.net:8000/gpt-oss/1/gpt-oss-120b/v1/chat/completions",
  "model": "openai/gpt-oss-120b",
  "apiKey": "${PLATFORM_LLM_TICKET}",
  "maxTokens": 500,
  "temperature": 0,
  "stream": false,
  "systemPrompt": "...",
  "headers": {
    "X-Dep-Ticket": "${PLATFORM_LLM_TICKET}",
    "Send-System-Name": "ai4se",
    "User-Id": "${PLATFORM_LLM_USER_ID}",
    "User-Type": "AD_ID",
    "Chat-Id": "00000000-0000-0000-0000-000000000000",
    "Accept": "application/json",
    "Content-Type": "application/json"
  },
  "authType": "header",
  "authHeader": "X-Dep-Ticket",
  ...

  "_comment": "OpenAI 외부 설정은 보존 — 추후 외부망 전환 시 swap",
  "_disabled_OpenAI": {
    "endpoint": "https://api.openai.com/v1/chat/completions",
    "model": "gpt-4o-mini",
    "apiKey": "${OPENAI_API_KEY}",
    ...
  }
}
```

`${PLATFORM_LLM_TICKET}` 같은 `${VAR}` 패턴은 Platform-API 가 startup 시 환경변수로 치환합니다 (시크릿 분리). `.env` 의 `PLATFORM_LLM_TICKET` 이 `ENC:...` 면 복호화 후 사용됩니다.

## 7. 빌드 + 가동 순서

### 7.1 Backend 빌드 (Windows 네이티브, PowerShell)

```powershell
# Go workspace 가 GoJIRA-API / Platform-API / gojira-shared 를 함께 인식
cd C:\Works\GoJIRA

# Platform-API
cd Platform-API
go build -o platform-api.exe .
cd ..

# GoJIRA-API
cd GoJIRA-API
go build -o gojira-api.exe .
cd ..
```

### 7.2 인프라 사전 확인

```powershell
# Mongo 접속 가능 확인 (mongosh 또는 curl)
curl http://<사내-mongo-호스트>:27017
# Gitea 접속
curl http://<사내-gitea-호스트>:3300/api/v1/version
# WSL 안 OpenCode (Windows 에서)
curl http://localhost:4096/event
```

### 7.3 서비스 가동 (PowerShell 4개 창 또는 nssm 서비스)

```powershell
# 1) Platform-API (먼저 가동 — peer 의존성 최소)
cd C:\Works\GoJIRA\Platform-API
.\platform-api.exe

# 2) GoJIRA-API
cd C:\Works\GoJIRA\GoJIRA-API
.\gojira-api.exe

# 3) Agent-Service
cd C:\Works\GoJIRA\Agent-Service
pip install -r requirements.txt   # 최초 1회
python main.py

# 4-5) Frontend (테스트 단계 — 운영은 build + serve)
cd C:\Works\GoJIRA\Platform-App
npm install   # 최초 1회
npm start     # :3000

cd C:\Works\GoJIRA\GoJIRA-App
npm install
npm start     # :3001
```

### 7.4 startup 로그 확인 포인트

각 서비스가 다음 줄을 출력하면 정상:

| 서비스 | 확인 |
|---|---|
| Platform-API | `[envload] loaded N entries from C:\Works\GoJIRA\Platform-API\.env` + `LLM 설정 로드 완료: ... gpt-oss-120b` + `MongoDB connected` |
| GoJIRA-API | `[envload] loaded N entries from ...` + `MongoDB connected` |
| Agent-Service | `LISTEN: 9002 (mode=opencode_bridge)` + `OpenCode peer: http://localhost:4096` |

`.env` 못 읽으면 `[envload] .env not found — tried: [...]` 로 시도한 절대경로가 모두 로그에 노출됩니다. 그 후 필수 환경변수 누락이 `[config] 필수 환경변수 미설정: MONGO_URI` 류로 fail-fast.

## 8. 브라우저 진입

| 환경 | URL |
|---|---|
| 테스트 (dev server) | `http://localhost:3000` (Platform-App) — GoJIRA-App 은 module federation 으로 자동 로드 |
| 운영 (build serve) | 운영 도메인 (사내 정책에 맞춰) |

SSO 미사용이므로 dev 모드 진입 — `DEV_USER` 환경변수의 사용자로 자동 로그인됨 (Platform-API 가 처리).

## 9. 트러블슈팅

### 9.1 `.env` 못 읽음

`[envload] .env not found` 가 보이면 시도한 절대경로 확인. 옵션:

| 조치 | 명령 |
|---|---|
| binary 옆에 .env 배치 (가장 자연스러움) | `C:\Works\GoJIRA\Platform-API\.env` 등 |
| 절대경로 override | `$env:PLATFORM_ENV_FILE = "C:\Path\To\custom.env"` (`GOJIRA_ENV_FILE` / `AGENT_ENV_FILE` 도 동일) |
| 환경변수로 직접 주입 (Windows 서비스로 띄울 때) | `nssm` 의 `AppEnvironmentExtra` 등 활용 |

### 9.2 OpenCode 연결 실패

`OpenCode peer: ...` 출력 후 채팅이 작동 안 하면:

```powershell
# WSL 안 OpenCode 가 실제로 listen 중인지
wsl -- ss -tlnp | findstr 4096
# Windows 에서 forwarded localhost 접근
curl http://localhost:4096/event --max-time 3
```

응답 없으면 WSL `.wslconfig` 의 `localhostForwarding=true` 확인.

### 9.3 사내 LLM (X-Dep-Ticket) 인증 실패

Platform-API 로그에 `LLM 호출 실패 401` 등이 보이면:

- `.env` 의 `PLATFORM_LLM_TICKET` 이 `ENC:` 접두사 + 올바른 마스터 키로 암호화됐는지 확인
- `Platform-API/cmd/encrypt` 로 평문 → 암호문 재생성
- `llm.json` 의 `${PLATFORM_LLM_TICKET}` placeholder 가 그대로 (수동 치환 X) — Platform-API 가 startup 시 환경변수로 치환

### 9.4 Mongo / Gitea 접근 차단 (사내 방화벽)

- 사내 보안 그룹에 사용자 PC IP 추가 요청
- 또는 운영 시점 (Windows Server) 에는 같은 사내 망 segment 라 자유롭게 접근

## 10. 운영 단계로 이행 (Windows Server)

테스트 후 운영으로 옮길 때 변경점:

| 항목 | 테스트 (사용자 PC) | 운영 (Windows Server) |
|---|---|---|
| 서비스 가동 | PowerShell 4창 직접 실행 | **nssm** 또는 Windows Service 로 등록 (자동 재시작) |
| Frontend | `npm start` dev server | `npm run build` 후 nginx/IIS 등으로 정적 serve |
| SSO | 미사용 (DEV_USER) | ADFS 연동 활성화 (`USE_SSO=true`) |
| OpenCode 위치 | 사용자 PC WSL | 운영 정책 결정 필요 — 서버 WSL or 별도 호스트 |
| 시크릿 마스터 키 (`GOJIRA_ENC_KEY`) | 임시값 가능 | 사내 KMS 또는 Group Policy 에 보관 |
| 로그 | stdout (콘솔) | nssm 의 stdout/stderr 리다이렉트, ELK / Splunk 등 수집 |

## 11. 관련 문서

- `docs/ports.md` — 전체 포트 매핑 (단일 출처)
- `docs/design-platform.md` — Platform-API / GoJIRA-API / Agent-Service 설계
- `docs/design-agent.md` — OpenCode bridge 구조 + 채팅 라이프사이클
- `docs/skill-infrastructure-v3.md` — 스킬 인프라 (drift-check, plantuml-fix 등)
- `Platform-API/.env.example` / `GoJIRA-API/.env.example` / `Agent-Service/.env.example` — 환경변수 템플릿
