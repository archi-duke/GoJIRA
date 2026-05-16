# 리팩토링 계획 — OpenCode 스킬의 로컬 파일시스템 직접 접근

> **상태**: Phase 0~5 + § 7 단계 A/B 완료. 후속 보강 § 8 추가 (2026-05-17).
> **작성 맥락**: 사내 폐쇄망 배포 패키지 (`deploy/`) 정리 중, OpenCode 노드와 Gitea 노드 분리 가능성을 검토하다가 **현재 구현과 원래 설계 의도 사이의 큰 차이**를 발견하여 정리
> **관련 문서**: `docs/deployment-topology.md` (운영/dev 토폴로지 + 사용자 등록 + TUI 흐름)

---

## 1. 배경 — 발견된 설계 의도와 구현의 차이

| | 원래 설계 의도 | 현재 구현 |
|---|---|---|
| 작업 사본 위치 | OpenCode 노드의 **로컬 git clone** | Platform-API 컨테이너 `/data/repos` 볼륨 |
| 스킬이 파일 접근하는 방법 | 직접 fs read/write | HTTP `curl` → Platform-API |
| git 책임 (clone/fetch/push) | OpenCode 노드 | Platform-API |
| Gitea 통신 주체 | OpenCode 노드 | Platform-API |
| `repos/` 디렉토리 사용 | 사용 | (사용되지 않음 — `.gitignore` 에만 존재) |

### 근거 (코드 위치)

- `.opencode/agents/gojira.md` — "write/edit 도구로 직접 생성 금지, 반드시 스킬 → doc-save.sh 경유"
- `.claude/skills/_scripts/doc-save.sh` — `PLATFORM_API` 로 PUT `/DocRepo/{code}/file` HTTP 호출
- `.claude/skills/_scripts/doc-read.sh` — `PLATFORM_API` 로 GET `/DocRepo/{code}/file?path=...` HTTP 호출
- `.claude/skills/_scripts/project-status.sh` — `GOJIRA_API` + `PLATFORM_API` 로 GET HTTP 호출
- `Platform-API/github/localrepo.go` — `EnsureClone()` / `EnsureWorktree()` / `GitCmd()` (`exec.Command("git", ...)`) 로 컨테이너 내부 fs 에서 직접 git 작업
- `Platform-API/handlers/docrepo.go:887` — `DocRepoFileUpdateHandler` 가 worktree 에 직접 write (push 안 함, PR 시점에 일괄)

즉 **모든 파일 read/write 가 Platform-API HTTP API 경유**이며, **OpenCode 노드의 로컬 fs 는 사용되지 않습니다**.

---

## 2. Platform-API 의 git 책임 현황 (이관/유지 판단 자료)

| 책임 | 핵심 위치 | 설명 |
|------|---------|------|
| Repo 생성 + Gitea 등록 | `DocRepoCreateHandler` (docrepo.go:57) | Gitea API 로 repo 생성 + DB 등록 |
| bare clone / fetch | `EnsureClone` (localrepo.go:70) | `/data/repos/<owner>/<repo>.git` |
| 사용자별 worktree | `EnsureWorktree` (localrepo.go:96) | `.worktrees/<userId>/` |
| 파일 read/write | `DocRepoFile*` (docrepo.go:848~) | 로컬 worktree 에 직접 write, **PR 시점까지 push 안 함** |
| EditSession lock | `db/edit_session.go` | 같은 projectCode 동시 편집 차단 |
| Sync (fetch) | `DocRepoSyncHandler` (docrepo.go:222) | Gitea → worktree |
| Publish (commit + push) | `DocRepoGitPublishHandler` (docrepo.go:1546) | PR 직전 일괄 push |
| PR 생성/리뷰/머지 | `DocRepoPR*` (docrepo.go:1253~) | Gitea API |
| Webhook 수신 | `handlers/webhook.go` | Gitea webhook → 알림 + branchAlert |
| UI 실시간 갱신 | `BroadcastDocEvent` SSE | worktree 변경 → DocEditor 자동 갱신 |

→ Platform-API 는 단순 file CRUD 가 아니라 **Gitea 캐시 + 사용자별 격리 worktree + 동시성 관리** 까지 책임지는 복잡한 git 관리자입니다. 통째로 OpenCode 노드로 옮기는 건 대공사이므로, **기능 자체는 유지하되 fs 사본을 OpenCode 와 공유하는 모델** 이 현실적입니다.

---

## 3. 새 구조 제안 — 단일 사본 공유 모델

### 3.1 핵심 아이디어

