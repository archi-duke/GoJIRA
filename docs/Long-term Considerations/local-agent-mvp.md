# 좁은 안 MVP — 에이전트 로컬 실행 + 웹→로컬 런처(B)

> 기록 일자: 2026-05-22
> 상태: **설계 노트 (실행 대기)** — 다음 세션에서 구현 착수 예정
> 상위 결정: [`architecture-pivot-vscode.md` §2026-05-22 재검토](./architecture-pivot-vscode.md) 의
> "좁은 안" 을 **실행 가능한 수준**으로 구체화한 문서. 충돌 시 pivot 문서의 결정이 우선.

## 0. 한 줄 요약

server-side OpenCode/Agent-Service 를 폐기하고, **에이전트는 사용자 PC 로컬에서 실행**한다.
웹은 조회·리뷰 + "수정" 버튼으로 **로컬 런처(B, 양방향 localhost 서비스)** 를 통해 VS Code +
선택 CLI(opencode | claude)를 소환한다. 편집 결과는 `docs/{user}` 브랜치 push → PR.

## 1. 범위 (제거 / 이전 / 보존)

### 제거 (server-side)
- **Agent-Service 전체** — 컨테이너/코드(`Agent-Service/`)/배포(`deploy/images/agent-service`,
  compose `agent-service` 서비스, `deploy/systemd/opencode@.service` 멀티유저 호스팅, `gojira-user-*.sh`).
- **OpenCode server-side 호스팅** 일체 (systemd, 인스턴스별 토큰/HOME 격리, `opencodeInstances` Mongo 컬렉션).
- **docs/{user} write 경로** — Platform-API 의 EditSession write / worktree commit / `_edit-acquire.sh`
  계열, doc-save write API. (조회용 read 는 유지)
- **docker repos bind-mount 경로 정합** 작업 (방법 A) — 로컬 실행이면 불필요.

> 정확한 파일/엔드포인트 목록은 구현 착수 시 코드 기준으로 재확인 후 확정 (아래 §6 미해결 참조).

### 이전 (server → 사용자 PC 로컬)
- OpenCode/Claude **CLI 실행 위치** → 사용자 로컬 clone.
- **문서 편집** → 로컬 작업본(에디터/IDE + pumlex `pex-vscode` 인라인 PlantUML).
- **저장** → 로컬 git commit → push `docs/{user}` → PR.

### 보존 (server-side 그대로)
- 웹 조회/대시보드/프로젝트 생성, PR 리뷰.
- JIRA-API / Platform-API 의 **read + 프로젝트 init + main read + push 이벤트 SSE**.
- Gitea(원격), Mongo(이슈 캐시).

## 2. 로컬 런처 서비스 (B) — 상세 스펙

**정체**: "웹→로컬 소환 + 상태 회신" 전용 **경량 localhost 데몬**. agent-service 부활 아님 —
prompt/SSE/세션 브리지 로직 없음(CLI 가 자체 처리).

### 2.1 API 표면 (초안)

| 메서드/경로 | 요청 | 응답 | 용도 |
|---|---|---|---|
| `GET /health` | — | `{version, agents:[...], ready}` | 웹이 런처 설치·기동 감지 → "수정" 버튼 활성화 |
| `POST /open` | `{code, user}` | `{status, repoPath}` | clone 보장 + VS Code 오픈 |
| `POST /edit` | `{code, user, file, agent}` | `{status, sessionId?}` | `/open` + 특정 file 오픈 + CLI spawn |
| `GET /status?code=` | — | `{state, branchDiff, lastPush}` | 로컬 작업/세션 진행 상태 |
| `WS /events` (옵션) | — | 진행 스트림 | 실시간 진행률·완료 통지 |

### 2.2 동작 흐름

```
[웹] "수정" 클릭 → fetch http://127.0.0.1:<port>/edit (POST {code,user,file,agent}, 토큰 헤더)
[런처] Origin·토큰 검증
      → 로컬 clone 보장 (없으면 git clone <gitea>/<org>/<code> → docs/{user} 체크아웃)
      → `code <repoPath>/<file>` 오픈
      → 사용자 설정 CLI spawn (opencode | claude) at repoPath
      → 상태/완료 회신 (응답 또는 WS)
[웹] 진행 표시 → 완료 후 문서 뷰 갱신 (또는 PR 생성 안내)
```

### 2.3 보안 (필수 — git/프로세스 실행 데몬)

- **`127.0.0.1` 전용 bind** (외부 인터페이스 노출 금지)
- **Origin 허용목록** = GoJIRA 웹 origin 만 (CORS preflight + `Origin` 헤더 검증)
- **설치별 토큰/핸드셰이크** — 인증 헤더 없는 요청 거부 (임의 웹페이지 `fetch`/CSRF/DNS-rebinding 차단)
- 실행 가능한 명령은 **화이트리스트**(clone/checkout/code/CLI spawn)로 한정 — 임의 명령 실행 경로 금지

### 2.4 사용자별 설정

```jsonc
// %USERPROFILE%\.gojira\launcher.json (예)
{
  "defaultAgent": "opencode",          // opencode | claude
  "repoRoot": "%USERPROFILE%\\GoJIRA-projects",
  "backend": { "platformApi": "http://<host>:9000/api/v2", "gojiraApi": "http://<host>:9001/api/v2" },
  "port": 5599,
  "webOrigin": "http://<gojira-web-host>:3000"
}
```
SSO/토큰은 설정에 넣지 않음 — 첫 실행 시 각 CLI 자체 로그인(`opencode auth login` / `claude` 로그인),
토큰은 로컬 사용자 홈에 저장.

