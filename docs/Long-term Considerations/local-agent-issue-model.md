# 트랙 E 설계 — 이슈 로컬 모델 (YAML 로컬 작성 + Mongo 캐시)

> 기록 일자: 2026-05-22
> 상태: **설계 (실행 대기)** — 코드 변경 전 방향 확정.
> 상위: [`local-agent-mvp.md`](./local-agent-mvp.md), 선행: [`local-agent-skill-execution.md`](./local-agent-skill-execution.md)(트랙 D).
> 배경: 트랙 D §4 서버 worktree 제거가 **이슈 YAML 영속화 경로**에 막힌 것을 해소.

## 0. 한 줄 요약

이슈 YAML(`data/issues/*.yaml`)을 문서와 동일하게 **로컬 clone 에서 작성**하고 문서와 **함께
commit/push** 한다. **YAML = 진실, Mongo = 라이브 읽기 캐시**. 이슈 편집 시 로컬이 YAML write +
Mongo upsert 를 **동시(dual-write)** 수행하고, 과제 열 때 로컬 YAML 을 Mongo 로 **ingest**(콜드
스타트)한다. 웹은 이슈도 **읽기 전용**.

## 1. 현재 메커니즘과 막힌 지점

- 이슈 생성/수정/삭제/링크(`GoJIRA-API/handlers/issue.go`, `link.go`)는:
  1. YAML doc build → `clients.SaveFile` = **Platform-API `PUT /DocRepo/{code}/file`** → 서버
     worktree write (`WriteWorktreeFile`). *(주석: "YAML 이 진실")*
  2. `db.UpsertIssue` 로 Mongo **캐시** upsert. *(주석: "실패해도 YAML 이 진실이므로 경고만")*
- 즉 **현 설계도 이미 YAML=진실 / Mongo=캐시**다. 다만 YAML 이 **서버 worktree** 에 쓰인다.
- 트랙 D §4 가 worktree write 인프라(`EnsureWorktree`/`WriteWorktreeFile`/worktree TTL/PR 시점
  `CommitAndPush`/`edit_session.go`)를 제거하려 했으나, 위 이슈 경로가 이를 잡고 있어 **제거 불가**.
- 역방향(YAML→Mongo) ingest 는 **현재 없음**(Mongo→YAML 단방향만 존재).
- `gojira-cli/cmd/issue.go` 도 `GoJIRA-API /Issue`(create/update/delete/link) 호출 = 같은 경로.

## 2. 목표 모델 (이슈도 로컬 clone)

문서 pivot 과 완전 대칭:

| | 과거 (서버 worktree) | 로컬 모델 (목표) |
|---|---|---|
| 문서 | 서버 worktree write | 로컬 clone write (doc-save.sh, 트랙 D 완료) |
| 이슈 YAML | 서버 worktree write + Mongo 캐시 | **로컬 clone write + Mongo 캐시 dual-write** |
| 커밋·푸시 | PR 시점 일괄 | 문서+YAML 한 번에 로컬 push → PR |
| 웹 | 편집 | **읽기/리뷰 전용** |

### 2.1 이슈 편집 1건 (로컬 dual-write)

```
로컬 이슈 편집 (gojira-cli issue / 이슈 스킬, 로컬 clone cwd)
  ① data/issues/<subkey>.yaml 로컬 fs write/delete   (doc-save.sh 와 같은 패턴)
  ② GoJIRA-API ingest 호출 → Mongo upsert/delete      (웹이 즉시 최신 반영)
  ③ (나중에) 문서와 함께 commit → push → PR             (git 이력)
```

- ①의 repo 루트 결정은 doc-save.sh 와 동일: `GOJIRA_REPO_ROOT`(런처 주입) → git toplevel fallback.
- ②는 **YAML 내용을 그대로 보내** GoJIRA-API 가 파싱→Mongo 변환(파생 로직 단일화). 로컬은 fs 만, 서버는 Mongo 만 책임.

### 2.2 콜드 스타트 (과제 열기 / 캐시 비어있음)

