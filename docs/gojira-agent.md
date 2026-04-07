# GoJIRA Agent 기획서

## 1. 개요

GoJIRA Agent는 기존 GoJIRA 플랫폼의 기능을 AI 기반으로 고도화하여, 사용자가 자연어 대화를 통해 프로젝트 관리, 이슈 관리, 설계문서 작성 등의 작업을 수행할 수 있도록 하는 시스템이다.

### 1.1 핵심 목표
- 자연어 기반 프로젝트 관리 자동화
- 각 메뉴의 생성/수정 기능에 Agent Assist 연동
- 이슈와 문서의 통합 관리 (생성 결과가 이슈로도 추적)
- Skill/MCP 기반 확장 가능한 아키텍처

### 1.2 주요 진입점
| 진입점 | 설명 |
|--------|------|
| 타이틀바 GoJIRA 클릭 | 전체 기능 대화형 채팅 인터페이스 |
| 각 메뉴 Agent Assist 버튼 | 해당 컨텍스트 내 자동 생성/수정 |

---

## 2. 아키텍처

### 2.1 시스템 구성도

```
[ Browser ]
    |
    |-- Chat UI (GoJIRA 타이틀 클릭)
    |-- Agent Assist (각 메뉴 내 버튼)
    |
    v
[ GoJIRA-API ]
    |-- POST /api/v2/Agent/chat       (대화형)
    |-- POST /api/v2/Agent/assist     (컨텍스트 기반 보조)
    |
    |-- Agent Router (의도 분석 → Skill 선택)
    |       |
    |       |-- Skill: issue-create
    |       |-- Skill: issue-update
    |       |-- Skill: doc-generate
    |       |-- Skill: doc-edit
    |       |-- Skill: project-analyze
    |       |-- Skill: relation-analyze
    |       |-- Skill: satam-analyze
    |       |-- Skill: pr-create
    |       |-- Skill: diff-summarize
    |       |-- ... (확장 가능)
    |       |
    |       v
    |   [ LLM Provider ] (llm.json 설정 기반)
    |
    |-- 기존 API 호출 (Issue, DocRepo, Project 등)
    |
    v
[ Platform-API ]
    |-- DocRepo (파일 수정, PR, 머지)
    |-- SSE (실시간 알림)
    |-- Webhook (Gitea 이벤트)
```

### 2.2 Agent 처리 흐름

```
사용자 메시지
  → Agent Router (LLM 의도 분석)
    → Skill 선택 + 파라미터 추출
      → Skill 실행 (API 호출 조합)
        → 결과 반환 + 이슈 자동 기록
          → SSE로 UI 갱신
```

### 2.3 MCP (Model Context Protocol) 연동

Agent가 외부 도구를 호출할 때 MCP 표준을 따른다. 각 Skill은 MCP Tool로 등록되며, LLM이 tool_use로 호출한다.

```json
{
  "tools": [
    {
      "name": "issue_create",
      "description": "JIRA 이슈를 생성합니다",
      "input_schema": {
        "type": "object",
        "properties": {
          "projectCode": { "type": "string" },
          "issueType": { "type": "string", "enum": ["VOS", "Feature", "Requirement", "Architectural Design", "Detailed Design"] },
          "summary": { "type": "string" },
          "description": { "type": "string" }
        },
        "required": ["projectCode", "issueType", "summary"]
      }
    }
  ]
}
```

---

## 3. Chat 인터페이스

### 3.1 UI 설계
- 타이틀바 "GoJIRA" 텍스트 클릭 → 오버레이 채팅 패널 (우측 슬라이드 또는 모달)
- 대화 히스토리 표시 (세션 기반)
- 마크다운 렌더링 지원
- 코드 블록, 테이블, 목록 등 구조화된 응답
- 실행 결과 카드 (이슈 생성됨, 문서 수정됨 등)

### 3.2 대화 예시

