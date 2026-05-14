# 환경변수 & 설정 카탈로그

> 새 환경에서 GoJIRA 를 셋업할 때 참고하는 **단일 출처**. 각 서비스의 `.env` 키 + 설정 JSON + 복구 절차를 모두 기록합니다.
> 시크릿 실제 값은 절대 이 문서에 적지 마세요 (이 문서는 git 에 커밋됨). 실제 값은 `.env` 파일에만 두고, `.env` 는 `.gitignore` 로 차단되어 있습니다.
> 셋업 절차 전체는 [docs/setup-windows.md](./setup-windows.md), 포트 매핑은 [docs/ports.md](./ports.md), 배포 토폴로지는 [docs/architecture/deployment.md](./architecture/deployment.md) 참조.

## 1. 파일 위치 한 눈에

| 경로 | gitignore | 역할 |
|---|---|---|
| `.env` (루트) | ✅ | **현재 어떤 서비스도 안 읽음**. 평문 OPENAI 키가 외부 도구용 (OpenCode CLI 수동 호출 등) 일 수 있어 임의 삭제 보류 |
| `Platform-API/.env` | ✅ | Platform-API 가 `envload` 로 자동 탐색 |
| `GoJIRA-API/.env` | ✅ | GoJIRA-API 가 `envload` 로 자동 탐색 |
| `Agent-Service/.env` | ✅ | pydantic-settings 가 cwd `.env` 로 로드 |
| `GoJIRA-App/.env` | ✅ | CRA dev server. `PORT` 등 |
| `Platform-API/.env.example` | ❌ | 키 템플릿 (committed) |
| `GoJIRA-API/.env.example` | ❌ | 동일 |
| `Agent-Service/.env.example` | ❌ | 동일 |
| `GoJIRA-App/.env.example` | ❌ | 동일 |
| `opencode.json` (루트) | ❌ | OpenCode 기본 모델 + permission. committed |
| `.opencode/agents/gojira.md` | ❌ | OpenCode 에이전트 정의. committed |
| `.claude/skills/*/SKILL.md` | ❌ | 워크플로우 스킬. committed |
| `Platform-API/config/llm.json` | ❌ | LLM 게이트웨이 정의 (active + `_disabled_Internal`). committed |
| `~/.opencode/auth.json` | (사용자 홈) | `opencode auth login` 결과. 환경마다 재발급 |

## 2. Platform-API

### 2.1 `.env` 키 사용 매트릭스

