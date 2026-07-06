# 배포 토폴로지 및 운영 모델

> **상태**: 2026-05-17 작성. 이번 세션에서 확정된 운영 모델, 토폴로지, 사용자 등록 흐름, TUI attach 흐름을 정리.
>
> **관련 문서**:
> - `docs/refactor-skill-fs-direct.md` — fs 직접 access 리팩토링 + § 7 OpenCode 인스턴스 운영 설계
> - `docs/ports.md` — 컨테이너/호스트 포트 컨벤션
> - `docs/design-platform.md`, `docs/design-agent.md` — 컴포넌트 책임

---

## 1. 두 가지 운영 모델

### 1.1 시스템 중심 모델 (현재 우선 진행) — 사내 OpenCode 서버 통합

```
[사용자 PC: Win/Ubuntu/Mac]                [사내 서버 (한 노드)]
  Claude / Cursor / 브라우저(UI)            ├─ OpenCode 인스턴스 (사용자별, systemd)
       │                                     │   └─ 스킬 실행 (여기서 fs 직접 access)
       │                                     │
       │  HTTP (X-User-Id 헤더 전파)         ├─ Docker compose
       └─────────────────────────────────────►│   ├─ platform-api    /data/repos ←┐
                                              │   ├─ gojira-api                  │
                                              │   ├─ agent-service  ─ user 라우팅 │
                                              │   ├─ platform-app                │
                                              │   ├─ gojira-app                  │
                                              │   ├─ jira-api                    │ 같은 fs
                                              │   └─ plantuml-server             │
                                              └─ host /home/.../repos ───────────┘
                                                            ▲
                                                            └─ bind mount
```

**핵심**:
- 스킬은 **사내 서버의 OpenCode 안에서만 실행** → docker volume(bind mount) 과 같은 fs 직접 access
- 사용자 PC OS 와 무관 — Windows/Ubuntu/Mac 어디서든 HTTP 만 호출
- fs 공유 mount 셋업 불필요 (사내 서버 자기 자신 안에서만 처리)
- 데이터 일관성: § 3 의 "단일 사본 공유 모델" 이 그대로 성립

### 1.2 분산 스킬 모델 (보류 — 향후 외부 에이전트 도구 제작 시)

```
[사용자 PC]                                   [사내 서버]
  ├─ Claude / Cursor / Codex                   ├─ Gitea
  │   └─ 다운로드된 스킬 + _scripts/          ├─ Docker compose
  ├─ 로컬 git clone (자기 worktree)            │   ├─ platform-api
  │   └─ fs 직접 read/write ← 자기 fs         │   ├─ gojira-api
  └─ gojira CLI / MCP (신규 — 미구현)         │   └─ ...
       ├─ project clone <code>                 │
       ├─ project status                       │
       └─ ...                                  │
       │                                       │
       └─── HTTP / git push 등 ───────────────►│
```

**상태**: 사용자 명시로 **보류**. 시스템 중심 모델 정리가 모두 끝난 뒤 별도 진행.

**필요 도구 (보류)**:
- `gojira CLI` — `project clone`, `project status`, `repo info` 등
- MCP server — Claude Desktop / Cursor 의 native 통합
- 스킬 패키징 — `.claude/skills/` 를 zip / OCI artifact / npm 등으로 배포
- drift 동기화 정책 — 사내 worktree 와 사용자 PC clone 사이 정합성

---

## 2. 토폴로지

### 2.1 운영 환경 (사내 서버 1대 가정)

| 항목 | 설명 |
|---|---|
| 노드 수 | 1 (OpenCode + Docker compose 통합) |
| 사용자별 격리 | OpenCode 인스턴스 (`opencode@<user>.service`) + worktree (`.worktrees/<userId>/`) |
| fs 공유 | 자연 — 한 노드 안 |
| Gitea / MongoDB | 외부 사내 서버 (HTTP/MongoDB protocol) |
| 사용자 PC 도달 | HTTP only |

### 2.2 dev 환경 (Mac + Parallels Ubuntu, 시연/검증용)

| 머신 | IP | 무엇 |
|---|---|---|
| Mac 호스트 | **10.211.55.2** | OpenCode (port 4096), 사용자 IDE/브라우저 |
| Parallels Ubuntu VM | **10.211.55.3** | Docker compose 전체 (platform-api/gojira/api/agent-service/...) |
| MongoDB / Gitea | 사내 또는 별도 호스트 | `.env` 의 `MONGODB_SERVER` 등에 명시 |

