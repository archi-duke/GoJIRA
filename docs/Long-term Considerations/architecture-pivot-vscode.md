# 시스템 방향 전환 — Web 조회 + VS Code 편집 아키텍처

> 기록 일자: 2026-04-21
> 상태: **'좁은 안(에이전트 로컬 실행)' 우선 추진 결정** (2026-05-22 갱신) — 이전 '실행 보류 중'(2026-05-01)에서 전환.
>
> 2026-05-22 재검토: 폐쇄망에 GoJIRA 를 배포하고 OpenCode 를 server-side 로 호스팅하던 중
> server-side 모델 고유의 구조적 한계(개인 SSO 토큰 격리, docker↔host git worktree 절대경로
> 불일치, Agent-Service 브리지 복잡도)가 드러났고, 보류 사유였던 웹 pumlex 인라인 편집이
> 배포 완료되어 재검토 트리거 충족. → full pivot 전체가 아니라 **Phase 1(에이전트 로컬 실행 +
> `docs/{user}` push/PR)만 "좁은 안"으로 우선 추진.** 상세는 아래 「## 2026-05-22 재검토」 참조.
>
> (2026-05-01 보류 사유 — 해소됨) 웹 측 사용성 개선(pumlex 인라인 PlantUML 편집 등)을 우선
> 진행 → 2026-05-22 배포 완료. VS Code full pivot(편집 UI 전면 이전)은 장기 목표로 보존.
> 자세한 배경은 `design-application.md` §3.7.1 참조.

## 2026-05-22 재검토 — 폐쇄망 한계로 '좁은 안(에이전트 로컬 실행)' 우선 추진

> 이 절은 아래 full pivot 설계를 **폐쇄망 배포 경험에 비추어 재평가**하고, 실제로 먼저 실행할
> **축소 범위**를 확정한다. 아래 「## 최종 아키텍처」(full pivot, VS Code 편집 전면 이전)는
> **장기 목표로 보존**하되, 1차로는 본 절의 좁은 안만 추진한다.

### 재검토 트리거 — server-side OpenCode 의 구조적 한계

폐쇄망에 GoJIRA 를 docker 로 배포하고 OpenCode 를 사내 서버에 호스팅하는 과정에서, server-side
모델에서만 발생하는 한계가 드러났다:

| 한계 | 내용 |
|------|------|
| **개인 SSO 토큰 격리** | OpenCode 인증(`opencode auth login` → `~/.opencode/auth.json`)은 **실행 OS 계정 홈에 묶임**. 공용 서비스 계정(`gojira`)으로 다수 사용자를 돌리면 개인별 SSO 토큰을 인스턴스별 `HOME` 으로 격리·반입·만료갱신해야 함 → 운영 부담 + SSO 만료 시 무응답. |
| **docker↔host git worktree 절대경로 불일치** | Platform-API(컨테이너, `/data/repos`)가 만든 git worktree 의 gitdir **절대경로**를, 도커 밖 OpenCode(호스트 `${GOJIRA_REPOS_DIR}`)가 못 따라가 깨짐. 안/밖 경로를 동일하게 강제해야 겨우 동작. |
| **Agent-Service ↔ OpenCode 브리지** | prompt_async / SSE / 세션 라우팅 / 클라이언트-서버 버전 정합 등 지속적 복잡도. |

동시에 2026-05-01 보류 사유였던 **웹 pumlex 인라인 PlantUML 편집이 배포 완료(2026-05-22)** 되어
"웹 사용성 우선" 블로커가 해소 → 재검토 트리거 충족.

### 결정 — full pivot 의 Phase 1 만 먼저 (좁은 안)

- **하는 것**: 에이전트 실행 위치를 server-side → **사용자 PC 로컬**로 이동. 사용자가 자기 로컬
  clone 에서 OpenCode 실행(오피스 PC 에 이미 설치됨), 편집 결과를 `docs/{user}` 브랜치로 push → PR.
- **안 하는 것(당장)**: GoJIRA 전용 VS Code 확장 빌드, RelationMap/SATAM 의 webview 이전,
  공유 컴포넌트 NPM 패키징(= full pivot Phase 2~3). **장기 목표로 보존.**
- **근거**: 위 server-side 한계(토큰/경로/브리지)는 **에이전트를 로컬로 옮기는 것만으로 구조적으로
  소멸**. full pivot 의 가장 무거운 부분(확장/공유 컴포넌트) 없이도 운영 이득의 대부분을 확보.

