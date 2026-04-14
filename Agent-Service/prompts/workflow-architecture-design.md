# 구조설계 워크플로우 (Architecture Design Workflow)

소프트웨어 구조설계의 전체 흐름을 정의한다.
에이전트가 대규모 설계 작업을 수행할 때 이 워크플로우를 참조하여 단계별로 진행한다.

---

## 전체 흐름

```
Phase 1: 요구 분석
  ① 과제 소개 분석 → ② VOS 수집 → ③ UseCase 분석 → ④ Requirements 정제

Phase 2: 구조 결정
  ⑤ Architecture Driver 선정 → ⑥ 시스템 스타일 정의

Phase 3: 설계 구체화
  ⑦ 디자인 포인트 선정 및 설계안 결정 → ⑧ 디자인 통합

Phase 4: 설계 평가
  ⑨ Architecture 평가 (SATAM)
```

---

## 공통 원칙 — 생성 단계의 검증 루프

모든 생성 단계(VOS 도출, Use Case 분석, Requirement 정제, 설계안 결정, SATAM 평가 등)는 다음 루프를 거친다:

```
[생성] → [검증(critique)] ──pass?──▶ 완료
            │
            └─ fail → [수정] → [재검증]  (최대 N회)
```

- 각 단계는 `**검증 기준**:`(criteria)과 `**검증 정책**:`(policy) 섹션을 가진다 — 단일 진실 출처(SSOT)
- 코드는 워크플로우 문서에서 두 섹션을 추출해 critique LLM 프롬프트에 그대로 주입
- 평가 결과는 `{"score": 0.0~1.0, "pass": bool, "critical": [...], "suggestions": [...], "reasoning": "..."}` JSON
- pass 판정: `critical`이 비어있고 `score >= threshold`일 때 통과
- **검증 정책**은 글로벌 기본값을 따르며, 단계별 예외만 명시
- 최종 산출물에 `critiqueTrail`을 포함해 검증 이력 추적

**글로벌 기본 정책** (`prompts/system.txt`에서 관리, 단계별 예외가 우선):
- 점수 임계값 0.8 이상까지 반복 (최대 2회 수정)
- 최종 실패 시 마지막 생성물을 그대로 사용하되 사용자에게 임계값 미달 알림
- 평가 파싱 실패 시 현재 결과로 종료 (무한 루프 방지)
- 하드 상한: max_iter 5회 클램프

**공용 헬퍼**: `skills/critique.py` — `iterate_generate_critique(...)`, `load_step_policy(workflow_md, step_heading)`

표준 섹션 형식 및 정책 파싱 규칙 상세는 [`docs/design-workflow.md`](../../docs/design-workflow.md) 참조.

현재 구현 상태:
- ✅ Phase 1 (VOS/Use Case/Requirement) — `vos_collect`, `usecase_analyze`, `requirement_refine`에 적용됨
- ⏳ Phase 2~4 — 구조 결정/설계/SATAM 스킬은 향후 추가 시 동일 표준 적용

Phase 1 단계별 검증 기준/정책은 [`workflow-requirement-analysis.md`](./workflow-requirement-analysis.md) 참조.

---

## Phase 1: 요구 분석

> **상세 워크플로우**: [`workflow-requirement-analysis.md`](./workflow-requirement-analysis.md)
> ②~④ 단계의 구체적 절차(VOS 도출 → Use Case/Feature 분석 → Requirement 정제), 산출 문서 양식, 자동 결정된 NFR Measure의 검토 흐름, 단계별 검증 기준/정책 등을 다룬다.

### ① 과제 소개 분석 (Project Description Analysis)

**목적**: 과제의 배경, 목적, 이해관계자, 핵심 요구를 파악한다.

**입력**: 과제 관련 정보 (사용자 인터뷰 또는 기존 문서)
**산출물**: `business/project-description.md`

**수행 방법**:
- 문서가 없으면 `projdesc_interview` → `projdesc_generate`로 생성
- 기존 문서가 있으면 `projdesc_generate(mode=enrich)`로 보강
- 과제 배경, 과제 목적, 이해관계자, 주요 사항을 명확히 기술