```
사용자: GOJIRA 프로젝트의 요구사항 이슈를 정리해줘
Agent: GOJIRA 프로젝트에 12개의 Requirement 이슈가 있습니다.
       - GOJIRA-5: 성능 요구사항 (QA: R1)
       - GOJIRA-8: 인증 요구사항 (QA: S1)
       - ...
       상세 분석이 필요하시면 말씀해 주세요.

사용자: 인증 요구사항의 구조설계를 자동 생성해줘
Agent: GOJIRA-8 기반으로 구조설계 이슈를 생성하겠습니다.
       [실행 중...] issue_create → GOJIRA-20
       [실행 중...] doc_generate → architectural-design.md 갱신
       완료: GOJIRA-20 "인증 구조설계" 이슈가 생성되었고,
       설계문서에 ADSN_008 섹션이 추가되었습니다.

사용자: SATAM 분석 결과를 보여줘
Agent: 현재 SATAM 매트릭스 분석 결과입니다:
       - 위험도 높음: 2건 (GOJIRA-5, GOJIRA-12)
       - 민감도 높음: 1건 (GOJIRA-8)
       - 미완료 경감조치: 3건
       [SATAM 매트릭스 카드 표시]
```

### 3.3 지원 명령 카테고리

| 카테고리 | 예시 명령 |
|----------|-----------|
| 조회 | "프로젝트 목록", "GOJIRA 이슈 현황", "최근 PR 상태" |
| 생성 | "요구사항 이슈 만들어줘", "설계문서 생성", "PR 올려줘" |
| 수정 | "이슈 상태 변경", "문서 섹션 수정", "스케줄 업데이트" |
| 분석 | "SATAM 분석", "관계도 분석", "Diff 요약", "진행률 리포트" |
| 자동화 | "이슈에서 설계문서 자동 생성", "PR 생성 후 리뷰 요청" |

---

## 4. Agent Assist (메뉴별 보조 기능)

### 4.1 공통 패턴
각 메뉴의 생성/수정 영역에 `Agent Assist` 버튼을 배치한다.
클릭 시 현재 컨텍스트(프로젝트, 이슈, 파일 등)를 자동으로 전달하여 AI가 내용을 생성/수정한다.

### 4.2 문서 생성 종류

Agent가 자동 생성하는 문서는 우선 2종류를 지원한다.

| 문서 종류 | 파일 | 설명 |
|-----------|------|------|
| **요구사양서 (SRS)** | `srs.md` | VOS → Feature(UC) → Requirement(QA/FR/CNST) 순서로 구조화된 요구사항 명세 |
| **설계서** | `architecture.md` | Requirement 기반 구조/상세 설계안 (후보안 포함) |

### 4.2.1 필수 파일 (Protected Files)

아래 3개 파일은 **필수 파일**로, 과제 생성 시 빈 내용으로 자동 생성되며 **이름 변경 및 삭제가 불가능**하다.

| 파일 | 용도 |
|------|------|
| `business.md` | 비즈니스 배경, 목표, 스테이크홀더, 범위 등 과제 기본 정보 |
| `srs.md` | 요구사양서 (VOS → Feature → Requirement) |
| `architecture.md` | 구조설계서 (ADSN, 후보안 포함) |

**규칙:**
- 과제(프로젝트) 생성 시 DocRepo에 위 3개 파일이 자동으로 생성된다
- UI에서 해당 파일의 이름 변경(✎) 및 삭제(✕) 버튼을 비활성화한다
- API에서도 해당 파일에 대한 DELETE 및 이름 변경(rename) 요청을 거부한다
- 내용 편집은 자유롭게 가능하다

### 4.3 메뉴별 Assist 기능

#### 4.3.1 Issues — 이슈 자동 생성

편집모드 시 각 이슈 타입 컬럼 하단에 **"이슈 자동 생성"** 버튼을 배치한다.
클릭 시 Agent가 필요한 자료를 확인하여 해당 타입의 이슈를 자동 생성하고, 연관 이슈를 **contain** 관계로 연결한다.

##### VOS (Vision/Objective/Scope) 자동 생성
- **입력**: 과제의 비즈니스 배경, 목표, 범위 등 제공된 자료
- **동작**: 예상되는 스테이크홀더들의 요구(Stakeholder Needs)를 분석하여 VOS 이슈를 작성
- **출력**: VOS 이슈 N건 생성 (각 스테이크홀더 관점별)