- **OpenCode 노드 = Platform-API 컨테이너가 실행되는 노드**
- Platform-API 컨테이너의 `/data/repos` 볼륨을 **호스트 bind mount** 로 노출
- 그 호스트 경로 = OpenCode 의 `${GOJIRA_REPOS_DIR}` (예: `~/gojira-repos` 또는 `${PROJECT_ROOT}/repos/`)
- **Platform-API 와 OpenCode 가 같은 fs 디렉토리를 본다** — sync drift 없음
- 노드 분리 (OpenCode ↔ Platform-API 다른 노드) 시나리오는 보류. 필요 시 NFS 폴백.

### 3.2 디렉토리 약속

```
${GOJIRA_REPOS_DIR}/
└── <gitea-org>/                     ← PLATFORM_GITHUB_ORG (모든 프로젝트 공통)
    ├── <repo-name>.git/             ← bare clone (Platform-API 만 직접 조작)
    └── <repo-name>.worktrees/
        ├── batman/                  ← 사용자별 worktree (UI + 스킬 공유)
        ├── alice/
        └── ...
```

용어:
- `<gitea-org>` — `PLATFORM_GITHUB_ORG` 환경변수 값 (Gitea organization 이름). 코드상 `owner` 변수
- `<repo-name>` — 프로젝트별 repo 이름 (LLM 번역 또는 명시 입력)
- `<userId>` — GoJIRA 사용자 계정 (예: `batman`). worktree 디렉토리명

예시 (`PLATFORM_GITHUB_ORG=archi-duke`, `repoName=gojira3-design`, 사용자 `batman`):
```
~/gojira-repos/
└── archi-duke/
    ├── gojira3-design.git/
    └── gojira3-design.worktrees/
        └── batman/
```

- Platform-API `config.RepoBaseDir` 기본값 = `${GOJIRA_REPOS_DIR}`
- OpenCode 스킬도 같은 환경변수 인식 → 같은 경로 read/write

### 3.3 통신 흐름 (Before → After)

**Before:**
```
스킬 (OpenCode) ──curl HTTP──> Platform-API ──git CLI──> /data/repos/...
                                       ↓
                                  Gitea HTTP (clone/fetch/push)
```

**After:**
```
스킬 (OpenCode) ──fs read/write──> ${GOJIRA_REPOS_DIR}/.../worktrees/<userId>/
                                      ↑
UI (DocEditor) ──HTTP──> Platform-API ──┘ (UI 흐름은 그대로)
                              ↓
                         Gitea HTTP (clone/fetch/push)
```

---

## 4. 영향 평가 — 앞서 짚은 race / sync / 인증 이슈 재평가

| 이슈 | 현재 | 새 구조 |
|------|------|---------|
| race condition (동시 세션) | EditSession DB lock | **거의 해소** — worktree 가 `.worktrees/<userId>/` 로 사용자별 분리되어 있어 다른 사용자 사이 race 는 없음. 잔여 케이스 (같은 사용자의 UI ↔ 스킬, 또는 스킬 ↔ 스킬) 만 EditSession lock 으로 처리 |
| 이중 사본 동기화 | OpenCode 사본 ↔ Platform-API 사본 drift 위험 | **해소** — 단일 사본 |
| OpenCode 노드 Gitea 자격증명 | 필요 (clone/pull/push) | **해소** — push/pull 은 Platform-API 만 |

### race condition 시나리오별 정리

| 시나리오 | race 가능? | 처리 방법 |
|---------|----------|---------|
| 사용자 A 와 사용자 B 가 같은 projectCode 작업 | ❌ | worktree 분리 (`.worktrees/<userId>/`) |
| 같은 사용자가 UI 두 탭에서 같은 projectCode 편집 | ❌ | `db/edit_session.go` unique index `(userId, projectCode, status=active)` 가 이미 차단 |
| 같은 사용자가 UI 편집 중 + 스킬 실행 | ⚠️ | **스킬도 EditSession acquire/release 호출**하면 자동 직렬화 |
| 같은 사용자가 OpenCode 두 세션 동시 실행 | ⚠️ | 위와 동일 — EditSession lock 으로 처리 |

→ **스킬용 별도 worktree 분리는 불필요**. 기존 EditSession 메커니즘을 스킬에도 적용하는 것으로 충분.

### 새 구조에서 새로 생기는 이슈

