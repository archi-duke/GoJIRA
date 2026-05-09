# Agent 워크플로우 진화 전략

> **상태**: 초안 (2026-04-15) / **목적**: ReACT 단일 루프 → 명시적 상태 머신 → Plan 모드 까지의 단계적 진화 청사진. Plan 모드 도입 시 본 문서 재참조.

## 1. 배경 — 현재 ReACT 단일 루프의 한계

오늘 (2026-04-15) 워크플로우 시도 중 관찰된 6가지 실패 패턴:

1. `vos_collect` 대신 `issue_create_vos` 직접 호출 (skill candidate filter 사각)
2. `projdesc_interview` 미호출, 인라인 텍스트 질문
3. `projdesc_generate` 를 answers 없이 호출
4. 인터뷰 답변 키 불일치 (한국어 label vs 영문 id)
5. belief state 가 템플릿 파일을 "완료" 로 오인
6. 같은 스킬 무한 재호출 루프

### 공통 근본 원인 4가지

| # | 원인 | 결과 |
|---|------|------|
| A | **상태 분산/불일치** — 파일/Mongo/belief state/스킬 내부 검증이 같은 사실을 다르게 봄 | 한 곳은 OK, 다른 곳은 NO → LLM 혼란 |
| B | **LLM 자유도 과잉** — 매 턴 도구 선택권을 LLM 에 일임 | 워크플로우 진행 중에도 임의 우회 |
| C | **세션 상태가 LLM 응답에만 의존** — 인터뷰 답변 누적, 단계 위치 등이 LLM 컨텍스트에만 존재 | 키 변형/누락 시 즉시 와해 |
| D | **스킬 계약 약함** — 필수 인자 없이 호출 가능, 부분 호출 허용, 다음 단계 hint 만 제공 | LLM 이 정해진 흐름 우회 |

## 2. 진화 로드맵

```
Phase 0 (현재) ──→ Phase A ──→ Phase B ──→ Phase C ──→ Plan 모드
ReACT only      ReACT+        명시적         + PDCA       Plan-then-
                Job 누적      State Machine  Reflection   Execute
```

| 단계 | 패턴 | 핵심 변화 | 비용 |
|------|------|----------|------|
| 0 | Single-loop ReACT | belief state 주입 | 현재 |
| A | + 가벼운 세션 누적 | `agent_jobs` 에 인터뷰 답변/부분 산출물 저장. LLM 부담 ↓ | 1~2일 |
| **B** | **명시적 State Machine** | 워크플로우 단계/전이를 서버가 보유. LLM 은 단계 내부 작업만 | **3~5일** |
| C | + Reflection / PDCA | 각 단계 결과 평가 → 자동 보정/대안 | 1~2주 |
| Plan 모드 | Plan-then-Execute + B의 실행기 | Planner 가 단계 계열 동적 산출. Executor = State Machine | 2~3주 |

## 3. Phase B — 명시적 State Machine (다음 작업)

### 3.1 핵심 컨셉

**현재 GoJIRA 의 "Option C 자동 위임 체인" 은 암묵적 state machine**.
이를 **명시적 자료구조** 로 격상해 다음을 얻음:

- 현재 위치를 LLM 이 추측 안 함 (jobState 에 명시)
- 전이 조건/방향이 코드로 검증 가능
- 실패/재시작 시 명확한 재개 지점
- Plan 모드의 자연스러운 실행기 역할

### 3.2 자료구조

#### Workflow Definition (정적, 코드)

```python
# core/workflows/business_design.py
WORKFLOW = {
    "id": "business_design",
    "name": "사업 분석 → 설계 통합",
    "stages": [
        {"id": "projdesc",          "skill": "projdesc_interview", "completion": "doc_real_content"},
        {"id": "vos",               "skill": "vos_collect",        "completion": "vos_count > 0"},
        {"id": "usecase",           "skill": "usecase_analyze",    "completion": "feature_count > 0"},
        {"id": "requirement",       "skill": "requirement_refine", "completion": "req_count > 0"},
        {"id": "driver",            "skill": "driver_select",      "completion": "driver_count > 0"},
        {"id": "style",             "skill": "style_define",       "completion": "chosenStyles"},
        {"id": "design_decide",     "skill": "design_decide",      "completion": "adsn+ddsn > 0"},
        {"id": "design_integrate",  "skill": "design_integrate",   "completion": "designIntegrated"},
        {"id": "satam",             "skill": "satam_analyze",      "completion": "satamDone"},
    ],
    # 분기 (선택사항): {"from": stage_id, "condition": ..., "to": stage_id}
    "branches": [],
}
```

#### Job State (동적, MongoDB `agent_jobs`)

```yaml
jobId: <uuid>
projectCode: GOJIRA3
workflowId: business_design
status: in_progress    # pending|in_progress|completed|failed|paused
currentStage: projdesc
stageState:            # 단계별 누적 상태
  projdesc:
    status: collecting    # collecting|generating|done
    answers:              # 서버측 누적 — LLM 이 매번 다 안 보내도 됨
      background: "..."
      purpose: "..."
      stakeholders: "..."
    remaining: [requirements, constraints]
  vos: { status: pending }
history:
  - { stage: projdesc, action: question_sent, at: ... }
  - { stage: projdesc, action: answer_received, keys: [background, purpose], at: ... }
createdBy: batman
createdAt: ...
updatedAt: ...
```

### 3.3 실행 모델

