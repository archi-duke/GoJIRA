# Agent-Service 아키텍처 설계 문서 (OpenCode 기반)

본 문서는 GoJIRA AI 에이전트 백엔드의 동작 구조, 설계 결정을 정리한 설계 자료다. 기존 자체 ReACT 루프 기반 Agent-Service 를 **OpenCode 서버 기반** 으로 전면 전환한다.

---

## 1. 개요

GoJIRA 에이전트는 사용자 자연어 요청을 받아 도메인 스킬을 호출하고, 결과를 한국어로 종합해 응답한다. LLM 호출·ReACT 루프·스킬 디스패치 엔진은 **OpenCode 서버** 가 담당하고, GoJIRA 측은 UI 계약 유지용 얇은 어댑터만 유지한다.

**구성 요소**:
- **UI (`GoJIRA-App/src/components/AgentChat.js`)** — 기존 `POST /Agent/chat/stream` SSE 계약 유지
- **Agent-Service (슬림 어댑터)** — `routers/agent.py` + `opencode_bridge/` — 세션 매핑, SSE 이벤트 번역, 워크플로우 stage 주입
- **OpenCode 서버 (`opencode serve`)** — LLM 호출, ReACT 루프, 스킬 discover/실행
- **스킬 (`.claude/skills/*2/SKILL.md`)** — curl/jq 기반 self-contained, OpenCode 가 자동 discover
- **Platform-API (:9000)** — DocRepo(Git 기반 문서), 사용자 브랜치 (`docs/{user}`)
- **GoJIRA-API (:9001)** — Issue CRUD/Link, JIRA DC v2 호환

---

## 2. 아키텍처

```
┌─────────────────────────────────────────────────────────────────┐
│  Browser (GoJIRA-App) — AgentChat.js                            │
│  POST /Agent/chat/stream ◀── SSE (session/reasoning/            │
│                               tool_call/tool_result/            │
│                               content/done)                     │
└────────────────────────────┬────────────────────────────────────┘
                             │ HTTP + SSE (계약 불변)
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│  Agent-Service (슬림 어댑터, FastAPI)                           │
│   · 세션 매핑 (Mongo: gojiraSessionId ↔ openCodeSessionId)      │
│   · 워크플로우 stage 주입 (system prompt prefix)                │
│   · SSE 이벤트 번역 (OpenCode → UI 포맷)                        │
└────────┬───────────────────────────────┬────────────────────────┘
         │ REST                          │ SSE (GET /event)
         │ POST /session                 │
         │ POST /session/:id/prompt_async│
         ▼                               ▼
┌─────────────────────────────────────────────────────────────────┐
│  ╔═══════════════════════════════════════════════════════════╗  │
│  ║                OpenCode Server (:4096)                    ║  │
│  ║  ┌─────────────────────────────────────────────────────┐  ║  │
│  ║  │ Session Manager   (/session, /message, /event)      │  ║  │
│  ║  │ Primary Agent     ← .opencode/agents/gojira.md      │  ║  │
│  ║  │ LLM Runtime (ReACT 내장, provider ← opencode.json)  │  ║  │
│  ║  │ Skill Registry (기동 시 .claude/skills 자동 스캔)   │  ║  │
│  ║  │ Tool Executor (Bash/Read — curl/jq 스니펫 실행)     │  ║  │
│  ║  └────────────────────┬────────────────────────────────┘  ║  │
│  ╚═══════════════════════╪═══════════════════════════════════╝  │
└──────────────────────────┼──────────────────────────────────────┘
                           │ curl (X-User-Id 헤더)
            ┌──────────────┴──────────────┐
            ▼                             ▼
┌──────────────────────┐      ┌────────────────────────┐
│  Platform-API :9000  │      │  GoJIRA-API :9001      │
│   · DocRepo (Git)    │      │   · Issue CRUD/Link    │
│   · docs/{user} 브랜치│      │   · JIRA DC 호환       │
└──────────────────────┘      └────────────────────────┘

┌─────────── 외부 LLM (OpenCode 가 직접 호출) ───────────┐
│  OpenAI / Anthropic / Gemini / LiteLLM (사내 프록시)   │
└────────────────────────────────────────────────────────┘
```

**데이터 흐름 한 줄**: UI → Agent-Service(어댑터) → OpenCode(LLM+ReACT+스킬) → Platform/GoJIRA API

---

## 3. OpenCode 동작 방식