| 이슈 | 대응 |
|------|------|
| 스킬이 worktree 위치를 어떻게 알아내나 | 환경변수 약속: `${GOJIRA_REPOS_DIR}/<owner>/<repo>.worktrees/<USER>` (Platform-API 가 알려주는 `GET /DocRepo/{code}/worktree-info` 도 가능하지만 환경변수 약속이 단순) |
| 스킬이 worktree 없을 때 ensure | 스킬 첫 단계에 `POST /DocRepo/{code}/edit/start` 호출 (기존 endpoint 재사용 — worktree 생성 + EditSession acquire 동시 수행) |
| 스킬 종료 시 lock 해제 | `POST /DocRepo/{code}/edit/end` (기존 endpoint, 신규 추가가 필요하면 명시) — 스킬 종료 시 호출 |
| 스킬이 git push 직접 할지 | **금지** — push 는 `POST /DocRepo/{code}/git/publish` 경유. 인증 / 리뷰어 검증 / PR 흐름 보존 |
| 스킬 write 후 UI 자동 갱신 | 스킬이 write 후 `POST /DocRepo/{code}/notify-change` (신규) 호출 → Platform-API SSE broadcast. 대안: inotify watch (복잡) |

---

## 5. 리팩토링 단계 계획

### Phase 0 — 약속 정리 ✅ 완료 (2026-05-16)
- `${GOJIRA_REPOS_DIR}` 환경변수 도입 — `deploy/.env.example`, `deploy/.env`
- `deploy/docker-compose.yml` — named volume → host bind mount + `user: "${UID}:${GID}"`
- `deploy/DEPLOY.md` — 호스트 디렉토리 준비/백업/트러블슈팅 절차
- `docs/env_info.md` § 8.5 폐쇄망 배포 특화 변수 신설
- `.opencode/agents/gojira.md` 환경변수에 worktree 경로 컨벤션 명문화
- 컨테이너 uid 1000 ↔ 호스트 uid 1000 권한 정합성 검증 완료

### Phase 1 — read 경로 마이그레이션 ✅ 완료 (2026-05-16)
- Platform-API: `GET /DocRepo/{code}/worktree-info` 신규 endpoint (lock 없이 worktree 보장 + owner/repo/branch/worktreeRelPath 반환)
- Platform-API: `DocRepoEditStartHandler` 응답에 owner/repo 추가 (Phase 2 write 헬퍼용)
- 신규 헬퍼: `_scripts/_worktree-ensure.sh` — worktree-info 호출 → 호스트 절대경로 echo
- 스킬 마이그레이션: `doc-read.sh` (cat), `doc-list.sh` (find), `project-status.sh` (문서 부분만 fs, 이슈는 HTTP 유지)
- 검증: GOJIRA3/batman 으로 worktree-info → doc-list (5 files) → doc-read (README.md) → project-status (nextStage=vos) 모두 통과
- 주의: 시나리오 A (OpenCode + Platform-API 같은 노드) 가정. 개발 환경 (Mac OpenCode ↔ Parallels 백엔드) 에서는 fs 공유 별도 셋업 필요 (sshfs 또는 Parallels guest 에서 OpenCode 실행)

### Phase 2 — write 경로 마이그레이션 ✅ 완료 (2026-05-16)
- `db/edit_session.go` — `holderKind` 필드 추가, `AcquireEditSession(ctx, code, user, holderKind)` 시그니처 확장. 같은 user 의 다른 holderKind 의 active 세션이 있으면 거부 + 기존 holderKind 반환
- `DocRepoEditStartHandler` — body/query 로 `holderKind` 받기 (기본 ui). 충돌 시 응답에 holderKind 포함
- 신규 endpoint `POST /DocRepo/{code}/notify-change` — body `{path, op?}` → BroadcastDocEvent `user_doc_update` 발사 (source=skill)
- 신규 헬퍼: `_scripts/_edit-acquire.sh` (skill lock + worktree 경로), `_scripts/_edit-release.sh` (명시 해제)
- 스킬 마이그레이션: `doc-save.sh` — HTTP PUT 제거, fs 직접 write + notify-change + post-save warnings 유지. `draft-io.sh` 는 `/tmp/` 임시 파일 I/O 만 다루므로 마이그레이션 대상 아님 (그대로 유지)
- `.opencode/agents/gojira.md` — 세션 lock 룰 추가 (충돌 보고 + 작업 종료 시 release 호출)
- 검증: doc-save → fs 파일 생성 (uid 1000) → notify-change 200. holderKind 충돌 양방향 모두 거부 동작 확인

### Phase 3 — 동시성 / 정합성 보강 ✅ 완료 (2026-05-16)

**결정 표:**

