# 📘 Git 기반 설계 문서 관리 시스템 요구사항서 (Full Version)

---

## 1. 📌 개요

본 시스템은 Git 기반으로 소프트웨어 설계 문서를 관리하며,
사용자별 브랜치를 통해 작업을 수행하고 PR(Pull Request)을 통해 main 브랜치에 반영한다.

---

## 2. 🧩 핵심 개념

| 개념        | 설명                  |
| --------- | ------------------- |
| main 브랜치  | 배포 기준 문서            |
| user 브랜치  | 사용자 작업 브랜치          |
| 로컬 워크스페이스 | 서버 내 사용자 작업 공간 (캐시) |
| PR        | 사용자 작업 → main 반영 요청 |

---

## 3. 📂 브랜치 정책

```
main
 └── docs/{userId}
```

---

## 4. 🔄 로그인 시 시나리오

### 4.1 CASE 1: 작업 이력 없음

조건:

* user 브랜치 없음
* workspace 없음

처리:

```bash
git clone <repo>
```

UI:

* main 기준 파일 목록 표시
* "새 작업 시작" 안내

사용자가 시작 시:

```bash
git checkout -b user/{userId}
git push origin user/{userId}
```

---

### 4.2 CASE 2: 작업 이력 있음

#### CASE 2-1: workspace 존재

```bash
git fetch origin
git status
git diff origin/main
```

---

#### CASE 2-2: workspace 없음

```bash
git clone <repo>
cd <repo>
git checkout user/{userId}
git fetch origin main
```

UI:

* "이전 작업 복원됨"

---

## 5. 📁 파일 목록 정책

### 탭 구성

문서 목록 UI 좌측에 두 개의 탭을 제공한다:

| 탭 | 기준 브랜치 | 설명 |
| -- | --------- | ---- |
| Main | origin/main | 배포된 문서 목록 |
| 작업공간 | docs/{userId} | 사용자 작업 중인 문서 목록 |

### 비교 기준

* 작업공간 탭에서 origin/main과 비교하여 변경 상태를 표시

### 변경 상태 표시 (main 대비)

| 상태    | 뱃지 | 색상 |
| ----- | --- | -- |
| 변경 없음 | 없음 | 기본 |
| 수정    | M | 노랑 |
| 추가    | A | 초록 |
| 삭제    | D | 빨강 |

### mainOnly 파일 표시 (main에만 존재)

main에 있지만 사용자 브랜치에 없는 파일을 작업공간 탭에서 별도 표시한다.

| 상태 | 뱃지 | 텍스트 | 동작 |
| --- | --- | --- | --- |
| main에만 있는 파일 | M (gray `#bbb`) | gray italic | 읽기 전용 미리보기 (opacity 0.4) |

* mainOnly 파일은 사용자 변경이 아니므로 발행신청 변경 카운트에서 제외
* mainOnly 파일만 존재하고 실제 변경이 없으면 발행신청 버튼 비활성화
* 비활성화 시 안내 메시지 표시: "main에 새 파일이 추가되었습니다. Git Pull로 동기화 후 작업하세요."

### 미커밋 상태 표시 (워크스페이스 로컬 변경)

| 상태 | 뱃지 | 색상 |
| --- | --- | --- |
| 미커밋만 (main과 동일) | ● | orange |
| 미커밋 + main과 차이 | ● | tomato |

---

## 6. ✏️ 작업 정책

### 저장 기준

👉 commit = 저장

---

### commit

```bash
git add .
git commit -m "작업 저장"
```

---

### push

```bash
git push origin user/{userId}
```

---

## 7. 🔄 main 동기화

* 사용자가 **명시적으로 요청**할 때만 수행 (편집모드 진입 시 자동 실행하지 않음)
* 편집모드 진입 시에는 worktree 생성/복원만 수행하고, 사용자 브랜치 상태를 그대로 보여준다

```bash
git fetch origin
git rebase origin/main
```

### 충돌 발생 시 (자동 해결 — conflict-resolve)

`POST /DocRepo/{code}/conflict-resolve` API로 자동 처리:

1. 사용자 브랜치에서 main과 다른 파일의 내용을 보관
2. 사용자 브랜치를 삭제하고 main HEAD에서 재생성 (sync)
3. 보관된 파일을 새 브랜치에 재반영 (커밋)

```
conflict-resolve 흐름:
  1. main vs user 트리 비교 → 다른 파일 목록
  2. 각 파일 내용 다운로드 (보관)
  3. 브랜치 삭제 → main에서 재생성
  4. 보관된 파일을 새 브랜치에 PUT (재반영)
  → "N개 파일이 재반영되었습니다"
```

이 기능은 PR 반려 시 생성자가 OK하면 자동 실행되며, 수동으로도 호출 가능하다.

---

## 8. 🚀 PR 정책

### 조건

* main 대비 변경 존재 (diff가 없으면 PR 생성 차단)
* 브랜치가 main의 ancestor인 경우 → "변경사항이 없습니다. 파일을 수정한 후 발행 신청해주세요." 안내

### PR 포함 내용

* diff
* 변경 파일
* LLM 요약

### 다중 PR 처리 정책

여러 사용자가 동시에 PR을 생성한 경우, 머지 순서에 따라 충돌이 발생할 수 있다.

#### 머지 순서

* **선입선출 (FIFO)**: PR 번호가 낮은 것(먼저 생성된 것)을 우선 처리
* 머지 시도 시 선행 open PR이 존재하면 → "PR #N이 먼저 대기 중입니다. 선행 PR을 먼저 처리해주세요." 안내
* 선행 PR이 닫히거나 머지되면 다음 PR 머지 가능

