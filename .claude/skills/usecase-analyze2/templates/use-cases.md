# Use Cases

<!--
  이 템플릿은 usecase-analyze2 Step 1.5 의 결정적 조립이 채우는 기준 파일입니다.
  marker (`<!-- INSERT <slot> HERE -->`) 는 _scripts/usecase-assemble.py 가 인식하는
  치환 지점입니다. marker 위치/이름을 변경하면 조립 스크립트도 함께 수정해야 합니다.

  지원 marker:
    business_domain     — 1.x 도메인/Actor/규칙
    user_stories_table  — 2.1 표
    use_cases_table     — 3.1 표
    context_diagram     — 4.1 PlantUML 블록
    usecase_diagram     — 4.2 PlantUML 블록
    features_section    — 5. Feature 상세 (Feature 별 시나리오 + Sequence Diagram)
    coverage_matrix     — 6. VOS 커버리지 매트릭스
-->

## 1. Business Domain 분석

<!-- INSERT business_domain HERE -->

---

## 2. User Story

### 2.1 User Story 목록

<!-- INSERT user_stories_table HERE -->

⚠️ **모든 VOS 는 최소 1개 User Story 로 파생되어야 함** — VOS 누락 시 critical.

---

## 3. Use Case 목록

### 3.1 Use Case 요약

<!-- INSERT use_cases_table HERE -->

⚠️ Feature 로 선정되지 않은 Use Case 도 **삭제하지 않음** — 목록과 다이어그램에 그대로 유지.

---

## 4. 다이어그램 (전체)

### 4.1 Context Diagram

<!-- INSERT context_diagram HERE -->

### 4.2 Use Case Diagram

⚠️ **모든 Use Case 포함** — Feature 선정 여부와 무관. Feature 와 비Feature UC 모두 `usecase "..." as UC_NNN` 으로 선언.

<!-- INSERT usecase_diagram HERE -->

---

## 5. Feature 상세

⚠️ **모든 Feature 에 시나리오 + Sequence Diagram 필수** — 누락 시 critical.

<!-- INSERT features_section HERE -->

---

## 6. VOS 커버리지 매트릭스

<!-- INSERT coverage_matrix HERE -->

⚠️ **VOS → User Story → Use Case → Feature** 경로가 **모든** VOS 에 대해 추적 가능해야 함.
⚠️ NFR/품질속성/정책 성격의 VOS 도 비-Feature 플레이스홀더로 빼지 말고 구현 기능 Feature 를 반드시 도출 — "이 요구를 만족시키려면 시스템이 무슨 동작을 해야 하는가?" 로 환원.

---

### 문서 작성 규칙
- **코드 블록 닫기 후 반드시 빈 줄 1개** — ` ``` ` 바로 뒤에 텍스트 이어붙이기 금지 (PlantUML 렌더 실패 원인)

### PlantUML 작성 규칙
- 모든 블록은 ` ```plantuml ` ... ` ``` ` 로 감싸고 내부는 `@startuml` ~ `@enduml` 쌍 필수
- Use Case Diagram: `actor` + `usecase "..." as UC_NNN` + `rectangle "..."` 묶음 (**절대 `class` 다이어그램 금지**)
- 기본 actor 스타일: `skinparam actorStyle stick` (UML 표준 stick figure)
- 레이아웃: `left to right direction` (use case / context 권장)
- include/extend 는 점선 화살표: `UC001 ..> UC002 : <<include>>`
- Sequence Diagram: `participant "이름" as 별칭` 또는 공백 없는 별칭 사용. 공백 있는 이름은 반드시 따옴표
- 외부 이미지 참조 (markdown image 문법 + .png/.jpg/.svg 등) 금지 — PlantUML 인라인만 허용