### 3.1 기동
```
OPENCODE_SERVER_PASSWORD=*** opencode serve --port 4096 --hostname 127.0.0.1
```
- 기동 시 `.opencode/agents/`, `.claude/skills/`, `.opencode/skills/` 자동 스캔 → 내부 레지스트리 적재
- TUI 없음. REST + SSE HTTP 서버로 상주
- OpenAPI 스펙: `GET /doc`

### 3.2 엔드포인트

| 용도 | 엔드포인트 |
|---|---|
| 세션 생성 | `POST /session` `{parentID?, title?}` |
| 메시지(비동기+스트림) | `POST /session/:id/prompt_async` → 204, 이후 `GET /event` 수신 |
| 메시지(동기) | `POST /session/:id/message` `{messageID?, model?, agent?, parts}` |
| 이벤트 스트림 | `GET /event` (SSE, 첫 이벤트 `server.connected`) |
| 세션 조회/삭제 | `GET/DELETE /session/:id` |
| 슬래시 커맨드 | `POST /session/:id/command` |

### 3.3 세션 내부 처리

```
[사용자 메시지 수신]
      ▼
[primary agent 로드 — .opencode/agents/gojira.md 본문 = system prompt]
[등록된 skill 목록을 tool 스키마로 LLM 에 제시]
      ▼
[LLM 호출]  ← opencode.json 의 provider/model
      ▼
LLM 판단:
  (a) 텍스트 응답만       → message.part(text) 이벤트 스트림
  (b) skill 호출 필요     → tool_call 이벤트
                         → 해당 SKILL.md 본문을 agent 컨텍스트에 주입
                         → Bash/Read 권한으로 curl/jq 스니펫 실제 실행
                         → tool_result 이벤트
                         → 루프 재진입 (ReACT 내장)
      ▼
[최종 답변 완료] → message.updated(completed)
```

즉 OpenCode 가 기존 Agent-Service 의 ReACT 루프 + LLM 호출 + 스킬 디스패치 역할을 **통째로 대체**.

---

## 4. 스킬 체계

### 4.1 저장 위치 (OpenCode 자동 discover)
- `.claude/skills/<name>/SKILL.md` — **본 프로젝트 기본** (Claude Code 호환)
- `.opencode/skills/<name>/SKILL.md` — OpenCode 전용 (필요 시)
- `~/.config/opencode/skills/<name>/SKILL.md` — 글로벌

OpenCode 필수 frontmatter: `name`, `description`. `allowed-tools` 는 Claude Code 와 동일하게 해석. Unknown 필드는 무시.

### 4.2 스킬 구조 (self-contained)
```
.claude/skills/<name>/SKILL.md
---
name: <name>
description: <1~1024자>
argument-hint: "<projectCode> <user>"
allowed-tools: Bash, Read
---
# 본문: curl/jq 기반 실행 지시문
```

Python executor 없음. LLM 이 SKILL.md 본문을 따라 Bash 로 curl 실행.

### 4.3 현재 스킬 목록 (skill_v2 브랜치)

**워크플로우 스킬 (self-loop 내장)**
- `vos-collect2` — VOS 수집
- `usecase-analyze2` — Use Case/Feature 분석 (VOS 커버리지 매트릭스 자가검증)
- `requirement-refine2` — FR/NFR/CNST 요구사항 정제
- `driver-select2` — Requirement 중 ASR 마킹 (ArchitectureDriver 이슈 생성하지 않음)
- `style-define2` — 아키텍처 스타일 추천 (카탈로그 점수 규칙을 본문에 서술)
- `design-decide2` — ADR 기반 설계 결정
- `design-integrate2` — 통합 설계 문서 작성 (이슈 없음)
- `satam-analyze2` — ADSN risk/sensitivity/mitigation 필드 업데이트

**이슈 생성 스킬 (타입별 분리)**
- `issue-create-vos`
- `issue-create-feature`
- `issue-create-requirement`
- `issue-create-design`

**프리폼 스킬**
- `projdesc-interview` — 5개 질문 뱅크 기반 대화형
- `projdesc-generate` — 비대화 문서 생성
- `doc-history`
- `reset-project` — `disable-model-invocation: true` + dry-run 기본 + `confirm=true` 가드

**이식 보류**
- `ui-navigate` — Agent-Service SSE → 브라우저 BroadcastChannel 의존. Platform-API 에 `POST /api/events/navigate` 엔드포인트 신설 필요

