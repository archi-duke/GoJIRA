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

### 변경 상태 표시

| 상태    | 색상 |
| ----- | -- |
| 변경 없음 | 기본 |
| 수정    | 노랑 |
| 추가    | 초록 |
| 삭제    | 빨강 |

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

```bash
git fetch origin
git rebase origin/main
```

### 충돌 발생 시

1. `git rebase --abort` 수행
2. 사용자에게 충돌 발생 알림
3. 사용자의 변경 내용을 diff로 표시
4. main 기준으로 reset 후 사용자가 변경 내용을 참고하여 재작업

```bash
git rebase --abort
git diff origin/main..HEAD > /tmp/changes.diff
git reset --hard origin/main
# 사용자에게 diff 내용을 UI에서 표시하여 재작업 유도
```

---

## 8. 🚀 PR 정책

### 조건

* main 대비 변경 존재

### PR 포함 내용

* diff
* 변경 파일
* LLM 요약

### Merge 후 처리

* 사용자 브랜치(docs/{userId})는 **유지**
* Workspace는 삭제
* 다음 접속 시 자동으로 `git rebase origin/main` 수행하여 브랜치를 main과 동기화

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
