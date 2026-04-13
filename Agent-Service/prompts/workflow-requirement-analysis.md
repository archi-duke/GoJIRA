# 요구사항 분석 워크플로우 (Requirement Analysis Workflow)

[`workflow-architecture-design.md`](./workflow-architecture-design.md) **Phase 1 — 요구 분석**의 상세 워크플로우를 정의한다.
`business/project-description.md`를 출발점으로 VOS → Use Case/Feature → Requirement(FR/NFR/CNST)까지 단계별로 진행한다.

---

## 전체 흐름

```
project-description.md
   │
   ▼ ① VOS 도출
VOS 이슈
   │
   ▼ ② Use Case 분석 → Feature 도출 + business/use-cases.md
Feature 이슈 + use-cases.md (Mermaid 다이어그램)
   │
   ▼ ③ Requirement 정제 → FR/NFR/CNST + business/requirements.md
Requirement 이슈 + requirements.md
```

이슈 관계 (contain chain):

```
VOS —(contains)→ Feature —(contains)→ Requirement (FR / NFR / CNST)
```

각 단계는 **생성 → 비판적 평가 → 수정 루프**를 내장한다 (아래 "품질 검증 루프" 섹션 참조).

---

## 품질 검증 루프 (Generate → Critique → Revise)

본 워크플로우의 모든 생성 단계(① ~ ③)는 다음 루프를 거친다:

```
[생성] ──→ [비판적 평가] ──pass?──▶ 완료
            │
            └─ fail ─→ [수정] ─→ [재평가] … (최대 N회)
```

**검증 정책의 단일 진실 출처**: 각 단계의 **검증 기준** / **검증 정책** 섹션이 권위 있는 출처다. executor가 본 문서를 읽어 그 텍스트를 critique LLM 프롬프트에 그대로 주입한다.

**글로벌 기본 정책** (`prompts/system.txt`):
- 평가 점수가 0.8 이상일 때까지 반복
- 최대 2회 수정 후 종료
- 임계값 미달 시 마지막 결과 사용 + 사용자 보고
- 평가 파싱 실패 시 현재 결과로 종료

**단계별 예외**: 각 단계 섹션의 "검증 정책"이 명시되어 있으면 그 정책이 우선. "기본 정책 적용"이라고만 적힌 경우 위 글로벌 정책을 따름.

**공용 헬퍼**: `skills/critique.py` — `iterate_generate_critique(...)`, `load_step_policy(workflow_filename, step_heading)`

**상세 설계**: `docs/design-workflow.md`

---

## ① VOS 도출

**목적**: 비즈니스 배경/목적/이해관계자/핵심 요구/제약/환경을 분석하여 사용자 관점의 요구를 VOS 이슈로 등록한다.

**입력**: `business/project-description.md`
**산출물**: VOS 이슈

**수행 방법**:
- `vos_collect` 스킬 실행
- Stakeholder 및 외부 시스템을 식별
- 사용자 관점 어투("~가 필요해요 / ~해야 합니다 / ~하면 좋겠어요")로 요구 도출
- 각 VOS의 `client`에 요구 제기 이해관계자 명시
- 기존 VOS와 중복되지 않는 항목만 추가

**검증 기준**:
- 사용자 관점 어투 사용 (`~가 필요해요`, `~해야 합니다`, `~하면 좋겠어요`)
- 각 VOS의 `client`(이해관계자 역할)가 명확히 지정됨
- 과제 설명의 주요 Stakeholder들이 모두 커버됨 (누락된 관점 없음)
- 기존 VOS와 의미적 중복 항목이 `create`로 잘못 생성되지 않음
- `summary`는 간결하고 `description`은 구체적 (배경/근거 포함)
- `mode`, `existingKey` 등 JSON 스키마 일관성
- 기능 외 품질 속성(성능/보안/안정성/사용성 등) 관련 VOS도 포함되어야 함
- VOS 총 개수가 3건 미만이면 다양한 관점 발굴 부족
- 모든 VOS의 client가 동일하면 다른 관점 누락

**검증 정책**: 기본 정책 적용

**완료 기준**: 주요 이해관계자별 VOS가 1개 이상 등록되고, 검증 기준을 통과함