**알려진 한계** (dev 한정):
- Mac fs ↔ VM fs 분리. Mac 의 OpenCode 가 스킬을 직접 실행하면 worktree fs (VM 의 docker volume) 도달 못함.
- 시연 단계에서는 UI(브라우저) 흐름만 검증하면 충분 — Platform-API 가 자기 fs 안에서 처리.
- 본격적인 스킬 fs 직접 access 검증은 OpenCode 를 VM 안에서 실행 (운영 모델 시뮬레이션) 또는 sshfs/Parallels Shared Folders 로 mount.

자세한 내용: `docs/refactor-skill-fs-direct.md` Phase 1 검증 메모.

---

## 3. 포트 컨벤션 (사내 운영)

`docs/ports.md` 의 컨테이너 내부 포트 표와 함께 호스트 매핑 포트는 다음과 같이 고정:

| 컨테이너 포트 | 호스트 포트 (사내 운영) | 서비스 |
|---|---|---|
| 9000 | **44350** | platform-api |
| 9001 | **44360** | gojira-api |
| 9002 | **44370** | agent-service |
| 3000 | 44300 | platform-app |
| 3001 | 44301 | gojira-app |
| 8080 | 44351 | plantuml-server |
| 4096 | 4096 | OpenCode (호스트 native) |

호스트 매핑 포트는 사내 firewall/policy 에 등록된 컨벤션 — **변경 금지**.

`deploy/.env` 의 `PLATFORM_API_PORT=44350` / `GOJIRA_API_PORT=44360` 등이 이 매핑을 결정. compose 의 fallback 체인은 `HOST_PORT:-PORT:-9000` 이므로 `PORT=44350` 만 잡혀도 동일 효과.

---

## 4. 사용자 등록 흐름 (§ 7 단계 A)

### 4.1 등록

```bash
# 사내 서버에서 (root 권한)
sudo ./deploy/scripts/gojira-user-add.sh <userId>
```

흐름:
1. Mongo `opencodeInstances` 의 최대 port + 1 (없으면 4096) 결정
2. `/etc/gojira/opencode-<userId>.env` 생성 (mode 600) — `OPENCODE_PORT`, `OPENCODE_SERVER_PASSWORD` 포함
3. Mongo `opencodeInstances.updateOne(...upsert)` — `{userId, host, port, status:"active"}`
4. `systemctl enable --now opencode@<userId>.service`

옵션:
- `--no-start` : systemctl 호출 생략 (env 파일 + Mongo 등록만)
- env override: `GOJIRA_NODE_HOST` (default localhost), `OPENCODE_BIN`

### 4.2 라우팅 (단계 B 7.5)

```
사용자 PC ─HTTP─► Agent-Service (44370)
                      │
                      ├─ X-User-Id 헤더 추출
                      │
                      ├─ Mongo opencodeInstances.find(userId, status="active")
                      │   ├─ 있음 → http://<host>:<port> 사용
                      │   └─ 없음 → settings.opencode_url 로 fallback + WARNING
                      │
                      └─ HTTP/SSE 호출 → 사용자별 OpenCode 인스턴스
```

매 호출마다 Mongo 조회 (캐시 없음 — status 변경 즉시 반영).

### 4.3 헬스체크 (단계 B 7.6)

- Platform-API 의 백그라운드 goroutine: 5분 주기 `/global/health` ping
- 3회 연속 실패 → `status="down"` 마킹 → Agent-Service 가 다음 호출부터 자동 fallback
- 운영자가 즉시 트리거: `POST /admin/opencode-healthcheck`

### 4.4 제거

```bash
sudo ./deploy/scripts/gojira-user-remove.sh <userId>
```

- `systemctl disable --now opencode@<userId>.service`
- env 파일 삭제
- `opencodeInstances` 레코드 삭제
- **worktree 는 보존** (별도 정리 필요)

---

## 5. TUI attach 흐름

OpenCode TUI 접속은 다음 두 단계:

### 5.1 UI 측 — "터미널 보기" 클릭

