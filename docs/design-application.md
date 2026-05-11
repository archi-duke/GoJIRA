# GoJIRA 애플리케이션 설계서

> GoJIRA-App (React SPA, :3001) — Module Federation Remote
> 최종 갱신: 2026-05-01

---

## 1. 개요

GoJIRA-App은 시스템/소프트웨어 엔지니어링 프로젝트를 관리하기 위한 싱글 페이지 애플리케이션이다. React 기반으로 동작하며, 라우터 라이브러리 없이 상태 기반 페이지 전환을 사용한다.

### 1.1 주요 기능

| 기능 | 설명 |
|------|------|
| **ProjectMap** | 간트 차트 형태의 프로젝트 타임라인 |
| **IssueList** | 이슈 목록 조회/편집 |
| **RelationMap** | VOS-Feature-Requirement-Design 관계 시각화 |
| **SATAM** | 안전성 분석 매트릭스 (Risk/Sensitivity/Mitigation) |
| **DocEditor** | Git 기반 문서 편집/PR/Diff |
| **AgentChat** | AI 에이전트 대화 (SSE 스트리밍) |

### 1.2 백엔드 연동

| 서비스 | 포트 | 용도 |
|--------|------|------|
| PLATFORM_API | :9000 | 인증, 계정, 공지, 문서 저장소 |
| API_BASE (GoJIRA-API) | :9001 | 이슈, 프로젝트, 알람 |
| AGENT_API | :9002 | Agent 채팅, 스트리밍 |

---

## 2. 앱 아키텍처

### 2.1 컴포넌트 트리

```mermaid
graph TD
    App["App.js<br/>(메인 라우터)"]

    App --> TitleBar["TitleBar<br/>로고 + 네비게이션 + 사용자"]
    App --> ProjectMap["ProjectMap.js<br/>간트 차트"]
    App --> IssueList["IssueList.js<br/>이슈 목록"]
    App --> RelationMap["RelationMap.js<br/>관계 맵"]
    App --> SATAM["SATAM.js<br/>안전성 매트릭스"]
    App --> DocEditor["DocEditor.js<br/>문서 편집기"]
    App --> AgentChat["AgentChat.js<br/>Agent 채팅 (GoJIRA 전용)"]
    App --> AlertModal["AlertModal.js<br/>전역 모달"]
    App --> NoticePopup["NoticePopup.js<br/>실시간 공지"]

    note1["※ AccountManager, NoticeManager는<br/>Platform-App 전용 (GoJIRA-App에 없음)"]
    style note1 fill:#fff3cd,stroke:#ffc107

    ProjectMap --> ContextMenu["ContextMenu.js"]
    ProjectMap --> ProjectDialog["ProjectDialog.js"]

    IssueList --> CommonBar["CommonBar.js"]
    IssueList --> IssueDetailDialog["IssueDetailDialog.js"]

    RelationMap --> CommonBar
    RelationMap --> IssueDetailDialog

    SATAM --> CommonBar
    SATAM --> IssueDetailDialog

    DocEditor --> CommonBar
    DocEditor --> DiffAnalysis["DiffAnalysis.js"]

    style App fill:#4cafe8,color:#fff
    style AgentChat fill:#70ad47,color:#fff
    style CommonBar fill:#ff9a1e,color:#fff
```

### 2.2 공통 유틸리티

```
src/
├── config.js          # API 엔드포인트 설정 (PLATFORM_API, API_BASE, AGENT_API)
├── utils/
│   ├── auth.js        # 인증 (SSO 쿠키 / 개발용 고정 사용자)
│   └── exportUtils.js # HTML/JPG/Clipboard 내보내기
└── components/
    ├── CommonBar.js         # 공통 상단바 (탭 네비게이션, 편집모드, Export)
    ├── AlertModal.js        # 전역 알림/확인 모달
    ├── NoticePopup.js       # SSE 기반 실시간 공지 팝업
    ├── ContextMenu.js       # 우클릭 컨텍스트 메뉴
    ├── ProjectDialog.js     # 프로젝트 생성/편집 다이얼로그
    ├── IssueDetailDialog.js # 이슈 상세 편집 모달
    └── DiffAnalysis.js      # Semantic/Unified/Summary Diff 뷰
```

