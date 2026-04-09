# 이벤트 처리 흐름 (Gitea → 서버 → 프론트엔드)

## 1. 아키텍처 개요

```
Gitea Event (webhook)
    ↓
Platform-API webhook.go (handlePREvent / handlePushEvent / handlePRReviewEvent)
    ↓
BroadcastDocEvent() → broadcast() → WebSocket 전송
    ↓
sseManager.js (WebSocket 수신 → 이벤트별 콜백 라우팅)
    ↓
eventQueue.js (비동기 큐 — 순차 처리)
    ↓
GoJIRAApp.js / ProjectMap.js / DocEditor.js / App.js (UI 갱신)
```

---

## 2. 이벤트 타입 및 라우팅

| WebSocket 이벤트 | 내부 type | 발생 조건 | 처리 컴포넌트 |
|-----------------|-----------|----------|-------------|
| `doc_update` | `project_alert` | main push (머지 포함) | GoJIRAApp, ProjectMap, DocEditor |
| `doc_update` | `pr_update` | PR 생성/닫기/머지 | GoJIRAApp, ProjectMap |
| `doc_update` | `pr_review` | PR 승인/반려 | App.js (반려 시 생성자 알림) |

---

## 3. 시나리오별 처리 흐름

### A. Main Push (문서 변경 / 머지에 의한 main 갱신)

```
Gitea push event → refs/heads/main
    ↓
webhook.go handlePushEvent()
    ├─ findProjectByRepo() → projectCode
    ├─ compareAllBranches() → HasUserCommits()로 diff 사용자 판정
    ├─ DB: branchAlerts 갱신
    └─ BroadcastDocEvent({type: "project_alert", projectCode, branchAlerts})
        ↓
    WebSocket → sseManager.js → listeners["doc_update"] 호출
        ↓
    ┌─ GoJIRAApp.js (컴포넌트 뷰)
    │   └─ eventQueue.push(async () => {
    │       compareTree(main, docs/user) → setProjectAlert({branchDiff})
    │   })
    ├─ ProjectMap.js (프로젝트 맵)
    │   └─ reloadAlerts() → GET /Project/alerts?user={user}
    │       → setAlerts() → ProjectRow 색상/배지 갱신
    └─ DocEditor.js (Docs 화면)
        └─ refreshDocState() → mainTree + userTree + fileStatus 재계산
```

**UI 변화:**
- ProjectMap: 프로젝트 라인/텍스트 → tomato 색상 + `alert-diff.svg` 배지
- CommonBar: 프로젝트명 옆 `alert-diff.svg` 배지
- DocEditor: 파일 목록 갱신, mainOnly 파일 `[M]` 표시

---

### A-2. 프론트엔드 문서 변경 (Git Pull, 파일 저장/생성/삭제/이름변경)

Gitea webhook이 아닌 **사용자 조작에 의한 문서 상태 변경**. 프론트엔드 내부에서 처리.

```
사용자 조작 (Git Pull / 파일 저장 / 파일 생성 / 파일 삭제 / 이름 변경)
    ↓
DocEditor 해당 핸들러 (handleGitPull / handleSave / handleNewFile / handleDelete / handleRenameFile)
    ├─ API 호출 (sync-branch / PUT file / DELETE file)
    └─ refreshDocState()
        ├─ main 트리 + user 트리 재로드
        ├─ fileStatus 재계산
        └─ onDocStateChange() 콜백 호출
            ↓
        GoJIRAApp.refreshAlert()
            ├─ compareTree(main, docs/user) → branchDiff 재계산
            └─ setProjectAlert({branchDiff}) → CommonBar 배지 갱신
```

**호출 포인트 (7곳):**

| 조작 | 핸들러 | API | UI 변화 |
|------|--------|-----|---------|
| Git Pull | `handleGitPull` | `POST /sync-branch` | 브랜치 리셋 → diff 해소 → 배지 제거 |
| 파일 저장 | `handleSave` | `PUT /file` | 변경 발생 → diff 배지 표시 |
| 파일 생성 | `handleNewFile` | `PUT /file` | 추가 파일 → diff 배지 표시 |
| 파일 삭제 (toolbar) | `handleDelete` | `DELETE /file` | 파일 제거 → diff 재계산 |
| 파일 삭제 (sidebar) | `handleDeleteFile` | `DELETE /file` | 파일 제거 → diff 재계산 |
| 이름 변경 | `handleRenameFile` | `PUT + DELETE /file` | 파일명 변경 → diff 재계산 |
| SSE/WS 수신 | SSE 핸들러 | — | 외부 변경 반영 |

