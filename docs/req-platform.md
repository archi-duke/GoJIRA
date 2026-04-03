# 공통 플랫폼 설계서

> 문서 버전: 1.0
> 최종 수정: 2026-04-03
> 작성자: Duke Kimm

---

## 1. 개요

### 1.1 목적
웹 시스템 개발에 반복적으로 필요한 공통 기능(인증, 계정/권한, 공지)을 플랫폼으로 제공하여,
신규 서비스 구축 시 비즈니스 로직 개발에만 집중할 수 있도록 한다.

### 1.2 적용 범위
- 사용자 인증 (SSO 연동)
- 계정 및 권한 관리 (그룹 기반 CRUD 비트마스크)
- 공지사항 관리 (게시/비상 공지/실시간 푸시)
- 네비게이션 Shell (동적 메뉴, 권한 기반 접근 제어)
- 마이크로서비스 확장 체계

### 1.3 용어 정의

| 용어 | 정의 |
|------|------|
| 플랫폼 서비스 | 인증, 계정/권한, 공지 등 모든 앱이 공유하는 공통 서비스 |
| 앱 서비스 | GoJIRA 등 특정 비즈니스 도메인을 처리하는 독립 서비스 |
| Shell | 타이틀바, 네비게이션, 인증을 담당하는 프론트엔드 공통 프레임 |
| 앱 레지스트리 | 프론트엔드에서 앱 메뉴/라우팅/권한을 선언적으로 등록하는 설정 |
| authGroup | 권한 그룹. 권한코드별 CRUD 비트마스크를 보유 |
| 비트마스크 | C=8, R=4, U=2, D=1 조합으로 권한 수준을 표현하는 값 |

---

## 2. 아키텍처

### 2.1 전체 구조

```
[클라이언트 SPA]
 ├── Shell (타이틀바, 네비게이션, 인증, 권한)
 ├── 앱 영역 ← 앱 레지스트리 기반 동적 로딩
 │
 ├── 직접 호출 → 공통 플랫폼 API  (인증, 계정, 공지, SSE)
 ├── 직접 호출 → GoJIRA API       (이슈, 프로젝트, JIRA 연동)
 └── 직접 호출 → (향후) 신규 앱 API
```

### 2.2 서비스 분류

| 구분 | 서비스 | 역할 | 공유 범위 |
|------|--------|------|----------|
| 공통 | Platform API | 인증, 계정/권한, 조직, 공지, SSE | 모든 앱 공유 |
| 앱 | GoJIRA API | 이슈/프로젝트 관리, JIRA DC 연동 | GoJIRA 전용 |
| 앱 | (향후) 신규 서비스 | 비즈니스 로직 | 각 앱 전용 |

### 2.3 기술 스택

| 계층 | 기술 | 선정 사유 |
|------|------|----------|
| 프론트엔드 | React (SPA) | 컴포넌트 기반 UI, 동적 import로 앱 분리 용이 |
| 백엔드 API | Go (chi) | 경량 HTTP 프레임워크, 컴파일 바이너리로 배포 단순 |
| 데이터베이스 | MongoDB | 스키마 유연성, 문서 기반으로 다양한 도메인 대응 |
| 인증 | SSO (ADFS) + 쿠키 | 사내 SSO 인프라 활용, 개발 모드로 독립 테스트 가능 |
| 실시간 통신 | SSE | 단방향 푸시에 적합, 브라우저 네이티브 지원(EventSource) |
| 암호화 | AES-256-GCM | 인증 정보(.env) 암호화, 표준 대칭키 암호화 |

---

## 3. 플랫폼 구성요소

### 3.1 사용자 인증

**인증 흐름:**
```
[접속] → ADSSO_USER 쿠키 확인
         ├── 있음 → 사용자 ID 추출 → /Account/me로 권한 조회
         └── 없음 → SSO 서버 리다이렉트 → 인증 완료 → 쿠키 발급 → 원래 페이지 복귀
```

**설정:**
- `USE_SSO`: true(운영) / false(개발). false 시 고정 사용자 ID로 동작
- `SSO_URL`: SSO 인증 서버 주소
- `DEV_USER`: 개발 모드 시 사용할 고정 사용자 ID

