# GoJIRA Agent 구현 작업 계획

기획서(`gojira-agent.md`) 기반 단계별 구현 작업을 정리한다.

---

## Phase 1: 기반 구축 (Agent Router + Chat UI)

### 1.1 백엔드 — Skill 프레임워크
- **파일**: `GoJIRA-API/skills/registry.go`
- **작업**:
  - [ ] Skill 인터페이스 정의 (`ID`, `Name`, `Description`, `Parameters`, `Execute`)
  - [ ] SkillRegistry 구현 (Register, Get, ListTools)
  - [ ] MCP Tool 형식 변환 (`ListTools() → tools[]`)
  - [ ] 기본 Skill 3개 구현:
    - `issue_query` — 프로젝트 이슈 조회 (`skills/issue_query.go`)
    - `doc_summarize` — 문서 요약 (`skills/doc_summarize.go`)
    - `schedule_analyze` — 일정 분석 (`skills/schedule_analyze.go`)

### 1.2 백엔드 — Agent Router
- **파일**: `GoJIRA-API/handlers/agent.go`
- **작업**:
  - [ ] `POST /api/v2/Agent/chat` 핸들러
  - [ ] Agent Router 구현: 사용자 메시지 → LLM에 tool_use로 Skill 선택 요청
  - [ ] LLM 응답에서 tool_use 파싱 → Skill 실행 → 결과 조합
  - [ ] 멀티턴 지원: tool_result를 LLM에 다시 전달하여 최종 응답 생성
  - [ ] 프롬프트 파일: `prompts/agent-system.txt` (시스템 프롬프트)
  - [ ] `callLLMWithTools()` 함수 — 기존 `callLLM()` 확장 (tools 파라미터 지원)

### 1.3 백엔드 — 대화 세션 관리
- **파일**: `GoJIRA-API/db/agent_session.go`
- **작업**:
  - [ ] MongoDB `agent_sessions` 컬렉션
  - [ ] `CreateSession()`, `GetSession()`, `AppendMessage()`, `DeleteSession()`
  - [ ] 세션 TTL (1시간, configurable)
  - [ ] 세션에 context 저장 (현재 프로젝트, 페이지 등)

### 1.4 백엔드 — 라우팅
- **파일**: `GoJIRA-API/main.go`
- **작업**:
  - [ ] `POST /Agent/chat` 라우트 추가
  - [ ] `GET /Agent/tools` 라우트 추가 (MCP tool 목록)
  - [ ] caseInsensitiveRouting에 `"agent": "Agent"` 추가

### 1.5 프론트엔드 — Chat UI
- **파일**: `Platform-App/src/components/AgentChat.js`, `AgentChat.css`
- **작업**:
  - [ ] 타이틀바 "GoJIRA" 텍스트에 onClick 핸들러
  - [ ] 우측 슬라이드 채팅 패널 컴포넌트
  - [ ] 메시지 목록 (user/assistant 구분)
  - [ ] 입력창 + 전송 버튼
  - [ ] 마크다운 렌더링 (ReactMarkdown)
  - [ ] 실행 결과 카드 (actions 배열 렌더링)
  - [ ] 후속 제안 버튼 (suggestions 배열)
  - [ ] 현재 컨텍스트 자동 전달 (page, projectCode, selectedFile 등)
  - [ ] 세션 ID 관리 (sessionStorage)

### 1.6 프론트엔드 — GoJIRA-App 동기화
- **작업**:
  - [ ] GoJIRA-App에도 동일 컴포넌트 배치

### Phase 1 완료 기준
- GoJIRA 클릭 → 채팅 패널 오픈
- "GOJIRA 이슈 목록 보여줘" → 이슈 목록 응답
- "프로젝트 일정 분석해줘" → 일정 분석 결과
- 대화 히스토리 유지 (같은 세션 내)

---

## Phase 2: 핵심 Skill 구현

### 2.1 이슈 CRUD Skill
- **파일**: `GoJIRA-API/skills/issue_create.go`, `issue_update.go`
- **작업**:
  - [ ] `issue_create` — 이슈 생성 Skill (issueType, summary, description, fields → POST /Issue)
  - [ ] `issue_update` — 이슈 수정 Skill (key, fields → PUT /Issue)
  - [ ] `issue_link` — 이슈 연결 Skill (contain 등 관계 설정 → POST /Issue/link)
  - [ ] 생성/수정 시 이슈 코멘트에 Agent 출처 기록