| 항목 | 결정 | 근거 |
|------|------|------|
| 스킬용 별도 worktree 분리 | **불필요** | 다른 user 간은 worktree 가 이미 `.worktrees/<userId>/` 로 분리됨. 같은 user 의 UI ↔ 스킬 race 는 Phase 2 의 holderKind lock 으로 해소 |
| 변경 통지 방식 | **notify endpoint 확정** (inotify 반려) | (1) 명시적 호출이라 op 메타데이터 포함 가능 (save/delete), (2) bind mount 환경의 inotify 이벤트 신뢰성/디바운스 부담 회피, (3) 스킬은 fs write 의 유일한 비-UI 주체라 명시 호출 누락 위험 낮음 |
| 같은 user + 같은 holderKind race | **flock(LOCK_EX) 직렬화** | EditSession lock 은 reentry 허용이라 같은 holderKind 의 동시 write 를 못 막음 (OpenCode 세션 2개, UI 탭 2개). file-level OS lock 으로 직렬화 |

**구현:**

- 신규 헬퍼: `_scripts/_notify-change.sh` — 다른 fs-write 스킬도 재사용 가능. doc-save.sh 에서 추출.
- `_scripts/doc-save.sh` — write 구간을 `flock -x` 로 감쌈. Lock 파일은 `${WT}/.gojira-locks/<sanitize(path)>.lock` (slash → underscore).
- `Platform-API/github/localrepo.go::WriteWorktreeFile` — 같은 lock 경로 위에 `syscall.Flock(LOCK_EX)`. UI ↔ 스킬, 스킬 ↔ 스킬 race 모두 차단.
- `Platform-API/github/localrepo.go::EnsureWorktree` — worktree 생성 시 `.git/info/exclude` 에 `.gojira-locks/` 한 줄 자동 추가 (user 자신의 `.gitignore` 와 분리, 기존 worktree 는 재생성 또는 수동 추가 필요).

**검증 (컨테이너 재빌드 + 통합 — 2026-05-16 추가):**

- bash 측 flock 단독: 동시 doc-save (200 줄 패턴 A vs B) → 한 패턴만, partial-mix 없음.
- 새 Platform-API 바이너리 (`syscall.Flock`) 로 재검증: 동일 결과. 두 호출 모두 200 OK, 컨테이너 내부 `.gojira-locks/business__p3-final.md.lock` 생성 확인.
- `EnsureWorktree` 의 `.git/info/exclude` 자동 등록: 처음 시도에서 worktree-specific `.git/info/exclude` 에 썼더니 git 이 무시 (worktree 의 GIT_DIR != common dir) → `--git-common-dir` 로 수정. 재빌드 후 bare repo `<bare>.git/info/exclude` 에 `.gojira-locks/` 등록 확인, worktree `git status` 에서 lock 디렉토리가 untracked 에서 빠짐.
- 기존 worktree 보정: early-return 분기에도 `ensureWorktreeExclude` 호출 추가 (idempotent) — 다음 `edit/start` 호출 때 자동 적용됨.

**미적용 (다음 단계로 보류):**

- UI 측 변경 감지 (read 시점 hash → save 시점 비교) : 향후 UI 코드 작업 시 검토. 현재 cost > benefit (스킬은 lock 으로 차단, UI 동시 탭은 EditSession reentry 의 정책 결정 필요).

### Phase 4 — Push / PR 흐름 유지 ✅ 완료 (2026-05-16)

**결정:** Push / commit / PR 흐름은 변경 없음. 스킬은 fs write 만, 커밋·push 는 Platform-API 가 전담.

**검증 (코드 인벤토리, 2026-05-16):**

| 책임 | endpoint | 호출 주체 |
|------|---------|---------|
| 명시적 commit + push (사용자 일괄) | `POST /DocRepo/{code}/git/publish` (`DocRepoGitPublishHandler`) | UI |
| 진행 중 commit (작업 단위) | `POST /DocRepo/{code}/edit/commit` (`DocRepoEditCommitHandler`) | UI |
| Gitea webhook 수신 | `POST /webhook/github` (`GitHubWebhookHandler`) | Gitea → Platform-API |

`grep -rn "git/publish\|push\|commit" .claude/skills/_scripts/*.sh` 결과: 스킬 측에 push/publish 호출 없음 (`issue-batch-create.sh` 의 매치는 주석 한 줄). → § 4 "스킬 push 금지" 약속이 그대로 지켜지고 있음.

**근거 (왜 유지):**
- Gitea 인증 토큰을 OpenCode 노드에 노출하지 않음 (이미 Platform-API 컨테이너만 보유).
- PR 직전 한 번에 commit 하는 UI 흐름 (`git/publish`) 이 변경 단위를 사람이 검토하기 쉽게 함.
- 스킬은 worktree 의 working tree 상태 (uncommitted) 만 다루고, 커밋 시점은 사용자가 결정.