### 문서 편집 환경 변화 (핵심 확인 사항)

현재는 **사람(웹 DocEditor)과 에이전트(스킬)가 같은 서버측 worktree 를 공유**하며 `docs/{user}` 를
편집하고, EditSession 락(`holderKind` = ui/skill)으로 조율한다(`Platform-API` + `_edit-acquire.sh`).

에이전트가 로컬 clone 에서 편집·push 하면 **같은 브랜치에 writer 가 둘(서버 worktree + 로컬 clone)**
이 되어 diverge/충돌. 따라서 **사람의 문서 편집도 로컬 작업본으로 함께 이동**해야 한다 — 이 한
가지만큼은 좁은 안이 full pivot 의 편집 모델(웹 read-only, 로컬 편집)로 **수렴**한다.

| | 변경 전 | 변경 후 |
|---|---|---|
| 편집 위치 | 웹 DocEditor (서버 worktree) | 사용자 PC 로컬 clone |
| 편집 도구 | 웹 에디터 + 웹 pumlex PexBlock | 로컬 에디터/IDE + pumlex `pex-vscode` 확장(인라인 PlantUML) |
| 저장 경로 | Platform-API write API → 서버 worktree commit | 로컬 git commit → push `docs/{user}` → PR |
| 조율 | EditSession ui/skill 락 | 불필요 (사용자 브랜치 writer 1명) |
| 웹의 역할 | 편집 + 조회 | 조회 + PR 리뷰 |

**충돌 모델**: 사용자 브랜치 내부는 writer 1명 → 충돌 0. 사용자 간 충돌은 `docs/{user}` → main
**PR 머지 시점**으로 이동 → 표준 git 리뷰/머지로 처리(기존 커스텀 EditSession 락 대체). 서버 사본은
읽기 전용이라 worktree 없이 브랜치 내용 조회만 하면 됨. (충돌이 사라지는 게 아니라 **정상적인
자리로 이동**.)

### 트레이드오프 (수용 전제)

- 비개발자 stakeholder 직접 편집 불가 → 코멘트/PR 리뷰/export 로 보완 (아래 §6 과 동일).
- 사용자 PC 셋업 분산(로컬 clone·OpenCode·SSO·백엔드 도달). 단 오피스 PC 엔 OpenCode 기설치,
  앱·Gitea 도달 가능 → 진입장벽 낮음.
- 중앙 채팅/세션 이력 소멸(로컬 세션만) → 감사 추적은 PR/이슈 로그로 대체.
- `agent-workflow-evolution.md` 의 **서버측 State Machine(jobState) 로드맵은 로컬 실행 전제와
  충돌** → 보류/재설계 필요(Plan 모드 포함).

### 1차 실행 범위 (세부는 별도 MVP 설계 노트로)

- **제거**: Agent-Service(컨테이너/코드/배포), OpenCode server-side 호스팅(systemd 멀티유저/토큰
  격리), docker repos bind-mount 경로 정합 작업, EditSession write 경로.
- **이전**: 사용자 로컬 OpenCode 실행 가이드, `docs/{user}` 로컬 편집 → push → PR 흐름,
  pumlex `pex-vscode` 인라인 편집.
- **보존**: 웹 조회/대시보드/프로젝트 생성, JIRA-API/Platform-API 의 read + 프로젝트 init, PR 리뷰.

### 웹 → 로컬 소환 — 로컬 런처 서비스 (양방향, **채택: B안**)

§10("프로젝트 열기" 딥링크)을 구체화한다. 웹에서 "수정"을 누르면 **로컬 VS Code + 사용자 설정의
에이전트 CLI(opencode | claude)** 를 소환한다. 두 방식 중 **B 채택**:

- **(A) 프로토콜 핸들러** (`gojira://...`): 상주 데몬 없음, URI 열릴 때 1회 실행. **단방향**(웹→로컬 트리거만).
- **(B) 로컬 런처 서비스** *(채택)*: localhost 에서 listen. **양방향** — 웹이 설치·기동 감지, 진행상태/결과
  수신, 완료 후 자동 갱신 가능. 양방향 UX 가 필요하므로 B.

