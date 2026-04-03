1. 관리자가 공지사항을 생성,수정,삭제 할 수 있어야 함.
2. 공지사항은 사용자가 접속시 팝업으로 표시되어야 함.
   2.1 팝업 표시 순서는 먼저 작성한 공지부터 표시 (작성일 오름차순)
   2.2 여러 공지가 있을 경우 이전/다음 버튼으로 페이징
3. 공지문 아래에는 "3일동안 보지 않기" 체크박스가 있어야 함.
   3.1 체크 시 localStorage에 3일 후 날짜 저장, 해당 기간 동안 팝업 미표시
4. 공지문 작성은 마크다운, html을 지원해야 함.
   4.1 본문 형식(bodyType)을 markdown/html 중 선택
5. 공지 게시 기간을 지정 할 수 있어야 함.
   5.1 시작일(startDate), 종료일(endDate)을 datetime-local로 지정
6. 공지문의 상태는 (게시전, 게시중, 게시종료)로 관리되어야 함.
   6.1 draft: 게시전, active: 게시중, expired: 게시종료
   6.2 게시 기간에 따라 서버에서 자동 상태 갱신 (draft→active, active→expired)
7. 공지문 목록에는 제목, 작성자, 작성일, 수정일, 조회수, 상태, 게시기간 표시되어야 함.
8. 비상 공지 기능
   8.1 공지 목록의 각 행에 "비상" 토글 버튼 배치 (삭제 버튼 앞)
   8.2 기본 상태: 일반 버튼 스타일 (lightgray)
   8.3 클릭 시 확인 팝업("비상 공지로 게시하시겠습니까?") 후 isEmergency=true, status=active로 변경, 버튼 강조(빨간색)
   8.4 다시 클릭 시 확인 팝업("비상 공지를 해제하시겠습니까?") 후 isEmergency=false로 변경, 버튼 기본 스타일 복원
   8.5 접속 시 팝업에서 비상 공지는 "긴급" 배지 표시
   8.6 비상 공지 지정 즉시 접속 중인 모든 클라이언트에 SSE(Server-Sent Events)로 푸시
   8.7 SSE 수신 시 "3일 보지 않기" 와 무관하게 즉시 팝업 표시
   8.8 SSE 엔드포인트: GET /Notice/stream (event: emergency)
9. 공지문 관리 접근은 권한 코드 "NOTMGR" 15 이상 권한을 가진 사용자만 접근 가능

## 구현 구조

### 백엔드 (GoJIRA-API)
- notices 컬렉션: title, body, bodyType, status, isEmergency, startDate, endDate, views
- API:
  - GET /Notice — 전체 목록 (관리용)
  - GET /Notice/active — 게시중 공지 (팝업용, 작성일 오름차순)
  - GET /Notice/{id} — 단건 조회
  - POST /Notice — 생성
  - PUT /Notice/{id} — 수정
  - DELETE /Notice/{id} — soft delete
  - POST /Notice/{id}/view — 조회수 증가

### 프론트엔드
- NoticeManager: 공지 관리 페이지 (NOTMGR 권한 필요)
- NoticePopup: 접속 시 게시중 공지 팝업 (전체 사용자)