---

## 3. 컴포넌트 설계

### 3.1 App.js (메인 라우터)

앱의 진입점. React Router를 사용하지 않고 `page` 상태로 화면을 전환한다.

**상태 관리:**

| 상태 | 타입 | 초기값 | 설명 |
|------|------|--------|------|
| `user` | string | `getAuthUser()` | 현재 로그인 사용자 |
| `authority` | object | null | 권한 정보 (USR, NOTICE 등) |
| `page` | string | `sessionStorage` 또는 'map' | 현재 페이지 |
| `selectedProject` | object | `sessionStorage` 또는 null | 선택된 프로젝트 |
| `allUsers` | array | [] | 사용자 목록 (관리자 전용) |

**권한 기반 메뉴:**

- 기본: Project
- `authority.USR >= 15`: Account 메뉴 노출
- `authority.NOTICE >= 15`: Notice 메뉴 노출

**Agent 독립 모드:**

- URL 파라미터 `?mode=agent` 감지 시 `<AgentChat standalone />` 단독 렌더링
- GoJIRA-App 로고 클릭 시 `window.open('/?mode=agent', 'gojira-agent', 'width=480,height=720')`
- Platform-App 로고 클릭 시 `window.open('http://localhost:3001/?mode=agent', ...)` (cross-origin)
- AgentChat은 **GoJIRA-App 전용** (Platform-App에서 제거됨). 향후 멀티 에이전트 시 공통화 검토

### 3.2 ProjectMap.js (간트 차트)

프로젝트 타임라인을 간트 차트로 시각화한다.

**축 구성:**

- 가로축: 시간 (기본 현재 -1년 ~ +2년, 슬라이더로 -5년 ~ +5년 조절)
- 세로축: 프로젝트 목록 (행 높이 48px)
- 사이드바: 프로젝트명 (180px 고정)

**마일스톤 표시:**

| 마일스톤 | 의미 | 표시 |
|----------|------|------|
| PA | Project Approval | 속이 빈 원 |
| EA | Engineering Approval | 속이 빈 원 |
| ER1 | Engineering Review 1 | 속이 빈 원 |
| ER2 | Engineering Review 2 | 속이 빈 원 |
| CA | Customer Approval | 속이 빈 원 |

**상태 색상:**

- 진행중: 파란색
- 완료: 회색
- 지연: tomato

**기능:**

- 조회기간 슬라이더 (range slider)
- 정렬: 시작일 / 종료일 / 과제명 / 부서명
- `+ Add` 버튼으로 프로젝트 생성
- 우클릭 → ContextMenu (상세보기, 편집, 삭제)
- 알람 배지: `branchDiff` (tomato), `hasOpenPR` (green)

**SSE 구독:**

- `EventSource` → `PLATFORM_API/Notice/stream`
- `doc_update` 이벤트: 브랜치 알림 갱신
- `pr_update` 이벤트: PR 상태 알림 재조회

### 3.3 IssueList.js

이슈 목록 테이블 뷰.

**필터 옵션 (sessionStorage 유지):**

| 키 | 이슈 타입 |
|----|-----------|
| V | VOS |
| F | Feature |
| R | Requirement |
| A | Architectural Design |
| D | Detailed Design |

**편집 모드:**

- `status`: 드롭다운 (To Do / In Progress / Done)
- `priority`: 드롭다운 (Highest ~ Lowest)
- `summary`: 인라인 텍스트 편집
- 삭제 버튼

**정렬:** 컬럼 헤더 클릭 시 오름차순/내림차순 토글

**이슈 클릭:** `IssueDetailDialog` 모달 열기

### 3.4 IssueDetailDialog.js

이슈 상세 편집 모달. 이슈 타입별 탭 구성.

**탭별 필드:**

| 탭 | 이슈 타입 | 주요 필드 |
|----|-----------|-----------|
| VOS | VOS | summary, description, priority, status |
| Feature (UC) | Feature | summary, description, 유스케이스 |
| Requirement | Requirement | summary, subtype(FR=`FR_001`, NFR→`QA_001`, CNST=`CNST_001`로 자동 매핑), ASR 여부 |
| Design (ADSN) | Architectural/Detailed Design | summary, 설계 상세 |
| Trade Off | 공통 | Risk/Sensitivity/Mitigation YAML 편집 |

