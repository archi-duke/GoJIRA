# Project Description — GoJIRA

> 본 문서는 GoJIRA 자체 프로젝트(플랫폼 개발 과제)의 설명입니다.
> GoJIRA 가 관리하는 사용자 프로젝트별 `business/project-description.md` 와는 별개입니다.

## 과제 배경

JIRA Datacenter 는 사내 표준 이슈 관리 도구로 자리 잡았으나, SW/시스템 엔지니어링 프로젝트 운영 측면에서 다음 한계가 누적적으로 드러났습니다.

- **이슈 ↔ 설계 문서 단절**: JIRA 의 이슈와 별도 위키/공유 드라이브의 설계 문서가 분리되어 있어, 변경 추적과 일관성 유지가 수동·인력 의존적입니다.
- **요구사항 → 아키텍처 → 설계 단계의 추적성 부재**: VOS(이해관계자 요구) → Use Case/Feature → Requirement(FR/NFR/CNST) → Architecture Driver → Style/Design 의 흐름을 일관되게 추적하는 도구가 없어, 결정 근거가 휘발됩니다.
- **반복적 산출물의 수작업 부담**: 문서 초안 작성, 다이어그램(PlantUML) 작성, QA 가중 평가 등 정형화 가능한 작업이 매번 사람의 시간을 먹습니다.
- **외부 SaaS LLM 직접 사용의 정책적 제약**: 사내 보안 정책상 OpenAI/Anthropic 등 외부 LLM 을 클라이언트가 직접 호출하는 구조는 채택이 어렵습니다. 키 관리·감사·게이트웨이 통제가 가능한 자체 인프라가 요구됩니다.
- **단일 LLM 벤더 런타임 종속의 위험**: Claude Agent SDK 등 단일 벤더 런타임을 채택하면 모델 교체·장애·정책 변경에 취약해집니다. 모델 중립적 실행 환경이 필요합니다.

## 과제 목적

**JIRA Datacenter 호환 이슈 관리 + Git 기반 설계 문서 + AI 에이전트 워크플로우 자동화** 를 단일 인터페이스로 제공하는 자체 호스팅 플랫폼을 구축합니다.

핵심 가치는 다음 세 가지입니다.

1. **Single Pane of Glass**: 이슈, 설계 문서, AI 에이전트 대화, 진행 현황 알림을 하나의 SPA 에서 다룹니다.
2. **Traceable Design Workflow**: VOS → Feature → Requirement → Driver → Style/Design 까지 이슈 링크 + Git 커밋으로 결정 근거를 영구 보존하고, 변경 영향(drift) 을 자동 감지합니다.
3. **Skill-Driven AI 자동화**: 자연어 요청을 받아 선언형 스킬(`SKILL.md`) 을 디스패치하는 에이전트로, 워크플로우 단계별 산출물을 LLM + 결정적 도구 조합으로 자동 생성합니다.

데이터·LLM 키·문서는 모두 자체 인프라(MongoDB, Gitea, 사내 LLM 게이트웨이) 에 보관되며, 외부 의존성은 정책 통제 가능한 단일 게이트웨이를 통해서만 발생합니다.

## 주요 이해관계자

| 이해관계자 | 역할 | 관심사 |
|---|---|---|
| **SW/시스템 엔지니어** | 일차 사용자 — 이슈 관리, 설계 문서 작성, AI 에이전트 호출 | 자연어 인터페이스 효율, 산출물 품질, 다이어그램 렌더 정확성 |
| **프로젝트 매니저** | 프로젝트 단위 운영자 | 진행률, 일정 지연 리스크, 알림, 양방향 JIRA 동기화 |
| **아키텍트** | 워크플로우 산출물 검수자 | Driver→Style→Design 추적성, ADSN/DDSN 결정 근거, SATAM 매트릭스 |
| **운영자(인프라)** | 자체 호스팅 운영 책임 | 서비스 가용성, 포트/네트워크 정책, 백업, 모니터링 (`docs/ports.md` 참조) |
| **보안/감사** | LLM/데이터 거버넌스 | OPENAI_API_KEY 등 시크릿 암호화, LLM 단일 게이트웨이 통제, 시크릿이 git 에 노출되지 않음 |
| **외부 시스템(간접)** | JIRA Datacenter / ADFS SSO / 사내 LLM 게이트웨이(gpt-oss) / Gitea | 표준 프로토콜 준수, 인증서·티켓 처리, 호환성 |

## 주요 사항

### 핵심 기능 요구사항

- **이슈 관리 + JIRA 양방향 동기화**: JIRA DC v2 REST API 호환. Mongo 가 운영 저장소, JIRA 와 양방향 sync. 이슈는 동시에 Gitea 의 YAML 파일로도 영속화되어 git 이력으로 보존됨 (3일 soft-delete 휴지통 정책 포함).
- **Git 기반 설계 문서 (DocRepo)**: Gitea 위에서 사용자별 브랜치(`docs/{user}`) 로 협업, PR/Review/Merge 워크플로우. 문서 편집 세션(편집 락 + heartbeat + conflict-resolve) 지원.
- **AI 에이전트 + 스킬 워크플로우**: 9 stage DAG (`projdesc → vos → usecase → requirement → driver → style → design-decide → design-integrate, satam`). 각 단계는 LLM 생성 + critique loop + 결정적 검증 + drift-check (이전 산출물 hash 기반 변경 감지) 로 구성.
- **LLM 게이트웨이 서비스**: Platform-API 가 `POST /api/v2/LLM/call` 단일 엔드포인트로 LLM 호출을 제공. OpenAI 호환 외부 LLM + 사내 LLM 게이트웨이(gpt-oss-120b, X-Dep-Ticket 인증) 를 동시 라우팅 (`llm.json` 기반 모델 선택). 다른 서비스(GoJIRA-API/Agent-Service) 는 키 없이 이 엔드포인트만 호출.
- **마이크로 프론트엔드**: Platform-App(통합 컨테이너, :3000) 안에 GoJIRA-App(:3001) 을 Module Federation 으로 동적 로드. GoJIRA-App 은 단독으로도 실행 가능.
- **실시간 통신**: SSE 로 에이전트 대화 스트리밍, WebSocket/SSE 로 공지/알람 브로드캐스트.
- **권한·조직**: ADFS SSO + 사용자/조직단위/권한그룹 + 프로젝트별 권한.