| 키 | 종류 | 코드 위치 | 설명 |
|---|---|---|---|
| PLATFORM_MONGO_URI | 필수 | `config/config.go:48` | `mongodb://<host>:27017` 또는 `mongodb://user:pw@host:27017/?authSource=admin` |
| PLATFORM_GITHUB_TOKEN | 필수 | `config/config.go:49` | Gitea PAT — `repo` + `admin:org` 권한 |
| PLATFORM_GITHUB_ORG | 필수 | `config/config.go:50` | Gitea organization 이름 (예: `gojira`) |
| GITHUB_URL | 필수 | `config/config.go:51` | Gitea/GitHub base URL. 끝에 `/` 포함 |
| PLATFORM_GOJIRA_API | 필수 | `config/config.go:52` | GoJIRA-API peer URL |
| PLATFORM_REPO_DIR | 선택 | `config/config.go:57` | 기본 `./repos`. 로컬 git 워크트리 디렉토리 |
| SMTP_HOST/PORT/USER/PASS/FROM | 선택 | `config/config.go:60-70` | 메일 발송 사용 시. `SMTP_PASS` 는 `ENC:` 형식 |
| OPENAI_API_KEY | 선택 | `config/config.go:80`, `llm/client.go:43` | `llm.json` 의 `${OPENAI_API_KEY}` 치환 — 활성 LLM 이 OpenAI 일 때만 필요. `ENC:` 형식 |
| PLATFORM_LLM_TICKET | 선택 | `config/config.go:80` | 사내 LLM (Code Mate 등) `X-Dep-Ticket`. `ENC:` 형식. `_disabled_Internal` 사용 시 필수 |
| PLATFORM_LLM_USER_ID | 선택 | `config/llm.json` | 사내 LLM `User-Id` 헤더 (AD ID). 평문 |
| GOJIRA_ENC_KEY | 선택 | `gojira-shared/secret/crypto.go:24` | `ENC:` 복호화 마스터 키. 미설정 시 코드 default 사용 (운영에선 반드시 override) |
| PLATFORM_ENV_FILE | 조건부 | `envload/envload.go` | `.env` 위치 override (서비스 등록 시 절대경로 지정용) |
| GOJIRA_FIX_REVIEW | 조건부 | `github/client.go` | `0` 이면 PR review 공식 플래그 처리 skip (Gitea 도커 미사용 환경) |
| GITEA_CONTAINER | 조건부 | `github/client.go` | docker exec 대상 컨테이너명. 기본 `gojira-gitea` |
| PLATFORM_SERVER_PORT | 옵션(미사용) | `config/config.go:55` | 기본 `:9000` |
| PLATFORM_MONGO_DB | 옵션(미사용) | `config/config.go:56` | 기본 `gojira` |
| PLATFORM_WEBHOOK_URL/SECRET | 옵션(미사용) | `config/config.go:58-59` | Gitea webhook |
| PLATFORM_EDIT_SESSION_TTL | 옵션(미사용) | `config/config.go:66` | 기본 30분 |
| PLATFORM_LLM_CONFIG | 조건부 | `config/llm.go` | `llm.json` 경로 override |

### 2.2 `Platform-API/config/llm.json` 구조

LLM 게이트웨이 정의. **활성 설정 1개** + **대체 설정들** (`_disabled_*` 키로 보존). Go `encoding/json` 이 unknown 키를 silent 무시하므로 활성/대체 스왑 = 키 이름 교체만으로 가능.

```json
{
  "endpoint": "...",
  "model": "...",
  "apiKey": "${ENV_VAR}",
  "headers": { "Header-Name": "${ENV_VAR}", ... },
  "authType": "bearer | header",
  "authHeader": "<header-name-if-authType-header>",
  "responseFormat": {
    "choicesPath": "choices",
    "contentPath": "message.content",
    "streamPrefix": "data: ",
    "streamDone": "[DONE]"
  },
  "_disabled_Internal": { ... }   // 비활성 보존
}
```

**현재 active**: OpenAI `gpt-4o-mini` (외부망 테스트용)
**보존 active 후보**: `_disabled_Internal` — 사내망 `apigw.samsungds.net:8000` 의 `gpt-oss-120b`

#### 사내망 ↔ 외부 전환 절차

1. 현재 활성 필드를 `_disabled_OpenAI` 로 이름만 바꿔 보존
2. `_disabled_Internal` 의 내용을 root 레벨로 끌어올림 (이름 제거)
3. `.env` 에 `PLATFORM_LLM_TICKET`, `PLATFORM_LLM_USER_ID` 가 들어있는지 확인 (ENC: 포함)
4. Platform-API 재기동

### 2.3 새 환경 복구 절차

```bash
# 1. .env.example 복사
cp Platform-API/.env.example Platform-API/.env

# 2. 필수 값 채우기 (편집기로 열어 <placeholder> 치환)
#    - PLATFORM_MONGO_URI
#    - PLATFORM_GITHUB_TOKEN / PLATFORM_GITHUB_ORG / GITHUB_URL
#    - PLATFORM_GOJIRA_API

# 3. 시크릿 암호화 (Gitea PAT, SMTP_PASS, OpenAI 키 등)
#    GOJIRA_ENC_KEY 를 먼저 환경변수로 export
export GOJIRA_ENC_KEY='<32-byte-key>'
cd Platform-API
go run cmd/encrypt/main.go '<plaintext-secret>'   # ENC:... 출력 → .env 에 붙여넣기

# 4. llm.json 의 active provider 확인 (외부=OpenAI / 사내=Internal swap)

# 5. 빌드 + 실행
go build -o platform-api .
./platform-api    # 또는 윈도우: platform-api.exe
```

