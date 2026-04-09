# GoJIRA Agent 고도화 요구사항

## 1. 현황 진단

### 1.1 현재 구조: ReACT (Tool-Use Agentic Loop)

```
사용자 질문 → LLM 추론 → tool_call → Skill 실행 → 결과 피드백 → LLM 재추론 → ... (최대 10턴)
```

- Claude Code 패턴 기반 단일 루프
- LLM이 매 턴마다 다음 행동을 즉석 결정
- 명시적 계획/검증/상태관리 없음

### 1.2 현재 수준 평가

| 항목 | 권장 | 현재 상태 | 수준 |
|------|------|----------|------|
| 단계 분해 | 단계별 출력물 정의 | 없음 — LLM 즉석 결정 | ✗ |
| 역할 분리 | Planner/Author/Executor 등 | 만능 1개 루프 | ✗ |
| 상태 저장 | jobId, stage, artifacts | 세션 히스토리(대화 로그)만 | △ |
| 도구 계약 | 입출력 스키마 고정 | SKILL.md 파라미터 정의 있음 | ○ |
| 단계별 검증 | 코드+LLM 이중 검증 | 없음 | ✗ |
| 전이 조건 | entry/exit condition | tool_call 없으면 종료 | ✗ |
| 실패 복구 | 재시도, 롤백, 회귀 | 에러를 LLM에 전달만 | △ |
| 사람 개입 | 승인 게이트 | SKILL.md safety:confirm 있음 | ○ |
| 프롬프트 구조 | 역할/출력/제약 명세 | system prompt 1개 (일반적) | △ |
| 로그/평가 | 단계별 성공률 추적 | 세션 저장만 | △ |

**결론: 단순 Q&A, 1~2턴 작업은 가능하지만 복잡한 멀티스텝 작업에서는 불안정**

---

## 2. 핵심 원칙

> 복잡한 일을 잘하는 에이전트는 "좋은 프롬프트"보다 **"좋은 분해, 좋은 상태관리, 좋은 검증"**으로 만들어진다.

에이전트의 성능은 모델 자체보다 **운영 구조**가 좌우한다.

### 일반적인 실패 원인

1. 해야 할 일을 너무 크게 한 번에 던짐
2. 중간 결과를 저장하지 않음
3. 도구 호출 결과를 검증하지 않음
4. 다음 단계로 넘어가는 조건이 없음
5. 실패했을 때 복구 루트가 없음

---

## 3. 요구사항 상세

### 3.1 단계 분해 (Task Decomposition)

큰 작업을 명확한 단계로 나누고, 단계마다 출력물을 정의한다.

**나쁜 예:**
```
"설계 문서 만들고, 등록하고, 관련 자료 찾고, 이슈 만들고, 리뷰 요청해"
```

**좋은 예:**
```
1. 입력 해석 → 구조화된 작업 명세
2. 현재 단계 판별 → 다음 수행할 단계 결정
3. 필요한 자료 수집 → 관련 문서/이슈 목록
4. 초안 생성 → markdown 문서
5. 검증 → 통과/실패 + 사유
6. 등록 → 문서 ID, 버전, 저장 위치
7. 후속 액션 → 이슈 생성, 리뷰 요청
```

**단계별 출력물 예시:**

| 단계 | 출력물 |
|------|--------|
| 요구사항 정리 | 구조화된 requirement JSON |
| 설계 초안 | markdown 문서 |
| 등록 | 문서 ID, 버전, 저장 위치 |
| 도구 호출 | 성공/실패, 반환 데이터 요약 |

---

### 3.2 역할 분리 (Role-based Execution)

하나의 만능 에이전트보다 역할을 나눈다. 멀티에이전트가 아니어도, **역할별 프롬프트/모드**로 충분하다.

| 역할 | 책임 |
|------|------|
| **Planner** | 지금 뭘 해야 하는지 결정 |
| **Researcher** | 필요한 자료 수집 |
| **Author** | 문서 작성 |
| **Tool Executor** | 외부 시스템 호출 |
| **Reviewer** | 결과 검증 |
| **Registrar** | 저장/등록/버전처리 |

**핵심: "지금 이 단계에서는 무엇만 해야 하는가"를 좁혀주는 것**

---

### 3.3 상태 저장 (Job State)

복잡한 작업은 대화 한 번으로 끝나지 않는다. 에이전트는 메모리가 아니라 **상태 저장소**가 필요하다.

**최소 저장 항목:**

```json
{
  "jobId": "DESIGN-2026-0410-001",
  "stage": "draft_design",
  "inputs": {
    "project": "A 시스템",
    "goal": "초기 설계서 작성"
  },
  "artifacts": [
    {"type": "requirements", "id": "REQ-001"},
    {"type": "designDraft", "id": "DOC-014"}
  ],
  "toolResults": [
    {"tool": "doc_create", "status": "success"},
    {"tool": "issue_create", "status": "failed", "error": "..."}
  ],
  "nextAction": "retry_issue_create",
  "humanApprovalRequired": false
}
```

**이 상태가 있어야:**
- 중간에 끊겨도 이어감
- 실패해도 복구 가능
- 사람이 어디까지 진행됐는지 파악 가능

---

### 3.4 도구 계약 (Tool Contract)

도구는 함수가 아니라 **명확한 계약(interface)**으로 다뤄야 한다.

**좋은 도구 설계 조건:**

