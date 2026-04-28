# TC3: 프로젝트 삭제

## 목적
ProjectMap에서 과제를 삭제하고, 설계문서 저장소(DocRepo)까지 정리되는지 검증한다.

## 사전 조건
- TC1(tc1-project-create.md) 시나리오 성공 — 삭제 대상 프로젝트가 존재하는 상태
- Platform-App(:3000), GoJIRA-App(:3001), Platform-API(:9000), GoJIRA-API(:9001) 가동
- Gitea(:3300) 정상 동작

## 테스트 환경
- 브라우저: 사용자 batman
- 테스트 대상: TC1에서 생성한 프로젝트 (예: E2E-TEST-PRJ)

---

## 시나리오

### Step 1: 프로젝트 삭제
| # | 동작 | 예상 결과 |
|---|------|----------|
| 1.1 | ProjectMap에서 삭제 대상 과제를 **우클릭** | 컨텍스트 메뉴 표시 |
| 1.2 | **삭제** 클릭 | 삭제 확인 다이얼로그 표시 |
| 1.3 | 확인 클릭 | 프로젝트 삭제 완료 |

---

## Pass 조건

| # | 검증 항목 | 확인 방법 |
|---|----------|----------|
| P1 | 삭제 확인 메시지 표시 | "과제를 삭제하시겠습니까?" 확인 다이얼로그가 정상 표시됨 |
| P2 | ProjectMap에서 제거 | 삭제 확인 후 프로젝트 목록에서 해당 과제가 사라짐 |
| P3 | 프로젝트 레포지토리 삭제 | Gitea(`http://localhost:3300`) 및 로컬 bare repo/worktree에서 해당 repo가 제거됨 |

## 비고
- 프로젝트 삭제 시 GoJIRA-API의 `ProjectUpdateHandler`가 `status: 'Deleted'`를 설정하고, Platform-API의 `DELETE /DocRepo/{code}/github`를 비동기 호출하여 Gitea repo + 로컬 클론을 정리함
- DB(projects 컬렉션)에는 `status: 'Deleted'`로 soft delete 처리됨