## 3. GoJIRA-API

### 3.1 `.env` 키

| 키 | 종류 | 코드 위치 | 설명 |
|---|---|---|---|
| GOJIRA_MONGO_URI | 필수 | `config/config.go:33` | Mongo 연결 문자열 |
| GOJIRA_PLATFORM_API | 필수 | `config/config.go:34` | Platform-API peer URL |
| GOJIRA_JIRA_URL | 필수 | `config/config.go:35` | JIRA mock 또는 실 JIRA URL |
| GOJIRA_JIRA_USER | 선택 | `config/config.go:44` | JIRA 인증. `ENC:` 형식 |
| GOJIRA_JIRA_PASS | 선택 | `config/config.go:51` | JIRA 인증. `ENC:` 형식 |
| GOJIRA_ENV_FILE | 조건부 | `envload` | `.env` 위치 override |
| GOJIRA_PROMPTS_DIR | 조건부 | `config/prompt.go` | 프롬프트 디렉토리 override |
| GOJIRA_SERVER_PORT | 옵션(미사용) | `config/config.go:38` | 기본 `:9001` |
| GOJIRA_MONGO_DB | 옵션(미사용) | `config/config.go:40` | 기본 `gojira` |
| GOJIRA_JIRA_API_BASE | 옵션(미사용) | `config/config.go:39` | 기본 `/rest/api/2` |
| GOJIRA_ENC_KEY | 선택 | `gojira-shared/secret/crypto.go` | Platform-API 와 동일 |

> **삭제된 키**: `OPENAI_API_KEY` — LLM 호출이 전부 Platform-API `/LLM/call` 게이트웨이 경유하므로 GoJIRA-API 가 OpenAI 키를 직접 보유할 필요 없음. (이전 잔여물)

### 3.2 복구 절차

```bash
cp GoJIRA-API/.env.example GoJIRA-API/.env
# 필수 3개 + JIRA 인증 ENC: 채우기
cd GoJIRA-API
go build -o gojira-api .
```

## 4. Agent-Service

### 4.1 `.env` 키

pydantic-settings 가 cwd `.env` + OS 환경변수 모두 읽음. 필수 4개 누락 시 startup `ValidationError`.

| 키 | 종류 | 코드 위치 | 설명 |
|---|---|---|---|
| GOJIRA_MONGO_URI | 필수 | `config.py:15` | Mongo 연결 |
| GOJIRA_API_URL | 필수 | `config.py:16` | GoJIRA-API peer (`/api/v2` 포함) |
| GOJIRA_PLATFORM_API | 필수 | `config.py:17` | Platform-API peer (`/api/v2` 포함) |
| OPENCODE_SERVER_URL | 필수 | `config.py:18` | OpenCode `serve` URL (`http://127.0.0.1:4096` 권장 — IPv4 명시) |
| AGENT_SERVICE_PORT | 선택 | `config.py:21` | 기본 9002 |
| GOJIRA_MONGO_DB | 선택 | `config.py:22` | 기본 `gojira` |
| OPENCODE_SERVER_USERNAME | 선택 | `config.py:23` | OpenCode Basic Auth user. 기본 `opencode` |
| OPENCODE_SERVER_PASSWORD | 선택 | `config.py:24` | OpenCode 가 `OPENCODE_SERVER_PASSWORD` env 로 띄워졌다면 일치 필요. 기본 빈 문자열(=무인증) |

### 4.2 복구 절차

```bash
cd Agent-Service
cp .env.example .env
# 필수 4개 채우기
python -m venv venv
source venv/bin/activate    # Windows: venv\Scripts\activate
pip install -r requirements.txt
python main.py
```

## 5. GoJIRA-App / Platform-App (React 프론트엔드)

### 5.1 `.env` 키

CRA 가 빌드 시점에 `REACT_APP_*` 변수를 번들에 inline. 미설정 시 `src/config.js` 의 default 사용.