### 4.4 Self-loop 정책 (워크플로우 스킬 공통)
- `score_effective = score − 0.5 × |critical|`, pass 임계 0.8
- 최대 3 iter (초안 1 + 수정 2)
- 종료 조건: pass | iter 3 소진 | 같은 critical 2회 연속 재발 | 새 수정이 이전 best 보다 낮은 score
- **best-so-far**: 매 iter 의 `(items, score_effective)` 기록 → 최댓값 iter 채택
- LLM 단일 호출 내부에서 generate → self-critique → revise 수행

### 4.5 공통 규약
- 모든 쓰기 curl 에 `X-User-Id: ${USER}` 헤더 필수 (403 방지)
- 문서 저장: `PUT /DocRepo/{code}/file` + X-User-Id → 자동으로 `docs/{user}` 브랜치
- 이슈 생성: `POST /Issue` → 응답 key 수신 → 부가 필드 `PUT /Issue` 로 업데이트
- 링크 타입: `Contains(WBSGantt)` (JIRA DC 표준)
- Mermaid 코드블록 필수 (`graph TB` for Use Case Diagram — `classDiagram` 금지)

---

## 5. Primary Agent 정의

`.opencode/agents/gojira.md`

```yaml
---
description: GoJIRA 프로젝트 관리 도우미. 이슈·문서·워크플로우를 실행한다.
mode: primary
model: <opencode.json 기본값>
permission:
  bash: allow
  edit: ask
---
[역할·규칙 본문]
- 사용자 입력에 프로젝트 코드(GOJIRA*, 또는 명시적 code) 포함 여부 확인
- 이슈 생성 전 issue_query 로 유사 이슈 사전 확인, 분기 처리
- 산출 문서 본문은 채팅에 출력하지 말고 파일 저장 + 요약 보고
- 현재 워크플로우 stage (어댑터 주입) 안에서만 동작
- 응답은 한국어, 간결, 친근한 톤
```

---

## 6. Agent-Service 어댑터 (슬림 레이어)

### 6.1 유지되는 API (UI 계약 불변)

| 엔드포인트 | 동작 |
|---|---|
| `POST /api/v2/Agent/chat/stream` | OpenCode 세션에 메시지 전달 + SSE 이벤트 번역 |
| `POST /api/v2/Agent/chat` | 동기 버전 (내부적으로 `/session/:id/message`) |
| `GET /api/v2/Agent/sessions` | Mongo 세션 목록 |
| `GET /api/v2/Agent/session/:id` | 세션 복원 |
| `GET /api/v2/Agent/tools` | OpenCode skill 레지스트리를 UI 포맷으로 변환 |

### 6.2 SSE 이벤트 매핑

| OpenCode (GET /event) | UI SSE |
|---|---|
| `server.connected` | (skip) |
| `session.updated` | `session` `{sessionId}` |
| `message.part.updated` (reasoning) | `reasoning` `{turn, step, content}` |
| `message.part.updated` (tool_call) | `tool_call` + `progress` |
| `message.part.updated` (tool_result) | `tool_result` `{tool, status, duration_ms, summary}` |
| `message.part.updated` (text delta) | `content` `{text}` |
| `message.updated` (completed) | `done` `{reply, actions}` |
| error | `error` `{message}` |

### 6.3 세션 매핑
Mongo `agent_sessions` 컬렉션에 `{gojiraSessionId, openCodeSessionId, userId, createdAt, messages[]}`. 첫 요청 시 OpenCode `POST /session` 호출 결과 ID 를 매핑 저장.

### 6.4 사용자 컨텍스트 (`X-User-Id`)
스킬이 curl 에 `X-User-Id: ${USER}` 를 사용. 전달 방식:
- 어댑터가 `/prompt_async` 호출 시 parts 에 `user=batman` 컨텍스트 prefix 포함
- 스킬 본문은 `$USER` 또는 인자 `$2` 로 수신 → 기존 방식 유지

---

## 7. 워크플로우 상태 관리 (State Machine)

단순 선형 워크플로우에서도 "LLM 이 본인이 어느 단계인지 매번 추론" 은 순환·누락을 유발. **엔진이 통제권을 갖고, LLM 은 현재 stage 내부만 수행** 하는 구조 유지.

### 7.1 구성

| 구성 | 위치 | 책임 |
|---|---|---|
| 워크플로우 정의 | `Agent-Service/core/workflows/business_design.py` | 9-stage 시퀀스 (projdesc → vos → usecase → requirement → driver → style → design_decide → design_integrate → satam) |
| Job 저장소 | `Agent-Service/core/job_store.py` + `workflow_jobs` 컬렉션 | 활성 Job + stage 상태/히스토리 영속화 (TTL 90일) |
| 실행 엔진 | `Agent-Service/core/workflow_engine.py` | load/start, advance, belief-state 검증, 전이 |
| Belief State | `Agent-Service/core/workflow_state.py` | `docs/{user}` 브랜치 우선 조회 |
| stage 주입 | 어댑터 `opencode_bridge/` | OpenCode 세션 메시지에 `[엔진 상태] 현재 stage=X / 호출할 스킬=Y` 시스템 메시지 prefix |