**정체 (중요)**: 이 런처는 **agent-service 부활이 아니다.** prompt/SSE/세션 라우팅 같은 에이전트 브리지
로직은 없다(그건 CLI 자신이 처리). 역할은 **"소환 + 상태 회신" 전용 경량 데몬**뿐이다.

흐름:
```
[웹] "수정" 클릭 → fetch http://127.0.0.1:<port>/edit  (POST {code, user, file, agent})
[로컬 런처]  Origin·토큰 검증 → 로컬 clone 보장 → `code <repo>/<file>` 오픈
            → 사용자 설정 CLI spawn (opencode | claude, repo cwd)
            → 진행상태/완료를 웹에 회신 (응답 또는 WS/SSE)
[웹] 상태 표시, 완료 후 문서 뷰 갱신
```

API 표면(초안):

| 엔드포인트 | 용도 |
|---|---|
| `GET /health` | 런처 설치·기동 여부 (웹이 "수정" 버튼 활성화 판단) |
| `POST /open` | 프로젝트 clone 보장 + VS Code 오픈 |
| `POST /edit` | `/open` + 특정 `file` 오픈 + agent CLI spawn |
| `GET /status?code=` | 로컬 작업/세션 진행 상태 |
| `WS /events` (옵션) | 진행 스트림 |

사용자별 설정: 기본 agent(`opencode`|`claude`), repo 루트 경로, 백엔드 URL, 포트. 첫 실행 시 CLI 자체
로그인(SSO) → 토큰은 로컬.

**보안 (필수 — git/프로세스 실행 데몬이므로)**:
- `127.0.0.1` 만 bind (외부 노출 금지)
- **Origin 허용목록** = GoJIRA 웹 origin 만 수락 (CORS preflight + Origin 검증)
- **설치별 토큰/핸드셰이크** — 인증 없는 요청 거부 (아무 웹페이지의 `fetch` 차단; CSRF/DNS-rebinding 방어)

**Windows 배포/등록**: 경량 런처(exe). **NSSM** 또는 작업 스케줄러(logon 트리거)로 사용자 세션 상주.
HKCU 설치(관리자 불필요), 사내 배포 패키지로 푸시. 전제: `code` / `opencode`|`claude` / `git` /
(스킬용) `bash`·`jq`·`python3` (WSL 또는 Git Bash).

**관계**: 서버 측 Agent-Service 제거는 그대로. 이 런처는 그 대체가 아니라 "웹→로컬 소환" 전용 컴포넌트.
(웹 안에서 채팅까지 원하면 런처 대신/병행해 `opencode serve` localhost:4096 에 브라우저 직결도 가능하나,
1차 범위는 "소환 + 편집 + 상태"로 한정.)

### 1차 실행 범위 (세부는 별도 MVP 설계 노트로) — 갱신
- 추가: **로컬 런처 서비스(B)** 패키징·등록·보안 잠금, 웹 "수정" 버튼 ↔ 런처 연동.

---

## 배경

현재 GoJIRA 는 웹에 편집(DocEditor) + 실시간 에이전트 채팅(AgentChat) + SSE 스트리밍을 모두 탑재해 운영 중이다. 채팅 스트림 안정화, `docs/{user}` 브랜치 관리, Agent-Service ↔ OpenCode 브리지 등 **웹 + 서버 브리지에서만 해결해야 할 복잡도가 지속적으로 누적**되고 있다.

엔지니어(타겟 편집자) 는 본래 VS Code 환경에 익숙하고, OpenCode 도 공식 VS Code 확장을 제공한다. 편집 작업을 VS Code 로 옮기고 웹은 조회/대시보드로 한정하면 **복잡도를 구조적으로 줄일 수 있다**고 판단했다.

## 최종 아키텍처

```
   ┌──────────────────────────┐         ┌──────────────────────────┐
   │  Web (read-only)         │         │  VS Code + 확장           │
   │  - 이슈/문서 조회         │         │  - OpenCode 확장 (채팅)   │
   │  - 프로젝트 생성          │         │  - Diff / RelationMap /   │
   │  - 대시보드               │         │    SATAM (편집)          │
   │  - Diff / RelationMap /  │         │  - 프로젝트 clone/open   │
   │    SATAM (읽기)          │         │  - 스킬(.claude/skills)  │
   │  - SSO 로그인             │         │    로컬 OpenCode 가 실행 │
   └─────────┬────────────────┘         └──────────┬───────────────┘
             │                                      │
             ▼                                      ▼
   ┌──────────────────────────────────────────────────────────────┐
   │  Platform-API (축소): main read + 프로젝트 init + SSE 브로드  │
   │  JIRA-API: 이슈 CRUD                                          │
   └──────────────────────────────────────────────────────────────┘

   ※ Agent-Service 폐기, docs/{user} 브랜치 및 관련 write API 삭제
```