```
런처가 clone/pull (data/issues/*.yaml 항상 동봉되어 내려옴)
  → 로컬 YAML 전부 스캔 → GoJIRA-API bulk ingest → Mongo 등록
  (Mongo 비었으면 필수, 평상시엔 reconcile 로 사용해도 됨)
```

- YAML 이 항상 로컬에 존재(clone 이 같이 내려줌)하므로 ingest 소스는 늘 로컬.

### 2.3 일관성 / self-heal

- ②(Mongo upsert)가 실패(오프라인 등)해도 **YAML 이 진실**이므로, 다음 콜드 스타트(2.2)의
  ingest 가 Mongo 를 YAML 기준으로 재정렬(reconcile) → "동일하게 일관된 정보 유지".
- 따라서 dual-write 의 일시적 어긋남은 **ingest 가 복구**한다(웹훅·분산 트랜잭션 불필요).

## 3. 변경 대상

### 3.1 신규 — GoJIRA-API ingest (YAML → Mongo, 파싱 단일화)
- `POST /Issue/ingest` (단건): YAML 본문 1개 → 파싱 → `UpsertIssue` (2.1 ② 용).
- `POST /Project/{code}/issues/ingest` (벌크): YAML 목록/디렉터리 페이로드 → 일괄 upsert (2.2 용).
- 삭제: `DELETE /Issue/ingest`(key) 또는 ingest 가 "현재 YAML 집합 = 진실"로 보고 누락분 정리.
- YAML→Mongo 변환은 현 `issue.go` 5단계(`cacheDoc` = doc + projectId + _syncedAt)를 헬퍼로 추출해 재사용.

### 3.2 이슈 쓰기 경로 로컬화 — 확정 구현 (2026-05-22)

> **설계 보정**: "로컬 YAML write + ingest 호출"을 그대로 가면 **키/subkey 할당**(전역 카운터
> `db.NextIssueNum`/`NextSubkey`, Mongo)과 update 병합·link 양방향 갱신 로직을 CLI 에 중복
> 구현해야 한다. 키 할당은 본질상 전역 조율이라 로컬 단독 불가(다중 사용자 키 충돌). 그래서
> **서버를 "작성 두뇌"로 유지하되 fs 만 떼는** 방식으로 확정:

- **`X-Local-Write` 헤더 게이트 듀얼 모드** (`handlers/issue.go`·`link.go`):
  - 헤더 있음(CLI/에이전트) → 서버는 할당·병합·link·Mongo upsert 를 그대로 하고 **worktree 에
    손대지 않음**. 대신 응답에 `localWrites: [{path, content}|{path, delete:true}]` 동봉.
  - 헤더 없음(웹/legacy) → 기존대로 `clients.SaveFile/DeleteFile`(서버 worktree). **웹 무중단**.
- **CLI** (`gojira-cli/cmd/issue.go` + `common.go`): create/update/delete/link 가 `apiWriteLocal`
  (헤더 부착)로 호출 → 응답 `localWrites` 를 `applyLocalWrites` 로 repo root(`GOJIRA_REPO_ROOT`
  또는 git toplevel) 기준 로컬 clone 에 기록/삭제. 로직·할당 중복 0.
- 즉 **별도 ingest 왕복 불필요**(서버가 같은 호출에서 Mongo upsert) — ingest 엔드포인트는
  콜드스타트 벌크·reconcile·수작업 YAML 편집 반영 용도로 잔존.
- `clients.SaveFile/DeleteFile` 제거는 **헤더 없는(웹) 경로가 사라진 뒤**(3.3/3.4) — 현재는
  legacy 분기로 보존.
- (덤) 기존 CLI `issue link` 가 `{fromKey,toKey,type}` 를 보내 서버 `{inwardKey,outwardKey,
  linkType}` 와 어긋나 깨져 있던 것도 이때 수정.

### 3.3 웹 이슈 UI → 읽기 전용 (track B 의 이슈판)
- `RelationMap`(드래그 연결), `SATAM`, `IssueList`(인라인 편집/생성/삭제), `IssueDetailDialog` 의
  **편집·생성·삭제 경로 제거**, `onEditChange` 게이트 해제. 웹은 Mongo 캐시에서 **조회만**.