| 조건 | 설명 |
|------|------|
| 입력 스키마 명확 | JSON Schema로 정의 |
| 출력 스키마 명확 | 고정된 응답 구조 |
| 실패 코드 분명 | 에러 타입 구분 가능 |
| 재시도 가능 여부 | retryable: true/false |
| 부작용 큰 작업 | 승인 게이트 필수 |

**입력 예시:**
```json
{
  "title": "string (필수)",
  "description": "string",
  "projectKey": "string (필수)",
  "priority": "low|medium|high"
}
```

**출력 예시:**
```json
{
  "success": true,
  "issueKey": "ARCH-102",
  "url": "http://..."
}
```

> LLM이 자유 텍스트를 도구 출력으로 받으면 다음 단계에서 흔들리기 쉽다.

현재 SKILL.md의 파라미터 정의가 이 역할을 부분적으로 수행하고 있으며, 출력 스키마를 추가하면 완성된다.

---

### 3.5 단계별 검증 (Validation)

복잡한 작업은 "생성"보다 **"검증"**이 더 중요하다.

**이중 검증 구조:**

| 검증 유형 | 방법 | 예시 |
|----------|------|------|
| **형식 검증** | 코드 | 필수 필드 존재, JSON 스키마, 파일 형식 |
| **내용 검증** | LLM | 논리 흐름, 요구사항 누락, 모순 탐지 |

**검증 체크리스트 예시 (설계 초안):**
- 필수 섹션이 모두 있는가?
- 입력 요구사항과 모순되지 않는가?
- 금지된 표현이나 누락된 항목이 있는가?
- 등록 가능한 형식인가?
- 다음 단계에 넘길 품질인가?

---

### 3.6 전이 조건 (Stage Transition)

각 단계에 **entry condition / exit condition**을 명시한다.

**전이 조건 예시:**

| 단계 | exit condition | 다음 단계 |
|------|---------------|----------|
| 요구사항 정리 | 품질 점수 ≥ 0.8 | 설계 초안 |
| 설계 초안 | 검증 통과 | 등록 |
| 등록 | 등록 성공 (ID 반환) | 이슈 생성 |
| 이슈 생성 | 생성 성공 | 완료 |
| 사람 승인 | Approved | 배포 |

> 결과가 애매한데도 계속 진행해버리는 것이 가장 흔한 실패 원인

---

### 3.7 실패 복구 (Error Recovery)

복잡한 작업은 반드시 실패한다. 문제는 실패 자체가 아니라 **실패 후 행동이 없을 때**이다.

| 실패 유형 | 복구 전략 |
|----------|----------|
| 도구 timeout | 2회 재시도 |
| 스키마 오류 | 같은 단계 재생성 |
| 외부 등록 실패 | 등록 전 상태로 롤백 |
| 승인 거절 | 수정 단계로 회귀 |
| 자료 부족 | 조사 단계로 되돌아감 |

> 에이전트를 "정답 생성기"로 보면 안 되고, **복구 가능한 프로세스 실행기**로 봐야 한다.

---

### 3.8 사람 개입 (Human-in-the-Loop)

완전 자동화보다 **사람 승인형 반자동**이 현실적이다.

**사람 검토가 필요한 시점:**
- 중요한 문서 등록 전
- 외부 시스템에 쓰기(write) 전
- 최종 배포 전
- 요구사항이 모호할 때
- 비용이 큰 도구 호출 전

> 멈춰야 할 순간을 아는 구조로 만들어야 한다.

---

### 3.9 프롬프트 구조 (Prompt as Contract)

좋은 프롬프트는 감성적인 주문이 아니라 **작업 명세서**에 가깝다.

**프롬프트 필수 요소:**

| 요소 | 설명 |
|------|------|
| 역할 | 너는 "설계 초안 작성자"이다 |
| 현재 단계 | draft_design |
| 입력 데이터 | 요구사항 JSON |
| 해야 할 일 | markdown 설계 초안 작성 |
| 하지 말아야 할 일 | 추측 금지, 입력에 없는 사실 추가 금지 |
| 출력 형식 | JSON { title, summary, markdown } |
| 성공 기준 | 필수 섹션 6개 포함 |
| 실패 시 행동 | 부족한 정보 명시 후 중단 |

---

### 3.10 로그와 평가 (Observability)

어디서 실패하는지 모르면 개선할 수 없다.

**최소 기록 항목:**
- 어떤 작업에서 실패했는지
- 어느 단계에서 멈췄는지
- 어떤 프롬프트가 사용됐는지
- 어떤 도구 호출이 실패했는지
- 재시도 후 성공했는지
- 사람 수정이 얼마나 들어갔는지

**평가 지표:**

| 지표 | 설명 |
|------|------|
| 단계 성공률 | 각 단계별 1회 통과율 |
| 재시도율 | 실패 후 재시도 비율 |
| 사람 수정량 | 에이전트 산출물의 수정 비율 |
| 최종 승인률 | 사람 승인 통과율 |
| 작업 완료 시간 | 요청 ~ 완료까지 소요 시간 |

---

## 4. 최종 목표 아키텍처: 멀티 에이전트 시스템

### 4.1 전체 구조

