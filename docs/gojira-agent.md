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
┌─────────────────────────────────────────────────────────┐
│  Agent (Claude Code / GoJIRA Web / 기타 Agent)          │
│    └─ SKILL.md (표준 Claude Code 형식)                  │
│         └─ !`gojira-cli ...` (쉘 명령 직접 실행)        │
└────────────┬────────────────────────────────────────────┘
             │ HTTP (REST API)
             v
┌─────────────────────────────────────────────────────────┐
│  GoJIRA-API (Agent 서버)                                │
│    ├─ POST /Agent/chat    (Agentic Loop — 대화형)       │
│    ├─ POST /Agent/assist  (컨텍스트 기반 보조)          │
│    ├─ GET  /Agent/tools   (MCP 호환 tool 목록)          │
│    │                                                     │
│    ├─ Agentic Loop (Claude Code 패턴)                   │
│    │   LLM 호출 → tool_call 확인 → Skill 실행           │
│    │   → 결과를 LLM에 전달 → 반복 (최대 10 turns)       │
│    │                                                     │
│    ├─ Skill Registry (SKILL.md 기반 로더)               │
│    │   └─ skills/*/SKILL.md (YAML frontmatter + md)     │
│    │                                                     │
│    └─ 기존 API (Issue, Project, etc.)                   │
│                                                          │
├─ gojira-cli (독립 CLI 바이너리)                          │
│    project/issue/doc/pr/schedule 서브커맨드              │
└────────────┬────────────────────────────────────────────┘
             │ HTTP
             v
┌─────────────────────────────────────────────────────────┐
│  Platform-API                                            │
│    ├─ DocRepo (파일, PR, 편집 세션, 머지)               │
│    ├─ SSE (실시간 알림 push)                            │
│    └─ Webhook (Gitea main push → 브랜치 비교 → SSE)    │
└─────────────────────────────────────────────────────────┘
```

### 2.2 Agentic Loop (Claude Code 패턴)

GoJIRA Agent는 Claude Code와 동일한 Agentic Loop 패턴으로 동작한다.

```
사용자 메시지 수신
    ↓
LLM 호출 (시스템 프롬프트 + 도구 정의 + 대화 기록)
    ↓
분기 → 텍스트 응답 (stop) OR tool_call 반환
    ↓
tool_call 실행 (Skill → API 호출)
    ↓
결과를 대화 기록에 추가 (에러 시 에러 메시지 전달 → LLM이 재시도/대안 판단)
    ↓
tool_call 없을 때까지 반복 (최대 10 turns)
    ↓
최종 텍스트 응답 반환
```

**채택 근거:** Claude Code의 agent loop가 검증된 패턴이며, tool_use 기반 자율 판단 + 에러 복원력이 핵심. 별도 "의도 분석 → Skill 매핑" Router 없이 LLM이 직접 tool을 선택하도록 위임.

### 2.3 Skill 실행 방식 — CLI 기반 (MCP 대신)

**설계 결정:** MCP 서버 방식 대신 **CLI(gojira-cli) + 쉘 실행** 방식을 채택.

| 비교 | MCP 서버 | CLI (채택) |
|------|----------|-----------|
| 실행 | 상주 프로세스 필요 | 호출 시만 실행 |
| 속도 | 프로토콜 핸드셰이크 | 직접 실행, 빠름 |
| 배포 | 서버 + 설정 | 바이너리 하나 |
| 이식성 | MCP 클라이언트 필요 | 쉘 실행만 되면 OK |
| Claude Code | MCP 설정 필요 | `!`command`` 바로 사용 |

**채택 근거:**
- Claude Code의 `!`command``와 `Bash` 도구로 바로 사용 가능
- MCP 프로토콜 오버헤드 제거 → 더 빠른 응답
- 단일 Go 바이너리로 배포 간편
- CI/CD, 스크립트 자동화에서도 동일하게 활용
- 필요 시 나중에 `gojira-cli mcp-server` 모드로 MCP 래퍼 추가 가능

### 2.4 Skill 정의 — SKILL.md 선언형 (Claude Code 표준)

**설계 결정:** Skill을 Go 코드 하드코딩 대신 **SKILL.md 마크다운 파일**로 정의.

```
.claude/skills/                     # Claude Code 표준 위치
├── project-query/SKILL.md
├── issue-query/SKILL.md
├── issue-manage/SKILL.md
├── doc-manage/SKILL.md
├── pr-manage/SKILL.md
└── schedule-analyze/SKILL.md

GoJIRA-API/skills/                  # 서버 내장 (웹 Agent Chat용)
├── project_query/SKILL.md
├── issue_query/SKILL.md
├── issue_manage/SKILL.md
├── doc_manage/SKILL.md
├── pr_manage/SKILL.md
├── schedule_analyze/SKILL.md
├── registry.go                     # 범용 SKILL.md 로더
├── executor.go                     # 범용 실행 엔진
└── helpers.go                      # LLM/API 호출 공통
```

SKILL.md 형식 (Claude Code 표준 YAML frontmatter + 마크다운):
```yaml
---
name: issue-query
description: 프로젝트의 이슈 목록을 조회합니다
argument-hint: "[project-code] [--type TYPE]"
allowed-tools: Bash
---

!`gojira-cli issue list --project $0`
```

**채택 근거:**
- 코드 변경 없이 Skill 추가/수정 가능 (md 파일만 편집)
- Claude Code에 `.claude/skills/`로 두면 자동 인식
- 비개발자도 Skill 작성 가능 (프롬프트 튜닝)
- 서버 재시작으로 핫 리로드

### 2.5 Skill 분리 원칙 — CRUD 단위 개별 분리

**설계 결정:** LLM이 직접 tool을 선택하는 구조이므로, **CRUD 단위로 개별 Skill**을 정의.

| 분류 | Skill | 설명 | 안전 등급 |
|------|-------|------|----------|
| **조회** | `project_query` | 프로젝트 목록 조회 | 자동 |
| | `issue_query` | 이슈 목록 조회/필터 | 자동 |
| | `doc_list` | 문서 파일 목록 | 자동 |
| | `pr_list` | PR 목록 조회 | 자동 |
| **분석** | `doc_summarize` | 문서 요약 (LLM) | 자동 |
| | `doc_diff` | 브랜치 Diff 해석 (LLM) | 자동 |
| | `schedule_analyze` | 일정 분석 (LLM) | 자동 |
| **변경** | `issue_create` | 이슈 생성 | 확인 요청 |
| | `issue_update` | 이슈 수정 | 확인 요청 |
| | `issue_delete` | 이슈 삭제 | 확인 요청 |
| | `issue_link` | 이슈 관계 설정 | 확인 요청 |
| | `pr_create` | PR 생성 | 확인 요청 |

총 12개 (OpenAI 권장 10~20개 범위 내)

**채택 근거:**
- 사용자 의도가 CRUD 단위로 갈림 → LLM이 1단계에서 정확 선택
- description이 구체적 → "이슈를 생성합니다" vs "이슈를 관리합니다"
- 조회(자동)/변경(확인 요청) 안전 정책을 Skill 단위로 분리
- 이전 도메인 통합 방식(action 파라미터)은 LLM이 action을 잘못 고르는 문제 발생

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

### 5.1 현행 Skill 목록 (구현 완료)

CRUD 단위 12개 Skill, SKILL.md 선언형 + gojira-cli 실행:

| Skill | CLI 명령 | 설명 | 안전 |
|-------|---------|------|------|
| `project_query` | `gojira-cli project list` | 프로젝트 조회 | 자동 |
| `issue_query` | `gojira-cli issue list --project CODE [--type TYPE]` | 이슈 조회/필터 | 자동 |
| `issue_create` | `gojira-cli issue create --project CODE --type TYPE --summary "..."` | 이슈 생성 | 확인 |
| `issue_update` | `gojira-cli issue update --key KEY --status "..."` | 이슈 수정 | 확인 |
| `issue_delete` | `gojira-cli issue delete --key KEY` | 이슈 삭제 | 확인 |
| `issue_link` | `gojira-cli issue link --from KEY --to KEY --type contain` | 이슈 관계 설정 | 확인 |
| `doc_list` | `gojira-cli doc list --project CODE` | 문서 파일 목록 | 자동 |
| `doc_summarize` | `gojira-cli doc summarize --project CODE --file FILE` | 문서 요약 (LLM) | 자동 |
| `doc_diff` | `gojira-cli doc diff --project CODE --file FILE` | Diff 해석 (LLM) | 자동 |
| `pr_create` | `gojira-cli pr create --project CODE` | PR 생성 | 확인 |
| `pr_list` | `gojira-cli pr list --project CODE` | PR 목록 조회 | 자동 |
| `schedule_analyze` | `gojira-cli schedule analyze --project CODE` | 일정 분석 (LLM) | 자동 |

### 5.2 향후 추가 예정 Skill (문서 가이드 제공 후)

| Skill | 설명 | 비고 |
|-------|------|------|
| `vos_generate` | VOS 이슈 배치 자동 생성 | business.md 기반 |
| `feature_generate` | Feature(UC) 배치 생성 + UC Diagram | VOS 기반 |
| `requirement_generate` | Requirement(QA/FR/CNST) 배치 생성 | Feature 기반 |
| `adsn_generate` | 설계안(후보안 포함) 배치 생성 | Requirement 기반 |
| `doc_generate_srs` | 요구사양서 자동 생성 | srs.md |
| `doc_generate_design` | 설계서 자동 생성 | architecture.md |

### 5.3 Skill 파일 구조

**외부 Agent용 (Claude Code 호환):**
```
.claude/skills/
├── project-query/SKILL.md      # !`gojira-cli project list`
├── issue-query/SKILL.md        # !`gojira-cli issue list ...`
├── issue-create/SKILL.md       # !`gojira-cli issue create ...`
├── issue-update/SKILL.md       # !`gojira-cli issue update ...`
├── issue-delete/SKILL.md       # !`gojira-cli issue delete ...`
├── issue-link/SKILL.md         # !`gojira-cli issue link ...`
├── doc-list/SKILL.md           # !`gojira-cli doc list ...`
├── doc-summarize/SKILL.md      # !`gojira-cli doc summarize ...`
├── doc-diff/SKILL.md           # !`gojira-cli doc diff ...`
├── pr-create/SKILL.md          # !`gojira-cli pr create ...`
├── pr-list/SKILL.md            # !`gojira-cli pr list ...`
└── schedule-analyze/SKILL.md   # !`gojira-cli schedule analyze ...`
```

**서버 내장용 (웹 Agent Chat):**
```
GoJIRA-API/skills/
├── */SKILL.md                  # 동일 정의 (12개)
├── registry.go                 # SKILL.md 자동 로더 (YAML frontmatter 파싱)
├── executor.go                 # 범용 실행 엔진 (SKILL.md → API 호출)
└── helpers.go                  # LLM/API 호출 공통

