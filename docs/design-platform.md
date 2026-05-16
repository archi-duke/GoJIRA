# GoJIRA 플랫폼 설계서

> 최종 갱신: 2026-04-10  
> 버전: 1.0  
> 상태: 확정 (단, 3.5 DocRepo / EditSession 은 리팩토링 진행 중 — 아래 참조)

> **⚠ 리팩토링 진행 중 (2026-05)** — 사내 폐쇄망 배포 검토 과정에서 다음 변경이 결정되었습니다. 본 문서는 구현 완료 후 본문이 갱신됩니다.
> - **단일 사본 공유 모델** — Platform-API `/data/repos` 볼륨이 호스트 bind mount 로 전환되어 OpenCode 와 같은 fs 공유 (3.5 DocRepo 영향)
> - **EditSession 의미 확장** — 현재 "사용자별 세션 트래킹" 에서 진정한 lock 으로 확장. `holderKind: "ui" | "skill"`, `holderInfo` 추가. UI ↔ 스킬 동시 작업 시 acquire 거부 + 상황 보고 (3.5 편집 세션 영향)
> - **file CRUD 핸들러 (`DocRepoFile*`) 유지** — UI 흐름 보존, 스킬만 fs 직접 접근으로 전환
> - **docker-compose `user: "${UID}:${GID}"`** — 호스트 / 컨테이너 권한 정합
>
> 상세: [`docs/refactor-skill-fs-direct.md`](./refactor-skill-fs-direct.md)

---

## 목차