### 3.5 RelationMap.js

이슈 간 관계를 컬럼 기반으로 시각화한다.

**컬럼 구성:**

| 컬럼 | 이슈 타입 | 추가 버튼 |
|------|-----------|-----------|
| VOS | VOS | VOS |
| Feature | Feature | Feature |
| Requirement | Requirement | FR, QA(NFR), CNST |
| Design | Architectural/Detailed Design | ADSN, DDSN |
| Test (선택) | Test | Test |

**카드:**

- SubKey 색상 코딩: VOS(녹색), UC(주황), QA/FR/CNST(빨강), ADSN(파랑), DDSN(보라)
- Key, Summary 표시

**연결선:**

- `issueLinks` (contains, relates) 기반 베지어 곡선
- 같은 타입 연결: 배경에 렌더링
- 다른 타입 연결: 전면에 렌더링
- 카드 선택 시 관련 체인 하이라이트

**편집 모드:**

- `+` 버튼으로 이슈 추가
- 드래그로 연결 생성
- ASR/Reserved 토글

**필터:**

- A: ASR 이슈만
- R: Reserved 이슈만
- T: Test 컬럼 표시/숨김

**정렬:** 강조 > 타입 > ASR > SubKey

### 3.6 SATAM.js (안전성 분석 매트릭스)

Design 이슈와 Requirement 이슈 간의 안전성 분석 매트릭스.

**매트릭스 구조:**

- 가로축 (열): Design 이슈
- 세로축 (행): Requirement 이슈 (SubType 정렬: QA → CNST → FR)

**셀 마크:**