```
┌─────────────────────────────────────────────────────────────────┐
│                        사용자 (Web / CLI)                        │
└────────────────────────────┬────────────────────────────────────┘
                             │
┌────────────────────────────▼────────────────────────────────────┐
│              Orchestration Agent (오케스트레이션)                 │
│                                                                   │
│  - 사용자 인터페이스 (대화, 명령 수신)                            │
│  - 작업 분석 → 실행 계획 수립                                     │
│  - 하위 에이전트 할당 및 조율                                      │
│  - 상태 관리 (Job State Machine)                                  │
│  - 승인 게이트 / 사람 개입 관리                                    │
│  - 최종 결과 취합 및 보고                                         │
│                                                                   │
│  ┌──────────┬──────────┬──────────┬──────────┬──────────┐       │
│  │          │          │          │          │          │       │
│  ▼          ▼          ▼          ▼          ▼          │       │
│                                                                   │
│  ┌─────┐ ┌─────┐ ┌─────┐ ┌─────┐ ┌─────┐              │       │
│  │ REQ │ │ DSN │ │ EVL │ │ CDA │ │ TST │              │       │
│  │Agent│ │Agent│ │Agent│ │Agent│ │Agent│              │       │
│  └──┬──┘ └──┬──┘ └──┬──┘ └──┬──┘ └──┬──┘              │       │
│     │       │       │       │       │                   │       │
└─────┼───────┼───────┼───────┼───────┼───────────────────┘       │
      │       │       │       │       │                            │
      ▼       ▼       ▼       ▼       ▼                            │
┌─────────────────────────────────────────────────────────────────┐
│  공유 인프라                                                      │
│  - Job State Store (MongoDB)                                     │
│  - Skill Registry (SKILL.md)                                     │
│  - Tool Executor (GoJIRA API, Gitea, JIRA)                      │
│  - 로그/평가 시스템                                               │
└─────────────────────────────────────────────────────────────────┘
```

### 4.2 에이전트 정의

#### Orchestration Agent (오케스트레이션 에이전트)

사용자와 직접 인터페이스하며, 하위 에이전트들을 조율하는 중앙 제어자.

| 항목 | 내용 |
|------|------|
| **역할** | 사용자 의도 분석, 실행 계획 수립, 에이전트 할당, 결과 취합 |
| **입력** | 사용자 자연어 요청 |
| **출력** | 실행 계획(Plan) + 최종 결과 보고 |
| **상태 관리** | Job State Machine — 전체 작업 진행 상태 관리 |
| **도구** | 하위 에이전트 호출, 승인 게이트, 상태 저장/조회 |
| **핵심 능력** | 작업 분해, 에이전트 간 의존성 관리, 병렬/순차 실행 판단 |

**실행 계획 예시:**
```json
{
  "jobId": "JOB-2026-0410-001",
  "goal": "A 프로젝트 설계서 작성 및 등록",
  "plan": [
    {"step": 1, "agent": "REQ", "task": "요구사항 수집 및 구조화", "depends": []},
    {"step": 2, "agent": "CDA", "task": "기존 코드 구조 분석", "depends": []},
    {"step": 3, "agent": "DSN", "task": "설계서 초안 작성", "depends": [1, 2]},
    {"step": 4, "agent": "EVL", "task": "설계서 품질 평가", "depends": [3]},
    {"step": 5, "agent": "ORCH", "task": "사용자 승인 요청", "depends": [4]},
    {"step": 6, "agent": "DSN", "task": "문서 등록 및 PR 생성", "depends": [5]}
  ]
}
```

#### Requirement Agent (요구사항 에이전트)

| 항목 | 내용 |
|------|------|
| **역할** | 요구사항 수집, 구조화, 분류, 누락 분석 |
| **입력** | 프로젝트 코드, 사용자 지시, 비즈니스 문서 |
| **출력** | 구조화된 요구사항 목록 (JSON), 누락 분석 리포트 |
| **도구** | issue_query, issue_create, issue_update, doc_summarize |
| **검증** | 필수 필드 존재, 요구사항 간 일관성, 분류 정합성 |
| **특화** | FR/NFR/CNST 분류, Use Case 추출, 요구사항 추적 매트릭스 |

#### Design Agent (설계 에이전트)

| 항목 | 내용 |
|------|------|
| **역할** | 아키텍처 설계, 설계 문서 작성, 설계 의사결정 기록 |
| **입력** | 요구사항 목록, 비즈니스 컨텍스트, 코드 분석 결과 |
| **출력** | 설계 문서 (markdown), 설계 결정 기록, 컴포넌트 다이어그램 |
| **도구** | doc_list, doc_summarize, doc_diff, pr_create |
| **검증** | 필수 섹션 존재, 요구사항 추적성, 설계 원칙 준수 |
| **특화** | Architecture Style 선정, Key Design Point 도출, SATAM 연계 |

#### Evaluation Agent (평가 에이전트)

| 항목 | 내용 |
|------|------|
| **역할** | 산출물 품질 평가, 일관성 검증, 개선 제안 |
| **입력** | 다른 에이전트의 산출물 (요구사항, 설계서, 코드 분석 결과) |
| **출력** | 평가 리포트 (점수, 통과/실패, 개선 사항) |
| **도구** | satam_analyze, issue_analyze, schedule_analyze |
| **검증 기준** | 완성도, 추적성, 일관성, 품질 메트릭 |
| **특화** | SATAM 평가, 요구사항-설계 추적성, 리스크 분석 |

#### Code Analysis Agent (코드 분석 에이전트)

