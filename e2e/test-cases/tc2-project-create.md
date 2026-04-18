# TC2: 프로젝트 생성

## 목적
ProjectMap에서 신규 과제를 생성하고, 설계문서 저장소(DocRepo)가 자동 생성되는지 검증한다.

## 사전 조건
- Platform-App(:3000), GoJIRA-App(:3001), Platform-API(:9000), GoJIRA-API(:9001) 가동
- Gitea(:3300) 정상 동작

## 테스트 환경
- 브라우저: 사용자 batman
- 테스트 대상: 신규 프로젝트 (예: E2E-TEST)

---

## 시나리오

### Step 1: 프로젝트 생성
| # | 동작 | 예상 결과 |
|---|------|----------|
| 1.1 | ProjectMap에서 **+ Add** 버튼 클릭 | 프로젝트 생성 다이얼로그 표시 |
| 1.2 | 프로젝트 정보 기입 (과제명, 일정, 담당자 등) | 입력 필드에 값 반영 |
| 1.3 | 저장 버튼 클릭 | 프로젝트 생성 완료 |

---

## Pass 조건

| # | 검증 항목 | 확인 방법 |
|---|----------|----------|
| P1 | ProjectMap에 과제 표시 | 프로젝트 목록에 생성한 과제가 간트차트와 함께 표시됨 |
| P2 | Gitea에 과제 레포지토리 생성 | `http://localhost:3300` 접속 → gojira 조직에 해당 repo 존재 확인 |
| P3 | Repository 메뉴에서 정보 확인 | Repository 탭 → 프로젝트 선택 → 파일 목록(business/, architecture/) 정상 표시 |

## 비고
- 프로젝트 생성 시 GoJIRA-API가 Platform-API의 `POST /DocRepo/{code}`를 자동 호출하여 Gitea repo + 로컬 bare repo를 생성함
- 초기 파일 4개 자동 생성: `business/project-description.md`, `business/use-cases.md`, `business/requirements.md`, `architecture/architecture.md`
