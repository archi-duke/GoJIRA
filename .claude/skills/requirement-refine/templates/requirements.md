# Requirements

## 1. 개요

본 문서는 {프로젝트명} 의 기능적 요구사항(FR), 비기능적 요구사항(NFR), 제약사항(CNST)을 정의한다.

## 2. 기능적 요구사항 (FR)

| ID | 요구사항 | 관련 Feature |
|----|----------|--------------|
| FR-1 | {역할}은(는) {목적}을 위해 {기능}을 할 수 있다 | ({Feature-KEY}) |
| FR-2 | ... | ({Feature-KEY}, {Feature-KEY2}) |
| ... | ... | ... |

⚠️ **User Story 형식 필수**: "~은(는) ~을 위해 ~할 수 있다"
⚠️ **관련 Feature 최소 1개 필수** — 빈 배열 금지 (critical)

---

## 3. 비기능적 요구사항 (NFR)

| ID | 품질 속성 | 요구사항 | 관련 Feature |
|----|-----------|----------|--------------|
| NFR-1 | Performance | ... 를 위해 ... 해야 한다 | ({Feature-KEY}) |
| NFR-2 | Security | ... | ({Feature-KEY}) |
| NFR-3 | Availability | ... | ({Feature-KEY}, {Feature-KEY2}) |
| ... | ... | ... | ... |

- 품질 속성은 Performance / Security / Availability / Usability / Maintainability / Scalability 등
- cross-cutting NFR (예: 로깅 규격 전체) 은 해당 Feature 전부 열거

---

## 4. 제약사항 (CNST)

| ID | 요구사항 | 관련 Feature |
|----|----------|--------------|
| CNST-1 | (기술 스택/규정/표준 원문 근거) | ({Feature-KEY}) |
| CNST-2 | ... | ({Feature-KEY1}, {Feature-KEY2}) |
| ... | ... | ... |

- 원문에 명시된 제약만 작성, 추측 금지
- cross-cutting CNST 는 해당 Feature 전부 열거

---

## 5. 요구사항 상세

각 Requirement 의 근거 / 측정 가능한 기준 / 검증 방법.

### 5.1 FR-1: {짧은 제목} (관련 Feature: {Feature-KEY})

**Description**: {전체 User Story 문장}

**근거**: {VOS / Feature 어느 맥락에서 도출되었는지}

**검증 기준**: {완료 조건 — DoD}

---

### 5.2 NFR-1: {품질 속성 제목} (관련 Feature: {Feature-KEY})

**Description**: ...

**QAS 6요소**:
- **Source**: ...
- **Stimulus**: ...
- **Environment**: ...
- **Artifact**: ...
- **Response**: ...
- **Measure**: ... (자동 결정은 `[자동결정]` 표기)
- **Response Scenario**: ...

---

### 5.3 CNST-1: {제약 제목} (관련 Feature: {Feature-KEY})

**Description**: ...

**근거**: {project-description.md / use-cases.md 어디 원문}

---

## 6. Feature 커버리지 매트릭스

| Feature 키 | 매핑 Requirement (FR/NFR/CNST) |
|-----------|-------------------------------|
| {Feature-KEY1} | FR_001, FR_002, QA_001, CNST_001 |
| {Feature-KEY2} | QA_002, CNST_002 |
| ... | ... |

⚠️ 모든 Feature 는 최소 1개의 Requirement 와 매핑되어야 함
