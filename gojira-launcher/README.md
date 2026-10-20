# gojira-launcher

웹→로컬 소환 전용 **경량 localhost 데몬**. 사용자 PC 에서 동작하며, GoJIRA 웹의 "수정"
버튼이 누르면 로컬 clone 을 보장하고 VS Code + 에이전트 CLI(`opencode` | `claude`)를
띄운다. 편집 결과는 `docs/{user}` 브랜치 push → PR 로 흐른다.

> 설계 근거: [`docs/Long-term Considerations/local-agent-mvp.md`](../docs/Long-term%20Considerations/local-agent-mvp.md) §2.
> **agent-service 부활이 아니다** — prompt/SSE/세션 브리지 로직 없음(CLI 가 자체 처리). 런처는
> "소환 + 상태 회신"만 한다.

이것은 §5 구현 순서 **1단계(런처 PoC)** 산출물이다.

## API (§2.1)

| 메서드/경로 | 요청 | 응답 | 토큰 |
|---|---|---|---|
| `GET /health` | — | `{version, agents:[...], ready}` | 불필요 |
| `POST /open` | `{code, user}` | `{status, repoPath}` | 필요 |
| `POST /edit` | `{code, user, file, agent}` | `{status, repoPath, agent, sessionId}` | 필요 |
| `GET /status?code=` | — | `{state, branch, ahead, behind, dirty, lastCommit}` | 필요 |

## 보안 (§2.3)

1. **127.0.0.1 전용 bind** — 외부 인터페이스 노출 금지.
2. **Origin 허용목록** — 브라우저發 요청(`Origin` 헤더 존재)은 `webOrigin` 과 일치해야 함.
   CORS preflight(OPTIONS) 처리 포함. CSRF / DNS-rebinding 차단.
3. **설치 토큰** — `/open` `/edit` `/status` 는 `X-Gojira-Launcher-Token`(또는
   `Authorization: Bearer`) 필요. 토큰은 첫 기동 시 `~/.gojira/launcher-token` 에 자동
   생성(0600)되며, 로그에 위치를 안내한다. 웹은 이 값을 헤더로 전송해야 한다.
   (구체 핸드셰이크 방식은 설계 §6 미해결 — PoC 는 파일 공유 토큰.)
4. **명령 화이트리스트** — `git clone/checkout/...`, `code`, `opencode|claude` 만 실행.
   `code`/`user`/`file` 은 영숫자·`. _ -` 만 허용하고 path traversal(`..`)을 차단한다.

## 설정 (§2.4)

`~/.gojira/launcher.json` (Windows: `%USERPROFILE%\.gojira\launcher.json`):

```jsonc
{
  "defaultAgent": "opencode",          // opencode | claude
  "repoRoot": "~/GoJIRA-projects",     // 로컬 clone 루트 (기본값). 실제 clone 은 <repoRoot>/<user>/<code>
  "backend": {
    "platformApi": "http://<host>:9000/api/v2",
    "gojiraApi": "http://<host>:9001/api/v2"
  },
  "port": 5599,
  "webOrigin": "http://<gojira-web-host>:3000",
  "git": {
    "base": "http://<gitea-host>:3000",
    "org": "gojira",
    "user": "<gitea-username>",      // 폐쇄망 권장 — 자동 인증
    "token": "<gitea-PAT>"           // user+token 둘 다 채우면 clone URL 에 자동 임베드
  }
}
```

- 파일이 없어도 기본값(port 5599, agent opencode, repoRoot `~/GoJIRA-projects`)으로 뜬다.
- 단, `webOrigin` 미설정 시 모든 브라우저 요청을 거부하고, `git.base` 미설정 시 clone 이 거부된다.
- **SSO/CLI 토큰은 설정에 넣지 않는다** — `opencode auth login` / `claude` 가 첫 실행 시
  자체 로그인하고 토큰을 로컬 홈에 저장한다.

### `git.user` / `git.token` — 폐쇄망 자동 인증

폐쇄망 Gitea 처럼 익명 접근을 차단하는 환경에서는 `git clone/fetch/push` 마다 인증 prompt 가 뜨고,
Git Credential Manager 의 OAuth 흐름이 launcher 가 띄운 자식 프로세스의 lifecycle 과 어긋나
인증을 받지 못한다 — 인증 페이지에서 "성공"이라 떠도 자식 프로세스는 이미 timeout 된 후.

`git.user` + `git.token` 을 채우면 launcher 가 clone URL 에 `http://<user>:<token>@host/...` 형태로
PAT 를 임베드해 GCM/netrc 없이도 자동 인증. 기존에 익명으로 clone 된 폴더가 있어도
launcher 가 다음 호출 때 origin URL 을 자동 갱신한다.

## 서브커맨드 (단일 exe)

배포 exe 하나가 데몬 실행 + 설치/제거 + 자동 업데이트를 모두 처리한다.