| 마크 | 의미 | 색상 |
|------|------|------|
| R | Risk | 빨강 (#e83a1e) |
| S | Sensitivity | 주황 (#ff9a1e) |
| M | Mitigation | 파랑 (#4cafe8) |

**Contains 관계:** 검은 원으로 표시

**편집 모드:**

- 셀 클릭 → R/S/M 추가/삭제
- Link as Solution (contains 관계 생성)

**필터:**

| 키 | 의미 |
|----|------|
| A | ASR 이슈만 |
| F | FR 이슈만 |
| S | 최종설계 (DDSN 제외) |
| D | DDSN 포함 |
| R | Reserved 포함 |

**Trade Off YAML 파싱:**

```yaml
- ID : R1
  QA : [GOJIRA-001, GOJIRA-002]
  Counter Risk : ["R1:GOJIRA-003"]
  Description : 위험 요소 설명
```

**SATAM 분석:** Agent Assist 버튼 → AgentChat으로 분석 요청

### 3.7 DocEditor.js (문서 편집기)

Git 기반 마크다운 문서 편집기.

**파일 트리:**

- Main 탭: main 브랜치 파일 (읽기 전용)
- Workspace 탭: 사용자 브랜치 (`docs/{user}`) 파일

**파일 상태 표시:**

| 상태 | 표시 |
|------|------|
| modified | 변경됨 배지 |
| added | 추가됨 배지 |
| deleted | 삭제됨 배지 |
| mainOnly | M 배지 (gray), italic, Pull 동기화 안내 |

**초기 폴더 구조 (프로젝트 생성 시 자동 생성):**

```
business/
  project-description.md    # 과제 배경, 목적, 주요 사항
  use-cases.md              # 유즈케이스 정의
  requirements.md           # FR/NFR/CNST 요구사양
architecture/
  architecture.md           # 아키텍처 설계
```

**파일 트리 표시:**

- 폴더별 그룹화: 📁 폴더 헤더 + 들여쓰기(16px)
- 루트 파일: 폴더 아래에 표시
- 폴더/파일 추가: `+` 버튼 → `modalPrompt`로 경로 입력 (`폴더명/파일명.md` 형식 지원)

**알림 아이콘:**

| 아이콘 | 파일 | 색상 | 의미 |
|--------|------|------|------|
| alert-diff.svg | branchDiff | tomato | main과 작업공간 차이 있음 |
| alert-pr.svg | hasOpenPR | #27ae60 | 열린 PR 있음 (우선 표시) |
| git-fetch.svg | Pull 버튼 | - | main → 작업공간 동기화 |
| git-pull.svg | 발행신청 앞 | - | PR 제출 아이콘 |

**편집 흐름:**

1. 초기 파일 생성 (`init-files` — 없는 파일만 생성)
2. 사용자 브랜치 자동 생성 (`ensure-branch`)
3. 트리 비교 (main vs workspace) — `refreshDocState` 통합 함수
4. 파일 편집 → 저장 → commit/push
5. 실시간 갱신: WebSocket `subscribe` (sseManager.js)

**AI생성 버튼:** 편집 모드에서 저장 버튼 좌측에 표시. 내용 충분 시 LLM 보강, 부족 시 채팅 인터뷰 제안

**PR 기능:** 생성 / 승인 / 반려 / 머지

**Diff 모드:** Semantic / Unified / Summary (DiffAnalysis 컴포넌트)

**SSE 구독:** `EventSource` → `PLATFORM_API/Notice/stream` → `doc_update` 이벤트로 main 변경 시 트리 실시간 갱신

#### 3.7.1 다이어그램 렌더링과 인라인 편집 (pumlex 통합)

ReactMarkdown 의 `components.code` 후크로 fenced code block 의 언어별 렌더러를 분기한다.

| 언어 | 컴포넌트 | 비고 |
|------|----------|------|
| `mermaid` | `MermaidBlock` | 클라이언트 mermaid 렌더 |
| `plantuml` / `puml` | `PexBlock` | pumlex 서버 (`PUMLEX_SERVER`, 기본 `:3030`) `/render-with-layout` SVG + 인라인 편집 |

**PexBlock 동작:**

- 마운트 시 `/render-with-layout` 으로 fence 본문을 POST → 응답 SVG 를 `useEffect` + ref 로 직접 inject (React 의 `dangerouslySetInnerHTML` 은 매 재렌더에서 setInnerHTML 을 다시 호출해 PexInline 이 단 listener 가 사라지므로 회피)
- 인라인 편집 활성 조건: `globalEditMode && !editMode` (편집모드 ON + textarea 가 아닌 preview)
- ✎ 편집 → `window.PexInline.activate(host, { source, onSourceChange })` → entity drag / edge 곡선화 / 핸들 오버레이
- ✓ 적용 → `pendingSrcRef` 의 메타 포함 source (PexMeta 가 `' @startmeta`/`' @endmeta` 코멘트 블록으로 fence 끝에 embed) 를 `onCommit(originalSource, newSource)` 으로 host 에 통보
- ✗ 취소 → 세션 deactivate 후 markdown 변경 없이 종료

**호스트 (DocEditor) 처리:**

- `replacePlantumlFenceBySource(md, originalSource, newBody)` — markdown 내에서 본문이 `originalSource` 와 일치하는 첫 plantuml/puml fence 의 본문을 `newBody` 로 교체. fence 식별을 idx 카운터가 아닌 본문 자체로 수행 — ReactMarkdown 의 `components.code` 호출 타이밍과 idx 매핑이 어긋나는 케이스 회피
- `setContent(next)` 후 즉시 `PUT /DocRepo/{code}/file` 호출 (modalConfirm 없이 자동 저장) — 인라인 편집은 즉각 디스크 반영 워크플로우

**pex-core 로딩 (마이크로프론트엔드 호환):**

- `public/pex-core/{geom,meta,inline}.js` — pumlex 의 `docs/pex-core/*.js` UMD 빌드를 vendor (수정 없음)
- `src/utils/pexLoader.js` — 동적 `<script>` inject. 컨테이너 (:3000) 에서 띄울 때 GoJIRA-App 자기 origin (`:3001`) 으로 cross-origin fetch. UMD 의 `module`/`exports`/`require` 분기가 webpack 환경에서 Node 분기로 빠지지 않도록 inject 직전·직후로 임시 가림
- 멱등 + Promise memoize — 여러 PexBlock 이 동시에 호출해도 1회 fetch

### 3.8 AgentChat.js (AI 에이전트 채팅)

AI 에이전트와의 대화 인터페이스.

**Props:**

| Prop | 타입 | 설명 |
|------|------|------|
| standalone | boolean | true면 전체 화면 모드 |
| autoMessage | string | 초기 로딩 시 자동 전송 메시지 (DocEditor의 AI생성 클릭으로 워크플로우 트리거 시 사용) |
| newSession | boolean | true면 기존 세션 무시하고 새 세션 시작 |
| interviewPrompt | string | 인터뷰 모드 시작 (project-description.md 등) |
| continueTo | string | 인터뷰 완료 후 이어갈 대상 파일 경로 |

**모드:**

| 모드 | 조건 | 동작 |
|------|------|------|
| standalone | `?mode=agent` | 전체 화면, 닫기 버튼 숨김 |
| standalone + autoMessage | `?mode=agent&newSession=1&autoMessage=...` | 새 세션에서 메시지 자동 전송 |
| panel | 기본 | 인라인 패널 |

**세션 관리:**

- `sessionStorage`에 `agent_sessionId` 저장
- 이력 검색: `GET /Agent/sessions?q=검색어`
- 세션 로드: `GET /Agent/session/{sid}`

**디버그 뷰 (Master 사용자, USR >= 15):**

- 접기/펼치기 블록으로 "작업 과정" 표시
- 추론(reasoning), 도구 호출(tool_call), 도구 결과(tool_result), **진행 단계(progress)** 표시
- 실행 시간(duration_ms), 상세 결과(detail) 확인 가능

**일반 사용자:** 스피너 + 진행 단계(progress) 메시지만 노출 (예: "VOS 수집 시작", "Use Case 분석 완료")

**워크플로우 트리거 흐름 (DocEditor → AgentChat 연동):**

- DocEditor에서 use-cases.md / requirements.md의 AI생성 클릭 → 채팅 창을 열고 `autoMessage`로 워크플로우 자연어 요청 전송 (REST 직접 호출 폐지)
- 인터뷰가 필요한 경우 projdesc 저장 후 DocEditor가 `postMessage({type:'gojira-continue', target, projectCode})`를 채팅 창에 전송 → AgentChat이 후속 작업 메시지를 자동 입력

### 3.9 CommonBar.js (공통 상단바)

프로젝트 하위 페이지에서 공통으로 사용하는 상단 바.

**레이아웃:**

```
[Title] [Docs] [Issues] [RelationMap] [SATAM] | [옵션...] | [Pull] [Export] [Agent분석] [편집모드]
```

**Export 메뉴:**

- HTML 파일
- JPG 이미지
- HTML 클립보드

---

## 4. 페이지 전환

### 4.1 상태 다이어그램

```mermaid
stateDiagram-v2
    [*] --> map : 앱 진입

    map --> issues : 프로젝트 클릭
    map --> relationMap : 우클릭 → Relation Map
    map --> satam : 우클릭 → SATAM
    map --> account : Account 메뉴 (USR≥15)
    map --> notice : Notice 메뉴 (NOTICE≥15)

    issues --> docs : Docs 탭
    issues --> relationMap : RelationMap 탭
    issues --> satam : SATAM 탭
    issues --> map : 뒤로가기

    docs --> issues : Issues 탭
    docs --> relationMap : RelationMap 탭
    docs --> satam : SATAM 탭
    docs --> map : 뒤로가기

    relationMap --> docs : Docs 탭
    relationMap --> issues : Issues 탭
    relationMap --> satam : SATAM 탭
    relationMap --> map : 뒤로가기

    satam --> docs : Docs 탭
    satam --> issues : Issues 탭
    satam --> relationMap : RelationMap 탭
    satam --> map : 뒤로가기

    account --> map : Project 메뉴
    notice --> map : Project 메뉴

    note right of map
        BroadcastChannel 수신 시
        projectCode + targetPage로
        직접 전환 가능
    end note
```

### 4.2 전환 메커니즘

```javascript
// 상태 기반 전환 (React Router 미사용)
const navigate = (page, project) => {
  setPage(page);
  setSelectedProject(project);
  sessionStorage.setItem('gojira_page', page);
  // project도 sessionStorage에 저장 → 새로고침 시 복원
};
```

**페이지 판별 로직:**

| 조건 | 렌더링 |
|------|--------|
| `page === 'docs' && selectedProject` | DocEditor |
| `page === 'issues' && selectedProject` | IssueList |
| `page === 'relationMap' && selectedProject` | RelationMap |
| `page === 'satam' && selectedProject` | SATAM |
| `?mode=agent` URL 파라미터 | AgentChat (standalone) |
| 그 외 | ProjectMap |

> **참고:** Account/Notice 페이지는 Platform-App(:3000) 전용. GoJIRA-App에서는 렌더링하지 않음.

---

## 5. 실시간 통신

### 5.1 SSE (Server-Sent Events) 구독

각 컴포넌트가 `EventSource`를 직접 생성하여 서버 이벤트를 구독한다.

```mermaid
sequenceDiagram
    participant Server as PLATFORM_API<br/>/Notice/stream
    participant PM as ProjectMap
    participant DE as DocEditor
    participant NP as NoticePopup

    Server->>PM: event: doc_update<br/>(project_alert)
    PM->>PM: branchDiff 배지 갱신

    Server->>PM: event: doc_update<br/>(pr_update)
    PM->>PM: hasOpenPR 배지 재조회

    Server->>DE: event: doc_update<br/>(project_alert)
    DE->>DE: 파일 트리 새로고침

    Server->>NP: event: notice
    NP->>NP: 공지 팝업 표시
```

**EventSource 연결 전략:**

- 컴포넌트 마운트 시 생성, 언마운트 시 `close()`
- 브라우저 내장 자동 재연결 (기본 3초)
- 각 컴포넌트가 독립적으로 구독 (공유 매니저 없음)

### 5.2 이벤트 타입

| 이벤트 | 소스 | 수신 컴포넌트 | 동작 |
|--------|------|---------------|------|
| `doc_update` (project_alert) | DocRepo 웹훅 | ProjectMap, DocEditor | 브랜치 차이/트리 갱신 |
| `doc_update` (pr_update) | PR 상태 변경 | ProjectMap | 알림 재조회 |
| `notice` | 공지 등록 | NoticePopup | 팝업 표시 |

---

## 6. Agent 채팅 설계

> **2026-05 갱신** — Agent-Service 가 자체 ReACT 를 돌리지 않고 OpenCode 서버(`:4096`, 사용자 PC 로컬) 가 LLM 호출·ReACT·스킬 디스패치를 담당. Agent-Service 는 세션 매핑 + SSE 번역 어댑터. 상세는 [docs/design-agent.md](./design-agent.md), [docs/design-platform.md §5](./design-platform.md).

### 6.1 SSE 스트리밍 시퀀스 (OpenCode bridge)

```mermaid
sequenceDiagram
    participant User as 사용자
    participant AC as AgentChat (SPA)
    participant Agent as Agent-Service<br/>(:9002)
    participant OC as OpenCode 서버<br/>(:4096, 사용자 PC)
    participant LLM as Platform-API<br/>/LLM/call

    User->>AC: 메시지 입력 + 로컬 user 버블 즉시 append
    AC->>Agent: POST /Agent/chat/send {sessionId, message}
    Agent->>OC: /session/{id}/prompt_async
    Agent->>Agent: ensure_subscriber (persistent — turn 사이도 alive)

    AC->>Agent: GET /session/{id}/events (EventSource)
    Note over OC,LLM: OpenCode 가 ReACT 루프 + 스킬 디스패치<br/>LLM 호출은 Platform-API 게이트웨이를 통해서만
    OC->>LLM: POST /LLM/call (사내/외부 모델)
    LLM-->>OC: 응답
    OC-->>Agent: /event SSE (message.updated / message.part.updated / .delta)
    Agent->>Agent: translator → UI 포맷 변환 + event_buffer push

    rect rgb(240, 248, 255)
        Note over Agent,AC: SSE 변환 이벤트 (lastEventId 기반 재연결 지원)
        Agent-->>AC: event: session {sessionId}
        Agent-->>AC: event: thinking (assistant 시작)
        Agent-->>AC: event: reasoning {content}
        Agent-->>AC: event: tool_call {tool, params}
        Agent-->>AC: event: tool_result {status, summary, detail}
        Agent-->>AC: event: content {text} (델타 누적)
        Agent-->>AC: event: user_message {text} (TUI 입력 또는 echo)
        Agent-->>AC: event: done (turn 종료, ES 는 유지)
    end

    Note over AC: ES 는 turn 종료 후에도 유지 — turn 사이 TUI 입력 즉시 수신.<br/>endTurn() 은 Promise resolve + UI cleanup, fullClose() 는 ES 까지 닫음.

    opt SSE 침묵 (10s+ 무이벤트)
        AC->>Agent: GET /session/{id}/state (폴링)
        Agent-->>AC: {finish, active, hasPermissionPending, lastActivityAt}
    end

    opt assistant 응답에 _action: navigate 포함
        AC->>AC: BroadcastChannel (same-origin) + window.opener.postMessage (cross-origin)
    end
```

### 6.2 채팅 라이프사이클 핵심

| 메커니즘 | 동작 |
|---|---|
| **persistent EventSource** | done 으로 ES 닫지 않음. 다음 chat-send 또는 명시적 cancel 까지 alive — TUI 입력을 채팅창에 즉시 반영하기 위함 |
| **endTurn vs fullClose** | endTurn: Promise resolve + loading=false + `_streaming` 플래그 해제. ES 유지. fullClose: endTurn + es.close — abort/unmount 에만 호출 |
| **loadSession preserveOnShrink** | finish 시점 Mongo 가 stale 이면 화면을 통째로 비우는 사고 방지 — 빈/축소 결과면 setMessages skip |
| **TUI 입력 (user_message)** | OpenCode TUI 에서 직접 입력한 메시지를 translator 가 `user_message` 이벤트로 yield. UI 가 user 버블로 append (chat-send echo 와 content 기반 dedup) |
| **단계별 인디케이터** | `_streaming` 메시지의 마지막 segment 종류로 라벨 결정 — `생각하는 중 / 추론 중 / 도구 사용 중: {skill} / 응답 작성 중` |

### 6.3 메시지 타입

| role | 설명 |
|------|------|
| `user` | 사용자 입력 (chat 발송 + TUI 입력 echo 둘 다) |
| `assistant` | AI 응답 (Mongo 에는 OpenCode 의 info+parts 원본 그대로 저장, 렌더는 클라이언트 partsToSegments) |
| `process` (레거시) | 옛 ReACT 시절 작업 과정 — 호환용 표시만 유지 |

### 6.3 크로스 윈도우 통신

```mermaid
sequenceDiagram
    participant MainApp as 메인 앱<br/>(Platform :3000 또는 GoJIRA :3001)
    participant Logo as 로고 클릭
    participant AgentWin as Agent 창<br/>(:3001/?mode=agent)

    Logo->>AgentWin: window.open<br/>('http://localhost:3001/?mode=agent')

    Note over AgentWin: 사용자가 Agent에게<br/>"PROJ-001 relation map 보여줘"

    AgentWin->>AgentWin: tool_result에<br/>_action: navigate 수신

    alt same-origin (GoJIRA-App :3001)
        AgentWin->>MainApp: BroadcastChannel<br/>{projectCode, projectName, page}
    else cross-origin (Platform-App :3000)
        AgentWin->>MainApp: window.opener.postMessage<br/>{type:'gojira-navigate', projectCode, page}
    end

    MainApp->>MainApp: navigate(page, project)
```

---

## 7. 프로젝트 알람 설계

### 7.1 알람 흐름

```mermaid
flowchart LR
    subgraph 서버
        WH["Git 웹훅<br/>(push/merge)"]
        API["GoJIRA-API<br/>(:9001)"]
        SSE["PLATFORM_API<br/>Notice/stream"]
    end

    subgraph 클라이언트
        PM["ProjectMap"]
        DE["DocEditor"]
    end

    WH -->|"브랜치 diff 계산"| API
    API -->|"project_alert 이벤트"| SSE
    SSE -->|"EventSource"| PM
    SSE -->|"EventSource"| DE

    PM -->|"branchDiff: tomato 배지"| PM
    PM -->|"hasOpenPR: green 배지"| PM
    DE -->|"트리 새로고침"| DE
```

### 7.2 알람 배지

| 배지 | 색상 | 의미 | 위치 |
|------|------|------|------|
| `branchDiff` | tomato | main과 사용자 브랜치 간 차이 존재 | ProjectMap 행, CommonBar |
| `hasOpenPR` | green | 열린 PR 존재 | ProjectMap 행, CommonBar |

### 7.3 알람 데이터 구조

```typescript
// GET /Project/alerts?user={knoxId}
interface ProjectAlert {
  jiraCode: string;
  branchDiff: boolean;   // main과 사용자 브랜치 차이 존재
  hasOpenPR: boolean;    // 열린 PR 존재
  issueUpdated: boolean; // 이슈 변경 알림
}
```

---

## 8. 설계 결정 기록

### 8.1 React SPA (라우터 미사용)

**결정:** React Router를 사용하지 않고, `page` 상태 + `sessionStorage`로 페이지 전환을 관리한다.

**사유:**

- 페이지 수가 제한적 (7개)이고 모두 단일 앱 내 전환
- URL 기반 라우팅이 불필요 (딥링크 요구사항 없음)
- 상태 복원은 `sessionStorage`로 충분
- 의존성 최소화

### 8.2 독립 Agent 창 (팝업 방식)

**결정:** AgentChat을 인라인 패널이 아닌 `window.open` 팝업으로 분리한다.

**사유:**

- 초기 오버레이 패널 방식에서 부모 컴포넌트 리마운트 이슈 발생
- Wrapper 컴포넌트로 감싸는 방식도 상태 유실 문제 지속
- 독립 창은 자체 React 트리를 갖기 때문에 메인 앱 상태 변경의 영향을 받지 않음
- `BroadcastChannel`로 크로스 윈도우 통신을 해결

### 8.3 BroadcastChannel (크로스 윈도우 통신)

**결정:** `window.postMessage` 대신 `BroadcastChannel`을 사용한다.

**사유:**

- `window.opener` 참조 기반 통신은 창이 새로고침되면 참조가 끊어짐
- `BroadcastChannel`은 같은 origin의 모든 탭/창 간 통신 가능
- 송신 측이 수신 측의 존재를 알 필요 없음 (느슨한 결합)

### 8.4 SSE (EventSource) 구독 전략

**결정:** 공유 매니저 없이 각 컴포넌트가 독립적으로 `EventSource`를 생성한다.

**사유:**

- 컴포넌트별로 관심 이벤트가 다름 (ProjectMap: project_alert + pr_update, DocEditor: project_alert, NoticePopup: notice)
- 동시에 마운트되는 컴포넌트가 최대 2개 (예: ProjectMap은 다른 페이지와 동시 마운트되지 않음)
- EventSource는 브라우저가 HTTP/2 멀티플렉싱으로 처리하므로 연결 수 부담 적음
- 복잡한 구독 관리 레이어 불필요

### 8.5 sessionStorage (상태 유지)

**결정:** 전역 상태 관리 라이브러리(Redux 등) 대신 `sessionStorage`로 페이지 상태를 유지한다.

**사유:**

- 유지해야 할 상태가 단순 (page, selectedProject, 필터 옵션, 정렬 키, viewRange)
- 새로고침 시 복원만 되면 충분
- 탭 간 상태 공유 불필요 (각 탭은 독립 세션)
- 외부 의존성 없이 구현 가능

### 8.6 Agent 스트리밍 폴백

**결정:** SSE 스트리밍 실패 시 비스트리밍 REST API로 자동 폴백한다.

**사유:**

- 프록시/방화벽 환경에서 SSE 연결이 차단될 수 있음
- 폴백으로 기능 자체는 항상 동작 보장
- 스트리밍: `POST /Agent/chat/stream` → 폴백: `POST /Agent/chat`

### 8.7 인증 전략

**결정:** SSO 쿠키 기반 인증과 개발용 고정 사용자 모드를 `config.USE_SSO` 플래그로 전환한다.

**사유:**

- 운영 환경: ADSSO 쿠키로 자동 인증
- 개발 환경: SSO 인프라 없이 고정 사용자(`DEV_USER`)로 즉시 개발 가능
- 관리자(USR >= 15)는 사용자 전환 드롭다운으로 다른 사용자 시뮬레이션 가능