### Phase 5 — Platform-API file CRUD 핸들러 유지 ✅ 완료 (2026-05-16)

**결정:** `DocRepoEditFileHandler` (GET) / `DocRepoEditSaveHandler` (PUT) / `DocRepoFile*` 핸들러 모두 유지. 삭제·축소 없음.

**근거:**
- UI(DocEditor) 가 계속 HTTP 로 read/write. 스킬만 fs 직접.
- 두 경로가 같은 worktree fs 를 본다 → 같은 파일 변경이 양쪽에 즉시 반영.
- 동시 write race 는 Phase 3 의 flock (`WriteWorktreeFile`) 으로 OS 레벨에서 직렬화 — UI 핸들러도 같은 lock 경로를 사용하므로 추가 작업 없음.

---

## 6. 결정 사항 (합의 완료)

| # | 항목 | 결정 |
|---|------|------|
| Q1 | 사본 모델 | 단일 사본 + bind mount (한 노드 통합, 노드 분리 보류) |
| Q2 | Platform-API file CRUD 핸들러 | 유지 — UI 가 계속 HTTP 로 편집, 스킬만 fs 직접 |
| Q3 | 동시성 정책 | 스킬도 EditSession lock 공유 + **lock 충돌 시 상황 보고** (holderKind 등 EditSession 의미 확장 필요) |
| Q4 | fs 권한 | docker-compose `user: "${UID}:${GID}"` — 컨테이너가 호스트 user 로 동작 |
| Q5 | 운영 시나리오 | **시나리오 A** — 사내 서버 한 곳에 OpenCode + Platform-API + worktree 모두. 사용자는 자기 PC 에서 브라우저 + `opencode attach` |
| Q6 | OpenCode 운영 | **사용자별 OpenCode 인스턴스** (포트 분리). systemd 또는 docker-compose profile 로 관리 |
| Q7 | dirty worktree 정책 | UI 흐름 그대로 — 스킬은 write 만, 사용자가 UI 에서 확인 후 명시적 publish (commit+push) |
| Q8 | OpenCode 인스턴스 운영 (scale-out) | **systemd template unit** (`opencode@<user>.service`) + **Mongo 매핑** (`opencode_instances`) + 자동 등록 스크립트 |

### 배경 정리 (참고)

- **운영 부하 평가**: 2~3명 동시 작업 기준, OpenCode 자체 자원 소비는 인스턴스당 ~200MB 메모리 + 평시 idle CPU. 진짜 부하는 LLM 노드 측이며 시나리오와 무관. 시나리오 B (사용자 PC 에 OpenCode) 의 worktree 동기화 부담이 훨씬 큼.
- **race condition**: 다른 사용자 간은 worktree 분리로 자연 해소. 같은 사용자의 UI↔스킬, 스킬↔스킬 만 EditSession 확장으로 직렬화.
- **EditSession 의미 확장 필요** (Phase 2):
  - 현재 `AcquireEditSession` 은 사실상 lock 아님 (같은 사용자도 heartbeat 갱신만 후 acquire 성공).
  - 추가 필드: `holderKind: "ui" | "skill"`, `holderInfo` (스킬 이름 / OpenCode 세션 ID).
  - 다른 holderKind 의 active 세션 존재 시 acquire 거부 + 응답에 holder 정보 포함 → 사용자/스킬이 차단 사유 인지.

---

## 7. Scale-out 설계 — OpenCode 인스턴스 운영

