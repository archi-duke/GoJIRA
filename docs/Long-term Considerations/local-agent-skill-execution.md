# 트랙 D 설계 — 스킬 로컬 실행 + 직접 fs write 모델

> 기록 일자: 2026-05-22
> 상태: **설계 (실행 대기)** — `local-agent-mvp.md` §5-4 트랙 D / §6 "스킬 실행모델" 의 구체화.
> 이 문서는 코드 변경 없이 방향을 확정한다. 실행(스킬/서버 코드 수정·삭제)은 승인 후.
> 상위: [`local-agent-mvp.md`](./local-agent-mvp.md), 선행: [`local-agent-removal-inventory.md`](./local-agent-removal-inventory.md) 트랙 C.

## 0. 한 줄 요약

스킬은 **사용자 PC 로컬 CLI(opencode/claude)** 안에서, 런처가 체크아웃해 둔 **로컬 clone
(`docs/{user}`)** 작업 디렉터리에서 실행되어 **파일을 로컬 fs 에 직접 write** 한다. 서버측
worktree·EditSession lock·SSE notify 는 불필요해진다. 변경의 **단일 지점은 `doc-save.sh`**.

## 1. 현재 메커니즘 (server worktree)

문서 생성 스킬 8개 — `projdesc-generate`, `projdesc-interview`, `driver-select2`,
`style-define2`, `usecase-analyze2`, `requirement-refine2`, `design-decide2`,
`design-integrate2` — 는 모두 `.claude/skills/_scripts/doc-save.sh` 를 거쳐 저장한다:

```
doc-save.sh <CODE> <USER> <PATH> < content
  1) _edit-acquire.sh  → Platform-API POST /edit/start (holderKind=skill)
                         → 서버측 worktree 절대경로(WT) 반환 (GOJIRA_REPOS_DIR 기반)
  2) plantuml-fix.py   → ```plantuml 블록 결정적 자동수정
  3) fs write          → ${WT}/${PATH} (.gojira-locks flock 직렬화)
  4) _notify-change.sh → Platform-API POST /notify-change (웹 DocEditor SSE 갱신)
  5) post-save sanity  → @startuml/@enduml 짝, placeholder 잔존 등 경고
```

- **전제**: 스킬이 서버(또는 `GOJIRA_REPOS_DIR` bind-mount 접근 가능 위치)에서 실행.
- 직접 API write 우회: **`reset-project`** 스킬만 (`PUT/DELETE /file`, dev/test 전용).
- `gojira-cli doc` (`doc.go`) 도 `PUT /file` 사용 (CLI 경로).

## 2. 목표 모델 (local clone 직접 write)

- 런처가 `git clone … && checkout docs/{user}` 로 로컬 clone 준비(§5-1 완료) → CLI 를
  `repoPath` 를 cwd 로 spawn(§5-2). 스킬은 그 cwd(=clone 루트)에서 동작.
- **doc-save.sh 가 로컬 clone 루트에 직접 write** — 서버 왕복 없음.
- **단일 writer**(로컬 작업본 1개, §4) → EditSession lock 불필요. 브랜치 내부 충돌 0.
- 웹은 push 후 Gitea read 로 최신을 보므로 **서버 SSE notify 불필요**(웹은 PR/조회 전용).

## 3. 핵심 변경 — `doc-save.sh` 로컬 모드

```
doc-save.sh <CODE> <USER> <PATH> < content      # CODE/USER 는 호환 위해 유지(미사용 가능)
  1) repo 루트 결정:
       ROOT = ${GOJIRA_REPO_ROOT:-$(git rev-parse --show-toplevel)}   # cwd = clone
  2) plantuml-fix.py        (그대로 유지)
  3) fs write → ${ROOT}/${PATH}   (mkdir -p; flock 제거 — 단일 writer)
  4) (삭제) _notify-change 호출 없음
  5) post-save sanity       (그대로 유지)