> **결정 사유:** 사내 ADFS SSO가 이미 운영 중이므로 별도 인증 시스템 구축 없이 연동.
> 개발 환경에서는 SSO 서버 없이 독립 테스트할 수 있도록 개발 모드를 지원.

### 3.2 계정/권한 관리

**컬렉션 구조:**

| 컬렉션 | 용도 | 비고 |
|--------|------|------|
| users | 사용자 정보 | knoxId, name, email, authGroupIds, orgUnitIds |
| authGroups | 권한 그룹 | code, authority(권한코드→비트마스크) |
| orgUnits | 조직 단위 | 계층 구조 (parentOrgUnitId) |
| userProjects | 사용자-프로젝트 매핑 | userId, projectId, role |
| userConnections | 접속 상태 | connectionId, isOnline |

**권한 모델:**
```
비트마스크: C=8(Create), R=4(Read), U=2(Update), D=1(Delete)

예시:
  15 (1111) = CRUD 전체
   6 (0110) = 조회 + 수정
   4 (0100) = 조회 전용

사용자 최종 권한 = 소속 authGroups의 authority를 권한코드별 OR 연산
```

> **결정 사유:**
> - **비트마스크**: 단일 정수로 CRUD 조합을 표현하여 저장/비교가 효율적. OR 연산으로 다중 그룹 권한 합산이 간단.
> - **그룹 기반 계산**: users에 authority를 직접 저장하면 그룹 변경 시 모든 사용자를 갱신해야 함. 조회 시 계산하면 항상 최신 상태 보장.
> - **soft delete**: 물리 삭제 시 참조 무결성 문제. isActive=false로 비활성화하여 이력 보존.

**조직 계층:**
```
Company > Division > Team > Group
```
- parentOrgUnitId로 계층 표현, 유형(type)으로 자동 정렬
- 사용자 등록 시 여러 조직에 소속 가능 (orgUnitIds 배열)

**권한 코드 체계:**

| 코드 | 대상 | 설명 |
|------|------|------|
| USR | 사용자 관리 | Account 메뉴 접근 |
| ORG | 조직 관리 | 조직 CRUD |
| PRJ | 프로젝트 관리 | 프로젝트 CRUD |
| ALLPRJ | 전체 프로젝트 | 모든 프로젝트 접근 |
| NOTMGR | 공지 관리 | Notice 메뉴 접근 |
| GITRP | 문서 이력관리 | Docs 메뉴 접근 (15=repo 생성, 6=PR/승인, 4=조회) |

신규 서비스 추가 시 권한 코드를 추가하고, 필요한 authGroup에 비트마스크를 설정한다.

### 3.3 공지 관리

**상태 모델:**
```
draft(게시전) → active(게시중) → expired(게시종료)
              게시 기간 기반 자동 전환
```

**기능:**
- 공지 CRUD (제목, 본문, 게시 기간, 상태)
- 본문 형식: Markdown 또는 HTML 선택
- 접속 시 팝업 표시 (작성일 오름차순, 이전/다음 페이징)
- "3일동안 보지 않기" (localStorage 기반)
- 비상 공지: 목록에서 토글 버튼으로 활성화 → SSE로 즉시 전체 클라이언트 푸시

> **결정 사유:**
> - **SSE 선택**: 비상 공지는 서버→클라이언트 단방향 푸시만 필요. SSE는 브라우저 네이티브 지원(EventSource)으로 별도 라이브러리 불필요하고, HTTP 기반이라 방화벽/프록시 호환성 좋음.
> - **WebSocket/SignalR 미채택**: 양방향 통신이 필요하지 않은 현 시점에서 과도한 복잡도. 향후 채팅 등 양방향 기능 추가 시 검토.

### 3.4 문서 이력관리 (Docs)

**개요:** GitHub를 원본 저장소/형상관리 엔진으로 활용하여 설계 문서의 버전 관리, 승인, 소스코드 연결을 제공한다.
GitHub 미숙자도 사용할 수 있도록 과제 중심의 간편 UI를 제공한다.