##### Feature (Use Case) 자동 생성
- **입력**: VOS 이슈 + 제공된 자료 (비즈니스 컨텍스트)
- **동작**:
  1. 시스템의 **Use Case Diagram**을 작성 (액터-유즈케이스 관계)
  2. 각 액터와 1차 관계에 있는 유즈케이스를 **이미지(picture)**로 생성
  3. 생성된 Use Case Diagram 이미지를 문서 저장소에 저장
  4. 각 유즈케이스별 **User Story** 작성:
     - Actor (행위자)
     - Pre-condition (사전조건)
     - Flow (기본 흐름)
     - Alternative-flow (대안 흐름)
     - Exception-flow (예외 흐름)
     - Post-condition (사후조건)
- **출력**: Feature 이슈 N건 + Use Case Diagram 이미지 + User Story
- **관계**: VOS → Feature (contain)

##### Requirement (QA/FR/CNST) 자동 생성
- **입력**: Feature(UC) 이슈 + VOS 이슈
- **동작**: 각 유즈케이스에서 도출되는 요구사항을 타입별로 생성
  - **QA (Quality Attribute)**: 성능, 보안, 가용성 등 품질속성 요구사항
  - **FR (Functional Requirement)**: 기능 요구사항
  - **CNST (Constraint)**: 제약사항
- **출력**: Requirement 이슈 N건 (타입별 분류)
- **관계**: Feature → Requirement (contain)

##### ADSN (Architectural Design) 자동 생성
- **입력**: Requirement(QA, FR, CNST) 이슈
- **동작**: 각 요구사항에 대한 설계안을 **후보안 포함**하여 생성
  - 요구사항별 1~3개의 설계 후보안 작성
  - 각 후보안의 장단점(pros/cons), 리스크(risk), 민감도(sensitivity) 분석
  - 추천안 선정 및 근거 제시
- **출력**: Architectural Design 이슈 N건 (후보안 포함)
- **관계**: Requirement → ADSN (contain)

##### 이슈 자동 생성 흐름도

```
[VOS 자동 생성]  ← 비즈니스 배경/목표
       |
       | contain
       v
[Feature(UC) 자동 생성]  ← VOS + 제공자료
       |                    + Use Case Diagram 생성 (이미지 저장)
       |                    + User Story 작성
       | contain
       v
[Requirement 자동 생성]  ← Feature + VOS
       |    QA / FR / CNST 타입별 분류
       | contain
       v
[ADSN 자동 생성]  ← Requirement
       후보안 포함, 장단점/리스크 분석
```

#### 4.3.2 Docs (설계문서)
| 기능 | 동작 |
|------|------|
| SRS 자동 생성 | VOS → Feature → Requirement 이슈를 구조화하여 요구사양서 마크다운 생성 |
| 설계서 자동 생성 | ADSN/DDSN 이슈 기반 설계서 마크다운 생성 (후보안 포함) |
| Use Case Diagram 삽입 | Feature 생성 시 만든 다이어그램을 문서에 자동 삽입 |
| 섹션 자동 생성 | 이슈 데이터 기반 마크다운 섹션 생성 |
| 문서 요약 | 전체 문서 또는 변경사항 요약 |
| Diff 해석 | Semantic Diff 결과를 자연어로 설명 |
| 문서 리뷰 | 문서 품질/완성도 검토 및 개선 제안 |
| 템플릿 적용 | 빈 파일에 이슈 기반 템플릿 자동 적용 |

#### 4.3.3 Relation Map
| 기능 | 동작 |
|------|------|
| 관계 분석 | 이슈 간 의존성 분석 및 누락 관계 제안 |
| 영향도 분석 | 특정 이슈 변경 시 영향받는 이슈 체인 분석 |
| 관계 자동 생성 | 이슈 내용 기반 자동 Link 생성 제안 |
| contain 체인 검증 | VOS→Feature→Requirement→ADSN contain 관계 누락 검출 |

#### 4.3.4 SATAM
| 기능 | 동작 |
|------|------|
| 위험도 자동 평가 | 이슈 내용 분석 → Risk/Sensitivity 등급 제안 |
| 경감조치 생성 | 식별된 위험에 대한 Mitigation 방안 자동 작성 |
| SATAM 리포트 | 전체 매트릭스 분석 결과를 문서화 |