| 키 | 종류 | 기본값 (`src/config.js`) | 설명 |
|---|---|---|---|
| PORT | 선택 | (CRA 기본 3000/3001) | dev server 포트 |
| REACT_APP_PLATFORM_API | 선택 | `http://localhost:9000/api/v2` | Platform-API base URL |
| REACT_APP_API_BASE | 선택 | `http://localhost:9001/api/v2` | GoJIRA-API (GoJIRA-App 용) |
| REACT_APP_GOJIRA_API | 선택 | `http://localhost:9001/api/v2` | GoJIRA-API (Platform-App 용) |
| REACT_APP_AGENT_API | 선택 | `http://localhost:9002/api/v2` | Agent-Service |
| REACT_APP_PLANTUML_SERVER | 선택 | `http://localhost:8080/svg/` | PlantUML 렌더링 서버 (GoJIRA-App) |
| REACT_APP_PUMLEX_SERVER | 선택 | `http://localhost:3030` | pumlex 인라인 편집 서버 (GoJIRA-App) |
| REACT_APP_SSO_URL | 선택 | `https://10.227.37.90:443` | SSO endpoint |

> 운영 빌드 시 (`npm run build`) 호스트 다른 백엔드를 보게 하려면 빌드 직전에 `REACT_APP_*` 를 환경변수로 export 해서 webpack 이 inline 하도록 해야 함.

### 5.2 복구 절차

```bash
cd GoJIRA-App   # 또는 Platform-App
cp .env.example .env   # PORT 만 있는 경우는 새로 작성
npm install
npm start              # dev. 운영은 npm run build && nginx/IIS 로 정적 serve
```

## 6. 루트 `.env` (참고)

| 키 | 사용 여부 | 비고 |
|---|---|---|
| OPENAI_API_KEY | ❌ | 어떤 서비스도 안 읽음. 평문 — 외부 도구용 (사용자 수동 호출) 일 수 있어 임의 삭제 안 함 |
| OPENCODE_SERVER_USERNAME | ❌ | start-all.bat 가 export 안 함. Agent-Service 는 `Agent-Service/.env` 만 읽음 |
| OPENCODE_SERVER_PASSWORD | ❌ | 동일 |

**복구 전략**: 루트 `.env` 는 만들 필요 없음. 필요 시 각 서비스의 로컬 `.env` 에 값 추가.

## 7. OpenCode 설정 (`opencode.json` + auth.json)

### 7.1 `opencode.json` (루트, committed)

```json
{
  "$schema": "https://opencode.ai/config.json",
  "model": "<provider/model-id>",
  "small_model": "<provider/small-model-id>",
  "permission": { "bash": "allow", "edit": "allow", ... }
}
```

- **`model`**: 1차 reasoning 모델 (예: `zai-coding-plan/glm-5.1`, `codemate/DSllmOCoderStable`, `openai/gpt-4o`)
- **`small_model`**: 보조 모델 (요약 등)
- **`permission`**: 사용자 확인 없이 자동 허용할 액션
- **provider 등록**은 `opencode.json` 이 아닌 `opencode auth login` (~/.opencode/auth.json)

### 7.2 `~/.opencode/auth.json` (사용자 홈, 환경마다 재발급)

OpenCode CLI 의 `opencode auth login` 실행 시 생성. 토큰/SSO 결과 저장. **git 에 절대 안 들어감.**

새 환경 복구:

```bash
# WSL 또는 Linux/macOS
opencode auth login
# → 프롬프트 따라 provider 선택 + 토큰 입력 또는 SSO

# 이미 .opencode/auth.json 백업본이 있다면 그냥 복사
cp ~/backup/auth.json ~/.opencode/auth.json
chmod 600 ~/.opencode/auth.json
```

### 7.3 `.opencode/agents/gojira.md` (committed)

GoJIRA 워크플로우 에이전트 정의 (시스템 프롬프트 + tools). 8KB. 수정 시 OpenCode 가 cwd 에서 다시 읽으므로 별도 등록 없음.