**데이터 저장:**
- 별도 컬렉션 없음 — `project.documents`에 repo 정보를 직접 저장
```json
{
  "documents": {
    "repoName": "GOJIRA_20260327134724",
    "repoOwner": "dkdoc7",
    "repoUrl": "https://github.com/dkdoc7/GOJIRA_20260327134724",
    "defaultBranch": "main"
  }
}
```
- 승인자(auditors)는 프로젝트 레벨에서 관리
- repo 이름 규칙: `{JIRA_CODE}_{프로젝트생성시간}` (예: `GOJIRA_20260327134724`)

> **결정 사유:**
> - **별도 컬렉션(docRepos) 미사용**: 1 프로젝트 = 1 repo 관계에서 별도 컬렉션은 관리 복잡도만 높임. 두 곳의 데이터 불일치 문제 발생.
> - project.documents에 모든 repo 정보를 통합하여 진실의 원천(source of truth)을 단일화.

**문서 작업 방식 (로컬 Git 기반):**
```
[앱 서비스] 문서 생성 요청
  → [Platform-API] repo 없으면 자동 생성 (GitHub API + project.documents 갱신)
  → 서버 로컬에 repo clone/pull
  → 브랜치 생성 (docs/{userId}) → 파일 생성/편집 → git add/commit/push
  → GitHub PR 생성 → auditors를 reviewer로 자동 할당
  → [승인자] Docs UI에서 승인/반려
  → 승인 완료 → 머지 → main 반영
```

> **결정 사유:**
> - **로컬 Git 방식 채택**: 여러 파일을 한 커밋으로 묶을 수 있고, git diff 등 표준 도구 활용 가능.
> - **브랜치 사용자별 고정**: `docs/{userId}` — 사용자당 1개 브랜치로 관리 단순화.
> - **열린 PR 확인**: 문서 생성 전 미완료 PR이 있으면 차단하여 충돌 방지.

**API (projectCode 기반):**

| 구분 | 엔드포인트 | 설명 |
|------|-----------|------|
| Repo | POST /DocRepo/{code} | repo 생성 (없으면 자동, project.documents에 저장) |
| Repo | GET /DocRepo | repo가 있는 프로젝트 목록 |
| Sync | POST /DocRepo/sync | GitHub vs 프로젝트 비교 (orphan/missing 감지) |
| 조회 | GET /DocRepo/{code}/tree | 문서 트리 |
| 조회 | GET /DocRepo/{code}/file | 파일 내용 (Markdown) |
| 조회 | GET /DocRepo/{code}/commits | 커밋 이력 |
| 조회 | GET /DocRepo/{code}/branches | 브랜치 목록 |
| Git | POST /DocRepo/{code}/git/publish | 브랜치 생성 + 파일 쓰기 + commit/push + PR 생성 |
| PR | POST /DocRepo/{code}/pr | PR 생성 (auditors reviewer 할당) |
| PR | GET /DocRepo/{code}/prs | PR 목록 |
| PR | POST /DocRepo/{code}/pr/{num}/review | 승인/반려 |
| PR | POST /DocRepo/{code}/pr/{num}/merge | 머지 |
| 삭제 | DELETE /DocRepo/{code}/github | GitHub repo 삭제 + project.documents 비우기 |
| 삭제 | DELETE /DocRepo/{code}/purge | project.documents만 비우기 |

**Sync 동작 (↻ 버튼):**
- GitHub 전체 repo 목록 vs 프로젝트 documents 비교
- **ok**: GitHub 존재 + 프로젝트 연결됨
- **orphan** (주황): GitHub에 있지만 프로젝트 연결 없음 → 삭제 여부 확인
- **missing** (빨강): 프로젝트에 연결됐지만 GitHub에 없음 → 정보 삭제 여부 확인

**GitHub Webhook:**
- repo 생성 시 자동 등록 (`PLATFORM_WEBHOOK_URL` 설정 시)
- PR merge/push 이벤트 수신 → SSE `doc_update` 브로드캐스트 → 프론트 자동 갱신

**승인 흐름:**
- auditors는 프로젝트 레벨에서 관리 (별도 동기화 불필요)
- PR 생성 시 프로젝트 auditors를 GitHub reviewer로 자동 할당
- auditors 없으면 PR 생성 차단 + "승인자를 먼저 지정하세요" 안내
- Docs UI에서 승인/반려/머지 가능