### 핵심 비기능 요구사항

| 속성 | 목표 |
|---|---|
| 데이터 주권 | 모든 이슈/문서/세션이 자체 인프라(Mongo + Gitea) 에 저장. 외부 SaaS 의존 없음 |
| 시크릿 보안 | `gojira-shared/secret` 의 AES-256-GCM 으로 `ENC:` 접두사 암호화 저장. 평문 키가 .env / git / 로그에 남지 않음 |
| LLM 호출 통제 가능성 | LLM 호출이 Platform-API 한 곳을 거치므로 키 회수·감사 로그·요청 차단·rate limit 을 단일 지점에서 적용 가능 (게이트웨이 자체는 FR) |
| 모델 중립성 | 단일 벤더 런타임(Claude Agent SDK 등) 회피. OpenCode 기반 + `llm.json` 모델 교체 가능 구조 |
| 실시간 UX | 에이전트 응답 SSE 스트리밍, turn 사이 OpenCode TUI 입력도 즉시 채팅창 반영 |
| 다이어그램 정확성 | PlantUML 결정적 자동 보정(`plantuml-fix.py`) — sequence destroy 화살표/이중 fence 등 6+ 패턴 ~30ms 처리 |

### 기술 스택 (확정)

| 계층 | 기술 |
|---|---|
| Backend (`:9000` Platform-API, `:9001` GoJIRA-API) | Go 1.21+, chi router, Go workspace (`gojira-shared`) |
| Backend (`:9002` Agent-Service) | Python 3.9+, FastAPI, OpenCode bridge slim adapter |
| Frontend | React, Module Federation, AgentChat SSE 스트리밍 |
| Database | MongoDB 7.0 (`gojira` DB) |
| Git Platform | Gitea (self-hosted, Docker, host `:3300` → 컨테이너 `:3000`) |
| LLM Runtime | OpenCode 서버(`:4096`, 사용자 PC 로컬) — ReACT 루프 + 스킬 디스패치를 OpenCode 가 담당 |
| 다이어그램 | PlantUML (Pumlex `:3030`, Docker), 결정적 fix 스크립트 |
| 인증 | ADFS SSO (Platform-App 진입), 서비스 간 `X-User-Id` 헤더 |

전체 포트와 의존 매트릭스는 `docs/ports.md` 가 단일 출처입니다.

### 핵심 아키텍처 결정 (요약)

- **LLM 호출은 Platform-API 가 단독 게이트웨이**. GoJIRA-API 는 `POST /LLM/call` 호출, Agent-Service 는 OpenCode 서버 위에서 동작.
- **Agent-Service 는 슬림 어댑터**: 자체 ReACT 루프 폐기 → OpenCode 서버가 LLM/ReACT/스킬 디스패치 담당. Agent-Service 는 세션 매핑 + SSE 이벤트 번역 + 워크플로우 stage 주입만 수행.
- **스킬은 SKILL.md 단일 파일에 bash/curl 인라인**: LLM 이 그 자리에서 실행하는 프롬프트로 설계 (스크립트 분리 시 인자 추론 비용 + hallucination 증가). 공통 동작(이슈 배치 생성/링크, doc-save 등) 만 `_scripts/` 로 분리.
- **결정적 자동화 + LLM 분할 패턴**: 큰 LLM generation 의 스톨/품질 저하 방지를 위해 Step 분할 + 결정적 조립(LLM 0회) 적용. design-integrate2/usecase-analyze2 가 대표 사례.
- **사용자별 브랜치 기반 협업**: 모든 문서 편집은 `docs/{user}` 브랜치에서 시작하고, main 으로의 반영은 PR + Review 를 거침.

### 제약사항

- **사내 보안 정책**: 외부 SaaS LLM 직접 호출 금지. Platform-API 게이트웨이 + 사내 LLM 게이트웨이(gpt-oss) 만 허용.
- **OpenCode 의 호스트 환경**: OpenCode 서버는 사용자 PC 에서 로컬 실행. Windows 환경에서는 Git Bash + 사내 SSO/프록시 정책에 따른 추가 셋업 필요.
- **JIRA Datacenter 호환**: API 경로/필드 모두 v2 REST 호환. JIRA Cloud 전용 기능은 미지원.
- **단일 벤더 런타임 회피**: Claude Agent SDK 등 사내 정책상 채택 불가. 자체 OpenCode 기반 + 모델 교체 가능한 구조 유지.
- **자체 호스팅 전제**: SaaS 형태 배포 미고려. 모든 의존(Mongo/Gitea/OpenCode/LLM 게이트웨이) 자체 인프라 운영.

> 본 문서는 GoJIRA 플랫폼 자체에 대한 과제 설명이며, 상세 설계는 `docs/design-platform.md`(서비스 전체), `docs/design-agent.md`(에이전트), `docs/design-application.md`(GoJIRA-App), `docs/design-microfrontend.md`(프론트엔드 통합), `docs/design-workflow.md`(워크플로우 정책), `docs/skill-infrastructure-v3.md`(스킬 인프라), `docs/ports.md`(포트 단일 출처) 를 참조합니다.