### 7.4 `.claude/skills/*/SKILL.md` (committed)

워크플로우 스킬 정의. OpenCode 가 cwd 에서 disk 로 읽음 — 새 환경에서 git clone 만 하면 자동으로 들어옴.

## 8. Mongo / Gitea 외부 인프라

`.env` 가 아닌 인프라 자체 설정이지만, 새 환경 셋업 시 반드시 확인:

| 항목 | 값 (사내망 예) | 비고 |
|---|---|---|
| MongoDB host | `10.227.37.89:44330` | 별도 서버. Agent-Service / Platform-API / GoJIRA-API 모두 같은 DB 사용 |
| MongoDB auth | `gojira` / `<password>` | URI 에 `gojira:P%40ssw0rd@host:port/?authSource=admin` 형식 (특수문자는 URL encoding) |
| Gitea host | `10.227.37.92:44350` | Docker 또는 native install — 둘 다 OK. `FixReviewState` 는 docker 미발견 시 자동 skip |
| Gitea org | `gojira` (또는 환경별) | Platform-API `.env` 의 `PLATFORM_GITHUB_ORG` 와 일치 필요 |
| Gitea PAT | 토큰 | scope: `repo` + `admin:org` (org webhook 등록용) |

## 9. 시크릿 마스터 키 (`GOJIRA_ENC_KEY`)

| 항목 | 내용 |
|---|---|
| 코드 위치 | `gojira-shared/secret/crypto.go` |
| 알고리즘 | AES-256-GCM, base64 encoding, `ENC:` 접두사 |
| Default | 코드에 fallback 값 있음 (`G0J1R4-...`) — **운영 환경에선 반드시 override** |
| 영향 범위 | Platform-API + GoJIRA-API 둘 다 같은 키 필요 (양쪽 모두 ENC: 복호화) |
| 키 변경 시 | 모든 `ENC:` 값을 새 키로 다시 암호화 후 `.env` 업데이트 |

암호화 도구:

```bash
cd Platform-API
GOJIRA_ENC_KEY='<key>' go run cmd/encrypt/main.go '<plaintext>'
# → ENC:... 출력
```

## 10. 새 호스트에서 전체 복구 (체크리스트)

1. [ ] `git clone` repo 클론
2. [ ] MongoDB / Gitea 호스트 도달 확인 (`telnet` / `curl`)
3. [ ] 각 서비스 `.env.example` → `.env` 복사 후 placeholder 치환
4. [ ] `GOJIRA_ENC_KEY` 결정 (기존 환경과 동일해야 ENC: 값 복호화 가능)
5. [ ] 시크릿 값을 `cmd/encrypt` 로 ENC: 변환해 `.env` 에 입력
6. [ ] `Platform-API/config/llm.json` — 활성 provider 확인 (외부=OpenAI / 사내=Internal)
7. [ ] Go 서비스 빌드 (`go build -o ...`)
8. [ ] Agent-Service venv 구성 + `pip install`
9. [ ] React 앱 `npm install`
10. [ ] OpenCode 설치 (`curl -fsSL https://opencode.ai/install | bash`) + `opencode auth login`
11. [ ] OpenCode 는 **반드시 repo 클론 디렉토리에서** `opencode serve --port 4096 --hostname 0.0.0.0` 실행
12. [ ] `start-all.bat` (Windows) 또는 개별 실행으로 전 서비스 가동
13. [ ] 브라우저에서 `:3000` (Platform-App) / `:3001` (GoJIRA-App) 접속 확인
14. [ ] SSE 채팅 흐름 확인 (한 번 메시지 보내보기)

## 11. 관련 문서

- `docs/setup-windows.md` — Windows 환경 셋업 절차 (전체)
- `docs/ports.md` — 포트 매핑 (단일 출처)
- `docs/architecture/deployment.md` — 운영 토폴로지 + scale-out
- 각 서비스의 `.env.example` — 키 템플릿 (committed)