1. [개요](#1-개요)
2. [서비스 아키텍처](#2-서비스-아키텍처)
3. [Platform-API 설계](#3-platform-api-설계)
4. [GoJIRA-API 설계](#4-gojira-api-설계)
5. [Agent-Service 설계](#5-agent-service-설계)
6. [데이터베이스 설계](#6-데이터베이스-설계)
7. [외부 연동](#7-외부-연동)
8. [실시간 통신 설계](#8-실시간-통신-설계)
9. [보안 설계](#9-보안-설계)
10. [설계 결정 기록 (ADR)](#10-설계-결정-기록-adr)

---

## 1. 개요

GoJIRA는 JIRA Datacenter 기반 프로젝트 관리 시스템을 보완하는 통합 플랫폼이다.  
이슈 관리, 문서 이력 관리, AI 에이전트를 하나의 인터페이스로 제공한다.

### 1.1 설계 목표

| 목표 | 설명 |
|------|------|
| **양방향 동기화** | JIRA DC와 MongoDB 간 이슈 데이터의 실시간 양방향 동기화 |
| **문서 이력 관리** | Gitea 기반 문서 버전 관리, PR/Review 워크플로 |
| **AI 에이전트** | LLM 기반 프로젝트 분석, 이슈 관리 자동화 |
| **실시간 UX** | SSE/WebSocket을 통한 즉시 알림 및 상태 반영 |
| **데이터 주권** | 모든 데이터를 자체 인프라에서 관리 |

### 1.2 기술 스택

| 계층 | 기술 |
|------|------|
| Backend API | Go 1.21+ (chi router), Python 3.9+ (FastAPI) |
| Database | MongoDB 7.0 |
| Git Platform | Gitea (self-hosted, Docker) |
| LLM | OpenAI SDK (gpt-4o), 엔드포인트 교체 가능 |
| 실시간 통신 | SSE, WebSocket (gorilla/websocket) |
| 컨테이너 | Docker Compose |

---

## 2. 서비스 아키텍처

### 2.1 시스템 컨텍스트 (C4 Container)

```mermaid
graph TB
    subgraph External["외부 시스템"]
        JIRA["JIRA Datacenter<br/>이슈 관리 원본"]
        LLM["LLM API<br/>(OpenAI / 자체 호스팅)"]
        ADFS["SSO (ADFS)<br/>인증 서버"]
    end

    subgraph GoJIRA["GoJIRA Platform"]
        subgraph FrontEnd["프론트엔드"]
            PA["Platform-App<br/>React SPA"]
            GA["GoJIRA-App<br/>React SPA"]
        end

        subgraph BackEnd["백엔드 서비스"]
            PAPI["Platform-API<br/>Go :9000<br/>계정/공지/문서"]
            GAPI["GoJIRA-API<br/>Go :9001<br/>이슈/프로젝트/Agent프록시"]
            AGENT["Agent-Service<br/>Python :9002<br/>AI 오케스트레이터"]
        end

        subgraph Infra["인프라"]
            MONGO[("MongoDB<br/>gojira DB")]
            GITEA["Gitea<br/>:3300<br/>Git 서버"]
        end
    end

    PA --> PAPI
    GA --> GAPI
    GAPI -- "리버스 프록시<br/>/Agent/*" --> AGENT
    GAPI -- "/LLM/call (게이트웨이)" --> PAPI
    PAPI --> MONGO
    GAPI --> MONGO
    AGENT --> MONGO
    PAPI --> GITEA
    PAPI --> LLM
    GAPI --> JIRA
    AGENT --> GAPI
    AGENT --> PAPI
    AGENT --> LLM
    PA -. "SSO 쿠키" .-> ADFS
    GITEA -- "Webhook" --> PAPI
```

> **LLM 호출 책임**: Platform-API 가 단일 게이트웨이로 OPENAI_API_KEY / 모델 / 엔드포인트를 관리한다.
> GoJIRA-API 는 키 없이 `POST /api/v2/LLM/call` 만 호출한다.
> Agent-Service 는 별도 운영 정책상 자체 LLM 클라이언트 사용 (Python 런타임).

### 2.2 서비스 간 통신

| 경로 | 프로토콜 | 설명 |
|------|----------|------|
| GoJIRA-App -> GoJIRA-API | HTTP REST | 이슈/프로젝트 CRUD |
| GoJIRA-API -> Platform-API (`/LLM/call`) | HTTP REST | LLM 게이트웨이 (PR 요약 등) |
| GoJIRA-API -> Platform-API (`/DocRepo/...`) | HTTP REST | repo 생성·조회·편집 |
| GoJIRA-API -> Agent-Service | HTTP 리버스 프록시 | `GOJIRA_AGENT_PROXY` 환경변수로 제어 |
| Agent-Service -> GoJIRA-API | HTTP REST | 스킬 실행 시 데이터 조회/변경 |
| Agent-Service -> Platform-API | HTTP REST | 문서/계정 정보 조회 |
| Platform-API -> Gitea | REST API (v1) | 문서 CRUD, PR, 브랜치 관리 |
| Platform-API -> LLM | OpenAI 호환 API | 게이트웨이 호출 (`config.LLM` 기반) |
| Gitea -> Platform-API | Webhook (POST) | push/PR/review 이벤트 수신 |
| GoJIRA-API -> JIRA DC | REST API (v2) | 이슈 동기화 |
| Platform-API -> Client | SSE / WebSocket | 실시간 알림 브로드캐스트 |

---

## 3. Platform-API 설계

> Go, chi/v5 라우터, 포트 :9000

### 3.1 라우트 구조

```
/webhook/github                    # Gitea Webhook 수신
/api/v{version}/
  Account/
    me                             # 현재 사용자 (SSO 쿠키 기반)
    users, users/{id}              # 사용자 CRUD
    users/{id}/authority           # 권한 계산 (비트마스크)
    authGroups, authGroups/{id}    # 권한 그룹 CRUD
    orgUnits, orgUnits/{id}        # 조직 단위 CRUD
    userProjects, userProjects/{id}# 사용자-프로젝트 매핑
    connections, connections/{id}  # 접속 상태 관리
  Notice/
    stream                         # SSE 연결
    ws                             # WebSocket 연결
    active                         # 활성 공지 조회
    {id}, {id}/view                # 공지 CRUD + 조회수
  Notification/
    unread                         # 미읽은 알림
    read                           # 읽음 처리
  DocRepo/
    {code}/tree                    # 파일 트리
    {code}/file                    # 파일 읽기/쓰기/삭제
    {code}/pr, {code}/prs          # PR 생성/목록
    {code}/pr/{num}/review         # PR 리뷰 (승인/반려)
    {code}/pr/{num}/merge          # PR 머지
    {code}/edit/start              # 편집 세션 시작
    {code}/edit/commit             # 편집 커밋
    {code}/edit/pr                 # 편집 -> PR 생성
    {code}/edit/heartbeat          # 세션 유지 (TTL 갱신)
  Git/users                        # Gitea 사용자 목록
  LLM/call                         # LLM 게이트웨이 — Platform-API 가 단독으로 키/모델/엔드포인트 보유.
                                   # GoJIRA-API/Agent-Service 는 키 없이 이 엔드포인트만 호출.
                                   # llm.json 의 active 모델 + ${VAR} 치환 + ENC: 시크릿 복호화 (메모리만).
                                   # 사내 LLM(gpt-oss + X-Dep-Ticket) / OpenAI 호환 동시 지원, _disabled_* 로 비활성 설정 보존.
  Project/{code}/archive           # 프로젝트 아카이브 (zip 백업 + 정리)
```

### 3.2 인증 설계

```mermaid
sequenceDiagram
    participant Browser as 브라우저
    participant ADFS as SSO (ADFS)
    participant PAPI as Platform-API

    alt SSO 모드 (USE_SSO=true)
        Browser->>ADFS: 로그인 요청
        ADFS-->>Browser: ADSSO_USER 쿠키 발급
        Browser->>PAPI: API 요청 + ADSSO_USER 쿠키
        PAPI->>PAPI: 쿠키에서 knoxId 추출
        PAPI->>PAPI: DB에서 사용자 조회
        PAPI-->>Browser: 200 OK + 사용자 데이터
    else 개발 모드 (USE_SSO=false)
        Browser->>PAPI: API 요청
        PAPI->>PAPI: DEV_USER 환경변수 사용
        PAPI-->>Browser: 200 OK
    end
```

### 3.3 계정 모델

#### User

| 필드 | 타입 | 설명 |
|------|------|------|
| knoxId | string | SSO 고유 ID |
| name | string | 사용자명 |
| email | string | 이메일 |
| authGroupIds | []string | 소속 권한 그룹 |
| orgUnitIds | []string | 소속 조직 단위 |
| isActive | bool | 활성 여부 (Soft Delete) |

#### AuthGroup (권한 그룹)

| 필드 | 타입 | 설명 |
|------|------|------|
| code | string | 그룹 코드 |
| authority | map[string]int | 리소스별 비트마스크 (C=8, R=4, U=2, D=1) |

**비트마스크 예시:**

```
authority: {
  "issue": 15,     // C+R+U+D = 8+4+2+1
  "project": 6,    // R+U = 4+2
  "notice": 4      // R만 = 4
}
```

#### OrgUnit (조직 단위)

4단계 계층: Company > Division > Team > Group  
`parentOrgUnitId` 자기참조로 트리 구조 표현

### 3.4 공지 (Notice)

**상태 흐름:** `draft` -> `active` -> `expired`

| 필드 | 설명 |
|------|------|
| status | draft / active / expired |
| isEmergency | 비상 공지 여부 (true이면 SSE 즉시 브로드캐스트) |
| startDate / endDate | 게시 기간 |
| bodyType | markdown / html |

### 3.5 DocRepo (문서 저장소)

Gitea 레포지토리와 1:1 매핑되며, 프로젝트별 문서 이력 관리를 담당한다.

#### 편집 세션 (EditSession)

| 필드 | 설명 |
|------|------|
| projectCode | 프로젝트 코드 |
| userId | 편집 사용자 |
| status | active / expired |
| lastHeartbeat | 마지막 하트비트 시각 |

- **TTL**: 기본 30분 (`PLATFORM_EDIT_SESSION_TTL`)
- **워커**: 백그라운드 TTL Worker가 주기적으로 만료된 workspace 정리
- **Worktree**: git worktree 기반 사용자별 독립 작업 공간

---

## 4. GoJIRA-API 설계

> Go, chi/v5 라우터, 포트 :9001

### 4.1 라우트 구조

```
/api/v{version}/
  Check                            # 서버 상태 확인
  Issue                            # 이슈 CRUD
  Issue/label                      # 라벨 추가/삭제
  Issue/link, Issue/link/{id}/of/{key}  # 이슈 링크 CRUD
  Issue/comment                    # 댓글 CRUD
  Issue/fetch                      # JIRA Pull 동기화
  Issue/docgen                     # 이슈 -> Markdown 문서 생성
  Issue/field                      # 필드 단건 수정
  Issue/relations                  # 관계 일괄 조회
  Jira/Account                     # JIRA 계정 등록
  Page/images                      # 페이지 이미지 관리
  Project                          # 프로젝트 CRUD
  Project/alerts                   # 프로젝트 알림 (브랜치 diff)
  Agent/*                          # Agent-Service 프록시 또는 자체 처리
```

### 4.2 Agent 프록시

`GOJIRA_AGENT_PROXY` 환경변수가 설정되면 `/Agent/*` 요청을 Agent-Service로 리버스 프록시한다.

```go
// 설정 예: GOJIRA_AGENT_PROXY=http://localhost:9002
proxy := httputil.NewSingleHostReverseProxy(target)
r.HandleFunc("/Agent/*", proxy.ServeHTTP)
```

미설정 시 GoJIRA-API 내장 핸들러가 직접 처리한다 (Phase 0 호환).

### 4.3 이슈 데이터 구조

이슈는 JIRA DC의 필드를 MongoDB 캐시 스키마로 변환하여 저장한다.

| MongoDB 필드 | 설명 |
|--------------|------|
| key | 이슈 키 (예: PROJ-123) |
| subkey | 타입별 가독성 키 (자동 부여): VOS→`VOS_001`, Feature→`UC_001`, Requirement(FR)→`FR_001`, Requirement(NFR)→`QA_001`, Requirement(CNST)→`CNST_001`, Architectural Design→`ADSN_001`, Detailed Design→`DDSN_001`. 같은 프로젝트 내 동일 prefix 중 최대 +1, 3자리 zero-pad |
| summary | 제목 |
| issueType | VOS / Feature / Requirement / Design / Test |
| reqType | Requirement 세부 타입: QA/NFR, FR, CNST |
| issueLinks | 관계 배열. **JIRA DC v2 표준 형식**: `{id:"10001", type:{name:"Contains(WBSGantt)"}, outwardIssue:{key}, inwardIssue:{key}}`. linkType enum: `contains`/`relates`/`duplicates`/`blocks`. 전역 숫자 link id 부여(10001부터). |
| _dirty | 동기화 대기 플래그 |
| _syncedAt | 마지막 동기화 시각 |
| _deletedLinks | JIRA 삭제 대기 링크 ID |

### 4.4 이슈 타입 계층

```
VOS (Value Objective Stream)
  └── Feature (Use Case)
       └── Requirement
            ├── QA/NFR (품질/비기능)
            ├── FR (기능)
            └── CNST (제약)
       └── Design
            ├── ADSN (아키텍처)
            └── DDSN (상세)
       └── Test
```

### 4.5 프로젝트 모델

| 필드 | 타입 | 설명 |
|------|------|------|
| name | string | 프로젝트명 |
| jiraCodes.main.jiraCode | string | JIRA 프로젝트 키 |
| documents | map | 문서 저장소 설정 (repoName 등) |
| issues | []string | 프로젝트 이슈 키 배열 |
| schedules | []map | 일정 정보 |
| branchAlerts | []string | 브랜치 diff가 있는 사용자 ID |
| diagrams | map | 다이어그램 데이터 |

---

## 5. Agent-Service 설계

> Python 3.9+, FastAPI, 포트 `:9002` — **OpenCode bridge 단일 모드**.
> 본 절은 Platform-API 관점의 요약. 상세는 [docs/design-agent.md](./design-agent.md) 가 권위 문서.

### 5.1 역할 — OpenCode 슬림 어댑터

이전엔 Agent-Service 가 자체 ReACT 2-pass 루프를 돌렸으나, 단일 LLM 벤더 종속 회피 + 모델 교체 가능성 + 사내 정책 대응을 위해 OpenCode 서버(`:4096`, 사용자 PC 로컬) 가 LLM 호출·ReACT·스킬 디스패치를 담당하도록 전환됨. Agent-Service 는 다음만 수행:

- 세션 매핑 — Mongo `agent_sessions` 의 `gojiraSessionId ↔ openCodeSessionId`
- 메시지 전달 — `POST /Agent/chat/send` → OpenCode `POST /session/{id}/prompt_async`
- SSE 이벤트 번역 — OpenCode `/event` SSE → UI 가 기대하는 포맷 (`session/thinking/reasoning/tool_call/tool_result/content/user_message/done/error`)
- 세션 영속화 — 매 턴 done 직후 OpenCode 의 message info+parts 를 Mongo 에 그대로 push (history 복원용)
- TUI 입력 반영 — OpenCode TUI 에서 사용자가 직접 입력한 메시지도 채팅창에 user 버블로 표시 (translator 의 `user_message` SSE 이벤트)

### 5.2 라우트 구조

```
/api/v2/Agent/
  POST   chat/send                  # 메시지 전달 + 백그라운드 subscriber 시작
  POST   chat/stream                # 레거시 — chat/send + events 조합
  GET    session/{id}/events        # SSE 스트림 (event_buffer 에서 읽기, lastEventId 재연결 지원)
  GET    session/{id}/state         # 폴링용 경량 상태 (finish/active/hasPermissionPending/lastActivityAt)
  GET    session/{id}               # Mongo 영속 메시지 조회 (loadSession)
  POST   session/{id}/cancel        # 진행 중 abort
  GET    sessions                   # 세션 목록 (최근 50개, title 추출)
  DELETE session/{id}, sessions     # 세션 삭제
  POST   session/{id}/terminal      # OpenCode TUI 열기 헬퍼
/health                             # 헬스체크
```

### 5.3 채팅 SSE 라이프사이클

```mermaid
sequenceDiagram
    participant UI as AgentChat (SPA)
    participant AS as Agent-Service
    participant Buf as event_buffer (메모리)
    participant Sub as _subscribe_loop
    participant OC as OpenCode 서버
    participant LLM as LLM 게이트웨이 (Platform-API)

    UI->>AS: POST /Agent/chat/send {message}
    AS->>Sub: ensure_subscriber(persistent)
    AS->>OC: /session/{id}/prompt_async
    OC-->>Sub: /event SSE 스트림 (메시지 단위)
    Sub->>Sub: translate → push(event_type, data)
    Sub->>Buf: append (in-memory, lastEventId 채번)

    UI->>AS: GET /session/{id}/events
    loop ES 유지 (turn 종료 후에도)
        Buf-->>UI: SSE replay since lastEventId
    end

    OC->>LLM: POST /api/v2/LLM/call (사내/외부 모델)
    LLM-->>OC: 응답
    OC-->>Sub: message.updated finish=stop
    Sub->>AS: _save_to_mongo (info + parts 그대로)
    Sub->>Buf: push("done", ...)
    Buf-->>UI: event: done

    Note over UI,OC: TUI 에서 사용자가 직접 입력해도 동일 subscriber 가 user_message 이벤트 yield → UI 는 user 버블로 표시
```

### 5.4 핵심 컴포넌트

| 컴포넌트 | 파일 | 책임 |
|---|---|---|
| `event_buffer._subscribe_loop` | `opencode_bridge/event_buffer.py` | OpenCode `/event` SSE 를 persistent 하게 구독 — turn 종료 후에도 다음 입력 대기. done 마다 `_save_to_mongo` |
| `translator.translate` | `opencode_bridge/translator.py` | OpenCode 이벤트(`message.updated` / `message.part.updated` / `message.part.delta`) 를 UI 포맷으로 변환. messageID 는 `part.messageID` 에 있음. 미등록 assistant 의 terminal-finish 는 done 으로 수용하는 안전망 포함 |
| `event_buffer._save_to_mongo` | 동 | OpenCode message info + parts 를 Mongo `agent_sessions.messages` 로 push. user/assistant 모두 저장, ocMessageId / content 기반 dedup. 단, done 이벤트는 save 이후 buf push → 프론트 loadSession 이 stale Mongo 를 읽는 race 차단 |
| `router.sse_stream` | `opencode_bridge/router.py` | event_buffer 를 polling 해서 lastEventId 이후 이벤트 yield. done 으로 stream 종료하지 않음 — turn 사이 TUI 이벤트도 같은 ES 로 전달 |
| `router.get_session_state` | 동 | 경량 상태 조회 — finish/active/hasPermissionPending/lastActivityAt/assistantMsgId. UI 가 SSE 침묵 시 폴백으로 사용 |

### 5.5 스킬 디스패치

스킬은 `.claude/skills/<name>/SKILL.md` 단일 파일에 bash/curl 인라인. OpenCode 가 기동 시 자동 scan 해 SKILL description 매칭으로 디스패치. 자세한 워크플로우 정책은 [docs/design-workflow.md](./design-workflow.md), 스킬 인프라(drift-check, 결정적 조립, plantuml-fix 등)는 [docs/skill-infrastructure-v3.md](./skill-infrastructure-v3.md) 참조.

### 5.6 세션 영속화

- Mongo `agent_sessions` 컬렉션 — `{_id: gojiraSessionId, openCodeSessionId, userId, messages, createdAt, updatedAt}`
- messages 구조: `[{role:"user", content, timestamp, ocMessageId?}, {role:"assistant", ocMessageId, info, parts, timestamp}, ...]`
- 채팅창 발송분(user) 과 OC echo 의 dedup 은 content 기반 (`pending_user_contents` 카운트)
- loadSession 은 user 발송분과 assistant 영속분을 그대로 그려주므로 UI 측 history 와 1:1 매칭

---

## 6. 데이터 저장소 설계

> **하이브리드 구조** — 이슈는 Git(YAML), 나머지는 MongoDB 7.0 (`gojira`).

### 6.0 저장소 역할 분담 (2026-04-15 개정)

| 데이터 | 원천 | 사유 |
|--------|------|------|
| **이슈** (VOS / Feature / Requirement / ADSN / DDSN) | **Git YAML** (각 과제 docs repo `data/issues/<subkey>.yaml`) | 이력/diff/PR 리뷰 가치, 과제당 100~3000건 규모로 git 처리 충분 |
| 프로젝트 메타 (archSettings, chosenStyles, satamDone 등 워크플로우 플래그) | MongoDB `projects` | 워크플로우 휘발성 플래그. 이력 가치 낮음 |
| 사용자/권한/조직/공지 | MongoDB | 트랜잭션/TTL 필요 |
| 에이전트 세션/잡/로그 | MongoDB | 고빈도 쓰기, 이력 가치 낮음, TTL 적용 |
| 문서 (business/*.md, architecture/*.md) | Git | 기존부터 git 관리 |

Mongo `issues` 컬렉션은 **git → Mongo 단방향 동기화된 파생 캐시**. 손상/유실 시 `git pull` 후 전체 리빌드. 쓰기는 반드시 git 을 경유한 뒤 sync 훅이 캐시 갱신.

#### 쓰기 흐름
```mermaid
sequenceDiagram
    participant S as Skill/UI
    participant API as Platform-API (or Agent-Service)
    participant Git as docs repo
    participant DB as MongoDB (issues cache)

    S->>API: 이슈 수정 요청
    API->>Git: data/issues/<subkey>.yaml 저장 (user 브랜치 커밋)
    API->>DB: 동일 내용 upsert (캐시 갱신)
    API-->>S: 성공
```

#### 읽기 흐름
- 조회/카운트 쿼리는 **MongoDB 캐시**에서 처리 (기존 인터페이스 보존, 성능 유지).
- 단일 이슈 상세 읽기는 캐시 우선, 주기적으로 git 과 해시 비교해 drift 감지.

#### 머지/동기화
- 사용자 브랜치 → main 머지 시: 변경된 `data/issues/*.yaml` 재파싱해 Mongo 캐시 갱신.
- 머지 충돌:
  - **이슈 파일 단위 격리** (파일명=`subkey`) — 같은 이슈 동시 수정만 충돌.
  - 기본 `ours` + UI 충돌 배너로 사용자 재작업 유도.

#### 스키마
- `docs/issue-yaml-schema.md` 참조. 공통 필드 + issueType별 확장.
- pydantic 기반 검증, 미지 필드 허용 (forward compat).

### 6.1 컬렉션 ERD

```mermaid
erDiagram
    users ||--o{ userProjects : "참여"
    users ||--o{ userConnections : "접속"
    users }o--o{ authGroups : "소속 (authGroupIds)"
    users }o--o{ orgUnits : "소속 (orgUnitIds)"
    orgUnits ||--o| orgUnits : "parent"

    projects ||--o{ issues : "포함 (issues[])"
    projects ||--|| docRepos : "1:1 (documents)"
    issues ||--o{ comments : "댓글"
    issues ||--o{ issues : "issueLinks"

    editSessions }o--|| projects : "편집 세션"
    editSessions }o--|| users : "편집자"

    agent_sessions }o--|| users : "채팅 세션"
    agent_jobs }o--|| agent_sessions : "작업"

    notices ||--o{ notifications : "알림 생성"
    notifications }o--|| users : "수신자"

    users {
        string knoxId
        string name
        string email
        array authGroupIds
        array orgUnitIds
        bool isActive
    }

    authGroups {
        string code
        string name
        map authority
        bool isActive
    }

    orgUnits {
        string code
        string name
        string type
        string parentOrgUnitId
        int sortOrder
    }

    projects {
        string name
        object jiraCodes
        map documents
        array issues
        array schedules
        array branchAlerts
    }

    issues {
        string key UK
        string summary
        string issueType
        string reqType
        array issueLinks
        bool _dirty
        date _syncedAt
    }

    comments {
        string issueKey
        string commentId
        string body
    }

    notices {
        string title
        string body
        string status
        bool isEmergency
        date startDate
        date endDate
    }

    notifications {
        string userId
        string type
        string message
        bool read
        date createdAt TTL_7d
    }

    editSessions {
        string projectCode
        string userId
        string status
        date lastHeartbeat
    }

    agent_sessions {
        string userId
        array messages
        object context
    }

    agent_jobs {
        string sessionId
        string status
        array plan
    }

    userProjects {
        string userId
        string projectId
        string role
    }

    userConnections {
        string userId
        string connectionId
        bool isOnline
    }
```

### 6.2 컬렉션 목록 (16개)

| 컬렉션 | 서비스 | 인덱스 |
|---------|--------|--------|
| users | Platform-API | knoxId (unique) |
| authGroups | Platform-API | code (unique) |
| orgUnits | Platform-API | code (unique) |
| userProjects | Platform-API | userId + projectId |
| userConnections | Platform-API | userId + connectionId |
| notices | Platform-API | status, startDate |
| notifications | Platform-API | userId + read, createdAt (TTL 7일) |
| editSessions | Platform-API | projectCode + userId, lastHeartbeat |
| docLinks | Platform-API | taskId, docRepoId |
| issues | GoJIRA-API (캐시) | key (unique) — git YAML 파생 캐시, 직접 쓰기 금지 (sync 훅 경유) |
| projects | GoJIRA-API | jiraCodes.main.jiraCode |
| comments | GoJIRA-API | issueKey |
| accounts | GoJIRA-API | jiraID |
| agent_sessions | Agent-Service | userId, updatedAt |
| agent_jobs | Agent-Service | sessionId |
| agent_logs | Agent-Service | jobId, timestamp |
| branchAlerts | Platform-API | projectCode |

### 6.3 공유 데이터베이스 전략

세 서비스 모두 동일한 `gojira` 데이터베이스에 접속한다.  
각 서비스는 자신의 컬렉션만 쓰기하고, 다른 서비스의 컬렉션은 읽기만 수행한다.

| 서비스 | 쓰기 컬렉션 | 읽기 전용 |
|--------|-------------|-----------|
| Platform-API | users, authGroups, orgUnits, notices, notifications, editSessions, docLinks, userProjects, userConnections | - |
| GoJIRA-API | projects, comments, accounts (issues 는 git→캐시 sync 훅만 쓰기) | users (권한 확인) |
| Agent-Service | agent_sessions, agent_jobs, agent_logs (이슈 원천 쓰기는 git repo) | projects, issues(캐시) |

---

## 7. 외부 연동

### 7.1 JIRA Datacenter 동기화

```mermaid
sequenceDiagram
    participant UI as GoJIRA UI
    participant API as GoJIRA-API
    participant DB as MongoDB
    participant JIRA as JIRA DC

    Note over UI,JIRA: Pull 동기화 (전체 덮어쓰기)
    UI->>API: GET /Issue/fetch
    API->>JIRA: GET /rest/api/2/search (JQL)
    JIRA-->>API: 이슈 목록 (JSON)
    API->>API: TransformToCache (필드 매핑)
    API->>DB: UpsertIssue (key 기준)
    API-->>UI: {count: N}

    Note over UI,JIRA: Push 동기화 (dirty 플래그)
    UI->>API: PUT /Issue (수정)
    API->>DB: UpdateIssueFields + _dirty=true
    API-->>UI: 200 OK
    API->>API: SyncIssueToJira (goroutine)
    API->>API: CacheToJiraFields (역매핑)
    API->>JIRA: PUT /rest/api/2/issue/{key}
    JIRA-->>API: 200 OK
    API->>DB: _dirty=false, _syncedAt=now
```

#### 동기화 규칙

| 방향 | 트리거 | 전략 |
|------|--------|------|
| **Pull** (JIRA -> MongoDB) | `/Issue/fetch` 호출 | 전체 덮어쓰기 (JQL 필터 가능) |
| **Push** (MongoDB -> JIRA) | 이슈 수정 시 | 비동기 goroutine, `_dirty` 플래그 |

#### 커스텀 필드 매핑

`customFieldMap`으로 약 30개 JIRA 커스텀 필드를 MongoDB 필드명으로 변환한다.  
`TransformToCache()`/`CacheToJiraFields()`가 양방향 변환을 수행한다.

#### 링크 동기화

- 새 링크: `tmp_` 접두사로 MongoDB에 저장 -> Push 시 JIRA에 생성 -> 실제 ID로 교체
- 삭제 링크: `_deletedLinks` 배열에 기록 -> Push 시 JIRA에서 삭제

### 7.2 Gitea 연동

```mermaid
sequenceDiagram
    participant User as 사용자
    participant PAPI as Platform-API
    participant Gitea as Gitea Server
    participant WT as Worktree (로컬)

    Note over User,WT: 문서 편집 -> PR -> Merge 흐름

    User->>PAPI: POST /edit/start
    PAPI->>PAPI: EditSession 생성 (active)
    PAPI->>WT: git worktree add (docs/{userId})
    PAPI-->>User: 세션 정보

    loop 편집 중
        User->>PAPI: PUT /edit/file (저장)
        PAPI->>WT: 파일 쓰기 (worktree)
        User->>PAPI: POST /edit/heartbeat
        PAPI->>PAPI: lastHeartbeat 갱신
    end

    User->>PAPI: POST /edit/commit
    PAPI->>WT: git add + commit (worktree)
    PAPI->>Gitea: git push (docs/{userId} 브랜치)

    User->>PAPI: POST /edit/pr
    PAPI->>Gitea: POST /repos/{owner}/{repo}/pulls
    Gitea-->>PAPI: PR 생성 완료 (PR #{num})
    PAPI->>Gitea: POST /pulls/{num}/requested_reviewers
    PAPI-->>User: PR 정보

    Note over User,WT: 리뷰어 승인 후

    User->>PAPI: POST /pr/{num}/merge
    PAPI->>Gitea: POST /pulls/{num}/merge (force_merge)
    Gitea-->>PAPI: 머지 완료
    PAPI->>Gitea: DELETE /branches/docs/{userId}
    PAPI->>WT: git worktree remove
    PAPI->>PAPI: EditSession expired
```

#### 브랜치 전략

| 브랜치 | 용도 |
|--------|------|
| `main` | 승인된 문서 (보호됨) |
| `docs/{userId}` | 사용자별 작업 브랜치 |

#### 브랜치 보호

- main 브랜치에 보호 규칙 설정 (직접 push 불가)
- PR 필수, 지정 리뷰어 전원 승인 필요
- Gitea/GitHub 양쪽 API 지원 (`setBranchProtectionGitea`/`setBranchProtectionGitHub`)

#### Worktree TTL

- 기본 30분 미활동 시 자동 삭제
- 활성 세션 또는 미커밋 작업이 있으면 삭제하지 않음
- `StartTTLWorker()`가 주기적으로 (TTL/2 간격) 만료 workspace 정리

### 7.3 LLM 연동

#### 7.3.1 단일 게이트웨이

LLM 호출은 **Platform-API 가 단독으로 관리**한다. 키·모델·엔드포인트 정책의 회전·교체가 한 곳에서만 일어나도록 게이트웨이 패턴을 채택했다.

```
[GoJIRA-API]                     [Platform-API]                       [OpenAI / 사내 프록시]
 callPlatform("/LLM/call")  ───►  POST /api/v2/LLM/call  ───►  llm.Call(config.LLM, msgs, opts)
                                  - llm.json 기반                     ─► chat/completions
                                  - OPENAI_API_KEY 보유
```

| 항목 | 위치 |
|------|------|
| 라이브러리 코드 | `Platform-API/llm/{config.go, client.go}` |
| 게이트웨이 endpoint | `POST /api/v2/LLM/call` (`Platform-API/handlers/llm.go`) |
| 설정 파일 | `Platform-API/config/llm.json` |
| OPENAI_API_KEY | Platform-API 의 `.env` 만 (ENC: 형태) |
| 호출 사이트 | Platform 내부: `translateRepoName` (in-process) <br/> 외부: GoJIRA-API `summarizePRContent` (HTTP) |

#### 7.3.2 llm.json 스키마 + 탐색 순서

| 필드 | 설명 |
|------|------|
| endpoint | OpenAI 호환 API (사내 프록시 등 교체 가능) |
| model | 기본 `gpt-4o` |
| authType | `bearer` / `header` / `none` |
| maxTokens, temperature | 기본 호출 파라미터 (호출별 Options 로 override 가능) |
| responseFormat.choicesPath/contentPath | 응답 파싱 경로 |

탐색 순서 (`Platform-API/config/llm.go LoadLLMConfig`):
1. `PLATFORM_LLM_CONFIG` 환경변수 경로
2. 소스 파일 디렉토리 기준 `Platform-API/config/llm.json` (개발 환경)
3. cwd 기준 `config/llm.json`, `llm.json`

#### 7.3.3 fail-fast 정책

LLM 호출이 실패하면 **폴백 없이 작업 자체를 실패 처리** 한다. 폴백 동작이 silent 한 잘못된 결과를 만드는 것을 방지하기 위함.

| 호출 사이트 | 실패 시 동작 |
|---|---|
| `translateRepoName` (Platform 내부) | `(string, error)` 반환 → DocRepoCreateHandler 가 502 |
| `summarizePRContent` (GoJIRA-API) | `(title, body, err)` 반환 → docgen 핸들러가 502 |
| `ensureDocRepo` (GoJIRA-API) | `error` 반환 → Project 핸들러가 502 |

원인 체인이 응답 본문에 그대로 포함되어 호출자가 진단 가능:
```
"DocRepo 생성 실패: repo 이름 LLM 번역 실패: LLM 호출 실패: API 오류 (401): ..."
```

#### 7.3.4 프로젝트 생성 흐름 — repo 성공 후에만 doc 생성

```
[Browser POST /Project] ──► [GoJIRA-API ProjectCreateHandler]
                                │
                                │ ① POST /DocRepo/{code} {projectName}
                                ▼
                            [Platform-API DocRepoCreateHandler]
                                │ ② translateRepoName (LLM)  ─ 실패 → 502 종료
                                │ ③ github.CreateRepo (Gitea) ─ 실패 → 502 종료
                                │ ④ 초기 파일 + branch protection
                                │ ⑤ response { repoName, repoOwner, repoUrl }
                                ▼
                            [GoJIRA-API]
                                │ ⑥ documents 필드 채워서 UpsertProject (Mongo)
                                ▼
                            [Browser 200 OK]
```

이전엔 doc 가 먼저 만들어진 뒤 repo 생성이 실패하면 "doc 만 있고 repo 없는" 절반 상태가 남았다. 새 흐름은 repo 가 모두 성공한 다음에야 doc 을 생성하므로 절반 상태가 발생하지 않는다.

---

## 8. 실시간 통신 설계

### 8.1 SSE + WebSocket 이중화

```mermaid
graph LR
    subgraph Platform-API
        BC["broadcast()"]
        SSE["SSE 핸들러<br/>/Notice/stream"]
        WS["WebSocket 핸들러<br/>/Notice/ws"]
        CH["sseClients<br/>map[chan]bool"]
    end

    subgraph 이벤트_소스
        WH["Webhook 핸들러"]
        NH["Notice 핸들러"]
        DH["DocRepo 핸들러"]
    end

    subgraph 클라이언트
        C1["브라우저 1"]
        C2["브라우저 2"]
        C3["브라우저 3"]
    end

    WH --> BC
    NH --> BC
    DH --> BC
    BC --> CH
    CH --> SSE --> C1
    CH --> WS --> C2
    CH --> WS --> C3
```

### 8.2 이벤트 채널

| 이벤트 | 소스 | 데이터 |
|--------|------|--------|
| `connected` | 연결 시 | "ok" |
| `emergency` | 비상 공지 생성 | 공지 전체 데이터 |
| `doc_update` | DocRepo 변경 | type, repo, projectCode |
| `pr_update` | Webhook (PR) | action, prNumber, state, merged, reviewers |
| `pr_review` | Webhook (Review) | reviewer, state, body |
| `project_alert` | Webhook (Push) | branchAlerts (diff가 있는 사용자 목록) |

### 8.3 SSE와 WebSocket 통합 설계

SSE와 WebSocket이 동일한 `sseClients` 채널 맵을 공유한다.  
WebSocket 핸들러도 SSE 채널을 등록하여 `broadcast()`로 통합 전송한다.

```go
// WebSocket도 동일한 sseClients 맵에 등록
ch := make(chan sseMessage, 10)
sseMu.Lock()
sseClients[ch] = true  // SSE/WS 구분 없이 동일한 맵
sseMu.Unlock()
```

### 8.4 Agent SSE 스트리밍

Agent-Service의 `/chat/stream`은 별도의 SSE 스트림이다.

| 이벤트 | 설명 |
|--------|------|
| `session` | 세션 ID 전달 |
| `thinking` | LLM 호출 중 (턴 번호) |
| `reasoning` | Pass 1 사고 과정 (디버그 뷰) |
| `tool_call` | 도구 호출 시작 (도구명, 파라미터) |
| `tool_result` | 도구 실행 결과 (요약, 소요시간) |
| `content` | 최종 답변 텍스트 (청크 단위) |
| `done` | 완료 (전체 응답 + actions) |
| `error` | 에러 발생 |

---

## 9. 보안 설계

### 9.1 인증

| 항목 | 구현 |
|------|------|
| SSO | ADFS 쿠키 (`ADSSO_USER`) 기반 |
| 개발 모드 | `USE_SSO=false` -> `DEV_USER` 환경변수 |
| 사용자 식별 | `X-User-Id` 헤더 (Agent-Service) |
| Gitea 인증 | Bearer 토큰 (관리자), 사용자별 토큰 생성 가능 |
| JIRA 인증 | Basic Auth (ID/PW 암호화 저장) |

### 9.2 권한 (Authorization)

권한은 **조회 시점에 계산**한다 (캐시하지 않음).

```
사용자 최종 권한 = OR(소속 그룹1.authority, 소속 그룹2.authority, ...)
```

비트마스크 OR 연산으로 여러 그룹의 권한을 합산한다.  
그룹 변경 시 즉시 반영되며, 캐시 불일치 문제가 없다.

### 9.3 암호화

#### 9.3.1 보호 대상

| 대상 | 방식 |
|------|------|
| JIRA 계정 (ID/PW) | AES-256-GCM, ENC: prefix 형태 |
| SMTP 비밀번호 | AES-256-GCM, ENC: prefix 형태 |
| OpenAI API 키 (`OPENAI_API_KEY`) | AES-256-GCM, ENC: prefix 형태 |
| Webhook 시그니처 | HMAC-SHA256 검증 |

`.env` 파일은 모두 `.gitignore` 처리되어 있고, 위 시크릿들은 `ENC:base64` 형태로 저장된다. 평문이 들어 있어도 그대로 통과시켜 점진 도입을 가능하게 한다 (역호환).

#### 9.3.2 암호화 단일 소스

`gojira-shared/secret/crypto.go` 가 모든 서비스의 단일 암복호화 구현이다. Go workspace (`go.work`) 로 구성되며, 각 서비스의 `config/crypto.go` 는 9 라인 thin wrapper 로 호출만 위임한다.

```
GoJIRA/
├── go.work                        # use 3 modules + replace 명시
├── gojira-shared/
│   ├── go.mod                     # module gojira.local/shared
│   └── secret/crypto.go           # 단일 Encrypt/Decrypt 구현
├── GoJIRA-API/config/crypto.go    # → gojira-shared/secret 위임
└── Platform-API/config/crypto.go  # → gojira-shared/secret 위임
```

- 호출자는 기존처럼 `config.Encrypt` / `config.Decrypt` 사용 — 인터페이스 무변경
- 보안 코드 drift 위험 제거 (이전엔 byte-identical 복제본 두 개)

#### 9.3.3 마스터 키 (`GOJIRA_ENC_KEY`)

`GOJIRA_ENC_KEY` 환경변수가 마스터 키. 미설정 시 코드 내 default 사용 (`gojira-shared/secret/crypto.go`).

- 매 호출 시 `os.Getenv` 로 읽어 캐시하지 않음 → `.env` 로딩 후에도 정상 반영
- 머신/환경별로 별도 키를 권장 (default 키는 코드에 노출되어 있어 약함)
- `GoJIRA-API/cmd/encrypt/main.go` 가 평문을 ENC 형식으로 변환하는 일회용 유틸 (재사용 가능)

#### 9.3.4 init 흐름

```go
// 양 서비스 config.go init() 패턴
loadEnvFile(".env")
raw := os.Getenv("OPENAI_API_KEY")
if val, err := Decrypt(raw); err == nil {
    os.Setenv("OPENAI_API_KEY", val) // 직접 os.Getenv 호출 핸들러 보호
}
```

`os.Setenv` 로 평문을 다시 환경에 주입해 `translateRepoName` 등 직접 `os.Getenv` 를 호출하는 핸들러도 ENC 도입 영향 없이 동작한다.

### 9.4 Soft Delete

모든 주요 엔티티에 `isActive` 필드를 사용한다.  
삭제 시 `isActive=false`로 설정하고, 조회 시 `isActive=true` 필터를 적용한다.  
참조 무결성과 이력 추적을 위해 물리적 삭제는 수행하지 않는다.

---

## 10. 설계 결정 기록 (ADR)

### ADR-001: 비트마스크 권한 체계

**상태:** 확정  
**맥락:** 리소스별 CRUD 권한을 효율적으로 저장하고 계산해야 한다.  
**결정:** 4비트 비트마스크 (C=8, R=4, U=2, D=1)로 권한을 표현한다.  
**사유:**
- 단일 정수로 4개 권한을 표현 (저장 효율)
- 비트 OR 연산으로 복수 그룹 권한 합산 가능
- 그룹 변경 시 사용자 레코드 수정 불필요 (자동 반영)

### ADR-002: 조회 시 권한 계산

**상태:** 확정  
**맥락:** 권한 정보를 캐시할지 실시간 계산할지 결정해야 한다.  
**결정:** API 호출 시마다 사용자의 그룹을 조회하고 비트마스크를 OR 합산한다.  
**사유:**
- 캐시 무효화 로직 불필요 (캐시 불일치 원천 방지)
- 그룹 변경이 즉시 모든 사용자에게 반영
- MongoDB 인덱스로 조회 성능 충분 (사용자 수 100명 이하)

### ADR-003: Soft Delete

**상태:** 확정  
**맥락:** 데이터 삭제 시 참조 무결성과 이력 추적이 필요하다.  
**결정:** `isActive` 필드로 논리적 삭제, 물리적 삭제 금지.  
**사유:**
- 삭제된 사용자/그룹을 참조하는 기존 데이터의 무결성 유지
- 감사 이력 추적 가능
- 실수로 삭제한 데이터 복구 가능

### ADR-004: SSE에서 WebSocket 전환 (이중 지원)

**상태:** 확정  
**맥락:** HTTP/1.1 환경에서 SSE 연결이 브라우저당 6개 제한에 걸린다.  
**결정:** WebSocket을 추가하고, SSE와 동일한 브로드캐스트 채널을 공유한다.  
**사유:**
- SSE는 HTTP/1.1에서 6개 연결 제한 (다른 API 호출 차단)
- WebSocket은 단일 연결로 양방향 통신 가능
- 기존 SSE 클라이언트와의 하위 호환성 유지

### ADR-005: API Gateway 미사용

**상태:** 확정  
**맥락:** 3개 서비스 앞에 API Gateway를 둘지 결정해야 한다.  
**결정:** API Gateway 없이 서비스가 직접 포트를 노출한다.  
**사유:**
- 서비스 2개 (Platform-API, GoJIRA-API)로 Gateway 복잡도 대비 이점 미미
- SPOF(Single Point of Failure) 방지
- Agent-Service는 GoJIRA-API가 리버스 프록시로 중계
- CORS 허용으로 프론트엔드에서 직접 호출 가능

### ADR-006: Gitea 자체 호스팅

**상태:** 확정  
**맥락:** 문서 Git 저장소로 GitHub, GitLab, Gitea 중 선택해야 한다.  
**결정:** Gitea를 Docker로 자체 호스팅한다.  
**사유:**
- 데이터 주권 확보 (사내 데이터 외부 유출 방지)
- GitHub API 호환 (코드 변경 최소화)
- 경량 (SQLite 기반, 단일 컨테이너)
- 라이선스 비용 없음

### ADR-007: project.documents 통합 저장

**상태:** 확정  
**맥락:** 프로젝트와 문서 저장소의 관계를 별도 컬렉션으로 분리할지 결정해야 한다.  
**결정:** projects 컬렉션 내 `documents` 필드에 통합 저장한다.  
**사유:**
- 프로젝트:문서저장소 = 1:1 관계 (분리 불필요)
- 단일 source of truth (join 없이 한 번에 조회)
- DocRepo 컬렉션은 매핑 메타데이터만 별도 관리

### ADR-008: git worktree 기반 편집

**상태:** 확정  
**맥락:** 사용자별 독립 작업 공간을 어떻게 구현할지 결정해야 한다.  
**결정:** `git worktree`로 사용자별 독립 작업 디렉토리를 생성한다.  
**사유:**
- 사용자 간 편집 충돌 원천 방지
- 각 worktree가 독립 브랜치 (`docs/{userId}`)
- git clone보다 디스크/네트워크 효율적 (object store 공유)
- TTL 워커로 유휴 workspace 자동 정리

### ADR-009: eventQueue FIFO

**상태:** 확정  
**맥락:** WebSocket 메시지 전송 시 경쟁 조건이 발생할 수 있다.  
**결정:** 채널 기반 FIFO 큐로 메시지 순서를 보장한다.  
**사유:**
- Go 채널이 자연스러운 FIFO 큐 역할
- 버퍼 크기 10으로 일시적 부하 흡수
- 버퍼 초과 시 drop (select default)하여 slow consumer 차단 방지

### ADR-010: Agent-Service Python 분리

**상태:** 확정  
**맥락:** AI 에이전트를 Go 서비스에 내장할지 별도 서비스로 분리할지 결정해야 한다.  
**결정:** Python (FastAPI) 별도 서비스로 분리한다.  
**사유:**
- Python LLM 생태계 활용 (OpenAI SDK, LangChain, Jinja2 등)
- Go에서 Python 라이브러리 호출 불가
- 독립 배포/스케일링 가능
- GoJIRA-API는 리버스 프록시로 투명하게 중계

### ADR-011: CLI에서 SKILL.md 전환

**상태:** 확정  
**맥락:** 스킬 정의 방식을 코드 기반에서 선언형으로 변경할지 결정해야 한다.  
**결정:** `SKILL.md` 마크다운 파일로 선언적 정의, Go/Python 양쪽에서 파싱한다.  
**사유:**
- 선언형 정의로 비개발자도 스킬 추가/수정 가능
- Claude Code 호환 (마크다운 기반 도구 정의)
- Go `AutoLoadSkills()`와 Python `load_skills_from_dir()` 공유

### ADR-012: CRUD 개별 스킬 분리

**상태:** 확정  
**맥락:** 이슈 CRUD를 하나의 스킬로 통합할지 개별 스킬로 분리할지 결정해야 한다.  
**결정:** issue_create, issue_update, issue_delete 등 개별 스킬로 분리한다.  
**사유:**
- LLM이 구체적 도구를 더 정확하게 선택 (function calling 정확도 향상)
- 파라미터 스키마가 명확해져 hallucination 감소
- 각 스킬의 description이 독립적이어 LLM 판단 용이

### ADR-013: gpt-4o 모델 선택

**상태:** 확정  
**맥락:** gpt-4o와 gpt-4o-mini 중 기본 모델을 선택해야 한다.  
**결정:** gpt-4o를 기본 모델로 사용한다 (llm.json으로 교체 가능).  
**사유:**
- gpt-4o-mini의 tool_use(function calling) 정확도가 현저히 낮음
- 복잡한 파라미터 매핑 시 오류율 높음
- 비용 차이보다 정확도가 서비스 품질에 더 큰 영향

### ADR-014: 독립 채팅 창

**상태:** 확정  
**맥락:** Agent 채팅 UI를 메인 앱 오버레이로 할지 별도 창으로 할지 결정해야 한다.  
**결정:** `window.open()`으로 독립 창, `BroadcastChannel`로 통신한다.  
**사유:**
- 오버레이가 메인 UI 작업을 방해 (클릭 가림, 스크롤 차단)
- 독립 창은 듀얼 모니터 활용 가능
- BroadcastChannel로 탭 간 navigate 명령 전달 (SPA 라우팅)

### ADR-015: SSE 스트리밍 + 2-pass ReACT

**상태:** 확정  
**맥락:** Agent 응답을 동기로 기다릴지 스트리밍할지 결정해야 한다.  
**결정:** SSE로 스트리밍하고, 2-pass ReACT(추론/도구 분리)를 적용한다.  
**사유:**
- 실시간 UX: 사용자가 사고 과정(reasoning)을 즉시 확인
- 도구 호출 진행 상황 실시간 표시 (tool_call/tool_result)
- Pass 1(추론)과 Pass 2(도구 호출) 분리로 디버그 뷰 구현 가능
- `tool_choice=none/auto` 제어로 추론과 실행을 명확히 분리

---

## 부록: 환경 변수

### Platform-API

| 변수 | 기본값 | 설명 |
|------|--------|------|
| PLATFORM_SERVER_PORT | :9000 | 서버 포트 |
| MONGO_URI | mongodb://localhost:27017 | MongoDB URI |
| PLATFORM_MONGO_DB | gojira | DB 이름 |
| PLATFORM_GITHUB_TOKEN | - | Gitea 관리자 토큰 |
| PLATFORM_GITHUB_ORG | - | Gitea 조직명 |
| GITHUB_URL | https://github.com/ | Git 플랫폼 URL |
| PLATFORM_REPO_DIR | ./repos | 로컬 repo 디렉토리 |
| PLATFORM_WEBHOOK_URL | - | Webhook 수신 URL |
| PLATFORM_WEBHOOK_SECRET | gojira-webhook-secret | Webhook 시크릿 |
| PLATFORM_EDIT_SESSION_TTL | 30 | 편집 세션 TTL (분) |
| SMTP_HOST/PORT/USER/PASS/FROM | - | 이메일 설정 (PASS 는 암호화) |
| OPENAI_API_KEY | - | LLM API 키 (암호화, **단일 보유** — §7.3) |
| PLATFORM_LLM_CONFIG | - | llm.json 경로 override (미설정 시 `Platform-API/config/llm.json`) |
| GOJIRA_ENC_KEY | (코드 내 default) | 시크릿 마스터 키 — 양 서비스 공통 (§9.3.3) |

### GoJIRA-API

| 변수 | 기본값 | 설명 |
|------|--------|------|
| GOJIRA_SERVER_PORT | :9001 | 서버 포트 |
| GOJIRA_PLATFORM_API | http://localhost:9000/api/v2 | Platform-API URL |
| GOJIRA_JIRA_URL | http://localhost:8000 | JIRA URL |
| GOJIRA_JIRA_API_BASE | /rest/api/2 | JIRA API 경로 |
| GOJIRA_JIRA_USER | - | JIRA 계정 (암호화) |
| GOJIRA_JIRA_PASS | - | JIRA 비밀번호 (암호화) |
| MONGO_URI | mongodb://localhost:27017 | MongoDB URI |
| GOJIRA_MONGO_DB | gojira | DB 이름 |
| GOJIRA_ENC_KEY | (코드 내 default) | 시크릿 마스터 키 — Platform-API 와 공통 (§9.3.3) |
| GOJIRA_AGENT_PROXY | - | Agent-Service 프록시 URL |

> GoJIRA-API 는 LLM 키를 보유하지 않는다. PR 요약 등 LLM 작업은 Platform-API `/LLM/call` 게이트웨이로 위임 (§7.3).

### Agent-Service

| 변수 | 기본값 | 설명 |
|------|--------|------|
| AGENT_SERVICE_PORT | 9002 | 서버 포트 |
| MONGO_URI | mongodb://localhost:27017 | MongoDB URI |
| GOJIRA_MONGO_DB | gojira | DB 이름 |
| GOJIRA_API_URL | http://localhost:9001/api/v2 | GoJIRA-API URL |
| GOJIRA_PLATFORM_API | http://localhost:9000/api/v2 | Platform-API URL |
| OPENAI_API_KEY | - | LLM API 키 |
| GOJIRA_LLM_CONFIG | - | llm.json 경로 |
| GOJIRA_SKILLS_DIR | - | 스킬 디렉토리 경로 |