### 2.2 문서 생성 Skill
- **파일**: `GoJIRA-API/skills/doc_generate.go`
- **작업**:
  - [ ] `doc_generate_srs` — SRS 생성 (VOS→Feature→Requirement 이슈 기반 → srs.md)
  - [ ] `doc_generate_design` — 설계서 생성 (ADSN 이슈 기반 → architecture.md)
  - [ ] 기존 DocGen 로직 활용 + LLM 품질 향상
  - [ ] 프롬프트: `prompts/skill-doc-srs.txt`, `prompts/skill-doc-design.txt`

### 2.3 문서 편집/분석 Skill
- **파일**: `GoJIRA-API/skills/doc_edit.go`, `diff_explain.go`
- **작업**:
  - [ ] `doc_edit` — 문서 섹션 편집 (instruction 기반 수정 → PUT /DocRepo/{code}/file)
  - [ ] `diff_explain` — main vs 사용자 브랜치 Diff를 자연어로 해석

### 2.4 PR Skill
- **파일**: `GoJIRA-API/skills/pr_create.go`
- **작업**:
  - [ ] `pr_create` — PR 생성 (LLM으로 제목/본문 자동 작성 → POST /DocRepo/{code}/pr)

### 2.5 필수 파일 정책 구현
- **작업**:
  - [ ] 프로젝트 생성 시 `business.md`, `srs.md`, `architecture.md` 자동 생성 (GoJIRA-API `ensureDocRepo` 확장)
  - [ ] Platform-API: DELETE 요청 시 필수 파일 거부
  - [ ] 프론트엔드: 필수 파일에 ✎/✕ 버튼 숨김

### Phase 2 완료 기준
- 채팅: "요구사항 이슈 만들어줘" → 이슈 생성 + 확인
- 채팅: "SRS 문서 생성해줘" → srs.md 갱신
- 채팅: "PR 올려줘" → PR 생성 + URL 반환
- 필수 파일 보호 동작

---

## Phase 3: Agent Assist 연동

### 3.1 백엔드 — Assist API
- **파일**: `GoJIRA-API/handlers/agent_assist.go`
- **작업**:
  - [ ] `POST /api/v2/Agent/assist` 핸들러
  - [ ] action 타입: `generate`, `review`, `suggest`, `explain`
  - [ ] 컨텍스트 기반 자동 Skill 선택 (page + action → Skill 매핑)
  - [ ] 결과에 `preview: true` 포함 → 프론트에서 확인 후 적용

### 3.2 Issues 화면 Assist
- **파일**: `Platform-App/src/components/IssueList.js` (또는 해당 컴포넌트)
- **작업**:
  - [ ] 편집모드 시 각 이슈 타입 컬럼 하단에 "이슈 자동 생성" 버튼
  - [ ] VOS 자동 생성: business.md 내용 → LLM → VOS 이슈 N건
  - [ ] Feature(UC) 자동 생성: VOS → UC Diagram + User Story → Feature 이슈 N건
  - [ ] Requirement 자동 생성: Feature → QA/FR/CNST 이슈 N건
  - [ ] ADSN 자동 생성: Requirement → 설계안(후보안 포함) 이슈 N건
  - [ ] contain 관계 자동 연결
  - [ ] 생성 진행 상태 표시 (프로그레스)
  - [ ] 생성 결과 미리보기 → 확인 후 적용

### 3.3 이슈 자동 생성 Skill (백엔드)
- **파일**: `GoJIRA-API/skills/vos_generate.go`, `feature_generate.go`, `requirement_generate.go`, `adsn_generate.go`
- **작업**:
  - [ ] `vos_generate` — 비즈니스 배경 → VOS 이슈 배치 생성
  - [ ] `feature_generate` — VOS → Feature 이슈 + UC Diagram + User Story
  - [ ] `usecase_diagram` — UC Diagram 이미지 생성 (Mermaid/PlantUML → 이미지 변환 또는 SVG 직접 생성)
  - [ ] `requirement_generate` — Feature → Requirement(QA/FR/CNST) 배치 생성
  - [ ] `adsn_generate` — Requirement → ADSN(후보안 포함) 배치 생성
  - [ ] 프롬프트: `prompts/skill-vos.txt`, `prompts/skill-feature.txt`, `prompts/skill-requirement.txt`, `prompts/skill-adsn.txt`

### 3.4 Docs 화면 Assist
- **파일**: `Platform-App/src/components/DocEditor.js`
- **작업**:
  - [ ] 편집 툴바에 "Agent Assist" 버튼 추가
  - [ ] 클릭 시 액션 선택: 섹션 생성 / 리뷰 / 요약 / 템플릿
  - [ ] 결과를 에디터에 미리보기 → 확인 후 삽입/대체
  - [ ] UC Diagram 이미지 자동 삽입 (Feature 생성 시 저장된 이미지)