**앱 서비스 연동 (GoJIRA 예시):**
- GoJIRA-API `/Issue/docgen`: 이슈 데이터 → 타입별 Markdown 자동 생성 → Platform-API `/git/publish` 호출
- 프로젝트 컨텍스트 메뉴 "설계문서 생성" 클릭으로 실행
- 생성 파일: README.md, vos.md, feature.md, requirement.md, architectural-design.md, detailed-design.md
- repo 없으면 자동 생성, 열린 PR 있으면 차단 안내

**프론트 UI (DocManager):**
- 사이드바: 프로젝트별 repo 목록 (이름/프로젝트명/조직), ↻ 동기화 버튼
- 탭: Files / Commits / PRs / Graph
- Files: 폴더/파일 트리 탐색 + Markdown 렌더링 (react-markdown, GFM, UTF-8 base64 디코딩)
- Commits: 커밋 이력 테이블
- PRs: PR 목록, 승인/반려/머지, SSE로 실시간 갱신
- Graph: SVG 기반 브랜치/커밋 시각화

> **추가 결정 사유:**
> - **GitHub 활용**: 형상관리 기능을 중복 구현하지 않고 GitHub API/Git CLI를 활용.
> - **PAT 인증**: 초기 설정 간단. 조직 규모 확장 시 GitHub App으로 전환 가능.
> - **Markdown 기반**: 가볍고 Git 친화적, 계층 구조는 폴더 구조로 자연스럽게 표현.

### 3.5 네비게이션 Shell

**구조:**
```
┌─────────────────────────────────────────────────┐
│ [GoJIRA]  [Project] [Account] [Notice]  user_id │ ← 타이틀바
├─────────────────────────────────────────────────┤
│                                                 │
│              앱 컨텐츠 영역                       │ ← 앱 레지스트리 기반 동적 렌더링
│                                                 │
└─────────────────────────────────────────────────┘
```

- 메뉴: 사용자 권한에 따라 동적 노출/숨김
- 페이지 상태: sessionStorage로 현재 페이지/선택 항목 유지 (새로고침 대응)
- 권한 미달 시 접근 차단 및 기본 페이지로 리다이렉트

---

## 4. 마이크로서비스 전략

### 4.1 분리 원칙

- **공통 플랫폼 서비스**: 인증/계정/권한/공지는 한 번만 구축, 모든 앱이 공유
- **앱 서비스**: 각 앱의 비즈니스 로직만 구현, 독립 배포/스케일링
- **데이터 분리**: 공통 컬렉션(users, authGroups, orgUnits, notices)은 플랫폼 DB, 앱 데이터는 앱별 DB(또는 컬렉션)
- **서비스 간 통신**: 프론트가 각 서비스를 직접 호출 (중앙 집중 경유 없음)

### 4.2 단계별 전환 로드맵

**1단계: 코드 레벨 분리 (현재)**
```
[단일 Go 서버 :9000]
 ├── handlers/account_mgmt.go  ← 플랫폼 (계정/권한)
 ├── handlers/notice.go        ← 플랫폼 (공지)
 ├── handlers/sse.go           ← 플랫폼 (실시간)
 ├── handlers/issue.go         ← GoJIRA (이슈)
 └── handlers/project.go       ← GoJIRA (프로젝트)
```
- 단일 서버 내 패키지로 논리적 분리
- 배포 단순, 개발 속도 빠름
- 서비스 규모가 작은 초기 단계에 적합

**2단계: 프로세스 분리**
```
[클라이언트]
    ├── 직접 호출 → Platform Service :9000  (인증, 계정, 공지)
    ├── 직접 호출 → GoJIRA Service :9001    (이슈, 프로젝트)
    └── 직접 호출 → NewApp Service :9002    (신규 앱)
```
- 각 서비스를 독립 Go 프로젝트/프로세스로 분리
- 프론트 config에서 서비스별 URL 관리
- 독립 배포, 독립 스케일링 가능
- CORS 설정으로 교차 호출 허용

```javascript
// config.js (2단계)
{
  PLATFORM_API: 'http://platform.internal:9000/api/v1',
  GOJIRA_API: 'http://gojira.internal:9001/api/v1',
  NEWAPP_API: 'http://newapp.internal:9002/api/v1',
}
```