> **상태 (2026-05-16)**: 단계 A (7.2 / 7.3 / 7.4 / 7.8) 완료. 단계 B (7.5 Agent-Service 라우팅 전환 / 7.6 헬스체크) 보류.
>
> 단계 A 산출물:
> - `Platform-API/db/opencode_instance.go` — `opencodeInstances` 컬렉션 + 인덱스(`userId` unique / `status+lastHeartbeat`) + Upsert/Find/Delete/NextOpencodePort 헬퍼
> - `Platform-API/main.go` — `InitOpencodeInstanceIndexes` 호출 추가 (재빌드 후 로그에서 `opencodeInstance indexes initialized` 확인)
> - `deploy/systemd/opencode@.service` + `deploy/systemd/opencode-user.env.example`
> - `deploy/scripts/gojira-user-add.sh` / `gojira-user-remove.sh` (mongosh 경유, root 권한 가정)
> - `deploy/scripts/dev-opencode-register.sh` + `start-all.bat` 의 OpenCode 등록 한 줄 (개발 환경에서도 동일 컬렉션에 1건 동기화)
> - `deploy/DEPLOY.md` ⑦-1 절 신설 (운영자용 설치 절차)
>
> **2026-05-16 갱신**: 단계 B 의 7.5 (Agent-Service 라우팅 전환) 완료. 7.6 (헬스체크) 만 보류.
>
> 단계 B 7.5 산출물:
> - `Agent-Service/core/database.py` — `opencode_instances()` 컬렉션 + `find_active_opencode_instance(user_id)` 헬퍼
> - `Agent-Service/opencode_bridge/client.py` — `_resolve_url(user_id)` 신설. 모든 함수에 `user_id` 인자 추가. Mongo 미등록 사용자는 `settings.opencode_url` 로 자동 fallback + WARNING 로그. URL 캐시 (`_url_cache`) + `invalidate_url_cache()` 헬퍼 (7.6 활성화 시 status=down 표시와 연동 예정).
> - `Agent-Service/opencode_bridge/event_buffer.py` — `ensure_subscriber/reset_subscriber/_subscribe_loop/_save_to_mongo` 시그니처에 `user_id` 추가. 백그라운드 task 가 시작 시 받은 user_id 로 라우팅.
> - `Agent-Service/opencode_bridge/router.py` — `open_terminal/cancel_session/get_session/get_session_state/health` 핸들러에 `X-User-Id` 헤더 추가. 세션 doc 의 `userId` 가 헤더보다 우선 (다른 탭/세션 복원 호환).
>
> 검증 (컨테이너 재빌드 후):
> 1. `GET /api/v2/Agent/health` (헤더 없음) → settings.opencode_url 사용, response.user=fallback
> 2. `opencodeInstances` 에 batman 등록 후 `GET /api/v2/Agent/health -H 'X-User-Id: batman'` → Mongo 라우팅, response.user=batman
> 3. 미등록 alice 헤더 → fallback + WARNING 로그 ("opencodeInstances 미등록 user=alice")
>
> 단계 B 7.6 (헬스체크) 완료 (2026-05-16).
>
> 산출물:
> - `Platform-API/handlers/opencode_healthcheck.go` — `StartOpencodeHealthcheck` goroutine (5분 주기, /global/health ping, 3회 연속 실패 → `status=down`). `RunOpencodeHealthcheckHandler` 즉시 트리거 endpoint (`POST /admin/opencode-healthcheck`).
> - `Platform-API/main.go` — goroutine 시작 + admin 라우트.
> - `Agent-Service/opencode_bridge/client.py` — `_url_cache` 제거. 매 호출마다 `find_active_opencode_instance` 로 `status="active"` 확인 → status=down 즉시 fallback 으로 전환됨.
>
> 검증: alice 에 닿지 못하는 fake 인스턴스 등록 → admin trigger 3회 → `status=down` 마킹 (로그: "alice status=down 마킹 (3회 연속 실패)") → Agent-Service `/api/v2/Agent/health -H 'X-User-Id: alice'` 가 fallback URL 응답.

### 7.1 핵심 원칙

- 사용자당 OpenCode 인스턴스 1개 (Q6) — 세션/메모리/포트 격리
- 인스턴스 lifecycle 은 **systemd template unit** 으로 표준화 (Q8)
- 사용자 → 인스턴스 URL 매핑은 **Mongo 컬렉션** 에서 조회 — Agent-Service 의 라우팅 추상화 (단일 노드 → 다중 노드 확장 시 코드 변경 없음)
- 신규 사용자 등록 시 한 명령으로 unit enable + Mongo 등록 동시 수행

### 7.2 systemd template unit

**파일**: `/etc/systemd/system/opencode@.service` (1회 배포)

```ini
[Unit]
Description=OpenCode server for user %i
After=network.target

[Service]
Type=simple
User=gojira
Environment="OPENCODE_PORT=${OPENCODE_BASE_PORT}"   # 등록 스크립트가 사용자별 결정 후 EnvironmentFile 로 주입
EnvironmentFile=/etc/gojira/opencode-%i.env
WorkingDirectory=/opt/gojira
ExecStart=/usr/local/bin/opencode serve --port ${OPENCODE_PORT} --hostname 0.0.0.0
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
```

**사용자별 환경파일**: `/etc/gojira/opencode-batman.env`
```
OPENCODE_PORT=4096
OPENCODE_SERVER_PASSWORD=<per-user-secret>
```