| 명령 | 동작 |
|---|---|
| `gojira-launcher` | 데몬 실행(기본). 기동 시 임베드 스킬 추출 + 자동 업데이트 체크. |
| `gojira-launcher install` | 자기 자신을 설치 위치로 복사 → 스킬 배포 → `launcher.json`/토큰 생성 → 자동시작 등록(Windows HKCU Run) → 데몬 기동. **최초 1회만 실행하면 끝.** |
| `gojira-launcher uninstall` | 자동시작 해제 + 설치 exe 정리(설정/스킬은 보존). |
| `gojira-launcher version` | 버전 출력. |

### 에이전트 자산 배포 (스킬 + opencode 설정)

다음 셋을 **exe 에 embed** 한다(`//go:embed all:assets`, 빌드 시 레포 루트에서 동기화):

| 엔트리 | 용도 | 읽는 CLI |
|---|---|---|
| `.claude/skills` | GoJIRA 스킬(SKILL.md) | claude, opencode |
| `opencode.json` | opencode 모델·권한 설정 | opencode |
| `.opencode/agents/gojira.md` | opencode `primary` 에이전트 정의 | opencode |

첫 기동/버전 변경 시 `skillsSourceDir`(기본 `~/.gojira/skills`) 아래로 추출되고, 각 프로젝트
clone 에 symlink/junction(단일 파일 `opencode.json` 은 복사)으로 연결된다. 에이전트는
cwd=clone 에서 spawn 되므로(설계 §5-2) 두 CLI 모두 스킬+설정을 그대로 인식한다.
사용자가 clone 에 직접 둔 동일 이름 파일이 있으면 건드리지 않는다.
→ **폐쇄망에 exe 단일 파일만 전달하면 스킬+에이전트 설정까지 배포된다.**

### 자동 업데이트 (ClickOnce 식)

기동 시 1회 + `update.intervalMinutes` 주기로 백엔드 매니페스트
(`<platformApi>/launcher/version`)를 확인한다. 임베드된 자기 버전보다 높으면 새 exe 를
`<platformApi>/launcher/download/{os}` 에서 받아 **sha256 검증** 후 staged swap(현재 exe →
`.old`, 새 exe 배치) 하고 재기동한다. 스킬은 exe 에 임베드돼 있어 함께 갱신된다.

- 설정: `update.{manifestUrl, checkOnStart, intervalMinutes}`. `manifestUrl` 생략 시
  `backend.platformApi` 에서 유추. `intervalMinutes` 음수면 주기 체크 끔.
- 폐쇄망 OK — 매니페스트/바이너리를 사내 백엔드(platform-api)가 서빙(`deploy/launcher-dist`
  bind mount). 배포물은 `deploy/scripts/build-launcher.sh` 가 OS별 빌드 + `version.txt` +
  `.sha256` 까지 생성.

## 빌드 / 실행

배포용 전체 빌드는 외부망에서 한 번:

```bash
deploy/scripts/build-launcher.sh           # windows/linux/darwin 전부 → deploy/launcher-dist/
deploy/scripts/build-launcher.sh --os windows
```

개발 중 단건 빌드(스킬 미동기화 — 임베드 없이도 컴파일됨):

```bash
docker run --rm -v "$PWD":/src -w /src -e CGO_ENABLED=0 -e GOWORK=off golang:1.26-alpine \
  go build -o gojira-launcher .
./gojira-launcher        # 127.0.0.1:5599 에서 listen
```

전제 툴체인: `git`, `code`(VS Code), (`opencode`|`claude`). `/health` 의 `ready` 는
이 셋이 모두 PATH 에 있을 때 `true`.

## 동작 흐름 (§2.2)

```
[웹] "수정" → fetch http://127.0.0.1:5599/edit (POST {code,user,file,agent} + 토큰)
[런처] Origin·토큰 검증
      → 사용자별 로컬 clone 보장 (<repoRoot>/<user>/<code>, 없으면 git clone → docs/{user} 체크아웃.
        분리 이전 공유 clone <repoRoot>/<code> 가 docs/{user} 상태면 그대로 이관)
      → code <repoPath> + code -r -g <repoPath>/<file>
      → 새 터미널에서 opencode|claude (repoPath)
      → {status, sessionId} 회신
```

## PoC 한계 / 다음 단계

- CLI spawn 은 OS별 터미널(Windows `cmd /k`, macOS Terminal, Linux x-terminal-emulator)로
  대화형 실행한다. 비대화 환경(헤드리스 Linux)은 detached 폴백 — VS Code 통합 터미널
  연동은 추후 과제(설계 §6 VS Code 확장 채택과 연계).
- 웹↔런처 토큰 핸드셰이크는 파일 공유 토큰(PoC). 자동 핸드셰이크는 §6 미해결.
- §5 다음 단계: 2) 웹 "수정" 버튼(`/health` 감지 → `/edit`), 3) `docs/{user}` push→PR,
  4) server-side 제거, 5) Windows 패키지(NSSM/작업 스케줄러, HKCU).