**핵심 구조:**
- `refreshDocState()` — DocEditor 내부 상태 갱신 (mainTree, userTree, fileStatus)
- `onDocStateChange()` — GoJIRAApp 콜백 → `projectAlert` 재계산 (CommonBar 배지)
- 모든 문서 변경 포인트에서 `refreshDocState()` 1회 호출로 양쪽 모두 갱신

---

### B. PR 생성

```
Gitea pull_request event → action: "opened"
    + pull_request event → action: "review_requested" (리뷰어 지정)
    ↓
webhook.go handlePREvent()
    ├─ findProjectByRepo() → projectCode
    ├─ requested_reviewers 추출
    ├─ DB: gitOpenPRReviewers = ["reviewer1", ...]
    └─ BroadcastDocEvent({type: "pr_update", action: "opened"/"review_requested", reviewers, projectCode})
        ↓
    WebSocket → sseManager.js
        ↓
    ┌─ GoJIRAApp.js
    │   └─ eventQueue.push(() => {
    │       isReviewer = reviewers.includes(user)
    │       if (isReviewer) setProjectAlert({hasOpenPR: true})
    │   })
    └─ ProjectMap.js
        └─ isReviewer 확인 → setAlerts({hasOpenPR: true})
```

**UI 변화 (리뷰어만):**
- ProjectMap: 프로젝트 라인/텍스트 → `#27ae60` 색상 + `alert-pr.svg` 배지
- CommonBar: 프로젝트명 옆 `alert-pr.svg` 배지

---

### C. PR 리뷰 — 승인

```
Gitea pull_request_approved event
    ↓
webhook.go handlePRReviewEvent()
    ├─ reviewState → "approved" (이벤트명으로 보정)
    ├─ BroadcastDocEvent({type: "pr_review", state: "approved"})
    └─ 대기 리뷰어 0명이면 → notifyMergeable() 메일 발송
        ↓
    WebSocket → sseManager.js
        ↓
    App.js: state !== "changes_requested" → 무시
```

**UI 변화:** 없음 (DocManager에서 리뷰 목록 갱신은 별도 fetch)

---

### D. PR 리뷰 — 반려

```
Gitea pull_request_rejected event
    ↓
webhook.go handlePRReviewEvent()
    ├─ reviewState → "changes_requested" (이벤트명으로 보정)
    ├─ DB: SaveNotification(prCreator, "pr_rejected", ...) ← 미접속 사용자용
    └─ BroadcastDocEvent({type: "pr_review", state: "changes_requested", prCreator, reviewer, body})
        ↓
    WebSocket → sseManager.js
        ↓
    App.js (PR 생성자에게만):
        └─ prCreator === user 확인
            └─ modalConfirm("PR이 반려되었습니다. 재반영하시겠습니까?")
                ├─ OK → POST /DocRepo/{code}/conflict-resolve
                │       1. 변경 파일 보관
                │       2. 브랜치 삭제 → main에서 재생성
                │       3. 보관 파일 재반영
                │       → modalAlert("N개 파일이 재반영되었습니다")
                └─ 취소 → 무시
```

**미접속 사용자:**
- 다음 접속 시 `GET /Notification/unread?user={user}` → 미읽은 알림 조회
- 동일 팝업 표시 → 동일 처리
- 처리 후 `POST /Notification/read?user={user}` → 읽음 처리

---

### E. PR 머지

```
Gitea pull_request event → action: "closed", merged: true
    + push event → refs/heads/main (머지 커밋)
    ↓
webhook.go handlePREvent()
    ├─ merged=true, state="closed"
    ├─ ListPRs(open) → 남은 open PR 확인
    ├─ open PR 0개면 → DB: gitOpenPRReviewers = []
    └─ BroadcastDocEvent({type: "pr_update", action: "closed", merged: true})
        ↓
    WebSocket → sseManager.js
        ↓
    ┌─ GoJIRAApp.js
    │   └─ eventQueue.push(() => {
    │       setProjectAlert({hasOpenPR: false})  ← null로 만들지 않음
    │   })
    └─ ProjectMap.js
        └─ reloadAlerts() → alerts API 재조회

webhook.go handlePushEvent() (동시 발생)
    ├─ compareAllBranches() → diff 사용자 판정
    └─ BroadcastDocEvent({type: "project_alert", branchAlerts})
        ↓
    ┌─ GoJIRAApp.js
    │   └─ eventQueue.push(async () => {  ← pr_update 완료 후 실행
    │       compareTree() → setProjectAlert({branchDiff})
    │   })
    ├─ ProjectMap.js
    │   └─ reloadAlerts()
    └─ DocEditor.js
        └─ refreshDocState()
```

