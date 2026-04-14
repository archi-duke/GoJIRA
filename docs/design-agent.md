# Agent-Service 아키텍처 설계 문서

본 문서는 Agent-Service의 동작 구조, 설계 결정, 그리고 향후 진화 방향에 대한 논의를 정리한 설계 자료다.

---

## 1. 개요

Agent-Service는 GoJIRA 프로젝트 관리 시스템의 AI 에이전트 백엔드다. 사용자 자연어 요청을 받아 ReACT(Reasoning + Acting) 루프로 도구를 호출하고, 결과를 한국어로 종합해 응답한다.

**주요 구성**:
- `routers/agent.py` — 자체 구현한 ReACT 루프, 시스템 프롬프트 로딩, SSE 스트리밍
- `skills/` — 19개 도메인 스킬 (각 스킬은 SKILL.md + executor.py + prompts/ 구조)
- `skills/registry.py` — 스킬 등록 및 요청 기반 선별
- `prompts/system.txt` — 에이전트 최상위 시스템 프롬프트
- `clients/` — GoJIRA-API HTTP 클라이언트

---

## 2. 스킬(Skill) 구조

### 2.1 디렉터리 레이아웃

```
skills/<skill_name>/
├── SKILL.md          # Claude Code 표준 형식 (frontmatter + 본문)
├── executor.py       # Python 실행 로직
└── prompts/          # 내부 LLM 호출용 프롬프트 (선택)
    ├── generate.txt
    └── critique.txt
```

### 2.2 두 가지 스킬 부류

| 부류 | 예시 | 동작 방식 |
|------|------|----------|
| **단순 CRUD 스킬** | `issue_query`, `issue_create_*`, `issue_update`, `project_query`, `pr_list` | executor가 GoJIRA-API HTTP 호출 후 결과 반환 |
| **LLM 오케스트레이션 스킬** | `vos_collect`, `usecase_analyze`, `requirement_refine`, `satam_analyze`, `projdesc_generate` | executor 내부에서 LLM을 호출, critique 루프로 검증, 산출 문서 파일 생성, 후속 이슈 등록까지 수행 |

### 2.3 워크플로우 의존 체인 (LLM 오케스트레이션 스킬)

```
project-description.md → vos_collect → usecase_analyze → requirement_refine
```

- 하위 스킬은 선행 산출물이 부족하면 **스킬 내부에서 자동 호출**한다 (`prerequisitesRan` 필드에 기록)
- 예외: `project-description.md` 부족은 멀티턴 인터뷰가 필요하므로 자동 위임 불가 → `projdesc_interview` 호출을 에이전트에 위임

---

## 3. 에이전트 루프 (ReACT)

### 3.1 자체 구현 방식

`routers/agent.py`가 직접 ReACT 루프를 구현한다:

```
1. system 프롬프트 + 사용자 메시지로 messages 구성
2. LLM 호출 (Anthropic/OpenAI 등 자체 어댑터)
3. tool_calls 파싱
4. executor.run() 디스패치
5. tool 결과를 messages에 append
6. LLM 재호출 → 2~5 반복
7. tool_calls 없으면 최종 답변 반환
```

### 3.2 시스템 프롬프트