#### 4.3.5 Project Map
| 기능 | 동작 |
|------|------|
| 일정 분석 | 마일스톤 간 일정 적정성 분석 |
| 진행률 리포트 | 이슈 상태 기반 프로젝트 진행률 생성 |
| 리스크 알림 | 지연 위험 프로젝트 자동 감지 및 알림 |

---

## 5. Skill 정의

### 5.1 Skill 목록

각 Skill은 독립된 Go 함수 + LLM 프롬프트 템플릿으로 구성된다.

| Skill ID | 설명 | 입력 | 출력 | API 호출 |
|----------|------|------|------|----------|
| `vos_generate` | VOS 이슈 자동 생성 | projectCode, 비즈니스 배경/목표 | VOS 이슈 N건 | POST /Issue (반복) + POST /Issue/link |
| `feature_generate` | Feature(UC) 자동 생성 | projectCode | Feature 이슈 + UC Diagram + User Story | POST /Issue + PUT /DocRepo (이미지) |
| `requirement_generate` | Requirement 자동 생성 | projectCode | QA/FR/CNST 이슈 N건 | POST /Issue + POST /Issue/link |
| `adsn_generate` | ADSN 자동 생성 (후보안 포함) | projectCode | 설계안 이슈 N건 | POST /Issue + POST /Issue/link |
| `issue_create` | 이슈 단건 생성 | projectCode, issueType, summary, description, fields | 생성된 이슈 | POST /Issue |
| `issue_update` | 이슈 수정 | key, fields | 수정 결과 | PUT /Issue |
| `issue_query` | 이슈 조회/검색 | projectCode, filters | 이슈 목록 | GET /Issue |
| `issue_analyze` | 이슈 관계 분석 | projectCode | 관계 그래프 | GET /Issue + PUT /Issue/relations |
| `doc_generate_srs` | 요구사양서(SRS) 생성 | projectCode | requirement.md | POST /Issue/docgen |
| `doc_generate_design` | 설계서 생성 | projectCode | architectural-design.md, detailed-design.md | POST /Issue/docgen |
| `doc_edit` | 문서 섹션 편집 | projectCode, path, instruction | 수정된 내용 | PUT /DocRepo/{code}/file |
| `doc_review` | 문서 리뷰 | projectCode, path | 리뷰 코멘트 | GET /DocRepo/{code}/file |
| `doc_summarize` | 문서/변경 요약 | projectCode, path | 요약 텍스트 | GET /DocRepo/{code}/file |
| `usecase_diagram` | UC Diagram 생성 | projectCode, features | 이미지 파일 | PUT /DocRepo (이미지 저장) |
| `pr_create` | PR 생성 | projectCode, title, body | PR URL | POST /DocRepo/{code}/pr |
| `diff_explain` | Diff 해석 | projectCode, path | 자연어 설명 | GET /DocRepo/{code}/file (main vs branch) |
| `satam_analyze` | SATAM 분석 | projectCode | 분석 결과 | GET /Issue (requirements + designs) |
| `schedule_analyze` | 일정 분석 | projectCode | 분석 리포트 | GET /Project |

### 5.2 Skill 구조 (Go)

```go
// skills/skill.go
type Skill struct {
    ID          string
    Name        string
    Description string
    Parameters  json.RawMessage  // JSON Schema
    Execute     func(ctx context.Context, params map[string]interface{}, user string) (interface{}, error)
}

type SkillRegistry struct {
    skills map[string]*Skill
}

func (r *SkillRegistry) Register(skill *Skill) { ... }
func (r *SkillRegistry) Get(id string) *Skill { ... }
func (r *SkillRegistry) ListTools() []map[string]interface{} { ... }  // MCP tool 형식 반환
```

### 5.3 Skill 프롬프트 관리

```
GoJIRA-API/
  prompts/
    agent-router.txt        # 의도 분석 + Skill 선택
    agent-chat-system.txt   # 채팅 시스템 프롬프트
    skill-issue-create.txt  # 이슈 생성 보조
    skill-doc-review.txt    # 문서 리뷰
    skill-satam-analyze.txt # SATAM 분석
    skill-diff-explain.txt  # Diff 해석
    ...
```

