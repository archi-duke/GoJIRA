## IssueList — 이슈 테이블 뷰 컴포넌트

### 1. 기본 구조
- 프로젝트의 이슈를 테이블 형태로 표시
- 컬럼: Key, SubKey, Type, Summary, Status, Priority, Labels, DueDate

### 2. 필터 옵션
- V: VOS 이슈 표시/숨김
- F: Feature 이슈 표시/숨김
- R: Requirement 이슈 표시/숨김
- A: Architectural Design 이슈 표시/숨김
- D: Detailed Design 이슈 표시/숨김

### 3. 정렬
- 컬럼 헤더 클릭으로 오름/내림차순 정렬 토글

### 4. 편집모드
- Status: 드롭다운 선택 (To Do / In Progress / Done)
- Priority: 드롭다운 선택 (Highest / High / Medium / Low / Lowest)
- Summary: 인라인 텍스트 입력
- 변경 시 즉시 DB에 저장

### 5. Key/SubKey 클릭
- 이슈 상세 다이얼로그(IssueDetailDialog) 표시

### 6. CommonBar
- 네비게이션 탭: Issues / Relation Map / SATAM
- Pull 버튼으로 JIRA에서 이슈 재로딩
- 편집모드 토글

### 7. 이슈 삭제
- 편집모드에서 X 버튼으로 이슈 삭제
- 자기 과제 이슈: DB + JIRA 삭제
- 타 과제 이슈: 프로젝트 목록에서만 제거
