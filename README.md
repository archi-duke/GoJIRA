# GoJIRA

JIRA Datacenter 연동 프로젝트/이슈 관리 시스템

## 구성

```
GoJIRA/
├── JIRA-API/        # JIRA Datacenter v2 Mock 서버 (Python/FastAPI)
├── GoJIRA-API/      # 중간 API 서버 (Go/Chi + MongoDB)
├── GoJIRA-App/      # 프론트엔드 SPA (React)
├── docs/            # API 스펙, 이슈 스키마, 요구사항 문서
└── issues/          # 샘플 JIRA 이슈 데이터 (JSON)
```

## JIRA-API (Mock 서버)

JIRA Datacenter REST API v2를 모사하는 FastAPI 서버. `issues/` 디렉토리의 JSON 파일을 DB로 사용한다.

### 엔드포인트

| Method | Endpoint | 설명 |
|--------|----------|------|
| GET | `/rest/api/2/issue` | 이슈 목록 조회 |
| GET | `/rest/api/2/issue/{key}` | 이슈 단건 조회 |
| POST | `/rest/api/2/issue` | 이슈 생성 |
| PUT | `/rest/api/2/issue/{key}` | 이슈 수정 (라벨 추가/삭제 포함) |
| DELETE | `/rest/api/2/issue/{key}` | 이슈 삭제 |
| GET | `/rest/api/2/search` | JQL 검색 |
| POST | `/rest/api/2/issueLink` | 이슈 링크 생성 |
| DELETE | `/rest/api/2/issueLink/{id}` | 이슈 링크 삭제 |

### 실행

```bash
cd JIRA-API
pip install -r requirements.txt
python -m uvicorn main:app --host 0.0.0.0 --port 8000 --reload
```

## GoJIRA-API (Go 중간 서버)

JIRA Mock 서버와 MongoDB를 연동하는 Go 기반 API 서버. 이슈를 MongoDB에 캐싱하고, 변경분만 JIRA에 동기화한다.

### 주요 기능

- **Pull**: JIRA에서 이슈를 가져와 MongoDB에 저장 (변경분만 upsert)
- **Push**: MongoDB에서 변경된(`_dirty`) 이슈만 JIRA에 반영
- **비동기 동기화**: 30초 간격으로 dirty 이슈를 자동 JIRA 반영
- **CRUD**: 이슈/라벨/링크/코멘트/프로젝트/계정 관리

### 엔드포인트

| Method | Endpoint | 설명 |
|--------|----------|------|
| GET | `/api/v{v}/Check` | 서버 상태 확인 |
| POST | `/api/v{v}/Issue` | 이슈 생성 |
| GET | `/api/v{v}/Issue` | 이슈 목록 조회 (XBody 헤더) |
| PUT | `/api/v{v}/Issue` | 이슈 수정 |
| DELETE | `/api/v{v}/Issue` | 이슈 삭제 |
| POST/DELETE | `/api/v{v}/Issue/label` | 라벨 추가/삭제 |
| POST/DELETE | `/api/v{v}/Issue/link` | 링크 생성/삭제 |
| POST/PUT/DELETE | `/api/v{v}/Issue/comment` | 코멘트 CRUD |
| GET | `/api/v{v}/Issue/fetch` | JIRA -> MongoDB Pull |
| PUT | `/api/v{v}/Issue/cache` | MongoDB -> JIRA Push |
| DELETE | `/api/v{v}/Issue/cache` | MongoDB 캐시 삭제 |
| PUT | `/api/v{v}/Issue/field` | 이슈 필드 단건 수정 |
| PUT | `/api/v{v}/Issue/relations` | 이슈 연결관계 분석 |
| POST | `/api/v{v}/Jira/Account` | JIRA 계정 등록 |
| POST/PUT | `/api/v{v}/Page/images` | 이미지 다운로드 |
| POST/GET/PUT | `/api/v{v}/Project` | 프로젝트 CRUD |

### 실행

```bash
cd GoJIRA-API
go build -o gojira-api .
./gojira-api
```

- 포트: `9000`
- MongoDB: `localhost:27017` / DB: `gojira`

## GoJIRA-App (React 프론트엔드)

프로젝트 일정을 간트차트 형태로 시각화하는 SPA.

### ProjectMap 기능

- 가로축(날짜) / 세로축(과제) 간트차트 뷰
- 마일스톤: 속이 빈 원 + day 표시, 위에 마일스톤명
- 2단 헤더: 연도(끝2자리) + 월(1~12), 연 경계에 굵은 선
- 기간 슬라이더: 시작/종료 노브, 기본 3년 고정 (Shift+드래그로 자유 설정)
- 현재 날짜 빨간 점선 표시
- 과제 상태별 색상: 진행 중(파란색), 완료(그레이+투명도), 기한 초과(Tomato 경고)
- 범위 밖 과제: 경계에 삼각형 표시, 범위 내 마일스톤만 클리핑 표시
- 우클릭 컨텍스트 메뉴: 상세정보, 편집, 삭제, Relation Map, SATAM
- 상세정보/편집 다이얼로그: 과제 정보 조회 및 수정
- 과제 추가 버튼 (+ Add)

### 실행

```bash
cd GoJIRA-App
npm install
npm start
```

- 포트: `3000`

## 전체 실행 순서

```bash
# 1. MongoDB 시작
brew services start mongodb-community

# 2. JIRA Mock 서버
cd JIRA-API && python -m uvicorn main:app --port 8000 &

# 3. GoJIRA API 서버
cd GoJIRA-API && go run main.go &

# 4. React 앱
cd GoJIRA-App && npm start
```

## 기술 스택

| 구성 | 기술 |
|------|------|
| JIRA Mock | Python, FastAPI, JSON 파일 |
| API 서버 | Go, Chi Router, MongoDB Driver v2 |
| DB | MongoDB |
| 프론트엔드 | React, SVG |