---

## 6. API 설계

### 6.1 채팅 API

```
POST /api/v2/Agent/chat
Headers: X-User-Id, Content-Type: application/json
Body: {
  "sessionId": "uuid",           // 대화 세션 (신규 시 생략)
  "message": "사용자 메시지",
  "context": {                   // 선택: 현재 화면 컨텍스트
    "page": "docs",
    "projectCode": "GOJIRA",
    "selectedFile": "architectural-design.md"
  }
}
Response: {
  "sessionId": "uuid",
  "reply": "Agent 응답 (마크다운)",
  "actions": [                   // 실행된 액션 목록
    { "skill": "issue_create", "result": { "key": "GOJIRA-20", ... } },
    { "skill": "doc_generate", "result": { "files": 5, ... } }
  ],
  "suggestions": [               // 후속 제안
    "설계문서를 확인하시겠습니까?",
    "PR을 생성할까요?"
  ]
}
```

### 6.2 Assist API

```
POST /api/v2/Agent/assist
Headers: X-User-Id, Content-Type: application/json
Body: {
  "action": "generate" | "review" | "suggest" | "explain",
  "context": {
    "page": "issues" | "docs" | "satam" | "relationMap" | "project",
    "projectCode": "GOJIRA",
    "issueKey": "GOJIRA-5",      // 선택
    "filePath": "requirement.md", // 선택
    "selectedText": "...",        // 선택: 사용자 선택 텍스트
    "currentContent": "..."       // 선택: 현재 편집 내용
  },
  "instruction": "사용자 추가 지시"  // 선택
}
Response: {
  "result": "생성/수정된 내용 (마크다운)",
  "actions": [ ... ],
  "preview": true                // true면 사용자 확인 후 적용
}
```

### 6.3 Skill 목록 API (MCP 호환)

```
GET /api/v2/Agent/tools
Response: {
  "tools": [
    {
      "name": "issue_create",
      "description": "프로젝트에 새 이슈를 생성합니다",
      "input_schema": { ... }
    },
    ...
  ]
}
```

---

## 7. 대화 세션 관리

### 7.1 세션 저장소

```
MongoDB: agent_sessions
{
  "_id": "session-uuid",
  "userId": "duke.kimm",
  "messages": [
    { "role": "user", "content": "...", "timestamp": "..." },
    { "role": "assistant", "content": "...", "actions": [...], "timestamp": "..." }
  ],
  "context": { "projectCode": "GOJIRA", ... },
  "createdAt": "...",
  "updatedAt": "...",
  "ttl": 3600  // 1시간 후 만료
}
```

### 7.2 컨텍스트 자동 주입
- 현재 페이지, 선택된 프로젝트, 파일, 이슈 등을 자동 감지
- 대화 중 컨텍스트 전환 시 자동 업데이트
- 프로젝트의 이슈 목록, 문서 목록을 시스템 프롬프트에 포함

---

## 8. 이슈 추적 연동

모든 Agent 실행 결과는 이슈로 추적 가능해야 한다.

### 8.1 자동 이슈 기록
| Agent 액션 | 이슈 기록 |
|-----------|-----------|
| 이슈 생성 | 생성된 이슈 자체 |
| 문서 생성/수정 | 관련 이슈에 코멘트 추가 |
| PR 생성 | 관련 이슈에 PR 링크 코멘트 |
| SATAM 분석 | 분석 결과를 이슈 코멘트 또는 별도 이슈로 기록 |
| 일정 분석 | 리스크 항목을 이슈로 생성 |

### 8.2 추적 메타데이터
```json
{
  "source": "gojira-agent",
  "sessionId": "session-uuid",
  "skill": "doc_generate",
  "timestamp": "2026-04-07T12:00:00Z"
}
```

---

## 9. 구현 단계

### Phase 1: 기반 구축 (Agent Router + Chat UI)
- [ ] Agent Router 구현 (LLM 의도 분석 → Skill 매핑)
- [ ] Chat UI 컴포넌트 (타이틀바 GoJIRA 클릭 → 슬라이드 패널)
- [ ] POST /Agent/chat API
- [ ] 대화 세션 관리 (MongoDB)
- [ ] 기본 Skill 2~3개: issue_query, doc_summarize, schedule_analyze

