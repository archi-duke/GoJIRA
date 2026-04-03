# GoJIRA 공통 플랫폼 운영전략

## 1. 플랫폼 개요

GoJIRA의 네비게이션, 사용자 인증, 계정/권한 관리, 공지 관리는 웹 시스템 개발의 **공통 플랫폼**으로 운영한다.
새로운 웹 서비스를 구축할 때 이 플랫폼을 기반으로 비즈니스 로직만 추가하여 빠르게 개발할 수 있도록 한다.

## 2. 플랫폼 구성요소

### 2.1 사용자 인증
- **SSO 연동**: ADSSO 쿠키 기반 인증, `{SSO_URL}/Account/ADFSLogin?client={origin}` 리다이렉트
- **개발 모드**: `USE_SSO=false` 시 고정 사용자 ID로 동작 (별도 인증 서버 불필요)
- **인증 확인**: `getAuthUser()` → 쿠키 확인 또는 개발용 ID 반환
- **설정**: `config.js`의 `USE_SSO`, `SSO_URL`, `DEV_USER`

### 2.2 네비게이션
- **앱 타이틀바**: 좌측 앱 로고, 중앙 메뉴 탭, 우측 사용자 정보
- **메뉴 동적 구성**: 사용자 권한에 따라 메뉴 노출/숨김
- **페이지 상태 유지**: sessionStorage로 현재 페이지/선택 항목 유지

### 2.3 계정/권한 관리
- **컬렉션**: users, authGroups, orgUnits, userProjects, userConnections
- **권한 모델**: CRUD 비트마스크 (C=8, R=4, U=2, D=1)
  - 권한코드별 비트마스크: `{ "USR": 15, "NOTMGR": 15, "PRJ": 6 }`
  - 사용자 최종 권한 = 소속 authGroups의 authority를 OR 연산
  - users에 authority 직접 저장 금지 (항상 계산)
- **조직 구조**: Company > Division > Team > Group 계층
- **soft delete**: isActive=false 기반, 물리 삭제 금지
- **감사 필드**: createdAt, updatedAt, createdBy, updatedBy

### 2.4 공지 관리
- **공지 CRUD**: 제목, 본문(마크다운/HTML), 게시 기간, 상태 관리
- **상태 자동 전환**: draft → active → expired (게시 기간 기반)
- **접속 시 팝업**: 게시중 공지를 작성일 오름차순으로 표시
- **3일 보지 않기**: localStorage 기반
- **비상 공지**: SSE 실시간 푸시로 즉시 전체 클라이언트 게시

## 3. 기술 스택

| 계층 | 기술 | 비고 |
|------|------|------|
| 프론트엔드 | React (SPA) | sessionStorage/localStorage 상태 유지 |
| 백엔드 API | Go (chi) | REST API, SSE |
| 데이터베이스 | MongoDB | bson.M 기반, soft delete |
| 인증 | SSO (ADFS) + 쿠키 | 개발 모드 지원 |
| 실시간 통신 | SSE (Server-Sent Events) | 비상 공지 푸시 |
| 암호화 | AES-256-GCM | .env 인증 정보 암호화 |

## 4. 서비스 확장 절차

### 4.1 새 마이크로서비스 추가 (새 앱 서비스)

새로운 비즈니스 도메인을 독립 서비스로 추가하는 경우.

**백엔드:**
1. 새 Go 프로젝트 생성 (독립 모듈, 독립 포트)
2. 공통 플랫폼 API를 호출하여 인증/권한 연동
3. 앱 전용 MongoDB 컬렉션(또는 DB) 설계
4. REST API 구현 및 독립 배포
5. (3단계) API Gateway에 라우팅 규칙 등록

**프론트엔드:**
1. `src/apps/{AppName}/` 폴더에 앱 컴포넌트 개발
2. 앱 레지스트리(`apps.js`)에 등록 (key, label, component, authCode)
3. Shell이 자동으로 메뉴 생성, 라우팅, 권한 체크 처리