### 2.5 Windows 배포/등록

- 경량 런처(exe). **NSSM** 또는 작업 스케줄러(logon 트리거)로 **사용자 세션** 상주.
- **HKCU** 설치(관리자 권한 불필요), 사내 배포 패키지로 푸시.
- 전제 툴체인: `code` / (`opencode`|`claude`) / `git` / 스킬용 `bash`·`jq`·`python3` (WSL 또는 Git Bash).
- 패키지 부트스트랩: 런처 설치 + 설정 생성 + (옵션)repo 초기 clone + 툴체인 점검.

## 3. CLI 선택 — repo 가 양쪽을 이미 지원

| CLI | repo 내 설정 |
|---|---|
| **OpenCode** | `.opencode/agents/gojira.md` + `opencode.json` (model = 사내 DSllm-Ocoder) |
| **Claude Code** | `.claude/skills/*` + `CLAUDE.md` (네이티브) |

OpenCode 도 `.claude/skills/*/SKILL.md` 를 읽도록 돼 있어 **두 CLI 가 동일 스킬 공유**. 런처는
`agent` 값으로 바이너리만 선택하면 됨.

## 4. 문서 편집 + 충돌 모델

- 사용자 브랜치 `docs/{user}` 의 **writer 는 로컬 작업본 1개** → 브랜치 내부 충돌 0.
- 사용자 간 충돌은 `docs/{user}` → main **PR 머지 시점**에서 표준 git 리뷰/머지로 해소
  (기존 커스텀 EditSession 락 대체).
- 웹은 브랜치 내용 **읽기 전용** 조회 — 서버 worktree 불필요(`git show <branch>:<path>` 류).
- 인라인 PlantUML 편집: 로컬은 pumlex `pex-vscode` 확장, 웹은 조회용 PexBlock(이미 배포됨, 동일 pex-core).

## 5. 권장 구현 순서 (다음 세션)

1. ~~**런처 PoC** — `/health` `/open` `/edit` + 보안(127.0.0.1·Origin·토큰). opencode/claude spawn.~~
   **✅ 완료 (2026-05-22)** — `gojira-launcher/` (Go, zero-dep stdlib, 단일 exe). `/health`
   `/open` `/edit` `/status` + Origin 허용목록 + 설치 토큰(`~/.gojira/launcher-token`) +
   명령/입력 화이트리스트(path traversal·agent 검증). clone→`docs/{user}` checkout→VS Code+CLI.
   Windows 크로스컴파일 확인. 상세: [`gojira-launcher/README.md`](../../gojira-launcher/README.md).
2. ~~**웹 "수정" 버튼** — 런처 감지(`/health`) → `/edit` 호출 → 상태 표시.~~
   **✅ 완료 (2026-05-22)** — `GoJIRA-App`: `src/utils/launcher.js`(detect/requestEdit,
   AbortController 타임아웃), `config.LAUNCHER_BASE`(기본 `http://127.0.0.1:5599`),
   `DocEditor` 툴바에 "로컬 수정" 버튼(런처 `ready` 감지 시 노출, 포커스 복귀 시 재감지).
   토큰 핸드셰이크(§6 해소안): 런처 `/health` 가 **Origin 일치 요청에만 `token` 반환** →
   웹이 `/edit` 헤더로 전송. (DNS-rebinding 잔여 리스크는 §6 유지)
3. **docs/{user} push→PR 흐름** 정착 + 웹 PR 리뷰 연동. ← **다음 착수점**
4. **server-side 제거** — Agent-Service/OpenCode 호스팅/EditSession write 단계적 제거(롤백 가능하게 마지막).
5. **Windows 패키지** — 런처 설치 + 부트스트랩.

## 6. 미해결 / 다음 세션에서 확정할 것

- 제거 대상 **파일/엔드포인트 정확한 목록** (코드 기준 재확인).
- 웹 채팅 UI 존치 여부 — 폐기(편집은 CLI/IDE) vs 로컬 `opencode serve` 직결(client-side 브리지 부활).
- `agent-workflow-evolution.md` 의 **서버측 State Machine(jobState) 로드맵 재설계** (로컬 실행 전제와 충돌).
- 비개발자 stakeholder 보완(코멘트/export) 범위.
- ~~런처 ↔ 웹 토큰 핸드셰이크 구체 방식.~~ → **PoC 채택 (2026-05-22)**: 런처가 첫 기동 시
  토큰 생성(`~/.gojira/launcher-token`), `/health` 가 Origin 일치 요청에만 토큰을 반환,
  웹이 이후 호출 헤더에 사용. **잔여**: DNS-rebinding 방어(Host 헤더 검증 등), 토큰 회전.
- VS Code 확장(공식 OpenCode 확장 vs GoJIRA 전용) 채택 여부 — full pivot Phase 2 와 연계.

## 관련 문서
- [`architecture-pivot-vscode.md`](./architecture-pivot-vscode.md) — 상위 방향/결정 로그
- [`agent-workflow-evolution.md`](./agent-workflow-evolution.md) — (보류/재설계 대상) 서버측 워크플로우 진화
- `docs/setup-windows.md` — 현행 server-side Windows 셋업 (전환 후 갱신 예정)