### Phase 2: 핵심 Skill 구현
- [ ] issue_create, issue_update (이슈 CRUD)
- [ ] doc_generate, doc_edit (문서 생성/편집)
- [ ] diff_explain (Diff 해석)
- [ ] pr_create (PR 생성)
- [ ] 이슈 추적 연동 (액션 → 코멘트 자동 기록)

### Phase 3: Agent Assist 연동
- [ ] POST /Agent/assist API
- [ ] Issues 화면 Assist (이슈 자동 생성, 필드 채움)
- [ ] Docs 화면 Assist (섹션 생성, 리뷰, 템플릿)
- [ ] 각 Assist 결과 미리보기 → 사용자 확인 후 적용

### Phase 4: 분석 Skill + 고도화
- [ ] satam_analyze (SATAM 자동 분석)
- [ ] issue_analyze (관계 분석, 누락 관계 제안)
- [ ] schedule_analyze (일정 리스크 분석)
- [ ] 멀티턴 대화 고도화 (컨텍스트 유지, 후속 질문)

### Phase 5: MCP 표준화 + 확장
- [ ] GET /Agent/tools (MCP tool 목록)
- [ ] 외부 MCP 서버 연동 지원
- [ ] Skill 플러그인 아키텍처 (동적 Skill 등록)
- [ ] Slack 등 외부 시스템 Skill

---

## 10. LLM 설정 활용

기존 `config/llm.json` 설정을 Agent에서도 공유한다.

```json
{
  "endpoint": "https://api.openai.com/v1/chat/completions",
  "model": "gpt-4o-mini",
  "maxTokens": 500,
  "temperature": 0.3,
  "headers": { "Content-Type": "application/json" },
  "authType": "bearer"
}
```

Agent 전용 설정이 필요한 경우 `config/agent.json`으로 분리:
```json
{
  "routerModel": "gpt-4o",
  "skillModel": "gpt-4o-mini",
  "maxConversationTurns": 20,
  "sessionTTL": 3600,
  "maxTokensPerTurn": 2000
}
```

---

## 11. 보안 고려사항

- Agent 실행은 사용자 권한 범위 내에서만 동작
- 이슈 생성/수정은 해당 프로젝트 권한 확인
- 문서 수정은 편집 세션 lock 확인
- 모든 Agent 액션은 감사 로그 기록
- LLM 요청 시 민감 정보(API 키, 비밀번호 등) 마스킹

---

## 12. 기존 기능과의 통합 포인트

| 기존 기능 | Agent 통합 |
|-----------|-----------|
| DocGen (설계문서 자동 생성) | `doc_generate` Skill로 래핑, 채팅에서도 호출 가능 |
| Diff 분석 (Semantic/Unified/Summary) | `diff_explain` Skill로 자연어 해석 추가 |
| 커밋 그래프 | 채팅에서 "최근 커밋 이력 보여줘" 지원 |
| PR 생성/리뷰 | `pr_create` Skill + 리뷰 요약 |
| SATAM 매트릭스 | `satam_analyze` Skill로 자동 분석 |
| Relation Map | `issue_analyze` Skill로 관계 추천 |
| 프로젝트 알림 (SSE) | Agent 액션 결과도 SSE로 실시간 전파 |
| LLM 설정 (llm.json) | Agent Router/Skill에서 동일 설정 공유 |
| 편집 세션 (lock/TTL) | Agent 문서 수정 시 세션 자동 관리 |

---

## 부록: 용어 정의

| 용어 | 설명 |
|------|------|
| Agent Router | 사용자 의도를 분석하여 적절한 Skill을 선택하는 LLM 기반 라우터 |
| Skill | 특정 작업을 수행하는 독립 실행 단위 (MCP Tool과 1:1 매핑) |
| MCP | Model Context Protocol - AI 모델이 외부 도구를 호출하는 표준 프로토콜 |
| Agent Assist | 각 메뉴 내에서 컨텍스트 기반으로 동작하는 AI 보조 기능 |
| Session | 사용자와 Agent 간 대화 세션 (TTL 기반 만료) |
