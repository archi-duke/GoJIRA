GoJIRA API 요구사항 (Go Lang 기반)

JIRA Datacenter와 연동하여 이슈/프로젝트를 관리하는 중간 API 서버.
JIRA 백엔드는 JIRA-API (localhost:8000)의 Mock 서버를 사용한다.
GoJIRA에서 사용하는 DB는 MongoDB를 사용한다.
 - JIRA로 부터 이슈를 가져와서 MongoDB에 저장한다.
 - MongoDB에 저장된 이슈를 API 요청시 조회/수정/삭제 한다.
 - 위에서 수정/석재/추가 된 이슈는 JIRA에도 반영한다. (비동기로 수행할 것)
 - 이슈 push 요청은 db에 있는 내용을 JIRA에 반영하는 것이므로, 변경/추가된 이슈만 반영한다.
 - 이슈 pull 요청은 JIRA에 있는 내용을 db에 반영하는 것이므로, 변경/추가된 이슈만 반영한다.
 
## API 엔드포인트

### Check
- GET /api/v{version}/Check : 서버 상태 확인

### Issue
- POST /api/v{version}/Issue : JIRA 프로젝트에 이슈 생성
  - Body: prjID, issueType, reqType(옵션), summary, desc
- GET /api/v{version}/Issue : JIRA 프로젝트의 이슈 목록 조회
  - Header XBody: prjID, infoType("isBusy","issues","designs","nonTestIssues"), query
- PUT /api/v{version}/Issue : JIRA 이슈 정보 수정
  - Body: prjID, key, data(수정할 필드 목록)
- DELETE /api/v{version}/Issue : JIRA 이슈 삭제
  - Body: prjID, key

### Issue Label
- POST /api/v{version}/Issue/label : 이슈에 라벨 추가
  - Body: prjID, key, labelContent
- DELETE /api/v{version}/Issue/label : 이슈에서 라벨 삭제
  - Body: prjID, key, labelContent

### Issue Link
- POST /api/v{version}/Issue/link : 이슈 링크 생성
  - Body: prjID, inwardKey, outwardKey, linkType
- DELETE /api/v{version}/Issue/link : 이슈 링크 삭제
  - Body: prjID, linkId
- DELETE /api/v{version}/Issue/link/{linkId}/of/{key} : 특정 이슈의 링크 삭제

### Issue Comment
- POST /api/v{version}/Issue/comment : 이슈 코멘트 생성
  - Body: prjID, key, body
- PUT /api/v{version}/Issue/comment : 이슈 코멘트 수정
  - Body: prjID, key, commentId, body
- DELETE /api/v{version}/Issue/comment : 이슈 코멘트 삭제
  - Body: prjID, key, commentId

### Issue Cache / Fetch
- GET /api/v{version}/Issue/fetch : JIRA 이슈를 로컬 Cache로 Fetch
  - Header XBody: prjID, query, mergeType
- PUT /api/v{version}/Issue/cache : 로컬 Cache를 JIRA로 Push
  - Body: prjID, query, mergeType
- DELETE /api/v{version}/Issue/cache : 로컬 Cache 삭제

### Issue Field
- PUT /api/v{version}/Issue/field : 이슈 특정 필드 수정
  - Body: prjID, key, field, value

### Issue Relations
- PUT /api/v{version}/Issue/relations : 이슈 연결관계 분석
  - Body: prjID, query

### Jira Account
- POST /api/v{version}/Jira/Account : JIRA 계정 등록
  - Body: jiraID, pswd

### Page
- POST /api/v{version}/Page/images : 이슈 첨부 이미지 다운로드 (기존 로컬 파일 삭제)
  - Body: pageID, images
- PUT /api/v{version}/Page/images : 이슈 첨부 이미지 다운로드 (기존 로컬 파일 유지)
  - Body: pageID, images

### Project
- POST /api/v{version}/Project : 프로젝트 신규 등록
- GET /api/v{version}/Project : 프로젝트 목록 조회
  - Header XBody: jiraCode
- PUT /api/v{version}/Project : 프로젝트 수정

## 기술 스택
- Go (net/http + chi router)
- DB: MongoDB (이슈/프로젝트/코멘트/계정 저장)
- JIRA Mock: JIRA-API (FastAPI, localhost:8000)

## DB 설계 (MongoDB)
- Database: gojira
- Collections:
  - issues: JIRA 이슈 저장 (key를 unique index로 사용)
    - _dirty: bool (로컬 변경 여부, true면 JIRA 미반영 상태)
    - _syncedAt: datetime (마지막 JIRA 동기화 시각)
  - projects: 프로젝트 정보
  - comments: 이슈 코멘트
  - accounts: JIRA 계정 정보

## JIRA Mock 서버 연동
- JIRA-API의 REST API v2 엔드포인트를 호출하여 이슈 CRUD 수행
  - GET /rest/api/2/issue/{key} : 이슈 조회
  - GET /rest/api/2/issue : 이슈 목록 조회
  - POST /rest/api/2/issue : 이슈 생성
  - PUT /rest/api/2/issue/{key} : 이슈 수정
  - DELETE /rest/api/2/issue/{key} : 이슈 삭제
  - GET /rest/api/2/search?jql=... : 이슈 검색
  - POST /rest/api/2/issueLink : 링크 생성
  - DELETE /rest/api/2/issueLink/{id} : 링크 삭제