**완료 기준**: project-description.md에 배경/목적/이해관계자/주요사항이 모두 기술됨

---

### ② VOS 수집 (Voice of Stakeholder)

**목적**: 이해관계자별 요구사항을 식별하고 VOS 이슈로 등록한다.

**입력**: `business/project-description.md`
**산출물**: VOS 이슈 목록

**수행 방법**:
- `vos_collect` 스킬로 project-description.md를 분석
- 이해관계자(사용자, 관리자, 운영자, 외부 시스템 등) 관점에서 요구사항 추출
- 각 VOS에 client(요구 제기자) 명시

**이슈 타입**: `VOS`

**완료 기준**: 주요 이해관계자별 VOS가 1개 이상 등록됨

---

### ③ UseCase 분석 (Feature 도출)

**목적**: VOS를 기반으로 시스템이 제공할 기능(Feature)을 유즈케이스 관점에서 도출한다.

**입력**: VOS 이슈 목록
**산출물**: Feature 이슈 목록

**수행 방법**:
- 각 VOS를 "사용자가 시스템으로 무엇을 하는가" 관점에서 분석
- 유사한 VOS를 묶어 하나의 Feature로 통합
- Feature는 시스템이 제공하는 기능 단위 (유즈케이스 수준)
- 각 Feature를 관련 VOS의 하위로 `contain` 관계로 연결 (VOS가 Feature를 포함)

**이슈 타입**: `Feature`
**관계**: VOS —(contains)→ Feature

**완료 기준**: 모든 VOS가 최소 1개의 Feature에 연결됨

---

### ④ Requirements 정제 (Requirements Refinement)

**목적**: Feature를 구현하기 위한 구체적 요구사항을 도출하고 분류한다.

**입력**: Feature 이슈 목록, VOS 이슈 목록
**산출물**: Requirement 이슈 목록 (FR/NFR/CNST)

**수행 방법**:
- 각 Feature에서 기능 요구사항(FR)을 도출
- VOS와 과제 특성에서 품질속성 요구사항(NFR)을 도출
- 기술적/비즈니스적 제약사항(CNST)을 식별
- 각 Requirement를 관련 Feature의 하위로 `contain` 관계로 연결 (Feature가 Requirement를 포함)

**이슈 타입**: `Requirement`
**세부 분류**:
- `FR` — 기능 요구사항: 시스템이 수행해야 할 기능
- `NFR` — 품질속성 요구사항: 성능, 보안, 가용성, 유지보수성 등
- `CNST` — 제약사항: 기술 스택, 표준, 규제 등 변경 불가한 조건

**관계**: Feature —(contains)→ Requirement

**완료 기준**: 각 Feature에 FR이 1개 이상, NFR/CNST가 식별 가능한 만큼 등록됨

---

## Phase 2: 구조 결정

### ⑤ Architecture Driver 선정

**목적**: 아키텍처 결정에 가장 큰 영향을 미치는 핵심 요구사항을 선별한다.

**입력**: Requirement 이슈 목록 (FR/NFR/CNST)
**산출물**: Architecture Driver 목록 (Requirement 이슈에 driver 표시)

**수행 방법**:
- NFR 중 시스템 구조에 직접 영향을 미치는 품질속성을 선별
- FR 중 구조적 복잡도가 높은 기능을 선별
- CNST 중 아키텍처 선택을 제한하는 제약을 선별
- 각 후보 Requirement에 대해 다음 두 지표를 1~5점으로 평가:
  - **QAD (Quality Attribute Demand, 중요도)**: 비즈니스/품질 관점에서 해당 요구사항이 시스템에 미치는 영향도
  - **QAI (Quality Attribute Impact, 구현 난이도)**: 해당 요구사항을 만족시키기 위한 기술적 난이도 및 구조적 복잡도