**3단계: 서비스 메시 (규모 확장 시)**
```
[클라이언트]
    │
    ├── Service A ←[사이드카]→ Service B
    ├── Service B ←[사이드카]→ Platform
    └── Platform  ←[사이드카]→ Service A
```
- 각 서비스에 사이드카 프록시(Istio Envoy, Linkerd 등) 배치
- 서비스 간 통신을 사이드카가 투명하게 처리
- 트래픽 관리, mTLS 인증, 관찰성(observability) 확보
- 적용 시점: 서비스 수 증가, 운영 복잡도 상승 시

> **API Gateway를 채택하지 않는 이유:**
> Gateway는 모든 트래픽이 집중되는 단일 장애점(SPOF)이 되며, 가용성과 성능에 병목을 유발할 수 있다.
> 서비스 수가 적은 현재 규모에서는 직접 호출 방식이 충분하고,
> 규모 확장 시에는 중앙 집중형 Gateway보다 분산형 서비스 메시가 장애 격리와 확장성 측면에서 우수하다.

### 4.3 서비스 확장 절차

#### 새 마이크로서비스 추가 (새 앱 서비스)

새로운 비즈니스 도메인을 독립 서비스로 추가하는 경우.

| 단계 | 작업 | 담당 |
|------|------|------|
| 1 | 새 Go 프로젝트 생성, 독립 포트 할당 | 백엔드 |
| 2 | 공통 플랫폼 API 호출로 인증/권한 연동 | 백엔드 |
| 3 | 앱 전용 MongoDB 컬렉션(또는 DB) 설계 | 백엔드 |
| 4 | REST API 구현 및 독립 배포 | 백엔드 |
| 5 | 플랫폼 DB authGroups에 새 권한 코드 추가 | 플랫폼 |
| 6 | 앱 레지스트리 등록, config에 서비스 URL 추가 | 프론트 |

#### 기존 서비스 내 기능 추가

기존 마이크로서비스 안에서 새로운 기능/페이지를 추가하는 경우.

| 단계 | 작업 | 담당 |
|------|------|------|
| 1 | models/, db/, handlers/ 에 도메인 로직 추가 | 백엔드 |
| 2 | main.go 에 라우트 등록 | 백엔드 |
| 3 | 앱 폴더 내 컴포넌트 추가, 앱 내부 라우팅 등록 | 프론트 |
| 4 | 필요 시 기존 권한 코드 내 비트마스크로 접근 제어 | 공통 |

### 4.4 소스코드 구조

#### 현재 구조 (1단계: 코드 레벨 분리)

플랫폼과 앱이 동일 프로젝트 내에 혼재. 패키지로 논리적 분리만 되어 있음.

```
GoJIRA/
├── GoJIRA-API/                ← 단일 서버 (플랫폼 + 앱 혼재)
│   ├── handlers/
│   │   ├── account_mgmt.go   ← [플랫폼] 계정/권한
│   │   ├── notice.go         ← [플랫폼] 공지
│   │   ├── sse.go            ← [플랫폼] 실시간 푸시
│   │   ├── issue.go          ← [앱] GoJIRA 이슈
│   │   ├── project.go        ← [앱] GoJIRA 프로젝트
│   │   └── ...
│   ├── db/
│   │   ├── account.go        ← [플랫폼]
│   │   ├── notice.go         ← [플랫폼]
│   │   ├── mongo.go          ← [공통]
│   │   ├── sync.go           ← [앱]
│   │   └── transform.go      ← [앱]
│   ├── models/
│   │   ├── account.go        ← [플랫폼]
│   │   ├── notice.go         ← [플랫폼]
│   │   └── models.go         ← [앱]
│   └── config/, jiraclient/, confluence/
│
├── GoJIRA-App/                ← 단일 SPA (Shell + 앱 혼재)
│   └── src/
│       ├── App.js             ← [플랫폼] Shell, 인증, 네비게이션
│       ├── utils/auth.js      ← [플랫폼] 인증 유틸
│       └── components/
│           ├── CommonBar.js       ← [공통]
│           ├── AccountManager.js  ← [플랫폼]
│           ├── NoticeManager.js   ← [플랫폼]
│           ├── NoticePopup.js     ← [플랫폼]
│           ├── ProjectMap.js      ← [앱] GoJIRA
│           ├── IssueList.js       ← [앱] GoJIRA
│           ├── RelationMap.js     ← [앱] GoJIRA
│           └── SATAM.js           ← [앱] GoJIRA
│
└── docs/
```