| 항목 | 내용 |
|------|------|
| **역할** | 코드 기반 리버스 엔지니어링 — 구조, 요구사항, 의존성 추출 |
| **입력** | 소스 코드 저장소, 분석 범위 지시 |
| **출력** | 아키텍처 구조도, 추출된 요구사항, 의존성 그래프, 설계 준수율 |
| **도구** | 코드 파싱, AST 분석, 의존성 추출, Git 히스토리 분석 |
| **특화 기능** | |

| 기능 | 설명 |
|------|------|
| **구조 추론** | 코드로부터 모듈/패키지/레이어 구조 추출 |
| **요구사항 추출** | 코드 동작으로부터 암묵적 요구사항 역추출 |
| **의존성 분석** | 모듈 간 호출 관계, 순환 의존성 탐지 |
| **설계 준수 분석** | 설계 문서의 구조와 실제 코드 구조 비교 → 준수율 산출 |
| **변경 영향 분석** | 특정 모듈 변경 시 영향 받는 범위 예측 |

**설계 준수 분석 흐름:**
```
설계 문서 (architecture.md)  +  실제 코드 구조
         ↓                            ↓
  Design Agent 파싱            Code Analysis Agent 추론
         ↓                            ↓
    설계 구조 모델     ←비교→     코드 구조 모델
                    ↓
           준수율 리포트
    - 일치하는 컴포넌트: 85%
    - 설계에 있으나 코드에 없음: [ModuleX]
    - 코드에 있으나 설계에 없음: [UtilY]
    - 구조 위반: [ServiceA → DB 직접 접근]
```

#### Test Agent (테스트 에이전트)

| 항목 | 내용 |
|------|------|
| **역할** | 테스트 전략 수립, 테스트 케이스 생성, 테스트 결과 분석 |
| **입력** | 요구사항, 설계서, 코드 분석 결과 |
| **출력** | 테스트 케이스 목록, 테스트 커버리지 리포트 |
| **도구** | issue_create(type=Test), 코드 분석 도구 |
| **특화** | 요구사항 기반 테스트 설계, 경계값/동등분할, 리스크 기반 우선순위 |

### 4.3 에이전트 간 협업 패턴

```
┌────────────────────────────────────────────────────────────┐
│  패턴 1: 순차 파이프라인                                     │
│  REQ → DSN → EVL → 승인 → 등록                             │
│  (요구사항 → 설계 → 평가 → 사람 확인 → 최종 반영)            │
│                                                              │
│  패턴 2: 병렬 수집 → 합류                                    │
│  REQ ──┐                                                     │
│         ├─→ DSN → EVL                                        │
│  CDA ──┘                                                     │
│  (요구사항 + 코드분석 동시 수행 → 설계에 합류)                │
│                                                              │
│  패턴 3: 피드백 루프                                         │
│  DSN → EVL → [실패] → DSN (수정) → EVL (재평가)             │
│  (평가 실패 시 설계 에이전트에 수정 지시)                     │
│                                                              │
│  패턴 4: 교차 검증                                           │
│  CDA (코드 구조) ←비교→ DSN (설계 구조)                      │
│  → EVL (준수율 평가)                                         │
└────────────────────────────────────────────────────────────┘
```

### 4.4 상태 머신 (Job State Machine)

```
[계획수립]
    ↓
[에이전트 할당] → 병렬/순차 실행
    ↓
[단계 N 실행] → 에이전트 작업
    ├─ 성공 → [검증]
    │            ├─ 통과 → [다음 단계] 또는 [승인 게이트]
    │            └─ 실패 → [수정 지시] → [단계 N 재실행]
    └─ 실패 → [재시도 2회] → [롤백] → [계획 재수립]
                                              ↓
[승인 게이트]
    ├─ 승인 → [등록/후속처리]
    └─ 거절 → [수정 단계로 회귀]
                    ↓
[완료] → 로그 저장 + 결과 보고
```

---

## 5. 단계별 진행 계획

### Phase 1: 기반 구축 — 단일 오케스트레이터 강화

> 현재 ReACT 루프 위에 상태관리/검증/복구를 올려 안정화

| 항목 | 내용 |
|------|------|
| **목표** | 단일 에이전트의 복잡 작업 안정성 확보 |
| **기간** | 2~3주 |
| **산출물** | jobState 스키마, 오케스트레이터 코드, 검증기 |

**작업:**
1. Job State Store 구현 (MongoDB `agent_jobs` 컬렉션)
   - jobId, stage, inputs, artifacts, toolResults, nextAction
2. 코드 기반 오케스트레이터 (상태 머신)
   - 단계 정의, 전이 조건, entry/exit condition
3. 단계별 검증기
   - 코드 검증 (스키마, 필수값)
   - LLM 검증 (타당성, 누락)
4. 역할별 프롬프트 템플릿
   - Planner, Researcher, Author, Reviewer, Registrar 모드
5. 실패 복구 경로
   - 재시도, 롤백, 회귀 로직

**Phase 1 완료 기준:**
- "설계서 작성해줘" 5단계 파이프라인이 중단 없이 완주
- 중간 끊김 후 재시작 가능
- 실패 시 이전 단계로 복구

---

### Phase 2: 에이전트 분리 — Requirement + Design

> 단일 에이전트를 역할별로 분리 시작

| 항목 | 내용 |
|------|------|
| **목표** | 요구사항 에이전트, 설계 에이전트 독립 운영 |
| **기간** | 2~3주 |
| **산출물** | REQ Agent, DSN Agent, 에이전트 간 메시지 프로토콜 |