**조작 명령**:
```bash
systemctl enable --now opencode@batman.service    # 시작
systemctl status opencode@batman.service           # 상태
journalctl -u opencode@batman.service -f           # 로그
systemctl restart opencode@batman.service          # 재시작
```

### 7.3 Mongo 매핑 — `opencode_instances` 컬렉션

```javascript
{
  _id: ObjectId,
  userId: "batman",          // unique
  host: "10.227.37.89",      // 추후 노드 분산 시 노드별 host
  port: 4096,
  status: "active" | "down",
  lastHeartbeat: ISODate,
  pid: 12345,                 // 진단용 (옵션)
  createdAt: ISODate
}
```

- **unique index**: `{ userId: 1 }`
- **secondary index**: `{ status: 1, lastHeartbeat: 1 }` (헬스체크 / stale 정리 용)

### 7.4 사용자 등록 스크립트

**위치**: `deploy/scripts/gojira-user-add.sh`

```bash
#!/bin/bash
# 사용 예: gojira-user-add batman
USER_ID="$1"
NEXT_PORT=$(mongo --quiet \
  --eval "db.opencode_instances.find().sort({port:-1}).limit(1).toArray()[0]?.port + 1 || 4096")

# 1. 환경파일 생성
cat > /etc/gojira/opencode-${USER_ID}.env <<EOF
OPENCODE_PORT=${NEXT_PORT}
OPENCODE_SERVER_PASSWORD=$(openssl rand -hex 16)
EOF
chmod 600 /etc/gojira/opencode-${USER_ID}.env

# 2. Mongo 등록
mongo --eval "db.opencode_instances.insertOne({
  userId: '${USER_ID}', host: 'localhost', port: ${NEXT_PORT},
  status: 'active', createdAt: new Date()
})"

# 3. worktree 디렉토리 미리 생성 (선택)
mkdir -p ${GOJIRA_REPOS_DIR}/<gitea-org>/*.worktrees/${USER_ID}

# 4. systemd unit 시작
systemctl enable --now opencode@${USER_ID}.service
```

**삭제 스크립트** (`gojira-user-remove.sh`):
```bash
systemctl disable --now opencode@${USER_ID}.service
rm /etc/gojira/opencode-${USER_ID}.env
mongo --eval "db.opencode_instances.deleteOne({userId: '${USER_ID}'})"
# worktree 는 명시적 cleanup 필요 시 별도 호출
```

### 7.5 Agent-Service 의 라우팅 추상화

**현재** (`Agent-Service/config.py`):
```python
class Settings:
    opencode_url: str = Field(..., validation_alias="OPENCODE_URL")
```

**변경 후**:
```python
async def get_opencode_url(user_id: str) -> str:
    inst = await opencode_instances().find_one({"userId": user_id, "status": "active"})
    if not inst:
        raise UserInstanceNotFound(f"{user_id} 의 OpenCode 인스턴스가 없습니다")
    return f"http://{inst['host']}:{inst['port']}"
```

`Agent-Service/opencode_bridge/client.py` 의 `_base_url()` 이 `get_opencode_url(user_id)` 로 변경. user_id 는 `X-User-Id` 헤더에서 추출.

### 7.6 헬스체크 + 자동 복구

- **인스턴스 죽음 → systemd 자동 재시작** (`Restart=on-failure`)
- **stale 매핑 감지**: 별도 cron 작업 또는 Platform-API 의 헬스체크
  - 매 5분: `opencode_instances` 의 active 인스턴스에 `/global/health` ping
  - 실패 시 `status=down` 표시 + 알림 (UI 또는 admin 채널)
- **Agent-Service 호출 실패 시**: 즉시 `status=down` 표시, 사용자에게 보고 ("OpenCode 인스턴스가 응답하지 않습니다 — 운영자에게 문의")

### 7.7 노드 간 분산 (Phase 1 미고려, 설계만 확보)

향후 한 노드 자원 고갈 시:
- 새 노드에 OpenCode 설치 + 같은 worktree fs 마운트 (NFS 또는 분산 스토리지)
- `opencode_instances` 의 host 필드를 새 노드 IP 로 등록
- Agent-Service 라우팅은 host:port 로 조회 → 자동 라우팅 — **코드 변경 없음**

### 7.8 환경별 일관성

- **개발 PC (Mac, 단일 사용자)**: `opencode_instances` 에 batman 1건만. systemd 대신 `opencode serve` 직접 실행 (start-all.bat) — Mongo 매핑은 동일
- **사내 폐쇄망 (다중 사용자)**: systemd + 등록 스크립트
- → Agent-Service 코드는 동일. 환경별 차이는 운영 도구 (start-all.bat vs systemctl) 와 인스턴스 수뿐