## 핵심 결정 사항

### 1. 역할 분리
- **Web**: PR 이 머지된 main 내용만 조회. 편집 기능 없음. 프로젝트 생성 / 대시보드 / 코멘트(이슈에 대한) 기능 유지.
- **VS Code 확장**: 편집의 유일한 진입점. 로컬 clone + 로컬 OpenCode 실행.

### 2. Agent-Service 폐기
- OpenCode 가 사용자 PC 에서 로컬 실행되므로 브리지 역할 소멸.
- 워크플로우 stage 는 repo 내용에서 계산 (`project-status.sh` 방식 유지) — 별도 state 저장소 불필요.
- 채팅 이력은 OpenCode 로컬 세션만 사용, 중앙 저장소 불필요.
- LLM 키는 사용자 PC 로컬에서 관리 (사내 프록시 적용 여부는 별도 정책).
- 결과: Agent-Service 프로세스/코드/배포 전부 제거.

### 3. Platform-API 축소
유지:
- main 브랜치 read (tree, file)
- 프로젝트 생성 / 레포 init (웹에서 트리거)
- main push 이벤트 SSE 브로드캐스트 (웹 실시간 갱신용)

삭제:
- `docs/{user}` 브랜치 관련 모든 엔드포인트 (tree, file read/write, commit)
- worktree 관리 / EnsureWorktree
- doc-save 계열 write API
- init-files 등 사용자 브랜치 초기화 API

### 4. 사용자 인증
- 사내 SSO 연동.
- 웹: 기존 SSO 플로우.
- VS Code 확장: SSO 로그인 → `secrets` 저장 → API 호출 시 헤더 주입.

### 5. 공유 컴포넌트 (RelationMap / SATAM)
- 웹에서는 읽기, VS Code 에서는 편집 모드로 동일 컴포넌트 사용.
- 배포 방식: **초기엔 NPM 패키지**로 양쪽 번들. 안정화 후 Module Federation 검토.
- 컴포넌트 계약:
  - props: `{ mode: 'view' | 'edit', host: { save, load, openFile } }`
  - 저장/로드 I/O 는 호스트 주입 callback 으로 분리 (web 은 Platform-API 호출, VS Code 는 로컬 git).
  - 테마: CSS 변수로 추상화해 VS Code dark/light 및 웹 스타일 모두 대응.

### 6. 비개발자 Stakeholder 지원
가장 큰 약점으로 인식. 보완 전략:
- 웹에 이슈 코멘트 기능 유지 (JIRA 스타일 — read + comment 까지 허용).
- 필요 시 주요 산출물 PDF/HTML 스냅샷 export 버튼 추가.
- 편집 권한 없이 피드백은 Slack/Email → 엔지니어가 반영.

### 7. 감사 로그
- 스킬의 쓰기 호출은 모두 JIRA-API / Platform-API 를 경유하므로, 이들의 기존 access log 가 감사 추적으로 충분하다고 판단.
- 별도 텔레메트리 수집 불필요.

### 8. 마이그레이션
- 기존 `docs/{user}` 브랜치 전부 삭제 (데이터 캐리 없음).
- 기존 프로젝트는 main 브랜치 내용만 유지.

### 9. VS Code 확장 배포
- 사내 private extension registry 또는 수동 `.vsix` 배포.
- 자동 업데이트 채널은 registry 채택 후 결정.

### 10. "프로젝트 열기" 흐름
- 웹 프로젝트 카드에 "VS Code 에서 열기" 버튼 → `vscode://gojira.ext/open?code=...` 딥링크 → 확장이:
  - 로컬 clone 이 있으면 workspace 오픈
  - 없으면 지정 경로(예: `~/GoJIRA-projects/{code}`)에 clone 후 오픈

## 구현 전 남은 세부 설계 항목

1. **VS Code 확장 기능 스코프 확정**
   - OpenCode 확장 채팅 UI 그대로 사용 + GoJIRA 전용 panel 3개(Diff / RelationMap / SATAM)
   - 이슈 CRUD UI 를 VS Code 에 만들지, 계속 웹에서만 수행할지