**권한:**
1. 새 권한 코드 정의 (예: `NEWAPP`)
2. 플랫폼 DB의 authGroups에 해당 코드와 비트마스크 추가
3. 프론트 앱 레지스트리에 `authCode: 'NEWAPP'` 설정

### 4.2 기존 서비스 내 기능 추가

기존 마이크로서비스 안에서 새로운 기능/페이지를 추가하는 경우.

**백엔드:**
1. 해당 서비스의 `models/`에 도메인 모델 추가
2. `db/`에 컬렉션 접근 함수, 인덱스 추가
3. `handlers/`에 REST API 핸들러 추가
4. `main.go`에 라우트 등록
5. 필요 시 권한 코드 추가

**프론트엔드:**
1. 해당 앱 폴더 내에 컴포넌트 추가
2. 앱 내부 라우팅에 페이지 등록
3. 기존 권한 코드 내에서 비트마스크로 접근 제어

## 5. 권한 코드 체계

| 코드 | 대상 | 설명 |
|------|------|------|
| USR | 사용자 관리 | Account 메뉴 접근 |
| ORG | 조직 관리 | 조직 CRUD |
| PRJ | 프로젝트 관리 | 프로젝트 CRUD |
| ALLPRJ | 전체 프로젝트 | 모든 프로젝트 접근 |
| NOTMGR | 공지 관리 | Notice 메뉴 접근 |

**비트마스크 값:**
- 15 (CRUD): 전체 권한
- 6 (RU): 조회 + 수정
- 4 (R): 조회 전용

새 서비스 추가 시 권한 코드를 추가하고, 해당 코드의 비트마스크를 authGroup에 설정한다.

## 6. API 엔드포인트 규칙

- 기본 경로: `/api/v{version}/{Resource}`
- 대소문자 무관 라우팅 (caseInsensitiveRouting 미들웨어)
- 인증: Basic Auth (JIRA/Confluence), SSO 쿠키 (사용자)
- 응답: JSON, ObjectId는 문자열로 반환
- 삭제: soft delete (isActive=false)

## 7. 설정 관리

### 환경변수 (.env)
```
GOJIRA_SERVER_PORT=:9000
GOJIRA_JIRA_URL=http://localhost:8000
GOJIRA_JIRA_API_BASE=/rest/api/2
GOJIRA_JIRA_USER=ENC:...          # AES-256-GCM 암호화
GOJIRA_JIRA_PASS=ENC:...
GOJIRA_CONFLUENCE_URL=http://localhost:8090
GOJIRA_CONFLUENCE_USER=...
GOJIRA_CONFLUENCE_PASS=...
GOJIRA_MONGO_URI=mongodb://localhost:27017
GOJIRA_MONGO_DB=gojira
GOJIRA_ENC_KEY=...                 # AES 암호화 키 (32 bytes)
```

### 프론트엔드 설정 (config.js)
```javascript
{
  API_BASE: 'http://localhost:9000/api/v2',
  SSO_URL: 'https://sso.example.com',
  USE_SSO: false,       // 운영: true
  DEV_USER: 'duke.kimm' // 개발 전용
}
```

## 8. 실시간 통신 전략

| 용도 | 기술 | 엔드포인트 |
|------|------|-----------|
| 비상 공지 푸시 | SSE | GET /Notice/stream |
| 접속 상태 관리 | userConnections 컬렉션 | (향후 SignalR/WebSocket) |

SSE는 단방향(서버→클라이언트) 통신에 적합하며, 브라우저 네이티브 지원(EventSource)으로 별도 라이브러리 불필요.
양방향 통신이 필요한 경우 WebSocket/SignalR로 확장 가능.

## 9. 마이크로서비스 아키텍처

### 9.1 서비스 분류

```
[프론트엔드 SPA]
    │
    ├── /api/v1/Account/*  ──→  공통 플랫폼 서비스 (인증, 계정, 권한)
    ├── /api/v1/Notice/*   ──→  공통 플랫폼 서비스 (공지 관리)
    │
    ├── /api/v1/Issue/*    ──→  GoJIRA 서비스 (이슈 관리)
    ├── /api/v1/Project/*  ──→  GoJIRA 서비스 (프로젝트 관리)
    │
    └── /api/v1/{NewApp}/* ──→  향후 신규 앱 서비스
```