`prompts/system.txt`이 모든 ReACT 호출의 system 메시지로 주입된다 ([routers/agent.py:31](../Agent-Service/routers/agent.py#L31)).

주요 규칙:
- ReACT content 작성 강제 (도구 호출 전 사고 과정 명시)
- 추측 금지, 항상 도구로 실제 데이터 확인
- 이슈 생성 공통 규칙 (유사 이슈 사전 확인 → 분기 처리)
- 워크플로우 선행 자동 위임 처리 지침
- 산출 문서 본문을 채팅에 그대로 출력 금지
- 응답 스타일 (한국어, 간결, 친근한 톤)

---

## 4. 스킬 선별 (Tool Filtering)

### 4.1 동작 방식

`registry.select_tools()` ([skills/registry.py:153](../Agent-Service/skills/registry.py#L153))가 매 요청마다 19개 전체 스킬 중 5~9개만 LLM에 노출한다.

**5단계 파이프라인**:
1. **KEYWORD_MAP 하드코딩 매칭** (메인 신호) — 키워드 → 스킬 이름 사전 매핑
2. **description 단어 겹침 매칭** (보조) — 메시지 단어와 description이 2개 이상 겹치면 포함
3. **SKILL_GROUPS 확장** — 매칭된 스킬이 속한 그룹의 다른 스킬도 추가
4. **ALWAYS_INCLUDE** — 항상 포함되는 스킬 (예: project_query)
5. **MIN/MAX 범위 보정** — 너무 적으면 전체, 너무 많으면 우선순위로 절단

### 4.2 2-Pass 재선별 (스트리밍)

`agents/streaming.py`는 Pass 1 추론 결과를 바탕으로 한 번 더 스킬을 재선별해 Pass 2에 전달한다.

### 4.3 Claude Code 방식과의 비교

| 항목 | Agent-Service | Claude Code |
|------|--------------|-------------|
| 1차 선별 기준 | 하드코딩 KEYWORD_MAP | description 텍스트만 |
| description 활용 | 보조 (단어 겹침 카운트) | 유일한 신호 |
| 스킬 그룹 | 코드에 정의 | 없음 (LLM이 알아서) |
| 신규 스킬 추가 시 | KEYWORD_MAP/GROUPS 갱신 필요 | SKILL.md 추가만으로 끝 |
| 결정 주체 | Python 룰 + LLM | LLM 단독 |

**평가**:
- **장점**: 결정론적, 토큰 사용 적음, 디버깅 용이
- **단점**: 신규 스킬 추가 시 코드 수정 필요, "description이 곧 진실"이라는 Claude Code 철학과 어긋남

---

## 5. 비판적 평가 루프 (Critique Loop)

생성 단계 스킬(VOS, Use Case, Requirement, 설계안 등)은 다음 루프를 거친다:

```
[생성] → [비판적 평가] ──pass?──▶ 완료
           │
           └─ fail → [수정] → [재평가]  (최대 N회)
```

- 평가 결과: `{"pass": true/false, "issues": [...], "reasoning": "..."}` 형식의 JSON
- 최대 반복 제한 (기본 2회)으로 비용 통제
- 최종 산출물에 `critiqueTrail`을 포함하여 검증 이력 추적
- 공용 헬퍼: `skills/critique.py`의 `iterate_generate_critique()`

현재 적용 범위:
- ✅ Phase 1 (VOS/Use Case/Requirement) — `vos_collect`, `usecase_analyze`, `requirement_refine`
- ⏳ Phase 2~4 (구조 결정/설계/SATAM) — 향후 추가 시 동일 패턴

---

## 6. 이슈 생성 공통 규칙

`prompts/system.txt`의 "이슈 생성 공통 규칙"이 권위 있는 출처. 각 `issue_create_*` 스킬의 SKILL.md에서 이를 참조한다.

**핵심 규칙**:
1. 생성 전 반드시 `issue_query`로 유사 이슈 사전 확인
2. 분기 처리:
   - 유사 이슈 없음 → `issue_create_*` 호출
   - 유사 이슈 있음, 동일 → 기존 키 안내
   - 유사 이슈 있음, 보강 필요 → `issue_update` + 코멘트
   - 폐기 대상 → `mark_for_deletion` (실삭제는 사용자 명시 지시 시만)
3. 삭제 마킹: summary에 `[Deletion Required] ` 접두 + label `del_req` 추가
4. 사용자 직접 삭제 지시 시만 `issue_delete` (hard delete)

---

## 7. 이슈 체계 (Contain Chain)

```
VOS —(contains)→ Feature —(contains)→ Requirement —(contains)→ ADSN / DDSN
                                          ├── FR (기능)
                                          ├── NFR (품질속성)
                                          └── CNST (제약)
```

### 연결 규칙

| Requirement 분류 | ASR 라벨 | 필수 하위 이슈 |
|-----------------|----------|---------------|
| NFR (품질속성) | 무관 | ADSN 1개 이상 |
| 임의 타입 | `ASR` 라벨 있음 | ADSN 1개 이상 |
| FR (기능) | `ASR` 라벨 없음 | ADSN 또는 DDSN 1개 이상 |
| CNST (제약) | — | 하위 이슈 불필요 |

상세는 [`prompts/workflow-architecture-design.md`](../Agent-Service/prompts/workflow-architecture-design.md) 참조.

---

## 8. .claude/skills와의 관계

GoJIRA 리포에는 두 가지 스킬 시스템이 공존한다:

| 항목 | `.claude/skills/` | `Agent-Service/skills/` |
|------|------------------|------------------------|
| 호스트 | Claude Code (CLI/IDE) | Agent-Service (자체 ReACT) |
| 실행 방식 | SKILL.md 지시문에 따라 Claude가 직접 수행 | executor.py가 Python으로 실행 |
| 주요 도구 | `gojira-cli` (subprocess) | GoJIRA-API HTTP 클라이언트 |
| LLM 호출 | Claude Code의 Claude | Agent-Service 내부 LLM 어댑터 |
| 형식 호환성 | 표준 Claude Code SKILL.md | 동일 형식으로 변환 완료 (커밋 7d2cd22) |

### 이식 가능성

- **단순 CRUD 스킬**: SKILL.md만 다듬으면 거의 그대로 이식 가능
- **LLM 오케스트레이션 스킬**: executor의 critique 루프, 선행 자동 위임, 결정론적 JSON 파싱이 Python 코드에 의존 → Claude로 옮기면 LLM 지시문으로 풀어야 해서 동작 일관성 손실. **재설계 필요**

---

## 9. 향후 진화 방향에 대한 검토

### 9.1 Claude Agent SDK 채택 검토

**검토 결과: 채택 불가**

| 항목 | 자체 ReACT (현재) | Claude Agent SDK |
|------|------------------|------------------|
| 코드 양 | 많음 | 크게 줄어듦 |
| 컨텍스트 관리/스트리밍/캐시 | 직접 구현 | SDK 자동 |
| 멀티 LLM 지원 | 가능 | Anthropic 전용 |
| 동작 통제권 | 100% 우리 손 | SDK에 일부 양도 |

**불가 사유**: 사내 정책상 특정 LLM 벤더 종속을 받아들일 수 없음. 자체 ReACT 유지 + LLM 어댑터 추상화 방향으로 계속 발전.

### 9.2 gojira-cli로의 일원화 검토

**부분 일원화는 의미 있음**:
- 단순 CRUD 스킬은 gojira-cli 위임으로 통일 가능 → DRY 효과
- LLM 오케스트레이션 스킬은 executor 유지 필요

**전면 치환은 비추천**: subprocess 호출 오버헤드, 스트리밍 단절, LLM 호출 불가능

### 9.3 스킬 선별 방식 개선 검토

현재 KEYWORD_MAP 기반 → description 기반(또는 2-pass) 전환은 검토 가치 있음:
- **장점**: 신규 스킬 추가가 SKILL.md 작성만으로 완결, Claude Code 철학과 일치
- **단점**: 토큰 비용 증가, 결정 비결정론화

판단 기준: 스킬 수가 계속 늘어날 예정이라면 description 기반 전환이 유리. 안정적이면 현 방식 유지.

### 9.4 차용 가능한 패턴 (LLM 종속 무관)

Claude SDK 채택은 불가하지만 다음은 차용 가능:
- SKILL.md 표준 형식 (이미 적용)
- MCP 도구 스펙
- critique 루프 패턴 (이미 적용)
- 메모리/플랜 기능을 우리 방식으로 재구현

---

## 10. 상태 관리 (State Management)

### 10.1 ReACT 의 3단계 상태 관리

| 수준 | 이름 | 메커니즘 | 범위 |
|------|------|---------|------|
| 1 | Scratchpad | (Thought / Action / Observation) triple 을 프롬프트에 이어붙임 | 턴 내부 |
| 2 | Belief State | 외부 시스템(DB/API/문서)에서 현재 상태를 조회해 매 턴 context 에 주입 | 세션·프로젝트 |
| 3 | Explicit Plan / Workflow Graph | Plan 을 1급 객체로 영속화, 각 노드의 상태(pending/in_progress/done/blocked) 추적, 재시작·재개·분기 지원 | 장기 워크플로우 |

실무 대부분은 **수준 1 + 2 조합**. 수준 3 은 워크플로우가 길고 중단/재개/재시도가 잦을 때 가치가 있음.

### 10.2 현재 상태와 발견된 문제

현 에이전트는 **수준 1 만** 구현됨. Scratchpad 는 ReACT 2-pass 추론에서 충분히 작동하지만, **턴 경계를 넘는 상태 공유가 없다**. 다음과 같은 증상 발생:

- 사용자 "계속", "진행해줘" 같은 모호한 입력에서, 이전 턴에 `vos_collect` 가 성공했다는 사실을 LLM 이 prompt 문맥(assistant 메시지)으로만 추정 → 재호출 선택 오류
- 외부(다른 사용자/세션)에서 이슈가 변경되어도 인지 불가
- 재시작 시 직전 세션의 진행 위치 유실

**근본 원인**: 진실 상태(VOS/Feature/Requirement/archSettings)는 **Git YAML(이슈) + MongoDB(프로젝트 메타)** 에 존재하지만 LLM 에게는 prompt 문맥만 있을 뿐 직접 연결이 없음.

### 10.3 채택 방향: 수준 2 (Belief State) 추가

**설계**:

```python
# core/workflow_state.py (신규)
def get_workflow_state(project_code: str) -> dict:
    return {
        "projdesc_exists": bool,
        "vos_count": int,
        "feature_count": int,
        "requirement_count": int,
        "driver_count": int,          # Requirement 중 driverSelected=true
        "chosenStyles": dict | None,  # projects.archSettings.chosenStyles
        "design_count": {"adsn": int, "ddsn": int},
        "next_recommended_step": str, # 계산된 다음 워크플로우 스킬
    }
```

**주입 지점**: `agents/streaming.py` Pass 1 추론 직전, 사용자 메시지에서 프로젝트 코드가 식별되면 위 상태를 한국어 요약으로 system 메시지에 덧붙임.

**상태 원천**:
- 이슈 카운트/필드 — MongoDB `issues` **캐시** 조회 (원천은 git `data/issues/*.yaml`, 캐시는 sync 훅으로 동기 유지)
- `archSettings` (chosenStyles, satamDone, designIntegrated 등 플래그) — MongoDB `projects`
- 문서 존재 여부 — Gitea 트리 조회

`completedSteps` 계산은 이슈 카운트 + `archSettings` 플래그 조합. git YAML 을 직접 읽지 않고 캐시에서 읽기 때문에 hot path 성능 유지.

**효과**:
- LLM 이 "이미 VOS 13 건 / Feature 0 건" 을 인지 → "계속" = `usecase_analyze` 호출로 결정
- 외부 변경 반영 (다른 세션에서 이슈 삭제 등)
- 기존 ReACT 구조·스킬 변경 없음 — 주입 레이어만 얇게 추가

### 10.4 향후 수준 3 (PDCA 모드) 준비

수준 3 은 **PDCA (Plan-Do-Check-Act)** 로 구체화 예정. 거대 목표("과제 하나 완성") 또는 피드백 루프("SATAM 리스크 → 설계 수정 → 재평가")처럼 단순 선형 워크플로우로 표현할 수 없는 시나리오를 겨냥.

| PDCA | 수준 3 요소 |
|------|-----------|
| Plan | 목표 → 태스크 그래프 분해, 각 노드의 예상 산출/검증 기준 정의 |
| Do | 각 태스크를 스킬로 실행 |
| Check | 산출물이 계획된 기준을 만족하는가 (critique 의 상위 개념 — 전체 목표 대비 편차) |
| Act | 편차 시 plan 재작성 + 재실행 루프 |

**설계 포인트**:
- Plan 을 1급 객체(`plans` collection)로 영속화 — checkpointing / 재시작 자연스러움
- Check 는 스킬 내부 critique 와 구분: critique = "산출물 품질", Check = "목표 대비 편차"
- Act 의 두 분기: minor deviation → 노드 재실행, major deviation → plan 재작성 (replanner)
- Replanner 무한 루프 방지: hard cap + 사람 에스컬레이션

### 10.5 단계적 진화 로드맵

```
Now     : 수준 1 (Scratchpad) + 수준 2 (Belief State) 얇게 추가
Next    : 수준 2 안정화 · 워크플로우 상태 포맷 정제
Later   : 수준 3 (PDCA) — 거대 목표·피드백 루프에만 선택 적용
```

**원칙**: 수준 2 의 `workflow_state` 가 만드는 Belief State 는 그대로 수준 3 의 **Check 단계 관측(observation) 입력** 으로 재활용된다. 수준 2 는 수준 3 의 자산이지 부채가 아님.

단순 선형 워크플로우에 PDCA 를 씌우면 부담만 늘고 이익 없음. PDCA 는 **목표 모호성** 또는 **자동 재시도 루프** 가 가치 있을 때 적용.

---

## 11. 핵심 설계 원칙 (요약)

1. **자체 ReACT 유지** — LLM 종속 회피, 멀티 벤더 어댑터 지원
2. **스킬은 기능 단위로 격리** — SKILL.md(메타) + executor.py(구현) + prompts(LLM 지시) 3계층
3. **LLM 오케스트레이션은 Python에서 결정론적으로 통제** — critique 루프, 선행 자동 위임, 결정론적 JSON 파싱
4. **이슈 생성은 항상 사전 확인 → 분기 처리** — 중복 생성 방지, 폐기는 마킹 우선
5. **산출 문서 본문은 채팅에 출력하지 않음** — 파일 저장 + 1~3줄 요약 보고
6. **`prompts/system.txt`이 단일 진실 출처** — 이슈 생성 규칙, 응답 스타일, 워크플로우 처리 지침 통합
7. **상태 관리는 Belief State(수준 2) 까지 현재 채택** — LLM 이 매 턴 프로젝트의 진실 상태를 알고 판단. 수준 3(PDCA) 은 목표 모호성·피드백 루프 시나리오에만 선택 적용

---

## 부록: 관련 파일

- 시스템 프롬프트: [`prompts/system.txt`](../Agent-Service/prompts/system.txt)
- 요구분석 워크플로우: [`prompts/workflow-requirement-analysis.md`](../Agent-Service/prompts/workflow-requirement-analysis.md)
- 구조설계 워크플로우: [`prompts/workflow-architecture-design.md`](../Agent-Service/prompts/workflow-architecture-design.md)
- 에이전트 라우터: [`routers/agent.py`](../Agent-Service/routers/agent.py)
- 스킬 레지스트리: [`skills/registry.py`](../Agent-Service/skills/registry.py)
- 스트리밍 오케스트레이터: [`agents/streaming.py`](../Agent-Service/agents/streaming.py), [`agents/orchestrator.py`](../Agent-Service/agents/orchestrator.py)