- 이슈 편집은 로컬(CLI/에이전트)로 이전. (원하면 후속에 웹→런처 브릿지로 우회 — 현 범위 밖)

### 3.4 트랙 D §4 서버 제거 — E 완료 후 비로소 가능
이슈 경로가 worktree 를 안 잡으면:
- `PUT /DocRepo/{code}/file`, `DELETE /file` 제거(문서 read-only + 이슈 로컬화로 호출자 0).
- `EnsureWorktree`/`WriteWorktreeFile`/worktree TTL/`StartTTLWorker`, PR 시점 `CommitAndPush` 분기,
  `edit_session.go`(잔존 호출자 재확인) 제거.
- dead 엔드포인트(edit/start·release·status·pending·file, notify-change) 제거.
- 보존: `EnsureClone`/read 계열, Gitea PR 생성·리뷰·머지.

## 4. 선결정 — 확정 (2026-05-22)

1. **웹 이슈 = 외부 읽기 전용** ✅ — 편집은 로컬, 웹은 Mongo 캐시 조회.
2. **YAML = 진실, Mongo = 라이브 캐시** ✅ — 현 설계 철학 유지(issue.go 주석 근거).
3. **이슈 편집 = 로컬 dual-write** ✅ — 로컬 YAML write + GoJIRA-API ingest(Mongo) 동시.
4. **콜드 스타트 = 로컬 YAML → Mongo ingest** ✅ — clone 이 YAML 동봉하므로 소스는 항상 로컬.
5. **일관성 = ingest reconcile self-heal** ✅ — 웹훅/분산 트랜잭션 없음.
6. **YAML→Mongo 변환 단일화** ✅ — ingest 헬퍼 한 곳(현 cacheDoc 로직 추출).

## 5. 실행 순서 (제안)

1. ✅ GoJIRA-API ingest 엔드포인트(단건/벌크) + cacheDoc 변환 헬퍼 추출. (커밋 `559a55c`)
2. ✅ `gojira-cli issue` 로컬화 — `X-Local-Write` 듀얼 모드 + `applyLocalWrites`(3.2 확정안).
   서버 fs 분기는 헤더로 게이트, 웹 무중단. 이슈 스킬은 doc-save 패턴 따라 후속.
3. (3.2 에 흡수) `SaveFile/DeleteFile` 는 헤더 없는 웹 경로용 legacy 분기로만 잔존 → 5/6 후 제거.
4. ✅ 런처: `/open`·`/edit` 콜드스타트 시 로컬 YAML 벌크 ingest(reconcile=true) + `Push` 에
   `git pull --rebase`(경로 분리 무충돌) + spawn 시 `GOJIRA_REPO_ROOT`/`GOJIRA_USER`/`GOJIRA_API_URL`
   주입(`ingest.go`/`spawn.go`/`repo.go`).
5. ✅ 웹 이슈 UI 읽기 전용화(3.3) — RelationMap/SATAM/IssueList 가 `CommonBar` 에
   `onEditChange` 미전달 → 편집 토글 미노출. `editMode` 를 세터 없는 `useState(false)` 로
   고정해 모든 편집·생성·삭제·링크 UI 게이트 차단. IssueDetailDialog 는 `editMode=false` 로
   이미 readonly. write 함수는 dead JSX 에 lexically 잔존(런타임 도달 불가, lint 무경고).
6. 트랙 D §4 서버 worktree 제거(3.4) + legacy SaveFile 분기 제거 — 각 단계 go build 검증.

## 관련 문서
- [`local-agent-mvp.md`](./local-agent-mvp.md) — 상위 §5/§6
- [`local-agent-skill-execution.md`](./local-agent-skill-execution.md) — 트랙 D(문서 로컬화). §4 서버 제거는 본 트랙 E 완료가 선행.
- [`local-agent-removal-inventory.md`](./local-agent-removal-inventory.md) — 트랙 A~C 인벤토리