### 7.2 실행 흐름

1. 사용자 입력에서 프로젝트 코드 감지 → `load_or_start_job`
2. Belief State 조회 → `[엔진 상태]` system 메시지 구성
3. 어댑터가 OpenCode `POST /session/:id/prompt_async` 호출 시 메시지 prefix 로 삽입
4. OpenCode 가 stage skill 호출 → tool_result 도착
5. 어댑터가 `advance(skill_result)` → belief state 재검증 → 전이
6. 전이 성공 시 다음 stage 지시를 system 메시지로 추가 주입 (같은 세션 내 연속 진행)

### 7.3 핵심 설계 결정
- **엔진이 통제, LLM 은 stage 내부만** — 반복 루프·단계 건너뛰기 방지
- **Belief State 는 `docs/{user}` 브랜치 1순위** — agent 쓰기 브랜치와 판정 출처 일치
- **advance 는 belief state 기반 completion 판정** — 보조 skill 이 사실상 stage 완료시킨 경우에도 전이
- **`/btw` prefix** — 그 turn 만 workflow 우회 (Job/대화 유지, advance 스킵)

### 7.4 수준별 상태 관리 (참고)

| 수준 | 이름 | 적용 |
|---|---|---|
| 1 | Scratchpad (ReACT 내부) | OpenCode 가 세션 내부에서 자동 처리 |
| 2 | Belief State | 어댑터가 stage 주입 시 프로젝트 진실 상태 포함 |
| 3 | Workflow State Machine | 엔진 통제로 stage 전이 |
| 3+ | PDCA (목표 모호성·피드백 루프) | 선택 적용, 미래 과제 |

---

## 8. LLM 설정

**OpenCode 기본 설정 체계를 그대로 사용**. Agent-Service 쪽 LLM 설정 레이어(`llm.json`, `/Agent/llm` 전환 API) 는 **전면 제거**.

- 설정 파일: `opencode.json` (프로젝트 루트) / `~/.config/opencode/opencode.json` (글로벌)
- 병합 우선순위는 OpenCode 문서 그대로 (remote < global < custom < project < inline < managed)
- 프로바이더·모델·API 키 관리 모두 OpenCode 규약 (`{env:...}`, `{file:...}` substitution 포함)
- UI 에서 프로바이더/모델 전환이 필요하면 향후 요구 발생 시 OpenCode 의 공식 방식(설정 파일 편집 or 제공되는 API) 에 맞춰 검토. 현재 단계에서는 **GoJIRA 측 전용 전환 UI 미제공**

---

## 9. 제거되는 구성 요소

OpenCode 채택으로 Agent-Service 는 다음을 **제거**:

- `Agent-Service/agents/react.py` — ReACT 루프 (OpenCode 내장)
- `Agent-Service/agents/streaming.py` — 2-pass 추론 (OpenCode 내장)
- `Agent-Service/agents/orchestrator.py`
- `Agent-Service/core/llm.py` + `llm.json` — LLM 어댑터 (opencode.json 대체)
- `Agent-Service/skills/registry.py` — 스킬 선별 (OpenCode discover + description)
- `Agent-Service/skills/dispatcher.py` — 디스패치 (OpenCode tool executor)
- `Agent-Service/skills/<name>/executor.py` — 모든 Python executor (스킬 self-contained)
- `Agent-Service/skills/<name>/prompts/` — 프롬프트 (SKILL.md 내부 흡수)
- `Agent-Service/skills/critique.py` — critique 루프 (스킬 내부 self-loop)
- LLM 프로바이더 전환 UI/API (`/Agent/llm`) — OpenCode 설정 체계로 일원화
- `Agent-Service/skills/mermaid_utils.py` — Mermaid 검증 (SKILL.md 규약으로)
- `Agent-Service/prompts/system.txt` — system 프롬프트 (primary agent md 로 이관)

**유지**:
- `routers/agent.py` (축소)
- `core/workflow_engine.py`, `core/workflow_state.py`, `core/job_store.py`, `core/workflows/`
- `opencode_bridge/` (신설)
- Mongo 세션 저장소