| 구분 | 서비스 | 역할 | 공유 범위 |
|------|--------|------|----------|
| 공통 | Platform API | 인증, 계정/권한, 공지, SSE | 모든 앱 공유 |
| 앱 | GoJIRA API | 이슈, 프로젝트, JIRA 연동 | GoJIRA 전용 |
| 앱 | (향후) 신규 서비스 | 비즈니스 로직 | 각 앱 전용 |

### 9.2 분리 원칙

- **공통 플랫폼 서비스**: 인증/계정/권한/공지는 한 번만 구축, 모든 앱이 공유
- **앱 서비스**: 각 앱의 비즈니스 로직만 구현, 독립 배포/스케일링
- **데이터 분리**: 공통 컬렉션(users, authGroups, orgUnits, notices)은 플랫폼 DB, 앱 데이터는 앱별 DB
- **새 앱 추가**: 비즈니스 API만 구현 → 플랫폼에 서비스 등록 → 네비게이션 메뉴 추가

### 9.3 단계별 전환 로드맵

**1단계: 코드 레벨 분리 (현재)**
- 단일 Go 서버 내에서 패키지로 분리 (handlers/account_mgmt, handlers/notice vs handlers/issue)
- 프론트는 단일 SPA, config.js로 API 엔드포인트 관리
- 장점: 배포 단순, 개발 속도 빠름

**2단계: 프로세스 분리**
- 공통 플랫폼 API 서버와 앱 API 서버를 별도 프로세스로 분리
- 각 서버 독립 포트 (예: 플랫폼 :9000, GoJIRA :9001, 신규앱 :9002)
- 프론트 config에서 서비스별 URL 관리
```javascript
{
  PLATFORM_API: 'http://localhost:9000/api/v1',  // 인증, 계정, 공지
  GOJIRA_API: 'http://localhost:9001/api/v1',    // 이슈, 프로젝트
  NEWAPP_API: 'http://localhost:9002/api/v1',    // 신규 앱
}
```

**3단계: API Gateway 도입**
- 프론트는 단일 엔드포인트(Gateway)만 호출
- Gateway가 경로 기반으로 백엔드 서비스에 라우팅
- 인증/권한 검증을 Gateway에서 공통 처리
```
[클라이언트] → [API Gateway :443]
                ├── /api/v1/Account/* → Platform Service
                ├── /api/v1/Notice/*  → Platform Service
                ├── /api/v1/Issue/*   → GoJIRA Service
                ├── /api/v1/Project/* → GoJIRA Service
                └── /api/v1/NewApp/*  → NewApp Service
```

### 9.4 신규 앱 서비스 등록 절차

1. **백엔드**: 새 Go 서버 프로젝트 생성, 비즈니스 API 구현
2. **플랫폼 연동**: 공통 DB의 users/authGroups에서 권한 조회 API 호출
3. **권한 코드 등록**: authGroups에 새 앱 권한 코드 추가 (예: `NEWAPP: 15`)
4. **프론트 등록**: App.js 네비게이션에 메뉴 추가, 권한 체크 적용
5. **Gateway 등록**: 라우팅 규칙 추가 (3단계 시)

## 10. 프론트엔드 마이크로 앱 전략

### 10.1 개념

프론트엔드도 백엔드와 동일하게 앱 단위로 확장 가능한 구조를 적용한다.
공통 Shell(타이틀바, 네비게이션, 인증, 권한)은 고정하고, 앱 컴포넌트만 추가하는 방식.

```
[공통 Shell (App.js)]
 ├── 타이틀바 (앱 로고 + 동적 메뉴 + 사용자 정보)
 ├── 인증 / 권한 체크
 └── 앱 영역 ← 앱 레지스트리 기반 동적 로딩
```

### 10.2 앱 레지스트리

앱 등록 정보를 설정 파일(apps.js)로 관리한다. 새 앱 추가 시 이 파일에 한 줄 등록하면 메뉴와 라우팅이 자동 구성된다.