**작업:**
1. 에이전트 프로토콜 정의
   - 요청 형식: `{agent, task, inputs, jobContext}`
   - 응답 형식: `{status, artifacts, errors, nextSuggestion}`
2. Requirement Agent 분리
   - 전용 프롬프트 + 도구 세트 (issue_query, issue_create 등)
   - 요구사항 구조화 출력 스키마
3. Design Agent 분리
   - 전용 프롬프트 + 도구 세트 (doc_list, doc_summarize 등)
   - 설계 문서 출력 스키마
4. 오케스트레이터에서 에이전트 호출 인터페이스
   - 순차/병렬 실행 지원
   - 에이전트 간 산출물 전달

**Phase 2 완료 기준:**
- 오케스트레이터가 REQ → DSN 순차 파이프라인 실행
- 각 에이전트가 독립적으로 검증 가능한 산출물 반환
- 에이전트 간 산출물 형식이 호환

---

### Phase 3: 평가 + 코드 분석 에이전트

> 검증과 리버스 엔지니어링 능력 추가

| 항목 | 내용 |
|------|------|
| **목표** | 평가 에이전트, 코드 분석 에이전트 추가 |
| **기간** | 3~4주 |
| **산출물** | EVL Agent, CDA Agent, 설계 준수 분석 파이프라인 |

**작업:**
1. Evaluation Agent
   - 산출물 품질 평가 (SATAM, 추적성, 일관성)
   - 통과/실패 판정 + 개선 제안
   - 피드백 루프 (실패 → 수정 지시 → 재평가)
2. Code Analysis Agent
   - 코드 파싱 도구 (AST 분석, 의존성 추출)
   - 구조 추론 엔진 (모듈/레이어/패턴 인식)
   - 요구사항 역추출
3. 설계 준수 분석 파이프라인
   - 설계 문서 구조 파싱 (DSN Agent)
   - 코드 구조 추론 (CDA Agent)
   - 교차 비교 → 준수율 리포트 (EVL Agent)
4. 병렬 실행 패턴
   - REQ + CDA 동시 수행 → DSN에 합류

**Phase 3 완료 기준:**
- REQ + CDA 병렬 → DSN → EVL 파이프라인 실행
- 코드 분석 결과가 설계 초안에 반영
- 설계 준수율 리포트 자동 생성

---

### Phase 4: 테스트 에이전트 + 전체 통합

> 전체 소프트웨어 라이프사이클 커버

| 항목 | 내용 |
|------|------|
| **목표** | 테스트 에이전트 추가, 6개 에이전트 완전 통합 |
| **기간** | 2~3주 |
| **산출물** | TST Agent, 전체 파이프라인, 대시보드 |

**작업:**
1. Test Agent
   - 요구사항 기반 테스트 케이스 자동 생성
   - 코드 분석 결과 기반 테스트 범위 제안
   - 테스트 결과 분석 및 리포트
2. 전체 파이프라인 통합
   ```
   사용자 요청 → ORCH 계획 수립
     → REQ (요구사항) + CDA (코드분석) [병렬]
     → DSN (설계)
     → EVL (평가) → [피드백 루프]
     → TST (테스트 설계)
     → 승인 게이트
     → 등록/배포
   ```
3. 로그/평가 대시보드
   - 에이전트별 성공률, 재시도율, 소요시간
   - 작업별 진행 현황 추적
4. 에이전트 핫스왑
   - 에이전트별 LLM 모델/프롬프트 독립 설정
   - 에이전트 버전 관리

**Phase 4 완료 기준:**
- 6개 에이전트 전체 파이프라인 실행
- "요구사항 분석 → 설계 → 코드 분석 → 평가 → 테스트 → 등록" 자동 완주
- 에이전트별 성능 지표 추적 가능

---

### Phase 5: 고도화 — 자율 개선 + 외부 확장

| 항목 | 내용 |
|------|------|
| **목표** | 자율 개선 능력, 외부 시스템 연동 |
| **기간** | 지속적 |

**작업:**
1. 에이전트 자기 평가 — 이전 작업 로그로부터 개선점 학습
2. 프롬프트 자동 튜닝 — 성공/실패 패턴 기반
3. 외부 MCP 서버 연동 (Slack, Confluence, Jira Cloud 등)
4. CI/CD 연계 — 코드 분석 에이전트가 빌드/배포 파이프라인과 연동

---

## 6. 진행 상태 요약

| Phase | 내용 | 상태 |
|-------|------|------|
| 현재 | ReACT 단일 루프 + 14 Skills | **완료** |
| Phase 1 | 상태관리 + 검증 + 복구 (단일 강화) | 미착수 |
| Phase 2 | REQ + DSN 에이전트 분리 | 미착수 |
| Phase 3 | EVL + CDA 에이전트 추가 | 미착수 |
| Phase 4 | TST + 전체 통합 | 미착수 |
| Phase 5 | 자율 개선 + 외부 확장 | 미착수 |

---

## 7. GoJIRA 설계업무 적용 예시

### 시나리오 1: "A 프로젝트 초기 설계서 작성해줘" (Phase 1)