```
사용자 메시지
   │
   ▼
[1] 의도 분류 / 워크플로우 매핑 (또는 기존 jobId 재개)
   │
   ▼
[2] State Machine — 현재 stage 결정
   │
   ▼
[3] Stage Handler 호출
       Input: jobState, 사용자 메시지
       Output: { newJobState, userResponse, nextAction }
       내부적으로 LLM + 스킬 사용 (자유도는 단계 내부에서만)
   │
   ▼
[4] State Machine — 전이 조건 평가
       (completion 만족? → 다음 stage)
   │
   ▼
[5] 응답 송출 (SSE) + jobState 영속화
```

**LLM 의 책임 변화**
- 이전: "다음에 뭘 호출할까" + "단계 내부 작업"
- 이후: "단계 내부 작업"만. 단계 전환은 서버가 결정

### 3.4 핵심 컴포넌트

| 모듈 | 역할 |
|------|------|
| `core/workflow_engine.py` | State Machine 실행기 — load_job / advance / complete / fail |
| `core/workflows/*.py` | 워크플로우 정의 (시작은 `business_design.py` 1개) |
| `core/job_store.py` | `agent_jobs` CRUD (이미 컬렉션 있음) |
| `agents/streaming.py` 개정 | 사용자 메시지 → 워크플로우 매핑 → engine 호출 → SSE |
| 각 `skills/*/executor.py` | jobId/stageState 받도록 시그니처 확장. 결과로 jobState 변화 반환 |

### 3.5 마이그레이션 전략 (점진적)

1. **워크플로우 정의 + engine 골격 구현** — 기존 스킬 그대로
2. **`projdesc` 단계만 engine 으로 이전** — 인터뷰 답변 서버 누적 검증
3. 검증 후 `vos` → `usecase` → ... 순차 이전
4. 마지막에 ReACT free-form 모드와 workflow 모드 분기 정착

### 3.6 비-워크플로우 작업 처리

기존 ReACT 단일 루프 = "free-form 모드" 로 보존.
- 사용자가 "이슈 1건만 추가" / "프로젝트 상태 알려줘" 같은 단순 요청 → free-form
- 워크플로우 매핑 가능한 입력 → engine 모드

라우팅: 사용자 메시지 + belief state 로 임베딩 유사도 + 키워드 결합 라우터 결정. 모호하면 free-form 유지.

## 4. Phase C — Reflection / PDCA

각 stage 종료 후 **품질 평가 단계** 추가:
- 산출물 평가 (LLM critique 또는 규칙 기반)
- 임계값 미달 시 같은 stage 재시도 (현재 critique 루프와 유사)
- 계속 미달이면 **상위 단계 롤백** (vos 단계로 회귀해서 보강 등)

이는 워크플로우 정의의 `branches` 에 평가 결과 분기 노드로 추가.

## 5. Plan 모드 (장기, Phase C 이후)

### 5.1 컨셉

복잡한 사용자 요청 (예: "이 과제 끝까지 끌어줘 + SATAM 후 비용 분석도 해줘") 을:

1. **Planner LLM**: 워크플로우 정의들을 조합/확장해 **dynamic plan** 산출
   - 표준 stage + custom stage 조합
   - 분기 조건 명시
   - 종료 기준
2. **Executor**: Phase B 의 State Machine 으로 plan 실행
3. **사용자 승인 게이트**: plan 산출 후 사용자에게 보여주고 승인 받음
4. **실행 중 재계획**: 단계 실패 시 Planner 에게 plan 수정 요청

### 5.2 Plan 자료구조

```yaml
planId: <uuid>
goal: "GOJIRA3 과제 끝까지 + 비용 분석"
projectCode: GOJIRA3
stages:
  - { id: ..., type: standard, workflowStage: projdesc }
  - { id: ..., type: standard, workflowStage: vos }
  - ... (business_design 워크플로우의 9단계 펼침)
  - { id: cost_analysis, type: custom, skill: cost_estimate, completion: ... }
status: pending_approval | approved | executing | completed | failed
approvedBy: batman
approvedAt: ...
```

### 5.3 Plan 모드 활성 조건

- 사용자가 명시적으로 "plan 모드" 활성 (`/plan` 명령 또는 UI 토글)
- 또는 의도 분류기가 "복합/장기 작업" 으로 판정

### 5.4 Phase B 가 Plan 모드의 토대인 이유

- Plan = "워크플로우 정의의 동적 인스턴스"
- Executor = 이미 만든 State Machine
- jobState = Plan execution state
- 새로 만들 것: Planner LLM + plan 자료구조 + 승인 UI

즉 **Phase B 가 Plan 모드 비용의 70% 이상을 미리 지불** 하는 셈.

## 6. 참고 — 학계/업계 패턴 매핑

| 패턴 | 외부 사례 | GoJIRA 어디에 |
|------|----------|--------------|
| ReACT | 원조 ReACT 논문, OpenAI tool-use | Phase 0 (현재) |
| State Machine | LangGraph, Temporal, BPMN | Phase B |
| Plan-then-Execute | AutoGPT, BabyAGI, MRKL | Plan 모드 |
| Multi-agent | AutoGen, CrewAI | (검토 보류 — 단일 LLM 운영 정책상) |
| Hierarchical Task Network | HTN planners, classical AI | Plan 모드 + Reflection 결합 |

## 7. 결정 로그

- 2026-04-15: ReACT 단일 루프 한계 인지. Phase B (state machine) 우선 채택. Plan 모드는 Phase B 위에 얹는 자연스러운 진화로 합의.
- LLM 종속성 회피 정책 유지 — Plan 모드도 OpenAI 호환 endpoint 만 사용.