**이벤트 큐 순서 보장:**
```
queue: [pr_update(hasOpenPR=false)] → [project_alert(트리비교→branchDiff)]
         ↑ 먼저 완료                      ↑ 이후 실행 (비동기 완료 대기)
```

**UI 변화:**
- PR 배지 해제
- branchDiff 있으면 diff 배지 표시 (main이 변경되어 다른 사용자 브랜치와 차이 발생)

---

### F. PR 닫기 (머지 아님)

```
Gitea pull_request event → action: "closed", merged: false
    ↓
webhook.go handlePREvent()
    ├─ state="closed", merged=false
    ├─ ListPRs(open) → 남은 open PR 확인
    ├─ open PR 0개면 → DB: gitOpenPRReviewers = []
    └─ BroadcastDocEvent({type: "pr_update", action: "closed", merged: false})
        ↓
    GoJIRAApp: setProjectAlert({hasOpenPR: false})
    ProjectMap: reloadAlerts()
```

**UI 변화:** PR 배지 해제 (push 이벤트 없으므로 branchDiff 변화 없음)

---

## 4. 데이터 구조

### MongoDB 프로젝트 필드

| 필드 | 타입 | 갱신 시점 | 설명 |
|------|------|----------|------|
| `branchAlerts` | `[]string` | main push webhook | diff 있는 사용자 ID 목록 (ancestor 기반) |
| `gitOpenPRReviewers` | `[]string` | PR opened/closed webhook | 현재 open PR의 리뷰어 목록 |

### 프론트엔드 상태

| 상태 | 컴포넌트 | 구조 | 용도 |
|------|---------|------|------|
| `projectAlert` | GoJIRAApp | `{jiraCode, branchDiff, hasOpenPR}` | CommonBar 배지 표시 |
| `alerts` | ProjectMap | `{[jiraCode]: AlertInfo}` | ProjectRow 색상/배지 |
| `mainTree/userTree/fileStatus` | DocEditor | 트리 객체 + 파일별 상태 | 파일 목록 표시 |

---

## 5. 알림 배지 색상/아이콘 정책

| 조건 | 색상 | 아이콘 | 위치 |
|------|------|--------|------|
| PR 대기 (리뷰어만) | `#27ae60` | `alert-pr.svg` | ProjectMap 배지 + CommonBar |
| 문서 차이 (모든 사용자) | `tomato` | `alert-diff.svg` | ProjectMap 배지 + CommonBar |
| 동시 발생 | PR 우선 | 양쪽 모두 표시 | 좌: PR, 우: diff |

---

## 6. 핵심 파일 참조

| 역할 | 파일 |
|------|------|
| Webhook 수신/처리 | `Platform-API/handlers/webhook.go` |
| WebSocket + broadcast | `Platform-API/handlers/sse.go` |
| Notification DB | `Platform-API/db/notification.go` |
| conflict-resolve API | `Platform-API/handlers/docrepo.go` |
| WebSocket 클라이언트 | `Platform-App/src/utils/sseManager.js` |
| 이벤트 큐 | `Platform-App/src/utils/eventQueue.js` |
| 컴포넌트뷰 이벤트 | `Platform-App/src/apps/GoJIRAApp.js` |
| 프로젝트맵 이벤트 | `Platform-App/src/apps/gojira/ProjectMap.js` |
| Docs 트리 갱신 + refreshDocState | `Platform-App/src/components/DocEditor.js` |
| GoJIRAApp refreshAlert 콜백 | `Platform-App/src/apps/GoJIRAApp.js` (refreshAlert → onDocStateChange) |
| 반려 알림 팝업 | `Platform-App/src/App.js` |
| Alerts API (트리 비교) | `GoJIRA-API/handlers/project_alerts.go` |