```
Stage 1 — Planner (오케스트레이터)
  입력: "A 프로젝트 초기 설계서 작성해줘"
  출력: { project: "APRJ", goal: "initial_design", docType: "architecture" }
  검증: projectKey 유효성 확인

Stage 2 — Researcher (역할 모드)
  도구: project_query(APRJ), issue_query(APRJ, type=Requirement)
  도구: doc_list(APRJ), doc_summarize(APRJ, business.md)
  출력: { requirements: [...], existingDocs: [...], businessContext: "..." }
  검증: 요구사항 3개 이상 수집됨

Stage 3 — Author (역할 모드)
  입력: Stage 1 명세 + Stage 2 자료
  프롬프트: "설계 초안 작성자" 역할, 필수 섹션 6개, JSON 출력
  출력: { title: "...", summary: "...", markdown: "..." }
  검증(코드): 필수 섹션 존재, 길이 > 500자
  검증(LLM): 요구사항 ID 반영 여부, 논리 흐름

Stage 4 — 승인 게이트
  사용자에게 초안 표시 + 승인/수정 요청
  승인 → Stage 5
  거절(수정 요청) → Stage 3 (수정 사유 포함)

Stage 5 — Registrar (역할 모드)
  도구: doc_create, issue_create, pr_create
  실패 시: 2회 재시도 → 실패 로그 저장

완료 — 결과 보고 + jobState 저장
```

### 시나리오 2: "A 프로젝트 코드가 설계를 따르는지 분석해줘" (Phase 3)

```
Orchestrator 계획:
  Step 1: CDA — 코드 구조 추론 [병렬]
  Step 2: DSN — 설계 문서 파싱 [병렬]
  Step 3: EVL — 구조 비교 + 준수율 산출

Step 1 — Code Analysis Agent
  입력: 소스 코드 저장소 URL
  작업: AST 파싱 → 모듈/레이어 구조 추론 → 의존성 그래프 생성
  출력: { modules: [...], layers: [...], dependencies: [...] }

Step 2 — Design Agent
  입력: architecture.md
  작업: 설계 문서에서 구조 모델 추출
  출력: { designModules: [...], designLayers: [...], designPatterns: [...] }

Step 3 — Evaluation Agent
  입력: Step 1 + Step 2 결과
  작업: 교차 비교 → 준수율 산출
  출력:
    - 전체 준수율: 85%
    - 설계에 있으나 코드에 없음: [ModuleX]
    - 코드에 있으나 설계에 없음: [UtilY, HelperZ]
    - 구조 위반: [ServiceA → DB 직접 접근 (설계: Repository 패턴)]
    - 개선 제안: [...]
```

---

## 8. 상세 구현 계획

### 8.1 현재 자산 현황

**기존 인프라:**
- GoJIRA-API (9001): Agent 서버 + 이슈/프로젝트 관리
- Platform-API (9000): 문서 저장소 + WebSocket + Webhook
- MongoDB: 세션, 프로젝트, 이슈, 알림 저장
- gojira-cli: 14개 서브커맨드

**기존 스킬 (14개):**

| 분류 | 스킬 | API |
|------|------|-----|
| 프로젝트 | project_query | MongoDB 직접 |
| 이슈 조회 | issue_query | MongoDB 직접 |
| 이슈 생성 | issue_create | MongoDB 직접 |
| 이슈 수정 | issue_update | MongoDB 직접 |
| 이슈 삭제 | issue_delete | MongoDB 직접 |
| 이슈 관계 | issue_link | MongoDB 직접 |
| 문서 목록 | doc_list | Platform-API /DocRepo/{code}/tree |
| 문서 요약 | doc_summarize | Platform-API /DocRepo/{code}/file + LLM |
| 문서 비교 | doc_diff | Platform-API /DocRepo/{code}/file × 2 + LLM |
| PR 목록 | pr_list | Platform-API /DocRepo/{code}/prs |
| PR 생성 | pr_create | Platform-API /DocRepo/{code}/pr |
| 일정 분석 | schedule_analyze | MongoDB 이슈 통계 + LLM |
| SATAM 분석 | satam_analyze | MongoDB QA/ADSN 이슈 + LLM |
| 이슈 분석 | issue_analyze | MongoDB contain 체인 + LLM |

**미활용 API (스킬 미구현):**

| API | 설명 | 스킬 후보 |
|-----|------|----------|
| `PUT /DocRepo/{code}/file` | 파일 수정/생성 | doc_write |
| `DELETE /DocRepo/{code}/file` | 파일 삭제 | doc_delete |
| `POST /DocRepo/{code}/conflict-resolve` | 충돌 해결 | doc_conflict_resolve |
| `GET /DocRepo/{code}/commits` | 커밋 이력 | doc_history |
| `GET /DocRepo/{code}/branches` | 브랜치 목록 | doc_branches |
| `POST /DocRepo/{code}/sync-branch` | 브랜치 동기화 | doc_sync |
| `POST /DocRepo/{code}/pr/{num}/review` | PR 리뷰 | pr_review |
| `POST /DocRepo/{code}/pr/{num}/merge` | PR 머지 | pr_merge |
| `GET /DocRepo/{code}/graph` | 커밋 그래프 | doc_graph |
| `POST /Issue/docgen` | 문서 생성 | doc_generate |
| `PUT /Issue/field` | 이슈 필드 업데이트 | issue_field_update |
| `PUT /Issue/relations` | 이슈 관계 일괄 설정 | issue_relations_batch |
| `GET /Project/alerts` | 프로젝트 알림 조회 | project_alerts |

---

### 8.2 Phase 1 상세 — 기반 구축

#### 8.2.1 Job State Store