#### 충돌 자동 해결

첫 번째 PR 머지 후 후속 PR에 충돌이 발생할 수 있다.

```
머지 요청
  ├─ mergeable 확인
  ├─ 충돌 있음 → 자동 동기화 시도
  │     1) rebase 시도 (히스토리 깔끔)
  │     2) rebase 실패 → merge 방식 시도
  │     3) merge도 실패 → 사용자에게 수동 해결 안내
  └─ 충돌 없음 → 머지 진행
```

| 단계 | 동작 | 설명 |
|------|------|------|
| 1. 자동 rebase | Gitea update-branch API (style=rebase) | 히스토리 선형 유지 |
| 2. 자동 merge | Gitea update-branch API (style=merge) | rebase 실패 시 fallback |
| 3. 수동 해결 | "PR을 닫고 Git Pull로 동기화한 후 다시 작업해주세요." | 자동 해결 불가한 충돌 |

#### 수동 해결 절차

1. 충돌 PR 닫기
2. Docs 화면에서 Git Pull (sync-branch) 실행 → 브랜치가 main 최신으로 동기화
3. 파일 재수정
4. 새 PR 생성

#### 실시간 알림

* PR 생성/머지/닫기 시 Gitea webhook → Platform-API → WebSocket broadcast
* ProjectMap: 프로젝트에 PR 알림 아이콘 (gold)
* 컴포넌트 뷰: CommonBar 타이틀 옆 알림 아이콘
* 브랜치 diff: 알림 아이콘 (tomato)

### Merge 후 처리

* 사용자 브랜치(docs/{userId})는 **유지**
* Workspace는 삭제
* 다음 접속 시 worktree를 사용자 브랜치 기준으로 재생성 (rebase는 사용자가 명시적으로 요청할 때만 수행)
* 프로젝트 알림 플래그(gitOpenPR) 해제 — open PR이 0개가 되면 자동 해제

---

## 9. 🧠 LLM 기능

* PR 요약
* 변경 영향 분석
* 커밋 메시지 생성

### 입력

* git diff
* commit log

### 출력

* 커밋 및 변경 주요 내역을 간략한 리포트 형태로 제공

### 프롬프트 관리

* LLM 프롬프트는 외부 파일로 관리하여 개발자가 직접 수정 가능하도록 한다
* 예: `prompts/pr-summary.txt`, `prompts/commit-message.txt`

---

## 10. ⚠️ 예외 처리

| 상황           | 처리       |
| ------------ | -------- |
| workspace 손상 | 삭제 후 재생성 |
| rebase 실패    | abort    |
| push 실패      | 재시도      |
| 충돌           | 수동 해결    |

---

## 11. 🧠 Workspace 캐싱 전략 (TTL = 30분)

### 기본 원칙

1. workspace는 캐시이다
2. 원격 user 브랜치가 Source of Truth
3. 언제든 재생성 가능해야 함

---

### 저장 위치

```
/workspace/{repo}/{userId}
```

---

### TTL 정책

* 마지막 접근 기준 TTL 경과 시 자동 삭제
* TTL 기본값: 30분 (설정으로 변경 가능)
* ⚠️ TTL 만료 시 미커밋 작업은 유실됨 — 사용자에게 사전 경고 필요

---

### 재사용 조건

* 정상 상태
* lock 없음
* rebase 진행 중 아님
* fetch 성공

---

### 폐기 조건

* TTL 초과
* git 상태 이상
* fetch 실패 반복
* 사용자 초기화 선택

---

### 복구

* user 브랜치 기준 재생성

---

## 12. 🔄 미커밋 작업 정책

### 원칙

* commit되지 않은 작업 = 임시 상태
* ⚠️ 저장(커밋)하지 않으면 작업 내용이 소실될 수 있음을 UI에서 명확히 안내

---

### 로그인 시

👉 "이전 작업이 있습니다. 이어서 하시겠습니까?"

---

### 선택

| 선택   | 동작  |
| ---- | --- |
| 이어하기 | 유지  |
| 폐기   | 초기화 |

---

### 폐기

```bash
git reset --hard origin/docs/{userId}
git clean -fd
```

---

## 13. 🔐 동시 편집 제어

### 정책

👉 1 사용자 = 1 세션

---

### Lock 저장소

* DB에서 세션 lock 관리 (userConnections 컬렉션 활용)

---

### 편집 시작 시

```json
{
  "userId": "user1",
  "sessionId": "xxx",
  "status": "editing"
}
```

---

### 다른 접속 시

👉 read-only

메시지:
"다른 세션에서 작업 중입니다"

---

### 허용

| 기능     | 가능 |
| ------ | -- |
| 조회     | O  |
| 수정     | X  |
| commit | X  |

---

### 세션 종료

* lock 해제

---

### 세션 TTL

* 기본값: 30분 (설정으로 변경 가능)
* inactivity 기준으로 자동 해제

---

## 14. 🔁 시퀀스

---

### 신규 사용자

```
Login
 → clone main
 → 파일 표시
 → 브랜치 생성
 → commit
 → push
 → PR
```

---

### 기존 사용자

```
Login
 → workspace 확인
 → fetch
 → diff
 → 작업
 → commit
 → rebase
 → push
 → PR
```

---

### 이어하기

```
Login
 → 미커밋 감지
 → 이어하기 선택
```

---

### 동시 접속

```
Login
 → session 존재
 → read-only
```

---

## 15. 🎯 설계 원칙

1. main = 안정 상태
2. user 브랜치 = 작업 공간
3. commit = 저장 기준
4. workspace = 캐시
5. 원격 = 진실
6. PR = 반영 통로
7. 단일 세션 편집

---

# ✅ END