**규모 예상**: 현재 1만+ 줄 → 1~2천 줄

---

## 10. 이행 계획 (Phase)

### Phase 0 — 스파이크 (3~4일)
- [ ] `opencode serve` 기동, `/doc` OpenAPI 저장 → 이벤트명/payload 확정
- [ ] `.opencode/agents/gojira.md` + 세션 생성 + 메시지 송신 smoke test
- [ ] `.claude/skills/vos-collect2` 가 그대로 invoke 되는지 (user 인자 전달 방식 검증)
- [ ] SSE 이벤트 번역 최소 어댑터 (FastAPI 100줄) 작성
- [ ] UI 변경 없이 end-to-end 확인

### Phase 1 — 어댑터 본체
- `opencode_bridge/` 구현 (세션 매핑, 이벤트 번역, stage 주입)
- `routers/agent.py` 의 `/chat/stream` 을 어댑터 호출로 교체
- Mongo 세션 영속화 연결
- LLM 설정은 OpenCode `opencode.json` 에 직접 구성 (GoJIRA 측 설정 레이어 없음)

### Phase 2 — 워크플로우 엔진 통합
- `workflow_engine` + `workflow_state` 를 어댑터 레이어에서 호출
- stage 주입 메시지 포맷 확정
- `/btw` prefix 라우팅

### Phase 3 — 정리
- `Agent-Service/skills/`, `agents/`, `core/llm.py`, `llm.json`, `prompts/system.txt` 제거
- 문서·테스트 갱신
- main 머지

### Phase 4 — 후속
- `ui-navigate` 부활 (Platform-API `POST /api/events/navigate` 엔드포인트 신설 후)
- 토큰 사용량 / 품질 / 속도 실측 비교
- PDCA 확장 (목표 모호성 시나리오에만 선택 적용)

---

## 11. 리스크 / 검증 포인트

1. **OpenCode API 안정성** — 버전 고정 필수 (`opencode --version` 기록)
2. **SSE 이벤트 의미 매칭** — `tool_call` 세밀도가 Agent-Service `progress` 를 대체 가능한지
3. **멀티 세션 병행** — OpenCode 동시 세션 한도/격리
4. **스킬 내 `X-User-Id`** — 실행 환경 env 주입 방식
5. **워크플로우 stage 주입** — OpenCode 세션에 system 메시지 추가 삽입 가능 여부
6. **인증/포트/로컬 바인딩** — OpenCode 서버 외부 노출 차단 (`127.0.0.1` + password)
7. **스킬 discover 경로 우선순위** — `.claude/skills/` vs `.opencode/skills/` 충돌 방지
8. **API 스키마 미확인 사항**:
   - `POST /Issue` 가 priority/assignee 직접 수용 vs PUT 분리 필요
   - NFR `environment` 필드 customfield 매핑
   - `PUT /Project/${CODE}` archSettings.chosenStyles 엔드포인트 실존 여부

---

## 12. 핵심 설계 원칙

1. **LLM 런타임은 OpenCode 에 위임** — ReACT/스킬 디스패치 자체 구현 폐기, UI 계약만 유지
2. **스킬은 self-contained SKILL.md** — Python executor 없음, curl/jq 로 완결
3. **스킬 경로 표준은 `.claude/skills/`** — Claude Code/OpenCode 양쪽 호환, 다른 하네스 이식 용이
4. **LLM 설정은 OpenCode 체계 그대로** — GoJIRA 측 별도 설정 레이어 없음, `opencode.json` 이 단일 진실 출처
5. **엔진이 워크플로우 stage 통제, LLM 은 stage 내부만** — 순환/누락 방지, tool 화이트리스트
6. **Belief State 는 `docs/{user}` 브랜치 1순위** — agent 쓰기/읽기 출처 일치
7. **이슈 생성은 사전 확인 → 분기** — 중복 방지, 폐기는 마킹 우선
8. **산출 문서는 파일 저장, 채팅엔 요약만**

---

## 부록: 관련 파일

- Primary agent: `.opencode/agents/gojira.md`
- 프로바이더 설정: `opencode.json`
- 스킬: `.claude/skills/*2/SKILL.md`
- 어댑터: `Agent-Service/opencode_bridge/` (신설)
- 라우터: `Agent-Service/routers/agent.py` (축소)
- 워크플로우: `Agent-Service/core/workflow_engine.py`, `core/workflow_state.py`, `core/workflows/business_design.py`
- UI: `GoJIRA-App/src/components/AgentChat.js`