GoJIRA-App 의 채팅 헤더 "터미널 보기" 버튼 클릭:
1. `POST /api/v2/Agent/session/<sid>/terminal` (X-User-Id 헤더)
2. Agent-Service 가 사용자 OpenCode 인스턴스의 `baseUrl` 과 `opencode attach` 명령 반환
3. 클라이언트가 alert + 클립보드 복사 (https 컨텍스트가 아니면 사용자 수동 복사)

### 5.2 사용자 PC 측 — 터미널 attach

```bash
# alert 에 표시된 명령 그대로 사용자 터미널에서 paste
opencode attach http://<사내-OpenCode-host>:<port> -s <openCodeSessionId>
```

사용자 PC 에 `opencode` CLI 가 설치돼 있어야 하며, OpenCode host:port 가 사용자 PC 에서 도달 가능해야 함 (사내 LAN 전제).

### 5.3 설계 결정 (이번 세션)

- Agent-Service 의 server-side OS 분기 (Terminal.app / PowerShell) **폐기**. 컨테이너 안 Linux 에서는 의미 없음.
- 클라이언트(브라우저)는 native 터미널 자동 실행 권한 없음 → command 표시 + 클립보드 복사가 한계.
- 향후 UI 디자인 개선 (alert → modal) 은 후속 작업.

---

## 6. Worktree TTL 정책

`docs/refactor-skill-fs-direct.md` Phase 3 의 보강 결과:

| 항목 | 값 |
|---|---|
| Worktree TTL | **4시간** (`PLATFORM_WORKSPACE_TTL` 분 단위로 override) |
| 점검 주기 | TTL / 2 |
| Cleanup 함수 | `RemoveWorktreeOnly` — **branch 보존** (unpushed commit 영구 손실 방지) |
| Skip 조건 | (1) 최근 access, (2) 활성 EditSession, (3) 미커밋 변경 |
| EditSession TTL (별도) | 30분 — UI lock 동작에만 영향 |

---

## 7. 백업 정책 (archive)

`Platform-API/handlers/archive.go` 의 backup zip 저장 경로:

- 우선순위: `GOJIRA_BACKUP_DIR` env > `$HOME/.gojira-backups` > `/tmp/.gojira-backups` (fallback)
- `user: "${UID}:${GID}"` override 로 컨테이너의 `HOME` 이 `/` 가 되어 권한 오류가 나는 케이스를 fallback 으로 흡수
- 영구 보존 필요 시 docker-compose 에 `GOJIRA_BACKUP_DIR=/data/backups` + host bind mount 추가 (현재 미적용, 운영 결정 사항)

---

## 8. 이번 세션의 의사결정 요약

| # | 결정 | 근거 |
|---|---|---|
| 1 | 우선순위: 시스템 중심 모델 먼저, 분산 스킬 모델은 보류 | 사용자 명시 |
| 2 | 호스트 포트 44350/44360 컨벤션 유지 | 사내 firewall/policy 등록 컨벤션 |
| 3 | dev 환경 (Mac+VM) 의 fs 분리는 시연 한계로 인정. sshfs 등 임시 셋업도 보류 — 운영 환경에서 자연 해소 | UI(브라우저) 흐름은 정상 동작, 스킬 fs 직접 access 검증은 운영 환경 시뮬레이션으로 |
| 4 | TUI 자동 실행은 서버 측 GUI 띄우기 폐기, 클라이언트 alert + clipboard 로 단순화 | 컨테이너 안 Linux 에서 GUI 띄우기 무의미, 브라우저는 native 터미널 권한 없음 |
| 5 | Worktree TTL 30분 → 4시간 + branch 보존 (RemoveWorktreeOnly) | 사용자가 자리 비울 때 cleanup 위험 + unpushed commit 손실 위험 회피 |
| 6 | Archive backup dir 에 fallback 도입 (`/tmp/.gojira-backups`) | uid override 환경에서 `HOME=/` 권한 오류 흡수 |
| 7 | 스킬 default URL 을 `10.211.55.3:44350/44360` 으로 (dev 한정) | Mac OpenCode 가 VM 의 Docker 호출하는 시나리오 |

---

## 9. 향후 작업 (사내 시스템 잔여)

`docs/refactor-skill-fs-direct.md` § 7 + 메모리 `refactor-pending` 참조. 외부 에이전트용 도구 (gojira CLI / MCP / 분산 스킬 모델) 는 이 잔여가 모두 정리된 뒤 별도 진행.