- **우선순위 산정**: QAD와 QAI의 가중평균으로 결정 (기본 가중치 5:5)
  - `Priority = QAD × 0.5 + QAI × 0.5`
  - 프로젝트 특성에 따라 가중치를 조정할 수 있음 (예: 비즈니스 중심이면 QAD 비중 상향)
- 우선순위가 높은 순서대로 Architecture Driver를 선정
- 선정된 Requirement에 risk/sensitivity를 설정

**검증 기준**:
- NFR/FR/CNST 각 영역에서 누락 없이 후보가 검토되었는가
- QAD/QAI 점수에 명확한 근거가 제시되었는가 (단순 추정/임의값 금지)
- Driver 선정 결과가 시스템 핵심 품질속성을 고르게 대표하는가 (편향 없음)
- 우선순위 산정 가중치가 프로젝트 특성과 일치하는가

**검증 정책**: 기본 정책 적용

**완료 기준**: 5~10개 내외의 Architecture Driver가 식별되고, QAD/QAI 평가 및 우선순위가 지정됨

---

### ⑥ 시스템 스타일 정의 (Architecture Style)

**목적**: 시스템 전체의 구조적 스타일을 결정한다.

**입력**: Architecture Driver 목록
**산출물**: `architecture.md` — Architecture Style 섹션

**수행 방법**:
- Architecture Driver를 만족시킬 수 있는 스타일 후보를 검토
  - 예: Layered, Microservice, Event-Driven, Pipe-and-Filter, Client-Server 등
- Driver와의 적합성을 기준으로 스타일을 선정
- 선정 근거를 문서화

**검증 기준**:
- 후보 스타일 2개 이상을 비교 검토했는가
- 선정 스타일이 주요 Architecture Driver를 모두 만족시킬 수 있는가
- 트레이드오프(선정되지 않은 스타일의 약점/강점)가 명시되어 있는가
- 선정 근거가 Driver와 1:1로 추적 가능한가

**검증 정책**: 기본 정책 적용

**완료 기준**: architecture.md에 Architecture Style과 선정 근거가 기술됨

---

## Phase 3: 설계 구체화

### ⑦ 디자인 포인트 선정 및 설계안 결정 (Architectural Approach)

**목적**: 주요 설계 관심사(Design Point)를 식별하고, 각각에 대한 구체적 설계안을 결정한다.

**입력**: Architecture Driver, Architecture Style
**산출물**: Architectural Design 이슈 목록

**수행 방법**:
- Architecture Driver에서 설계 결정이 필요한 포인트를 식별
  - 예: 데이터 관리 전략, 인증/인가 구조, 통신 패턴, 배포 전략 등
- 각 디자인 포인트별 설계 대안(Approach)을 검토
- 트레이드오프를 분석하여 설계안을 결정
- 결정된 설계안을 ADSN 또는 DDSN 이슈로 등록
- 관련 Requirement의 하위로 `contain` 관계로 연결

**이슈 타입**:
- `ADSN` (Architectural Design) — 아키텍처 수준의 설계 결정 (구조, 스타일, 패턴)
- `DDSN` (Detailed Design) — 상세 수준의 설계 결정 (알고리즘, 데이터 구조, 인터페이스)

**관계**: Requirement —(contains)→ ADSN / DDSN

**검증 기준**:
- 모든 주요 Architecture Driver가 최소 1개의 디자인 포인트로 다루어졌는가
- 각 디자인 포인트에 대안(approach) 2개 이상이 비교되었는가
- 선정 설계안의 트레이드오프가 명시되어 있는가
- ADSN/DDSN 분류가 적절한가 (ADSN은 구조 결정, DDSN은 상세 결정)
- Requirement 와의 contain 관계가 누락 없이 연결되었는가

**검증 정책**: 기본 정책 적용

**완료 기준**: 주요 디자인 포인트별 설계안이 결정되고 근거가 기술됨

---

### ⑧ 디자인 통합 (Integrated Design Views)

**목적**: 개별 설계안을 통합하여 다양한 관점에서 아키텍처를 설명한다.

**입력**: ADSN/DDSN 이슈 목록, Architecture Style
**산출물**: `architecture.md` — Integrated Design Views 섹션