GoJIRA-API/prompts/
├── agent-system.txt            # Agent 시스템 프롬프트
├── skill-doc-summarize.txt     # 문서 요약 프롬프트
├── skill-diff-explain.txt      # Diff 해석 프롬프트
└── skill-schedule-analyze.txt  # 일정 분석 프롬프트
```

### 5.4 gojira-cli (독립 CLI 바이너리)

```
gojira-cli/
├── main.go                     # 서브커맨드 라우팅
└── cmd/
    ├── common.go               # HTTP 클라이언트, 출력, 플래그 파서
    ├── project.go              # project list
    ├── issue.go                # issue list/create/update/delete/link
    ├── doc.go                  # doc list/get/summarize/diff
    ├── pr.go                   # pr create/list
    └── schedule.go             # schedule analyze

환경변수:
  GOJIRA_API_URL      GoJIRA API (default: http://localhost:9001/api/v2)
  GOJIRA_PLATFORM_URL Platform API (default: http://localhost:9000/api/v2)
  GOJIRA_USER         사용자 ID
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

## 9. 구현 단계 및 현황

### Phase 1: 기반 구축 — ✅ 완료
- [x] Agentic Loop 구현 (Claude Code 패턴 — turn 기반 tool_call 반복)
- [x] Chat UI 컴포넌트 (타이틀바 GoJIRA 클릭 → 우측 슬라이드 패널)
- [x] POST /Agent/chat, GET /Agent/tools API
- [x] 대화 세션 관리 (MongoDB, TTL 1시간)
- [x] 시스템 프롬프트 외부화 (prompts/agent-system.txt)
- [x] Skill 프레임워크: SKILL.md 기반 선언형 로더 + 범용 실행 엔진
- [x] 기본 Skill: project_query, issue_query, schedule_analyze

### Phase 2: 핵심 Skill + 필수 파일 — ✅ 완료
- [x] issue_manage (create/update/delete/link) — 도메인 통합
- [x] doc_manage (list/summarize/diff) — 도메인 통합
- [x] pr_manage (create/list) — 도메인 통합
- [x] 필수 파일 정책 (business.md, srs.md, architecture.md)
- [x] 멀티턴 대화 + 멀티 tool_call 정상 동작
- [ ] 문서 생성/수정 Skill (doc_generate_srs, doc_generate_design) — 가이드 대기

### Phase 2.5: 표준화 리팩터링 — ✅ 완료
- [x] Go 하드코딩 → SKILL.md 선언형 전환 (Claude Code 표준)
- [x] gojira-cli 독립 CLI 바이너리 구현
- [x] .claude/skills/ 표준 위치에 SKILL.md 배치
- [x] MCP 대신 CLI + 쉘 실행 방식 채택
- [x] Agent를 Claude Code 패턴으로 리팩터링 (에러 복원, 재귀 tool_call)

### Phase 3: Agent Assist UI — 🔧 진행 중
- [x] POST /Agent/assist API
- [x] Issues 편집모드 자동생성 버튼 (VOS/Feature/Req/ADSN)
- [x] Assist 결과 미리보기 패널
- [ ] Docs 화면 Agent Assist 버튼
- [ ] 각 Assist 결과 확인 후 적용 흐름 개선

### Phase 4: 분석 Skill + 고도화
- [ ] satam_analyze Skill 추가
- [ ] issue_analyze (관계 분석, contain 체인 검증)
- [ ] 멀티턴 대화 고도화 (컨텍스트 요약, 후속 질문 자동 생성)
- [ ] Relation Map / SATAM 화면 Assist 버튼

### Phase 5: 확장
- [ ] 외부 MCP 서버 연동 (필요 시 gojira-cli mcp-server 모드)
- [ ] Skill 핫 리로드 (서버 재시작 없이)
- [ ] Slack 등 외부 시스템 Skill

---

## 10. LLM 설정 활용

기존 `config/llm.json` 설정을 Agent에서도 공유한다.
Agent의 tool_use는 max_tokens를 2000 이상으로 자동 확대하여 사용.

```json
{
  "endpoint": "https://api.openai.com/v1/chat/completions",
  "model": "gpt-4o",
  "maxTokens": 500,
  "temperature": 0.3,
  "headers": { "Content-Type": "application/json" },
  "authType": "bearer"
}
```

**모델 선택 근거:** gpt-4o-mini는 tool_use 선택 정확도가 낮아 gpt-4o로 변경.
Agent용 max_tokens는 코드에서 2000 이상으로 자동 조정.

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

## 부록 A: 설계 결정 기록 (ADR)

### ADR-001: Agentic Loop — Claude Code 패턴 채택
- **결정:** 별도 의도 분석 Router 대신 LLM이 직접 tool을 선택하는 Claude Code 방식
- **근거:** 검증된 패턴, tool_use 기반 자율 판단, 에러 복원력
- **대안:** 의도 분석 → Skill 매핑 (2단계) — 불필요한 복잡성

### ADR-002: CLI 기반 실행 — MCP 대신 gojira-cli
- **결정:** MCP 서버 대신 독립 CLI 바이너리 + 쉘 실행
- **근거:** MCP 프로토콜 오버헤드 제거, 배포 간편 (바이너리 1개), Claude Code `!`command`` 바로 사용, CI/CD 호환
- **대안:** MCP 서버 — 상주 프로세스 관리 부담, 현 단계에서 과도

### ADR-003: SKILL.md 선언형 — Go 하드코딩 대신
- **결정:** Claude Code 표준 SKILL.md 형식으로 Skill 정의
- **근거:** 이식성 (다른 Agent에 그대로 사용), 코드 변경 없이 Skill 추가, 비개발자 편집 가능
- **대안:** Go 코드로 각 Skill 구현 — 초기 구현은 빨랐으나 이식성 없음

### ADR-004: Skill 분리 단위 — CRUD 개별 분리 (수정)
- **결정:** 도메인 통합(action 파라미터) 대신 **CRUD 단위 개별 Skill**로 분리
- **근거:**
  1. LLM이 직접 tool을 선택하는 구조에서는 개별 Skill이 유리
  2. 사용자 의도가 CRUD 단위로 갈리는 경우가 많음 ("생성해줘" vs "삭제해줘")
  3. description이 명확해져 LLM 선택 정확도 향상 ("이슈를 생성합니다" vs "이슈를 관리합니다")
  4. 권한/안전 정책 분리 용이 (읽기 Skill은 자동, 쓰기 Skill은 확인 요청)
  5. 12개는 OpenAI 권장 범위(10~20개) 내
- **이전 결정:** 도메인 통합 6개 — LLM이 action 파라미터를 잘못 선택하는 문제 발생
- **대안 검토:** 도메인 통합은 사람이 UI에서 선택하는 경우에 적합

### ADR-005: 시스템 프롬프트 외부화
- **결정:** Go 코드 하드코딩 대신 prompts/agent-system.txt 파일
- **근거:** 프롬프트 튜닝에 코드 변경 불필요, 재시작만으로 반영
- **대안:** 코드 내 상수 — 변경 시 빌드/배포 필요

### ADR-006: gpt-4o 모델 선택
- **결정:** gpt-4o-mini에서 gpt-4o로 변경
- **근거:** gpt-4o-mini는 tool_use를 무시하고 텍스트만 응답하는 문제 빈발
- **트레이드오프:** 비용 증가, 응답 속도 약간 느림

---

## 부록 B: 용어 정의

| 용어 | 설명 |
|------|------|
| Agentic Loop | LLM → tool_call → 실행 → 결과 피드백 → 반복하는 자율 실행 루프 |
| Turn | tool_call 1세트의 왕복 (LLM 응답 → 도구 실행 → 결과 피드백) |
| Skill | SKILL.md로 정의된 독립 실행 단위, gojira-cli 명령과 1:1 매핑 |
| gojira-cli | GoJIRA API를 호출하는 독립 CLI 바이너리 (Go) |
| SKILL.md | Claude Code 표준 형식의 Skill 정의 파일 (YAML frontmatter + 마크다운) |
| Agent Assist | 각 메뉴 내에서 컨텍스트 기반으로 동작하는 AI 보조 기능 |
| Session | 사용자와 Agent 간 대화 세션 (MongoDB, TTL 1시간) |
| Protected Files | business.md, srs.md, architecture.md — 삭제/이름변경 불가 필수 파일 |
