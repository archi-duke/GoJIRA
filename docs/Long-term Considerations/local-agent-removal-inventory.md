# §5-4 server-side 제거 — 코드 기준 인벤토리 (제거 대상 확정)

> 기록 일자: 2026-05-22
> 상태: **인벤토리 (실행 대기)** — `local-agent-mvp.md` §6 첫 항목("제거 대상 파일/엔드포인트
> 정확한 목록")의 산출물. 실제 삭제는 미수행. 삭제 착수 시 **line range 는 코드 기준 재확인**
> (이 문서의 line 번호는 조사 시점 근사치).
> 상위: [`local-agent-mvp.md`](./local-agent-mvp.md) §1 제거 목록 / §5-4.

## 0. 결론 — 제거는 4개 트랙, 결합도 順 단계화

| 트랙 | 결합도 | 롤백 위험 | 순서 |
|---|---|---|---|
| **A. 호스팅 인프라** ✅ **완료(2026-05-22)** | 낮음(독립) | 낮음 | 1 |
| **B. 프론트 write UI** ✅ **완료(2026-05-22)** — B1(채팅)+B2(GoJIRA-App)+B3(Platform-App) | 중간(read 와 한 컴포넌트) | 중간 | 2 |
| **C. Platform-API write 엔드포인트** — 잔여 호출자 0 분만 ✅ **부분완료(2026-05-22)** | 높음(read 와 헬퍼 공유) | 높음 | 3 |
| **D. 스킬 write 경로** (`doc-save.sh`/`_edit-acquire`/`notify-change` — 스킬 실행 모델과 직결) | 높음(스킬 문서생성 전체) | 높음 | 4(별도 설계 선행) |

> **주의**: `/pr` 생성과 `/pr/{n}/review`·`/merge` 는 **보존**(웹 PR 리뷰). `/pr` 생성은 §5-3 에서
> `skipAutoCommit` 분기를 이미 넣었으므로 **삭제가 아니라 worktree-commit 분기 제거(부분 수정)**.
> 트랙 D 는 "스킬을 로컬 CLI 가 로컬 clone 에 직접 write" 모델 확정(§6 채팅 UI/실행모델)이 선행돼야
> 안전. 그 전엔 건드리지 않는다.

---

## 트랙 A — 호스팅 인프라 (독립, 안전) — ✅ 완료 (2026-05-22)

> 삭제 완료. **단, 프론트 React 코드(config.js `AGENT_API`, `AgentChat.js`, DocEditor AI생성)는
> 트랙 B/§6(채팅 UI 결정)로 보류** — 배포 플러밍(compose/.env/entrypoint/preflight)만 제거했으므로
> AGENT_API(:9002) 호출은 backend 부재로 graceful fail 상태. **docs 서술 갱신(deploy/README,
> DEPLOY.md, docs/architecture/*)은 별도 doc-sweep 으로 보류**(코드/배포 동작엔 영향 없음).
> 검증: Platform-API go.work 빌드 OK, 양 compose YAML 파싱 OK, 코드/배포 잔여 참조 0.


### A-1. 완전 삭제 (파일/디렉터리)
- `Agent-Service/` 전체 (Python 서비스 — 다른 모듈 import 없음)
- `deploy/images/agent-service/Dockerfile`
- `deploy/systemd/opencode@.service` (멀티유저 OpenCode 호스팅 unit)
- `deploy/scripts/gojira-user-add.sh`, `deploy/scripts/gojira-user-remove.sh` (사용자 프로비저닝)
- `deploy/scripts/dev-opencode-register.sh` (start-all 호출 제거 후)
- `Platform-API/db/opencode_instance.go` (opencodeInstances 컬렉션 CRUD)
- `Platform-API/handlers/opencode_healthcheck.go` (인스턴스 헬스체크 + `/admin/opencode-healthcheck`)

### A-2. 블록/라인 제거
- `deploy/docker-compose.yml`: `agent-service` 서비스 블록(~119–137), `REACT_APP_AGENT_API`(~21)
- `docker-compose.yml`(루트): agent-service 주석(~21–23)
- `start-all.bat`: venv 검사(~20–24), dev-opencode-register 호출(~59–61), 시작 블록(~83–91), 출력(~122)
- `stop-all.bat`: 종료(~15–16), 포트 9002 cleanup(~38)
- `deploy/.env.example`: `AGENT_API_SERVER`/`AGENT_API_PORT`(~83–84), `AGENT_API_HOST_PORT` 주석(~108)
- `deploy/images/{platform-app,gojira-app}/entrypoint.sh`: `REACT_APP_AGENT_API` 주입 라인
- `Platform-API/main.go`: `InitOpencodeInstanceIndexes()` 호출 / `StartOpencodeHealthcheck()` 고루틴 /
  `/admin/opencode-healthcheck` 라우트 (line 코드 재확인)
- `deploy/scripts/`: `build-images.sh`/`images.txt`/`preflight.sh` 의 agent-service 항목(있으면)

### A-3. Mongo
- 컬렉션 `opencodeInstances` drop (코드 제거 후). 인덱스는 컬렉션 drop 시 함께 제거.

---

## 트랙 B — 프론트엔드 write UI

### B-1. GoJIRA-App `src/components/DocEditor.js`
**제거 대상 핸들러+버튼** (line 근사):
- `handleSave`(PUT /file), `handleDelete`/`handleDeleteFile`(DELETE /file),
  `handleNewFile`/`handleRenameFile`(PUT/DELETE /file), `handleEditModeChange`(edit/start·release),
  `handlePlantumlCommit`(PUT /file, PexBlock.onCommit), 기존 `handleCreatePR`(서버 worktree 경유).
- 관련 JSX: 새파일(+)·발행신청·이름변경/삭제·저장·편집모드 토글.

**보존(신규 로컬 경로)**: `handleLocalEdit`, `handleLocalPublish`, `utils/launcher.js`, 파일 조회(GET) 일체.

> 정책 결정 필요: 인라인 PlantUML 편집(`handlePlantumlCommit`)은 로컬 pex-vscode 로 이전(설계 §4).
> 웹 PexBlock 은 **조회 전용**으로 강등.

### B-2. Platform-App `src/components/DocEditor.js`
- `handleSave`/`handleDelete`/`handleNewFile`/`handleRenameFile`/`handleDeleteFile`/`handleCreatePR`/
  `handleEditModeChange` 전부 제거(웹 편집 폐기). 보존: 파일 조회.
- **이 앱엔 로컬 런처 버튼 미반영** — 필요 시 GoJIRA-App 의 launcher 연동을 이식할지 결정(§6).

### B-3. Platform-App `src/components/DocManager.js`
- 내부 DocEditor 서브: `startEdit`/`discardEdit`/`handleSave`/`handleCommit`/`handleCreatePR` (worktree 편집) → 제거 또는 Edit 탭 비활성화.
- **보존**: `PRList` 의 `handleReview`/`handleMerge` (Gitea-API 기반 PR 리뷰/머지) — 그대로 둔다.

> **진행 현황 — 트랙 B 전체 완료 (2026-05-22)**:
> - **B1(웹 채팅 폐기) ✅ commit 818acc5** — AgentChat(양 앱) 삭제, `?mode=agent` 마운트/로고/
>   RelationMap·SATAM assist 트리거 제거, AGENT_API config·AI생성(enrich) 호출 제거.
> - **B2(GoJIRA-App DocEditor read-only) ✅ commit 280593b** — 파일 CRUD·편집모드·textarea·
>   인라인 PlantUML write·구 발행신청 제거. 보존: 조회/Diff/graph + 로컬 수정·로컬 push & 발행.
> - **B3(Platform-App read-only) ✅ commit 03b67c9** — DocEditor write 제거, DocManager 내부
>   worktree 편집기(Edit 탭) 제거. 보존: DocTree/Commits/PRList(리뷰·머지)/Graph. CommonBar
>   "편집모드" 토글은 onEditChange 게이팅(이슈/Relation/SATAM 편집은 그대로, 문서편집만 숨김).
> - **검증 한계**: node_modules 부재로 전체 craco 빌드는 미실행 — babel JSX 파싱 + 잔여참조
>   스캔까지. 실제 런타임 확인은 node 환경 빌드에서 권장.

### B-4. AGENT_API(:9002) 참조 — ✅ B1 에서 제거 완료
- `GoJIRA-App/src/config.js`, `Platform-App/src/config.js`: `AGENT_API` 항목.
- `*/DocEditor.js`: `AGENT_API`/`/Agent/doc/enrich`(AI생성) 호출.
- `*/AgentChat.js`: `/Agent/chat/send`·`/session/*`·`/llm`·`/terminal` 등 전부.
- **선결정(§6)**: 웹 채팅 UI 폐기 vs 로컬 `opencode serve` 직결. 폐기면 AgentChat 양쪽 + AI생성 제거.

---

## 트랙 C — Platform-API write 엔드포인트 (read 와 분리 후)

> **부분완료 (2026-05-22, commit a960cb6)** — 핵심 발견: EditSession/worktree write 의
> 상당수는 **웹 UI 전용이 아니라 서버측 스킬 문서생성 경로**다. 코드 전역 호출자 스캔 결과:
> - **잔여 호출자 0 → 제거함**: `sync-branch`, `git/publish`, `PUT edit/file`, `edit/commit`,
>   `edit/pr`, `edit/discard`, `edit/heartbeat`, `edit/rebase` (라우트+핸들러).
> - **아직 사용 중 → 보존**: `edit/start`·`edit/release`(스킬 `_edit-acquire/_edit-release`),
>   `PUT /file`·`DELETE /file`(vos-collect2/doc.go/reset-project), `notify-change`(doc-save.sh),
>   `init-files`·`ensure-branch`(프로젝트 셋업), `conflict-resolve`(알림 핸들러), `/pr`·`review`·
>   `merge`(launcher flow/cli/PRList).
> - **결론**: 나머지 write 제거는 **트랙 D(스킬 실행모델)와 한 묶음** — 스킬이 로컬 CLI 로
>   로컬 clone 에 직접 write 하도록 전환해야 `PUT /file`/`notify-change`/EditSession 을 뗄 수 있다.
> - localrepo.go/edit_session.go 의 worktree·세션 함수는 보존(잔존 엔드포인트가 계속 사용).
> - C-2 부분수정(PRCreate 의 `!skipAutoCommit` CommitAndPush 제거)도 cli/skill 호환 위해 **D 로 연기**.


### C-1. 안전 제거 (write 전용 핸들러) — `handlers/docrepo.go`
`DocRepoFileUpdateHandler`(PUT /file), `DocRepoFileDeleteHandler`(DELETE /file),
`DocRepoEditStartHandler`(edit/start), `DocRepoEditReleaseHandler`(edit/release),
`DocRepoEditHeartbeatHandler`(edit/heartbeat), `DocRepoEditRebaseHandler`(edit/rebase),
`DocRepoEditSaveHandler`(PUT edit/file), `DocRepoEditCommitHandler`(edit/commit),
`DocRepoEditDiscardHandler`(DELETE edit/discard), `DocRepoEditPRHandler`(edit/pr),
`DocRepoGitPublishHandler`(git/publish), `DocRepoEnsureBranchHandler`(ensure-branch),
`DocRepoSyncBranchHandler`(sync-branch), `DocRepoConflictResolveHandler`(conflict-resolve),
`DocRepoInitFilesHandler`(init-files).
→ `main.go` 라우터의 해당 등록 라인도 함께 제거.

### C-2. 부분 수정 (삭제 아님)
- `DocRepoPRCreateHandler`(POST /pr): worktree commit 분기(`!skipAutoCommit` 의 `CommitAndPush`) 제거,
  Gitea PR 생성/리뷰어/메일은 보존. (로컬 런처 push 전제)
- `DocRepoPRMergeHandler`(merge): worktree 정리(`RemoveWorktreeOnly`) 코드 제거, 머지 자체는 보존.
- `DocRepoCreateHandler`(POST /DocRepo/{code}): repo 생성·init 은 유지하되 로컬 worktree 초기화 부분만 정리.

### C-3. 보존 (READ)
`DocRepoListHandler`, `DocRepoTreeHandler`, `DocRepoFileHandler`(GET /file),
`DocRepoCommitsHandler`, `DocRepoGraphHandler`, `DocRepoBranchesHandler`,
`DocRepoPRListHandler`, `DocRepoPRReviewsHandler`, `DocRepoEditFileHandler`(GET edit/file),
`DocRepoEditStatusHandler`, `DocRepoEditPendingHandler`, `DocRepoWorktreeInfoHandler`,
`DocRepoNotifyChangeHandler`(SSE 트리거 — 트랙 D 와 연동 재검토).

### C-4. `github/localrepo.go` 함수
- **삭제(write 전용)**: `EnsureWorktree`, `WriteWorktreeFile`, `CommitAndPush`, `DiscardWorktree`,
  `RemoveWorktreeOnly`, `RebaseOnMain`, `CheckoutNewBranch`, `ensureWorktreeExclude`, `worktreeDir`(write만 쓰면).
- **보존(read 공용)**: `EnsureClone`, `GetLocalRepoDir`, `ListLocalRepos`, `ReadWorktreeFile`,
  `HasPendingWork`, `PendingFiles`.

### C-5. `db/edit_session.go` + Mongo
- **삭제(write)**: `AcquireEditSession`, `ReleaseEditSession`, `UpdateHeartbeat`, `ExpireStaleSessions`.
- **보존(read, 당분간)**: `FindActiveSession*`, `IsSessionExpired`.
- Mongo `editSessions` 컬렉션: write 제거 후 무의미 → 정리(리셋 스킬 flush).
- 헬퍼: `getUserToken`/`extractTreeFiles`/`translateRepoName` 등 write 전용 헬퍼 동반 제거,
  `getRepoInfo`/`toStringSlice`/`buildLocalTree` 등 공용은 보존.

---

## 트랙 D — 스킬 write 경로 (실행모델 선결정 후)

`.claude/skills/_scripts/`: `doc-save.sh`, `_edit-acquire.sh`, `_edit-release.sh`, `_notify-change.sh`,
(`plantuml-fix.py` 는 `doc-save` 의존 — 단, plantuml-doc-fix 스킬은 독립 호출도 있음, 재확인).

> 이 스크립트들은 **현재 스킬이 문서를 생성·저장하는 핵심 경로**다. 로컬 전환 모델에서는
> 로컬 CLI(opencode/claude)가 로컬 clone 파일에 직접 write 하고 사용자가 push 하므로 불필요해지지만,
> 그 실행모델(웹 채팅 폐기 / 로컬 직결)이 §6 에서 확정되기 전에는 **제거 보류**. 잘못 제거하면
> 전체 문서 생성 워크플로우가 끊긴다.

---

## 미해결 / 선결정 (삭제 착수 전 확정)
1. **웹 채팅 UI(AgentChat) 폐기 여부** — 폐기면 트랙 B-4 + 트랙 D 동시 정리 가능. (§6 항목)
2. **Platform-App 의 로컬 런처 연동 이식 여부** — 두 앱 중 어디를 "편집 진입점"으로 둘지.
3. **인라인 PlantUML 편집** 웹 강등(조회 전용) 시점 — pex-vscode 이전과 동기화.
4. **롤백 안전장치** — 트랙별 단일 커밋 + `git revert` 가능하게. compose/배포는 태그 분리.