**수행 방법**:
- 개별 설계안을 통합하여 다음 관점으로 정리:
  - **논리적 관점 (Logical View)**: 주요 컴포넌트와 책임
  - **프로세스 관점 (Process View)**: 런타임 동작, 동시성, 통신
  - **배포 관점 (Deployment View)**: 물리적 배치, 인프라
  - **개발 관점 (Development View)**: 모듈 구조, 패키지 의존성
- 관점 간 일관성을 검증
- architecture.md에 통합하여 기술

**검증 기준**:
- 최소 2개 이상의 관점이 기술되었는가 (논리/프로세스/배포/개발 중)
- 각 관점이 ADSN/DDSN 결정과 일관되는가
- 관점 간 모순 (예: 논리 모듈 ↔ 배포 단위 충돌) 없이 통합되었는가
- 다이어그램/도식이 서술과 일치하는가

**검증 정책**: 기본 정책 적용

**완료 기준**: architecture.md에 최소 2개 이상의 관점에서 통합 설계가 기술됨

---

## Phase 4: 설계 평가

### ⑨ Architecture 평가 (SATAM)

**목적**: 각 설계안이 다양한 Requirement에 미치는 영향도를 분석하고 리스크를 평가한다.

**입력**: ADSN/DDSN 이슈, Requirement(NFR) 이슈
**산출물**: SATAM 매트릭스

**수행 방법**:
- `satam_analyze` 스킬로 분석 수행
- 각 ADSN이 NFR에 미치는 영향을 평가:
  - **Sensitivity**: 해당 설계 결정이 품질속성에 민감한 정도
  - **Risk**: 설계 결정에 따른 위험 요소
  - **Mitigation**: 위험 경감 조치
- 미경감 리스크가 있으면 설계안 재검토 (⑦로 피드백)

**검증 기준**:
- 모든 Architecture Driver(특히 NFR)가 매트릭스에 포함되었는가
- 각 sensitivity/risk 평가에 정성적/정량적 근거가 있는가
- 식별된 risk마다 mitigation이 제시되거나 수용 근거가 명시되었는가
- 미경감 risk가 남아 있을 때 ⑦ 피드백 필요성이 명시되었는가

**검증 정책**: 점수 임계값 0.85 이상까지 반복 (최대 3회), 최종 실패 시 사용자에게 알리고 워크플로우 중단

**완료 기준**: 모든 Architecture Driver에 대해 sensitivity/risk가 평가되고, 미경감 리스크가 없거나 수용 근거가 기술됨

---

## 이슈 체계 (Contain Chain)

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
| CNST (제약) | — | 하위 이슈 불필요 (제약 조건 자체로 완결) |

- **ASR (Architecturally Significant Requirement)**: 아키텍처에 중대한 영향을 미치는 요구사항으로, Requirement 이슈에 `ASR` 라벨을 부여하여 표시
- NFR은 본질적으로 아키텍처 수준 결정이 필요하므로, ASR 라벨 유무와 관계없이 반드시 ADSN을 가짐
- CNST는 변경 불가한 제약이므로 별도 설계 이슈를 두지 않음

## 스킬 매핑

| 단계 | 관련 스킬 |
|------|----------|
| ① 과제 소개 분석 | `projdesc_interview`, `projdesc_generate` |
| ② VOS 수집 | `vos_collect` |
| ③ UseCase 분석 | `usecase_analyze` (산출: `business/use-cases.md`) |
| ④ Requirements 정제 | `requirement_refine` (산출: `business/requirements.md`) |
| ⑤ Architecture Driver 선정 | `issue_query`, `issue_update` |
| ⑥ 시스템 스타일 정의 | *(문서 작성)* |
| ⑦ 설계안 결정 | `issue_create_design`(designLevel: Architectural/Detailed), `issue_link` |
| ⑧ 디자인 통합 | *(문서 작성)* |
| ⑨ Architecture 평가 | `satam_analyze` |
| 공통 | `project_query`, `issue_analyze`, `doc_list`, `doc_summarize` |