### Phase 3 완료 기준
- Issues 편집모드: VOS 컬럼 하단 버튼 → VOS 이슈 자동 생성
- Feature 자동 생성 → UC Diagram 생성 + User Story 작성
- ADSN 자동 생성 → 후보안 포함 설계안
- Docs Assist: "이 섹션 리뷰해줘" → 개선 제안

---

## Phase 4: 분석 Skill + 고도화

### 4.1 분석 Skill
- **파일**: `GoJIRA-API/skills/satam_analyze.go`, `issue_analyze.go`
- **작업**:
  - [ ] `satam_analyze` — SATAM 매트릭스 자동 분석 (Risk/Sensitivity/Mitigation 평가)
  - [ ] `issue_analyze` — 이슈 관계 분석, 누락 관계 제안, contain 체인 검증
  - [ ] `schedule_analyze` 고도화 — 마일스톤 간 리스크 분석, 지연 예측

### 4.2 멀티턴 대화 고도화
- **작업**:
  - [ ] 대화 컨텍스트 요약 (긴 대화 → 요약 후 이어가기)
  - [ ] 후속 질문 자동 생성 (suggestions 개선)
  - [ ] 컨텍스트 전환 감지 ("다른 프로젝트로 바꿔줘")
  - [ ] 실행 확인 단계 ("이슈 5개를 생성합니다. 진행할까요?")

### 4.3 Relation Map / SATAM Assist
- **작업**:
  - [ ] Relation Map 화면에 Agent Assist 버튼
  - [ ] SATAM 화면에 Agent Assist 버튼
  - [ ] 분석 결과를 시각화 컴포넌트와 연동

### Phase 4 완료 기준
- 채팅: "SATAM 분석해줘" → 매트릭스 분석 + 리스크 항목
- 채팅: "관계도에서 누락된 연결 찾아줘" → 제안 목록
- 멀티턴: 10턴 이상 자연스러운 대화 유지

---

## Phase 5: MCP 표준화 + 확장

### 5.1 MCP 표준 API
- **작업**:
  - [ ] `GET /Agent/tools` — MCP tool 목록 반환 (전체 Skill)
  - [ ] MCP 메시지 형식 완전 호환 (tool_use, tool_result)
  - [ ] 외부 MCP 클라이언트에서 GoJIRA Skill 호출 가능

### 5.2 외부 MCP 서버 연동
- **작업**:
  - [ ] 설정 파일로 외부 MCP 서버 등록 (`config/mcp-servers.json`)
  - [ ] Agent Router에서 외부 Tool도 LLM에 제공
  - [ ] 외부 Tool 실행 → 결과 수집 → 응답 조합

### 5.3 Skill 플러그인
- **작업**:
  - [ ] 동적 Skill 등록/해제 API
  - [ ] Skill 핫 리로드 (서버 재시작 없이)

### Phase 5 완료 기준
- 외부 MCP 클라이언트(Claude Desktop 등)에서 GoJIRA tool 호출 가능
- 외부 MCP 서버의 tool을 GoJIRA Agent 대화에서 활용 가능

---

## 작업 우선순위 요약

| 순서 | 작업 | 예상 범위 |
|------|------|-----------|
| **1** | Skill 프레임워크 + Registry | GoJIRA-API 신규 패키지 |
| **2** | callLLMWithTools (tool_use 지원) | handlers/llm.go 확장 |
| **3** | Agent Router + POST /Agent/chat | handlers/agent.go 신규 |
| **4** | 대화 세션 관리 | db/agent_session.go 신규 |
| **5** | 기본 Skill 3개 (issue_query, doc_summarize, schedule_analyze) | skills/*.go 신규 |
| **6** | Chat UI 컴포넌트 | AgentChat.js/css 신규 |
| **7** | 타이틀바 연동 | App.js 수정 |
| → Phase 1 완료 | | |
| **8** | 이슈 CRUD Skill | skills/issue_*.go |
| **9** | 문서 생성 Skill (SRS, 설계서) | skills/doc_*.go |
| **10** | 필수 파일 정책 | 백엔드 + 프론트 |
| **11** | PR Skill + Diff Skill | skills/pr_*, diff_* |
| → Phase 2 완료 | | |
| **12** | Agent Assist API | handlers/agent_assist.go |
| **13** | VOS/Feature/Requirement/ADSN 자동 생성 Skill | skills/vos_*, feature_*, ... |
| **14** | Issues 화면 Assist UI | IssueList.js 수정 |
| **15** | Docs 화면 Assist UI | DocEditor.js 수정 |
| → Phase 3 완료 | | |
