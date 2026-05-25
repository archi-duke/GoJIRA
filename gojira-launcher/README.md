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
  "repoRoot": "~/GoJIRA-projects",     // 로컬 clone 루트 (기본값)
  "backend": {
    "platformApi": "http://<host>:9000/api/v2",
    "gojiraApi": "http://<host>:9001/api/v2"
  },
  "port": 5599,
  "webOrigin": "http://<gojira-web-host>:3000",
  "git": { "base": "http://<gitea-host>:3000", "org": "gojira" }
}
```

- 파일이 없어도 기본값(port 5599, agent opencode, repoRoot `~/GoJIRA-projects`)으로 뜬다.
- 단, `webOrigin` 미설정 시 모든 브라우저 요청을 거부하고, `git.base` 미설정 시 clone 이 거부된다.
- **SSO/CLI 토큰은 설정에 넣지 않는다** — `opencode auth login` / `claude` 가 첫 실행 시
  자체 로그인하고 토큰을 로컬 홈에 저장한다.

## 빌드 / 실행

로컬에 Go 가 없으면 도커 이미지로 빌드한다(레포 표준 `golang:1.26-alpine`).

```bash
# 현재 OS 용
docker run --rm -v "$PWD":/src -w /src -e CGO_ENABLED=0 golang:1.26-alpine \
  go build -o gojira-launcher .

# Windows exe 크로스컴파일 (§2.5 배포용)
docker run --rm -v "$PWD":/src -w /src \
  -e GOOS=windows -e GOARCH=amd64 -e CGO_ENABLED=0 golang:1.26-alpine \
  go build -o gojira-launcher.exe .

./gojira-launcher        # 127.0.0.1:5599 에서 listen
```

전제 툴체인: `git`, `code`(VS Code), (`opencode`|`claude`). `/health` 의 `ready` 는
이 셋이 모두 PATH 에 있을 때 `true`.

## 동작 흐름 (§2.2)

```
[웹] "수정" → fetch http://127.0.0.1:5599/edit (POST {code,user,file,agent} + 토큰)
[런처] Origin·토큰 검증
      → 로컬 clone 보장 (없으면 git clone <git.base>/<org>/<code> → docs/{user} 체크아웃)
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