```javascript
// apps.js — 앱 레지스트리
export default [
  {
    key: 'map',                                    // 라우팅 키
    label: 'Project',                              // 메뉴 표시명
    component: () => import('./apps/GoJIRA'),      // 동적 import
    authCode: null,                                // 권한 불필요 (전체 접근)
    subPages: ['issues', 'relationMap', 'satam'],  // 하위 페이지 (메뉴 활성 판별용)
  },
  {
    key: 'account',
    label: 'Account',
    component: () => import('./apps/AccountManager'),
    authCode: 'USR',                               // USR >= 15 필요
    authLevel: 15,
  },
  {
    key: 'notice',
    label: 'Notice',
    component: () => import('./apps/NoticeManager'),
    authCode: 'NOTMGR',
    authLevel: 15,
  },
  // 신규 앱 추가 예시
  // {
  //   key: 'dashboard',
  //   label: 'Dashboard',
  //   component: () => import('./apps/Dashboard'),
  //   authCode: 'DASH',
  //   authLevel: 4,
  // },
];
```

### 10.3 Shell 동작 방식

1. **메뉴 생성**: 앱 레지스트리를 순회하며 사용자 권한에 따라 메뉴 동적 구성
2. **라우팅**: `page` 상태와 레지스트리 `key`를 매칭하여 해당 앱 컴포넌트 렌더링
3. **동적 로딩**: `React.lazy()` + `Suspense`로 앱 컴포넌트를 필요 시점에 로딩 (초기 번들 크기 최소화)
4. **권한 체크**: `authority[app.authCode] >= app.authLevel` 조건으로 접근 제어

```
[사용자 접속]
  → getAuthUser() 인증
  → /Account/me 권한 조회
  → 앱 레지스트리 순회 → 권한 충족 앱만 메뉴 표시
  → 메뉴 클릭 → React.lazy()로 앱 컴포넌트 로딩 → 렌더링
```

### 10.4 앱 컴포넌트 규약

각 앱 컴포넌트는 다음 props를 받는다:
```javascript
{
  user: string,        // 현재 사용자 ID
  authority: object,   // 권한 객체
  navigate: function,  // 페이지 전환 함수
}
```

앱 내부의 하위 페이지 라우팅은 앱 컴포넌트 자체에서 관리한다.

### 10.5 신규 앱 추가 절차 (프론트엔드)

1. `src/apps/{AppName}/` 폴더에 앱 컴포넌트 개발
2. `apps.js` 레지스트리에 한 줄 등록 (key, label, component, authCode)
3. 백엔드 authGroups에 해당 권한 코드 추가
4. 끝 — Shell이 자동으로 메뉴 생성, 라우팅, 권한 체크 처리

### 10.6 향후 고도화

| 단계 | 방식 | 특징 |
|------|------|------|
| 현재 | 단일 SPA + 동적 import | 간단, 빌드 통합 |
| 2단계 | Module Federation (Webpack 5) | 앱별 독립 빌드/배포, 런타임 원격 모듈 로딩 |
| 3단계 | 독립 SPA + iframe/Web Component | 완전 독립, 기술 스택 혼합 가능 |

현재 단계에서는 동적 import 방식으로 충분하며, 앱 수가 늘어나고 독립 배포가 필요해지면 Module Federation으로 전환한다.

## 11. 향후 확장 포인트

- **SignalR Hub**: 양방향 실시간 통신 (채팅, 협업 알림 등)
- **RBAC 확장**: 리소스별 세분화된 권한 (프로젝트별 역할 등)
- **감사 로그**: 사용자 행동 추적 컬렉션
- **다국어 지원**: i18n 프레임워크 적용
- **테마/브랜딩**: 서비스별 타이틀바 로고/색상 커스터마이징
- **서비스 디스커버리**: 서비스 자동 등록/해제
- **헬스 체크**: 각 서비스 상태 모니터링 대시보드
- **공통 로깅**: 서비스 간 분산 추적 (trace ID)
