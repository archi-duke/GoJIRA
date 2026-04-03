## JIRA Mock Server (JIRA-API)
JIRA Datacenter 버전을 모사하는 API를 구현.
JIRA Datacenter에서 사용되는 API 규격은 https://developer.atlassian.comd에서 v2 API를 참고해서 구현
이슈 생성, 조회, 목록 조회, 삭제, 라벨 추가/삭제,링크 생성/삭제에 해당하는 API를 구현하면 됨.
FastAPI를 사용해서 구현하고, DB는 issue 에 있는 json 파일들을 사용하면 됨.
API는 JIRA-API 폴더에 구현하면 됨.

### JIRA Mock 응답 지연
- list/search 엔드포인트에 5초 지연 적용 (실제 JIRA DC 환경 시뮬레이션)

---

## GoJIRA API Server (GoJIRA-API)
Go(chi) 기반 백엔드 서버. MongoDB를 캐시로 사용하여 JIRA와 양방향 동기화.

### 주요 기능
1. **이슈 CRUD** — 생성/조회/수정/삭제, MongoDB 캐시 + JIRA 동기화
2. **Pull** — JIRA에서 이슈를 가져와 MongoDB에 upsert (dirty 여부 무관, 항상 덮어쓰기)
3. **Push** — 30초 주기 백그라운드 sync, dirty 이슈를 JIRA로 push
4. **프로젝트 CRUD** — 프로젝트 정보 관리, issues 배열로 이슈 연결
5. **라벨/링크/코멘트** — JIRA API 연동

### 캐시 스키마
- JIRA 커스텀필드 → 캐시 필드명 매핑 (transform.go customFieldMap)
- 역매핑 CacheToJiraFields()로 수정 시 JIRA 반영
- 내부 필드: `_dirty`, `_syncedAt` (응답에서 제거)
- 이슈 필드명은 소문자 사용: `subkey`, `reqtype` 등

### 프로젝트 스키마
- 필드명은 camelCase (name, status, devTeam, devGroup 등)
- jiraCodes는 중첩 구조: `{ main: { jiraCode: "GOJIRA" } }`
- MongoDB 쿼리 시 dot notation 사용: `"jiraCodes.main.jiraCode"`

### 프로젝트 기반 이슈 필터링
- 프로젝트 issues 배열에 등록된 이슈만 조회
- bson.A → []string 명시적 변환으로 $in 필터 정상 동작

### JIRA 인증
- Basic Auth (Authorization: Basic base64(user:pass))
- 인증 정보는 .env 파일에 AES-256-GCM 암호화 저장
- `ENC:` 접두사 있으면 자동 복호화, 평문이면 그대로 사용
- 암호화 도구: `go run cmd/encrypt/main.go "평문"`

### 설정 (.env / 환경변수)
- `GOJIRA_SERVER_PORT` — 서버 포트 (기본 :9000)
- `GOJIRA_JIRA_URL` — JIRA 서버 URL (기본 http://localhost:8000)
- `GOJIRA_JIRA_API_BASE` — JIRA API 경로 (기본 /rest/api/2)
- `GOJIRA_JIRA_USER` — JIRA ID (암호화 지원)
- `GOJIRA_JIRA_PASS` — JIRA Password (암호화 지원)
- `GOJIRA_MONGO_URI` — MongoDB URI
- `GOJIRA_MONGO_DB` — MongoDB 데이터베이스명
- `GOJIRA_ENC_KEY` — AES 암호화 키 (32 bytes)