#### 목표 구조 (2단계: 프로세스 분리)

플랫폼과 앱을 독립 프로젝트로 분리. 각각 독립 빌드/배포 가능.

```
GoJIRA/
├── Platform-API/              ← 공통 플랫폼 API 서버 (:9000)
│   ├── handlers/              (account_mgmt, notice, sse)
│   ├── db/                    (account, notice, 공통 mongo)
│   ├── models/                (account, notice)
│   ├── config/                (config, crypto)
│   └── main.go
│
├── Platform-App/              ← 공통 Shell + 플랫폼 UI
│   └── src/
│       ├── App.js             (Shell, 인증, 네비게이션, 앱 레지스트리)
│       ├── utils/             (auth, exportUtils)
│       ├── components/        (CommonBar, AccountManager, NoticeManager, NoticePopup, DocManager)
│       └── apps/              (앱 레지스트리 진입점)
│
├── GoJIRA-API/                ← GoJIRA 앱 서비스 (:9001)
│   ├── handlers/              (issue, project, fetch, label, link, comment, field)
│   ├── db/                    (sync, transform, 앱 전용 mongo)
│   ├── models/                (models)
│   ├── jiraclient/
│   ├── confluence/
│   └── main.go
│
├── GoJIRA-App/                ← GoJIRA 앱 프론트 (Platform-App에 등록)
│   └── src/components/        (ProjectMap, IssueList, RelationMap, SATAM, IssueDetailDialog, ProjectDialog)
│
├── {NewApp}-API/              ← 향후 신규 앱 서비스 (:9002)
├── {NewApp}-App/              ← 향후 신규 앱 프론트
│
├── JIRA-API/                  ← JIRA Mock 서버 (개발용)
├── Conf-Mock/                 ← Confluence Mock 서버 (개발용)
└── docs/
```

#### 전환 시 고려사항

| 항목 | 내용 |
|------|------|
| 전환 시점 | 두 번째 앱 서비스가 추가되는 시점에 분리 착수 |
| DB 분리 | 공통 컬렉션(users, authGroups, orgUnits, notices)은 플랫폼 DB에 유지, 앱 데이터는 앱별 DB |
| 공유 코드 | config, crypto 등 공통 유틸은 Go 모듈로 분리하거나 각 서비스에 복사 |
| 프론트 통합 | Platform-App이 Shell 역할, GoJIRA-App은 앱 레지스트리를 통해 동적 import |
| CORS | 서비스별 독립 포트이므로 CORS 설정 필요 |

> **결정 사유:** 현재는 단일 서비스 규모이므로 코드 레벨 분리로 충분.
> 물리적 분리는 두 번째 앱 서비스 추가 시점에 진행하여 불필요한 복잡도 증가를 방지.
> 현 단계에서는 플랫폼/앱 경계를 코드 내 주석과 패키지 구조로 명확히 해두어 향후 분리를 용이하게 한다.

---

## 5. 프론트엔드 마이크로 앱 전략

### 5.1 개념

프론트엔드도 백엔드와 동일하게 앱 단위로 확장 가능한 구조를 적용한다.
공통 Shell은 고정, 앱 컴포넌트만 추가하면 메뉴/라우팅/권한이 자동 구성된다.

### 5.2 앱 레지스트리

```javascript
// apps.js
export default [
  {
    key: 'map',                                    // 라우팅 키
    label: 'Project',                              // 메뉴 표시명
    component: () => import('./apps/GoJIRA'),      // 동적 import
    authCode: null,                                // 권한 불필요 (전체 접근)
    subPages: ['issues', 'relationMap', 'satam'],  // 하위 페이지
  },
  {
    key: 'account',
    label: 'Account',
    component: () => import('./apps/AccountManager'),
    authCode: 'USR',
    authLevel: 15,                                 // CRUD 전체 필요
  },
  {
    key: 'notice',
    label: 'Notice',
    component: () => import('./apps/NoticeManager'),
    authCode: 'NOTMGR',
    authLevel: 15,
  },
];
```