**컬렉션: `agent_jobs`**

```json
{
  "_id": "ObjectId",
  "jobId": "JOB-GOJIRA-20260410-001",
  "sessionId": "agent_session_id",
  "userId": "batman",
  "status": "running|paused|completed|failed",
  "plan": {
    "goal": "초기 설계서 작성",
    "stages": [
      {"id": "parse_input", "status": "completed", "result": {...}},
      {"id": "gather_data", "status": "running", "result": null},
      {"id": "draft_doc", "status": "pending"},
      {"id": "validate", "status": "pending"},
      {"id": "approve", "status": "pending"},
      {"id": "register", "status": "pending"}
    ]
  },
  "context": {
    "projectCode": "GOJIRA",
    "inputs": {"goal": "initial_design", "docType": "architecture"}
  },
  "artifacts": [
    {"stage": "gather_data", "type": "requirements", "data": [...]},
    {"stage": "draft_doc", "type": "markdown", "data": "..."}
  ],
  "toolLog": [
    {"tool": "issue_query", "status": "success", "duration": 120},
    {"tool": "doc_summarize", "status": "success", "duration": 2500}
  ],
  "errors": [],
  "createdAt": "2026-04-10T...",
  "updatedAt": "2026-04-10T..."
}
```

**구현 파일:**
- `GoJIRA-API/db/agent_job.go` — CRUD + 상태 전이
- `GoJIRA-API/handlers/agent.go` — agentLoop에서 jobState 연동

#### 8.2.2 오케스트레이터 (상태 머신)

**파일: `GoJIRA-API/agent/orchestrator.go`**

```go
type Stage struct {
    ID          string
    Role        string  // planner|researcher|author|reviewer|registrar
    EntryCheck  func(job *Job) error
    Execute     func(ctx context.Context, job *Job) (*StageResult, error)
    ExitCheck   func(result *StageResult) error
    OnFailure   string  // 실패 시 이동할 stage ID ("" = 재시도)
    MaxRetries  int
}

type Orchestrator struct {
    Stages    []Stage
    JobStore  *db.JobStore
}

func (o *Orchestrator) Run(ctx context.Context, job *Job) error {
    for job.CurrentStageIdx < len(o.Stages) {
        stage := o.Stages[job.CurrentStageIdx]
        // entry check
        // execute (역할별 프롬프트 + 도구)
        // exit check
        // 성공 → 다음 단계 / 실패 → 재시도 or 회귀
        // jobState 저장
    }
}
```

#### 8.2.3 역할별 프롬프트 템플릿

**디렉토리: `GoJIRA-API/prompts/roles/`**

| 파일 | 역할 | 핵심 지시 |
|------|------|----------|
| `planner.txt` | Planner | 작업 분해, 출력: 구조화된 명세 JSON |
| `researcher.txt` | Researcher | 자료 수집만, 생성 금지, 출력: 수집 결과 |
| `author.txt` | Author | 문서 작성, 추측 금지, 출력: markdown |
| `reviewer.txt` | Reviewer | 검증만, 수정 금지, 출력: 통과/실패 + 사유 |
| `registrar.txt` | Registrar | 등록/실행만, 출력: 등록 결과 |

#### 8.2.4 검증기

**파일: `GoJIRA-API/agent/validator.go`**

```go
// 코드 검증
func ValidateSchema(result *StageResult, schema JSONSchema) error
func ValidateRequiredFields(result *StageResult, fields []string) error

// LLM 검증
func ValidateWithLLM(ctx context.Context, result *StageResult, criteria string) (*ValidationResult, error)
```

#### 8.2.5 추가 스킬 (Phase 1)

| 스킬 | 용도 | API |
|------|------|-----|
| **doc_write** | 문서 파일 생성/수정 | `PUT /DocRepo/{code}/file` |
| **doc_history** | 커밋 이력 조회 | `GET /DocRepo/{code}/commits` |
| **doc_sync** | 브랜치 동기화 | `POST /DocRepo/{code}/sync-branch` |
| **project_alerts** | 프로젝트 알림 상태 | `GET /Project/alerts` |

---

### 8.3 Phase 2 상세 — REQ + DSN 분리

#### 8.3.1 에이전트 프로토콜

**메시지 형식:**

```json
// 요청
{
  "agent": "REQ",
  "task": "gather_requirements",
  "jobId": "JOB-001",
  "inputs": {
    "projectCode": "GOJIRA",
    "scope": "all_requirements"
  },
  "context": {
    "previousArtifacts": [...]
  }
}

// 응답
{
  "status": "success|failed|needs_input",
  "artifacts": [
    {"type": "requirements_json", "data": {...}}
  ],
  "errors": [],
  "metrics": {"duration": 3200, "toolCalls": 4},
  "nextSuggestion": "proceed_to_design"
}
```

#### 8.3.2 Requirement Agent 스킬 세트

| 기존 스킬 | 추가 스킬 | 용도 |
|----------|----------|------|
| issue_query | **req_extract** | 문서에서 요구사항 추출 (LLM) |
| issue_create | **req_classify** | FR/NFR/CNST 자동 분류 |
| issue_update | **req_trace** | 요구사항 추적 매트릭스 생성 |
| issue_analyze | **req_gap_analysis** | 누락 요구사항 탐지 |
| doc_summarize | | 비즈니스 문서 요약 |

#### 8.3.3 Design Agent 스킬 세트