2. **SSO 토큰 주입 경로**
   - VS Code 확장이 획득한 토큰을 OpenCode 가 실행하는 스킬의 curl 호출에 어떻게 전달할지
   - 환경변수 / 로컬 프록시 / 헤더 주입 레이어 중 선택

3. **Shared 모듈의 호스트 주입 인터페이스**
   - save/load/openFile 시그니처 확정
   - 테마 변수 네이밍 합의

4. **제거 스펙 (파일/엔드포인트 레벨)**
   - Agent-Service 디렉토리 통째 삭제
   - Platform-API 에서 삭제할 핸들러 목록
   - GoJIRA-App 에서 삭제할 컴포넌트 목록(DocEditor, AgentChat, 관련 라우팅)
   - 워크플로우 스킬의 `${AGENT_API}` 참조 제거 지점

## 권장 MVP 단계

### Phase 1 — 웹 축소 + Agent-Service 폐기
- DocEditor / AgentChat 제거
- `docs/{user}` 브랜치 관련 API 삭제
- Agent-Service 폐기
- 웹은 조회/대시보드/프로젝트 생성만 남김

### Phase 2 — VS Code 확장 MVP
- OpenCode 공식 확장 설치 가이드
- GoJIRA 확장: 프로젝트 clone/open 명령, 간단한 Diff 뷰
- SSO 토큰 주입 파이프라인

### Phase 3 — 편집 가능한 RelationMap / SATAM
- 공유 컴포넌트 NPM 패키지화
- VS Code webview 에 마운트
- 웹은 동일 컴포넌트를 view 모드로 재사용

## 상대적 강점 정리

- 구조적 복잡도 대폭 감소: SSE 스트리밍, 워크플로우 state, `docs/{user}` 브랜치 관리, OpenCode 브리지 전부 제거.
- 편집자 UX 개선: IDE 의 Git / LSP / 터미널을 그대로 활용.
- LLM 정책 대응: 로컬 실행으로 키 관리 / 프록시 정책 유연화.
- 공유 컴포넌트 재사용: 읽기/편집 모드만 스위치해서 양쪽 호스트 지원.

## 상대적 약점 / 리스크

- 비엔지니어 stakeholder 는 편집 참여 불가 → 코멘트/export 로 보완해야 함.
- VS Code 로 IDE 고정 → JetBrains 등 다른 IDE 사용자 배제.
- VS Code 확장 배포/버전관리 인프라 구축 필요.
- 공유 컴포넌트 테마/CSP 대응 비용.
- OpenCode 공식 확장의 성숙도에 의존.

## 기록 메모

- 당일 논의: 2026-04-21, `skill_v2` 브랜치 진행 중 발생한 SSE 스트림 안정화 / OpenCode delta 이벤트 처리 작업을 거치며 구조적 한계를 재검토.
- Agent-Service 의 `opencode_bridge` 코드는 `76dcbbb` 기준 기능적으로 작동하지만, 이 전환 이후 Phase 1 단계에서 폐기 예정.
- 2026-05-22: 폐쇄망 server-side OpenCode 운영 한계(개인 SSO 토큰 격리 · docker↔host git worktree 절대경로 불일치 · Agent-Service 브리지)와 웹 pumlex 인라인 편집 배포 완료를 근거로 재검토. **full pivot 의 Phase 1(에이전트 로컬 실행 + `docs/{user}` push/PR)을 "좁은 안"으로 우선 추진** 결정. 문서 편집 환경은 로컬 작업본으로 이전(웹은 조회·리뷰로 축소, EditSession 락 제거, 충돌은 PR 머지로 이동), full VS Code 확장/공유 컴포넌트는 장기 보존. 상세는 「## 2026-05-22 재검토」.
- 2026-05-22: 웹→로컬 소환 방식으로 **(B) 양방향 로컬 런처 서비스 채택**(단방향 프로토콜 핸들러 A 대신). 웹 "수정" → localhost 런처 → VS Code + 선택 CLI(opencode|claude) spawn + 상태 회신. 단 이 런처는 **agent-service 부활이 아니라 "소환+상태 회신" 전용 경량 데몬**(브리지 로직 없음). 보안(127.0.0.1 bind·Origin 허용목록·설치별 토큰) 필수. 상세는 「### 웹 → 로컬 소환」.