### 5.3 Shell 동작 방식

```
[사용자 접속]
  → getAuthUser() 인증
  → /Account/me 권한 조회
  → 앱 레지스트리 순회 → 권한 충족 앱만 메뉴 표시
  → 메뉴 클릭 → React.lazy()로 앱 컴포넌트 로딩 → 렌더링
```

1. **메뉴 생성**: 레지스트리 순회, `authority[authCode] >= authLevel` 충족 시 메뉴 노출
2. **라우팅**: page 상태와 레지스트리 key 매칭하여 앱 컴포넌트 렌더링
3. **동적 로딩**: React.lazy() + Suspense로 필요 시점에 로딩 (초기 번들 최소화)
4. **권한 미달**: 접근 시도 시 기본 페이지로 리다이렉트

### 5.4 앱 컴포넌트 규약

각 앱 컴포넌트는 다음 props를 받는다:
```javascript
{
  user: string,        // 현재 사용자 ID
  authority: object,   // 권한 객체 { USR: 15, NOTMGR: 15, ... }
  navigate: function,  // 페이지 전환 함수
}
```
앱 내부의 하위 페이지 라우팅은 앱 컴포넌트 자체에서 관리한다.

### 5.5 단계별 고도화

| 단계 | 방식 | 특징 | 전환 시점 |
|------|------|------|----------|
| 현재 | 단일 SPA + 동적 import | 간단, 빌드 통합 | - |
| 2단계 | Module Federation (Webpack 5) | 앱별 독립 빌드/배포 | 앱 수 증가, 독립 배포 필요 시 |
| 3단계 | 독립 SPA + Web Component | 완전 독립, 기술 스택 혼합 | 이기종 기술 통합 필요 시 |

> **결정 사유:** 현재 앱 수가 적으므로 단일 SPA + 동적 import가 가장 효율적.
> Module Federation은 Webpack 설정 복잡도가 증가하므로 앱 수가 5개 이상이 되거나 팀별 독립 배포가 필요해지는 시점에 검토.

---

## 6. API 설계 규칙

| 항목 | 규칙 |
|------|------|
| 경로 | `/api/v{version}/{Resource}` |
| 라우팅 | 대소문자 무관 (caseInsensitiveRouting 미들웨어) |
| 인증 | 외부 시스템: Basic Auth, 사용자: SSO 쿠키 |
| 응답 | JSON, ObjectId는 문자열로 변환 |
| 삭제 | soft delete (isActive=false), 물리 삭제 금지 |
| 감사 | 모든 주요 컬렉션에 createdAt, updatedAt, createdBy, updatedBy |
| 버전 | URL 경로에 버전 포함 (v1, v2) |

---

## 7. 설정 관리

### 7.1 백엔드 (.env)
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

- `ENC:` 접두사가 있는 값은 자동 복호화
- 환경변수 우선순위: OS 환경변수 > .env 파일 > 코드 기본값
- .env 파일은 .gitignore에 등록, 저장소에 포함하지 않음

> **결정 사유:** 인증 정보를 코드에 포함하지 않고 .env로 분리.
> AES-256-GCM 암호화로 .env 파일이 유출되더라도 평문 노출을 방지.

### 7.2 프론트엔드 (config.js)
```javascript
{
  API_BASE: 'http://localhost:9000/api/v2',
  SSO_URL: 'https://sso.example.com',
  USE_SSO: false,       // 운영: true
  DEV_USER: 'duke.kimm' // 개발 전용
}
```

---

## 8. 실시간 통신

| 용도 | 기술 | 엔드포인트 | 비고 |
|------|------|-----------|------|
| 비상 공지 푸시 | SSE | GET /Notice/stream | event: emergency |
| 접속 상태 관리 | userConnections | - | 향후 WebSocket 확장 |

> **결정 사유:**
> - SSE: HTTP 기반 단방향 푸시. 브라우저 네이티브 지원, 방화벽 친화적, 구현 단순.
> - WebSocket: 양방향 통신이 필요한 기능(채팅, 실시간 협업) 추가 시 도입 검토.