---

## ② Use Case 분석 → Feature 도출

**목적**: VOS를 기반으로 시스템의 Business Domain 컨텍스트와 Use Case를 분석하고, Actor와 직접 관계를 가진 Use Case를 Feature로 선정한다.

**입력**: VOS 이슈 + `business/project-description.md`
**산출물**:
- `business/use-cases.md` (Mermaid Context / Use Case / Sequence Diagram + 시나리오)
- Feature 이슈 (Use Case Story 필드 포함)
- VOS —(contains)→ Feature 관계

**수행 방법**:
- `usecase_analyze` 스킬 실행
- Business Domain Context Diagram 작성 (Actor / 외부 시스템 / 시스템 관계)
- 각 VOS를 충족시키는 Use Case 도출
- **Actor와 직접 관계를 가진 Use Case만 Feature로 선정** (보조/하위/include/extend 케이스 제외)
- 각 Feature 후보 Use Case에 시나리오 정의:
  - Basic Flow / Alternative Flow / Exception Flow / Precondition / Postcondition
- 각 Feature 후보에 Sequence Diagram 추가
- Feature 이슈 생성 시 Use Case Story 필드(`actor`, `precondition`, `basicflow`, `alternativeflow`, `exceptionflow`, `postcondition`) 채움
- 관련 VOS와 `contain` 관계 연결

**문서 양식**: `prompts/use-cases.md`

**검증 기준**:
- **Actor와 직접 관계된 Use Case만 Feature로 선정**됨 (보조/하위/include/extend는 Feature 아님)
- 각 Feature에 `actor`, `precondition`, `basicflow`, `alternativeflow`, `exceptionflow`, `postcondition` 모든 필드가 채워짐
- `relatedVOS`가 실제 존재하는 VOS 키를 사용
- **모든 VOS가 최소 1개 Feature에 매핑**되어 누락이 없음
- `document`에 Context Diagram / Use Case Diagram / 각 Feature별 Sequence Diagram (Mermaid) 포함
- Mermaid 다이어그램 구문 오류 없음
- `mode`, `existingKey` 일관성, JSON 형식 유효

**검증 정책**: 기본 정책 적용

**완료 기준**: 모든 VOS가 최소 1개의 Feature에 연결되고, use-cases.md 저장됨, 검증 기준 통과

---

## ③ Requirement 정제

**목적**: Use Case와 Feature를 기반으로 FR / NFR / CNST를 도출하고 각 타입에 맞는 형식으로 이슈를 등록한다.

**입력**: Feature 이슈 + `business/use-cases.md` + `business/project-description.md`
**산출물**:
- Requirement 이슈 (FR / NFR / CNST)
- `business/requirements.md`
- Feature —(contains)→ Requirement 관계

**수행 방법**:
- `requirement_refine` 스킬 실행
- 각 Feature/Use Case에서 다음 분류로 Requirement 도출:

### FR (Functional Requirement)

- **User Story 형식 필수**:
  - `{역할}은(는) {목적}을 위해 {기능}을 할 수 있다`
  - 예: "사용자는 관리자로서 사용자계정 관리를 위해 사용자를 추가/수정/삭제 할 수 있다"
- 도출 기준: 각 Feature가 동작하기 위해 시스템이 수행해야 하는 기능

### NFR (Quality Attribute Requirement)

- **QAS(Quality Attribute Scenario) 필수**
- 도출 기준:
  - FR이 측정 가능한 특정 수준의 동작을 요구하는 경우
  - VOS / 비즈니스 목적 / 요청에 수치화된 달성 목표가 있는 경우
- QAS 필드: `qa`, `source`, `stimulus`, `environment`, `artifact`, `response`, `measure`, `responseScenario`
- **Measure 자동 결정**: 원본에 명시되지 않은 경우 LLM이 일반적 수준에서 합리적 값으로 결정
  - 응답 시간(웹), API 응답, 가용성, 동시접속 등 도메인 휴리스틱 사용
  - **자동 결정된 항목은 결과에 명시 — 설계자가 검토/조정해야 함**
  - requirements.md 표의 비고에 `[자동결정]`으로 표시