---

## 8. 후속 보강 (2026-05-17 검증 세션)

검증 진행 중 발견한 추가 이슈와 그 자리에서 적용한 fix.

### 8.1 archive backup 디렉토리 fallback

| 증상 | 원인 | Fix |
|---|---|---|
| `mkdir /.gojira-backups: permission denied` | 컨테이너의 `user: "${UID}:${GID}"` override 로 `/etc/passwd` 에 uid 없음 → `HOME=/` 로 떨어짐 | `Platform-API/handlers/archive.go::getBackupDir()` 도입. 우선순위 `GOJIRA_BACKUP_DIR` env > `$HOME/.gojira-backups` > `/tmp/.gojira-backups` |

영구 보존 필요 시 docker-compose 에 `GOJIRA_BACKUP_DIR=/data/backups` + bind mount 추가 (운영 결정 사항).

### 8.2 Worktree TTL 정책 보강 (B + C)

| 항목 | 이전 | 이후 |
|---|---|---|
| Worktree TTL | `EditSessionTTL` 그대로 (30분) | **4시간** (`PLATFORM_WORKSPACE_TTL` 분 단위 override) |
| Cleanup 함수 | `DiscardWorktree` (worktree + branch 삭제) | `RemoveWorktreeOnly` (branch 보존) |
| 결과 | 작업 중 끊김 + unpushed commit 영구 손실 위험 | 점심/회의 동안 보존 + commit 영구 보존 |

코드: `Platform-API/config/config.go` (WorkspaceTTL 신설), `Platform-API/github/workspace_ttl.go` (StartTTLWorker → config.WorkspaceTTL 사용, cleanup → RemoveWorktreeOnly).

### 8.3 스킬 환경변수 default 갱신

| 항목 | 이전 | 이후 |
|---|---|---|
| `.opencode/agents/gojira.md` | `localhost:9000/9001` + macOS PROJECT_ROOT | `10.211.55.3:44350/44360` (dev 한정) + 환경변수 우선 패턴 |
| `_scripts/*.sh` (8개) | `localhost:9000/9001` | 동일하게 `10.211.55.3:44350/44360` |

운영 환경에서는 `PLATFORM_API` / `GOJIRA_API` env 자체를 override. 컨벤션 표는 `docs/deployment-topology.md` § 3.

### 8.4 TUI attach 흐름 단순화

| 영역 | 이전 | 이후 |
|---|---|---|
| `Agent-Service` `open_terminal` | OS 분기 (Terminal.app/PowerShell/fallback) | command + baseUrl 반환만 |
| `GoJIRA-App` AgentChat 의 "터미널 보기" | `await fetch(...)` 만 — 응답 무시 | 응답 수신 → alert + 클립보드 복사 |

근거: Agent-Service 는 컨테이너 안 Linux 라 server-side GUI 띄우기 무의미. 브라우저는 native 터미널 자동 실행 권한 없음 → 클라이언트가 사용자에게 노출하고 사용자가 자기 PC 터미널에서 수동 실행.

자세한 흐름: `docs/deployment-topology.md` § 5.

### 8.5 dev 환경(Mac + Parallels) 한계 명시

스킬의 fs 직접 access (Phase 1+) 가 동작하려면 OpenCode 와 docker volume 이 같은 fs 여야 함. 현재 dev 환경:
- Mac (10.211.55.2) — OpenCode + 사용자 IDE
- Parallels VM (10.211.55.3) — Docker compose / docker volume

→ 두 fs 가 분리. Mac OpenCode 의 스킬 fs 직접 access 는 깨짐. UI(브라우저) 흐름은 Platform-API HTTP 경유라 정상.

운영 환경 (사내 서버 1대 통합) 에서는 자동 해소. dev 한정 한계로 받아들임 (sshfs 등 셋업 보류).

자세한 토폴로지: `docs/deployment-topology.md` § 2.

---

## 9. 참고

- `docs/deployment-topology.md` — 운영 / dev 토폴로지, 단일 사본 / 분산 스킬 모델, 사용자 등록 / TUI attach 흐름 (이번 세션 정리)
- `docs/ports.md` — 컨테이너 내부 + 호스트 매핑 포트 컨벤션
- `docs/design-platform.md` — Platform-API DocRepo / EditSession / SSE 설계
- `docs/design-agent.md` — OpenCode 에이전트 + 스킬 흐름
- `deploy/` — 폐쇄망 배포 패키지 (이번 검토의 시발점)
- `.opencode/agents/gojira.md` — 현재 에이전트의 "스킬 경유" 약속