---

## 9. 향후 확장 포인트

| 항목 | 설명 | 우선순위 |
|------|------|---------|
| WebSocket/SignalR | 양방향 실시간 통신 (채팅, 협업 알림) | 중 |
| RBAC 확장 | 리소스별 세분화 권한 (프로젝트별 역할) | 중 |
| 감사 로그 | 사용자 행동 추적 컬렉션 | 중 |
| 서비스 메시 | Istio/Linkerd 사이드카 기반 트래픽 관리 | 낮음 (규모 확장 시) |
| Module Federation | 프론트 앱별 독립 빌드/배포 | 낮음 (앱 수 증가 시) |
| 서비스 디스커버리 | 서비스 자동 등록/해제 | 낮음 |
| 헬스 체크 | 서비스 상태 모니터링 대시보드 | 중 |
| 분산 추적 | trace ID 기반 서비스 간 로깅 | 낮음 |
| 다국어 | i18n 프레임워크 적용 | 낮음 |
| 테마/브랜딩 | 서비스별 로고/색상 커스터마이징 | 낮음 |

---

## 부록 A. 주요 설계 결정 기록

| 번호 | 결정 사항 | 채택 | 미채택 대안 | 사유 |
|------|----------|------|------------|------|
| D-01 | 권한 모델 | 비트마스크 OR 연산 | 개별 사용자 권한 저장 | 그룹 변경 시 전파 불필요, 항상 최신 상태 보장 |
| D-02 | 삭제 방식 | soft delete (isActive) | 물리 삭제 | 참조 무결성 보존, 이력 추적 가능 |
| D-03 | 실시간 통신 | SSE | WebSocket, SignalR | 단방향 푸시만 필요, 브라우저 네이티브 지원, 구현 단순 |
| D-04 | 서비스 간 통신 | 클라이언트 직접 호출 | API Gateway | Gateway는 SPOF/병목 위험, 현재 규모에서 직접 호출이 충분 |
| D-05 | 확장 시 통신 관리 | 서비스 메시 | API Gateway | 분산형으로 장애 격리/확장성 우수, SPOF 없음 |
| D-06 | 프론트 앱 분리 | 동적 import + 레지스트리 | Module Federation | 현재 앱 수 적음, 빌드 통합이 단순. 앱 증가 시 전환 |
| D-07 | 인증 정보 저장 | .env + AES-256-GCM | 코드 하드코딩 | 보안, 환경별 설정 분리 |
| D-08 | 조직 계층 | 유형 기반 자동 정렬 | sortOrder 수동 지정 | Company>Division>Team>Group 고정 순서, 관리 부담 감소 |
| D-09 | SSO 개발 모드 | USE_SSO=false 고정 사용자 | 항상 SSO 필수 | 폐쇄망 외부 개발 환경에서 SSO 서버 없이 독립 테스트 |
| D-10 | 문서 형상관리 | GitHub API 프록시 | 자체 Git 서버 | GitHub의 PR/리뷰/머지 기능을 그대로 활용, 중복 구현 방지 |
| D-11 | 문서 계층 구조 | GitHub 폴더 구조 활용 | 메타파일(SUMMARY.md) | 별도 메타 관리 불필요, tree API로 직접 조회 |
| D-12 | GitHub 인증 | PAT (Personal Access Token) | GitHub App | 설정 간단, 조직 규모 확장 시 전환 가능 |
| D-13 | 문서 커밋 방식 | 로컬 Git (clone→edit→push) | GitHub Contents API | 여러 파일 1커밋, Rate Limit 무관, 표준 Git 워크플로우 |
| D-14 | repo 정보 저장 | project.documents에 통합 | 별도 docRepos 컬렉션 | 1:1 관계에서 별도 컬렉션은 불일치 위험만 증가. 단일 source of truth |
| D-15 | Sync 전략 | GitHub + project.documents 비교 | docRepos 컬렉션 참조 | 별도 컬렉션 없이 직접 비교, 관리 복잡도 최소화 |
| D-16 | PR 상태 갱신 | GitHub Webhook + SSE | 주기적 폴링 | 즉시 반영, 불필요한 API 호출 제거 |