| 기존 스킬 | 추가 스킬 | 용도 |
|----------|----------|------|
| doc_list | **design_draft** | 설계 초안 생성 (LLM) |
| doc_write | **design_review** | 설계 자체 검증 (LLM) |
| doc_diff | **design_decision** | 설계 결정 기록 (ADR) 생성 |
| pr_create | | 설계 문서 PR |
| doc_summarize | | 기존 설계 요약 |

---

### 8.4 Phase 3 상세 — EVL + CDA

#### 8.4.1 Evaluation Agent 스킬 세트

| 기존 스킬 | 추가 스킬 | 용도 |
|----------|----------|------|
| satam_analyze | **eval_quality** | 산출물 품질 점수 산정 |
| issue_analyze | **eval_trace** | 요구사항-설계 추적성 검증 |
| schedule_analyze | **eval_consistency** | 산출물 간 일관성 체크 |
| | **eval_compliance** | 설계-코드 준수율 산정 |

#### 8.4.2 Code Analysis Agent 스킬 세트

모두 신규 — 코드 분석 전용 도구 필요

| 스킬 | 용도 | 구현 방식 |
|------|------|----------|
| **code_structure** | 모듈/패키지 구조 추출 | AST 파싱 또는 LLM + 파일 목록 |
| **code_dependencies** | 의존성 그래프 생성 | import/require 분석 |
| **code_extract_reqs** | 코드에서 암묵적 요구사항 역추출 | LLM 기반 |
| **code_patterns** | 아키텍처 패턴 인식 | LLM + 구조 분석 |
| **code_compare_design** | 설계 vs 코드 구조 비교 | 양쪽 모델 교차 비교 |
| **code_impact** | 변경 영향 분석 | 의존성 그래프 + 호출 체인 |

**코드 접근 방식:**
- 저장소가 Gitea에 있으므로 Gitea API로 파일 목록/내용 조회
- 기존 `doc_list` + `doc_summarize` 패턴 활용
- 대규모 코드는 파일 단위로 LLM에 분할 전달

#### 8.4.3 설계 준수 분석 파이프라인

```
Orchestrator Plan:
  Step 1 (병렬):
    CDA.code_structure(repo) → codeModel
    DSN.design_parse(architecture.md) → designModel
  Step 2:
    EVL.eval_compliance(codeModel, designModel) → complianceReport
  Step 3:
    EVL.eval_quality(complianceReport) → 점수 + 개선 제안
```

---

### 8.5 Phase 4 상세 — TST + 통합

#### 8.5.1 Test Agent 스킬 세트

| 스킬 | 용도 | 입력 |
|------|------|------|
| **test_design** | 테스트 전략 수립 | 요구사항 + 설계서 |
| **test_generate** | 테스트 케이스 자동 생성 | 요구사항 + 코드 분석 |
| **test_coverage** | 커버리지 분석 | 테스트 케이스 + 요구사항 |
| **test_priority** | 리스크 기반 우선순위 | SATAM 결과 + 테스트 목록 |

#### 8.5.2 전체 스킬 맵 (최종)

```
┌──────────────────────────────────────────────────────────────┐
│  Orchestration Agent                                          │
│  - job_create, job_status, job_resume, agent_dispatch         │
├──────────────────────────────────────────────────────────────┤
│  REQ Agent           │  DSN Agent          │  EVL Agent       │
│  - project_query     │  - doc_list         │  - satam_analyze │
│  - issue_query       │  - doc_write        │  - issue_analyze │
│  - issue_create      │  - doc_summarize    │  - eval_quality  │
│  - issue_update      │  - doc_diff         │  - eval_trace    │
│  - issue_delete      │  - doc_history      │  - eval_consist  │
│  - issue_link        │  - pr_create        │  - eval_comply   │
│  - req_extract       │  - pr_list          │  - schedule_anal │
│  - req_classify      │  - design_draft     │                  │
│  - req_trace         │  - design_review    │                  │
│  - req_gap_analysis  │  - design_decision  │                  │
├──────────────────────┼─────────────────────┼──────────────────┤
│  CDA Agent           │  TST Agent          │                  │
│  - code_structure    │  - test_design      │                  │
│  - code_dependencies │  - test_generate    │                  │
│  - code_extract_reqs │  - test_coverage    │                  │
│  - code_patterns     │  - test_priority    │                  │
│  - code_compare_dsn  │                     │                  │
│  - code_impact       │                     │                  │
└──────────────────────┴─────────────────────┴──────────────────┘

총 스킬 수: 14(기존) + 4(Phase1) + 8(Phase2) + 10(Phase3) + 4(Phase4) = 약 40개
```

---

### 8.6 구현 일정 요약

| Phase | 기간 | 핵심 산출물 | 스킬 추가 |
|-------|------|-----------|----------|
| **1** | 2~3주 | jobState, 오케스트레이터, 검증기, 역할 프롬프트 | +4 (doc_write, doc_history, doc_sync, project_alerts) |
| **2** | 2~3주 | REQ Agent, DSN Agent, 에이전트 프로토콜 | +8 (req_*, design_*) |
| **3** | 3~4주 | EVL Agent, CDA Agent, 준수 분석 파이프라인 | +10 (eval_*, code_*) |
| **4** | 2~3주 | TST Agent, 전체 통합, 대시보드 | +4 (test_*) |
| **5** | 지속 | 자율 개선, 외부 연동 | 필요시 |