```

- `_edit-acquire.sh`(1단계)·`_notify-change.sh`(4단계) **호출 제거**.
- 산출물 JSON 포맷(`{ok,path,status,bytes,warnings}`)은 호환 위해 유지 — 8개 스킬이 파싱.
- **8개 스킬 본문은 무수정**(doc-save.sh 인터페이스 동일) → choke point 한 곳만 바꾸면 됨.

### 제거 대상 스크립트 (D 실행 시)
- `.claude/skills/_scripts/_edit-acquire.sh`
- `.claude/skills/_scripts/_edit-release.sh`
- `.claude/skills/_scripts/_notify-change.sh`
- `plantuml-fix.py` 는 **유지**(doc-save.sh 2단계 + `plantuml-doc-fix` 스킬이 독립 사용).

> `_worktree-ensure.sh` 는 **read 헬퍼**(doc-read/doc-list/project-status 가 사용) — write 경로가
> 아니다. 로컬 모델에선 이들도 로컬 clone 에서 읽으므로 함께 로컬화(repo 루트 = clone)되지만,
> read 경로라 별도 항목으로 다룬다(D 의 write 제거와 분리).

## 4. D 완료 후 가능해지는 서버측 제거 (트랙 C 나머지)

스킬이 더 이상 서버 write 를 호출하지 않으면 아래를 제거할 수 있다(별도 커밋):
- 엔드포인트: `POST /edit/start`, `POST /edit/release`, `POST /notify-change`,
  `PUT /file`, `DELETE /file`(reset-project 로컬 재작성[결정] 후 제거 — `gojira-cli doc` 는 결정3 보류),
  `GET /edit/status`·`/edit/pending`·`/edit/file`(웹 read-only 후 잔여 호출자 재확인).
- `db/edit_session.go` 전체 + Mongo `editSessions` 컬렉션.
- `github/localrepo.go` 의 worktree write 군: `EnsureWorktree`, `WriteWorktreeFile`,
  `CommitAndPush`, `DiscardWorktree`, `RemoveWorktreeOnly`, `RebaseOnMain`,
  `ensureWorktreeExclude`, `worktreeDir`, `StartTTLWorker`/workspace TTL.
- `DocRepoPRCreateHandler` 의 `!skipAutoCommit` CommitAndPush 분기(이제 항상 로컬 push 전제).
- 보존: `EnsureClone`/read 계열, Gitea PR 생성·리뷰·머지.

## 5. 선결정 — 확정 (2026-05-22)

1. **전환 전략 = 하드 스위치(로컬 전용)** ✅ — doc-save.sh 를 로컬 fs 직접 write 전용으로 교체.
   `DOC_SAVE_MODE` 듀얼 모드 없음. (서버 호스팅이 트랙 A 에서 이미 제거됐으므로 API 모드 대상 없음)
2. **`reset-project` = 로컬 clone 기반으로 재작성** ✅ — 로컬 모델에선 리셋 시 "지울 서버 파일"이
   없다(문서는 로컬 clone/Gitea). 따라서 스킬을 로컬 fs 기반으로 수정 → 직접 `PUT/DELETE /file`·
   `init-files` 호출 제거. 이로써 서버 `PUT/DELETE /file` **완전 제거** 가능.
3. **`gojira-cli doc` write = 보류** ⏳ — "파일을 로컬에서만 생성·사용하는 흐름"이 확정된 뒤 결정.
   (실행 시 doc.go 의 실제 write 사용처를 감사해 폐기/로컬전환 판단)
4. **repo 루트 = 런처가 `GOJIRA_REPO_ROOT` 주입** ✅ — launcher 설정(launcher.json)으로 관리.
   doc-save.sh 는 `${GOJIRA_REPO_ROOT:-$(git rev-parse --show-toplevel)}` 로 안전망 fallback.
   (런처 §2.4 설정 스키마에 항목 추가 필요)
5. **`PLATFORM_API` 의존 = 제거** ✅ — doc-save.sh 에서 기본값/참조 삭제(1번 하드 스위치와 묶임).
6. **flock = 제거** ✅ — 단일 writer 라 불필요. 서버 EditSession lock 과 함께 제거(놔두면 혼란).

### 실행 순서(확정 결정 기준)
1. doc-save.sh 로컬 모드 재작성(결정 1·4·5·6) → 8개 doc 스킬 자동 마이그레이션.
2. reset-project 로컬 재작성(결정 2).
3. 스크립트 제거: `_edit-acquire.sh`/`_edit-release.sh`/`_notify-change.sh`.
4. 서버 제거(§4): edit/start·release·notify-change, PUT/DELETE file, edit_session.go,
   localrepo worktree write 군, PRCreate CommitAndPush 분기. (각 단계 go build 검증)
5. gojira-cli doc write 결정(결정 3) — 잔존 시 5단계 일부 보류.

## 관련 문서
- [`local-agent-mvp.md`](./local-agent-mvp.md) — 상위 §5 구현 순서
- [`local-agent-removal-inventory.md`](./local-agent-removal-inventory.md) — 트랙 A~C 제거 인벤토리