### CNST (Constraint)

- 도출 기준: 과제 소개나 use-case에서 확인된 **반드시 지켜야 하는** 사양/기능/규제/표준
- 변경 불가한 조건이므로 별도 설계 이슈를 가지지 않음

**문서 양식**: `prompts/requirements.md`

**검증 기준**:
- **FR은 User Story 형식 준수**: `{역할}은(는) {목적}을 위해 {기능}을 할 수 있다`
- **NFR은 QAS 전 필드 채움**: `qa`, `source`, `stimulus`, `environment`, `artifact`, `response`, `measure`, `responseScenario`
- **NFR Measure는 측정 가능한 수치 + 단위** ("빠르게", "안전하게" 같은 모호 표현 금지)
- **CNST는 진짜 제약** (단순 선호/권장이 아닌 반드시 지켜야 할 규제/표준)
- **각 Feature에 FR 최소 1개 연결** (`relatedFeatures`로 매핑)
- `relatedFeatures`가 유효한 Feature 키 사용
- 의미적 중복 Requirement 없음
- document에 FR/NFR/CNST/QAW 섹션 모두 포함하고 양식 준수
- 자동 결정된 Measure는 비고에 `[자동결정]` 표시 반영됨
- JSON 형식 유효, `mode`/`reqType` 일관

**검증 정책**: 기본 정책 적용

**완료 기준**:
- 각 Feature에 FR이 1개 이상 연결됨
- 식별 가능한 NFR/CNST가 모두 등록됨
- requirements.md 저장됨
- 자동 결정된 NFR Measure 항목이 사용자에게 보고되어 검토 흐름이 안내됨
- 검증 기준 통과

---

## 스킬 매핑

| 단계 | 관련 스킬 | 출력 문서 |
|------|----------|----------|
| ① VOS 도출 | `vos_collect` | — |
| ② Use Case 분석 / Feature 도출 | `usecase_analyze` | `business/use-cases.md` |
| ③ Requirement 정제 | `requirement_refine` | `business/requirements.md` |

이슈 생성은 다음 헬퍼 스킬을 통해 수행된다:

- VOS: `issue_create_vos`
- Feature: `issue_create_feature`
- Requirement: `issue_create_requirement` (reqType: FR/NFR/CNST)

이슈 관계 연결: `issue_link` 또는 워크플로우 스킬 내부의 `helpers.link_issues` 헬퍼 (linkType: `contains`)

---

## 이슈 생성 공통 규칙 (재사용/수정/삭제마킹)

본 워크플로우의 모든 이슈 생성은 다음 규칙을 따른다 (`system.txt` 공통 규칙 참조):

| 상황 | 처리 | 동작 |
|------|------|------|
| 기존 이슈 없음 | `create` | 신규 이슈 생성 |
| 의미 동일한 기존 이슈 있음 | `reuse` | 생성하지 않고 기존 이슈 키 안내 |
| 기존 + 보강/수정 필요 | `modify` | `update_issue_with_history`로 갱신 + 자동 코멘트로 사유 기록 |
| 기존이 더 이상 유효하지 않음 | `delete_mark` | `mark_for_deletion` — summary 앞 `[Deletion Required] ` + 라벨 `del_req` + 사유 코멘트. **실제 삭제는 하지 않음** |

- 워크플로우 스킬(`vos_collect`, `usecase_analyze`, `requirement_refine`)은 LLM이 의미 유사도를 기반으로 mode를 결정한다
- 단일 생성 스킬(`issue_create_*`)을 LLM이 직접 호출할 때는 호출 전 `issue_query`로 사전 확인 후 분기
- **사용자가 명시적으로 삭제를 지시한 경우에만 `issue_delete`(hard delete) 사용**
- 자동 폐기 마킹 결과는 사용자에게 보고되어 검토를 거쳐야 함

## 후속 단계

본 워크플로우 완료 후 [`workflow-architecture-design.md`](./workflow-architecture-design.md)의
**Phase 2 — 구조 결정** (⑤ Architecture Driver 선정 → ⑥ 시스템 스타일 정의)으로 진행한다.
