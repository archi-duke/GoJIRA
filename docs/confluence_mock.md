colfluence dc 의 mock를 만드는것을 복잡할까? page 생성,수정, 삭제, 조회를 해보고 싶은데...  회사는 폐쇄망이라 사외환경에서 구현 테스트 할수 잇었으면해
32s 동안 생각함

복잡하진 않아.
네가 하려는 범위가 page 생성/수정/삭제/조회 4개 정도라면, Confluence DC mock은 “중간 이하 난이도”야. 오히려 전부 다 흉내 내려는 순간 복잡해지고, /rest/api/content 계열만 부분 모킹하면 꽤 빨리 만들 수 있어. Confluence Data Center REST API는 표준 HTTP 메서드와 JSON 응답을 쓰고, URI도 /rest/api/... 구조라서 부분 구현에 적합하다. Atlassian의 DC 예시도 페이지 생성은 POST /rest/api/content, 수정은 PUT /rest/api/content/{id}, 삭제는 DELETE /rest/api/content/{id}, 조회/목록은 GET /rest/api/content... 형태로 제시한다.

내 추천은 **“진짜 Confluence를 흉내 내는 서버”가 아니라 “내 앱이 기대하는 최소 계약만 맞춘 fake Confluence”**를 만드는 거야.
즉 아래 4개만 먼저 맞추면 된다.

POST /rest/api/content : 페이지 생성
GET /rest/api/content/{id} : 단건 조회
PUT /rest/api/content/{id} : 페이지 수정
DELETE /rest/api/content/{id} : 페이지 삭제

필요하면 여기에 GET /rest/api/content?spaceKey=...&title=... 정도만 추가해도 “존재하면 수정, 없으면 생성” 테스트까지 가능해. Atlassian의 DC 예시 문서도 content browse, create, update, delete를 별도로 제공하고 있고, 조회는 pagination과 expand를 지원하는 구조다.

중요한 건 Confluence DC스러운 데이터 모양만 맞추는 거야.
예를 들면 생성 요청은 보통 이런 구조를 받는다.

{
  "type": "page",
  "title": "new page",
  "space": { "key": "TST" },
  "body": {
    "storage": {
      "value": "<p>Hello</p>",
      "representation": "storage"
    }
  }
}

수정은 여기에 id 와 version.number 가 들어가고, Atlassian 예시에서도 update 시 version.number 를 올려서 보낸다. 삭제 성공 시에는 204 No Content 가 기대 동작이다.

그래서 mock 난이도를 현실적으로 나누면 이래.

쉬운 부분

CRUD 기본 라우팅
in-memory 저장소나 SQLite 저장
id, title, space.key, body.storage.value, version.number 관리
목록 조회와 제목/spaceKey 필터링
삭제 시 204 반환

조금 귀찮은 부분

expand=body.storage,version,space 같은 응답 모양
ancestors 처리
pagination (start, limit, _links.next)
충돌 처리: 버전 안 올렸을 때 409/400 류 에러 흉내

지금은 안 해도 되는 부분

권한/SSO
매크로 렌더링
첨부파일
restriction
long task
실제 Confluence HTML 렌더링

즉, 사외 개발 테스트용이라면 1차 mock은 충분히 작게 갈 수 있어.

실전적으로는 두 가지 방식이 있어.

1) 직접 Fake Server 만들기

제일 추천.
FastAPI나 ASP.NET Core로 1~2일 안에 충분히 만들 수 있어.
네가 이미 FastAPI/ASP.NET Core 쪽 익숙하니까 이게 제일 빠를 가능성이 높아.

서버 내부 데이터 예시는 대충 이렇게:

{
  "id": "10001",
  "type": "page",
  "status": "current",
  "title": "테스트 페이지",
  "space": { "key": "GOJIRA" },
  "body": {
    "storage": {
      "value": "<p>본문</p>",
      "representation": "storage"
    }
  },
  "version": { "number": 1 },
  "ancestors": [],
  "_links": {
    "self": "http://localhost:8090/rest/api/content/10001",
    "webui": "/pages/viewpage.action?pageId=10001"
  }
}

네 클라이언트가 실제로 쓰는 필드만 넣으면 된다.

2) OpenAPI/Mock 툴로 자동 생성

Atlassian 쪽 커뮤니티 답변에 따르면 Confluence REST API용 OpenAPI 스펙을 유지하고 있다고 언급되어 있어서, Swagger Codegen 계열이나 mock 툴에 태워서 시작점으로 삼을 수는 있다. 다만 네 목적이 “CRUD 몇 개 로컬 테스트”라면 자동 생성 mock은 응답이 너무 장황하고, 원하는 오류 흐름 제어가 불편할 수 있다. 그래서 이 경우엔 hand-made fake가 더 낫다.

내 판단으로는 너한테는 이 전략이 제일 맞아.

추천 구현 전략
Fake Confluence DC 서버를 localhost:8090 에 띄운다.
네 앱의 base URL만 회사 URL 대신 로컬 fake 서버로 바꾼다.
아래만 먼저 구현한다.
create
get by id
search by spaceKey + title
update with version increment
delete
이후 회사망 들어갔을 때만 인증/실서버 차이를 맞춘다.

이렇게 하면 폐쇄망 밖에서도 대부분의 비즈니스 로직을 검증할 수 있다. 특히 “페이지 존재 여부 확인 → 없으면 생성 / 있으면 수정” 흐름은 로컬 mock으로 거의 완전히 테스트 가능하다. Confluence DC REST API가 일반적인 REST 구조를 따르기 때문에 이런 계약 테스트 방식이 잘 맞는다.

주의할 점도 하나 있어.
Cloud로 DC를 대신 테스트하는 것보다, DC 모양의 fake를 직접 만드는 게 훨씬 낫다. Cloud는 별도 v2 체계가 있고, DC는 /rest/api/content 기반 구조를 유지하므로, 네 목적이 DC 대응이면 mock도 DC 계약에 맞춰야 한다. Atlassian 문서도 Cloud는 v2 참조를 별도로 두고, Data Center는 별도 REST 참조를 유지한다.

원하면 내가 바로 다음 턴에서
FastAPI 기반 Confluence DC mock 서버 전체 코드를 만들어줄게.
그 안에 아래 5개 엔드포인트까지 넣어서 바로 로컬 실행 가능하게 해줄 수 있어.

POST /rest/api/content
GET /rest/api/content
GET /rest/api/content/{id}
PUT /rest/api/content/{id}
DELETE /rest/api/content/{id}

그리고 spaceKey, title, version.number, body.storage.value까지 실제 DC처럼 맞춰서.