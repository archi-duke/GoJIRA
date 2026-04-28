---
name: design_integrate
description: ADSN/DDSN 설계 결정과 시스템 스타일을 논리/프로세스/배포/개발 관점으로 통합해 architecture/architecture-integrated.md 를 작성합니다. self-loop(최대 3회) + best-so-far 내장. 이슈 변경 없이 문서만 산출.
---

## Parameters

```yaml
projectCode:
  type: string
  required: true
```

## 입력 (Adapter 주입)

- `projectCode`, `chosen_styles`, `normalized_attributes`, `designs` (ADSN/DDSN 이슈), `req_summary`, `criteria`, `policy`, `mermaid_rules`

## 역할

시니어 소프트웨어 아키텍트. 선정된 스타일과 ADSN/DDSN 설계 결정을 서로 다른 뷰(관점)로 통합해, 관점 간 일관성이 검증된 **단일 설계 문서**를 생산한다.

## 실행 흐름 (self-loop, 최대 3회)

### Step 1 — 초안 생성 (iter 1)

**1.1 관점 선택 (최소 2개)**
- Logical View — 주요 컴포넌트/모듈 책임
- Process View — 런타임 동작, 동시성, 통신 패턴
- Deployment View — 물리 배치, 인프라
- Development View — 코드 모듈/패키지 구조

**1.2 관점별 서술**
- ADSN/DDSN 결정이 어떻게 반영되는지 (모든 결정이 최소 1 관점에서 언급)
- 선정 스타일과 일관되는지
- 컴포넌트/프로세스/노드 명칭은 ADSN/DDSN 명명과 일치

**1.3 관점 간 일관성**
- 논리 모듈 ↔ 배포 단위 매핑
- 프로세스 뷰 통신 ↔ ADSN 통신 패턴 정합
- 충돌/모순은 명시 + 해결 방향 제시

**1.4 Mermaid 다이어그램 (인라인 필수)**
각 관점에 1개 이상:
- Logical → `graph TB` (컴포넌트 의존)
- Process → `sequenceDiagram`
- Deployment → `graph LR` (노드/구역)
- Development → `graph TB` (모듈/패키지)

**금지**: 외부 이미지(`![...](...png)`) 참조, `classDiagram` 으로 컴포넌트/배포 표현, Mermaid 노드 ID 에 공백/한글 직접 사용.

`mermaid_rules` 주입 내용 준수.

**1.5 참조 표기**
- ADSN/DDSN 은 subkey (`ADSN_NNN` / `DDSN_NNN`) 그대로 참조
- Requirement 는 subkey (`FR_NNN` / `QA_NNN` / `CNST_NNN`) 사용

**1.6 문서 구조**

```
# 통합 설계 뷰 (Integrated Design Views)

## 1. 개요
- 시스템 스타일 요약 (1~2줄)
- 주요 설계 결정 요약 (ADSN 별 한 줄, subkey 포함)

## 2. Logical View
### 2.1 주요 컴포넌트와 책임
### 2.2 의존 관계 (Mermaid graph TB)
### 2.3 반영된 ADSN/DDSN (subkey 목록)

## 3. Process View
### 3.1 런타임 상호작용 (Mermaid sequenceDiagram)
### 3.2 비동기/이벤트 흐름
### 3.3 반영된 ADSN/DDSN

## 4. (선택) Deployment View / Development View

## 5. 관점 간 일관성 점검
- 논리↔배포 매핑
- 논리↔프로세스 정합
- 모순/잔여 과제

## 6. 후속 단계 포인트
- SATAM 평가에서 집중 점검할 Sensitivity / Risk 후보
```

### Step 2 — 자가검증

**critical 기준 (이 중 하나라도 해당 시 critical)**
- `criteria` / `policy` 위반
- ADSN/DDSN `designs` 중 **단 1건이라도** 문서 본문에서 subkey 로 참조되지 않음 (upstream 미반영)
- 선정 스타일(primary/secondary/supporting) 중 본문 언급이 전혀 없는 항목
- 정규화 속성(`normalized_attributes`) 과 본문 설계 방향 모순
- 관점이 1개 이하
- Mermaid 다이어그램이 **전 관점 합쳐 1개 이하** 또는 모든 관점에서 누락
- Mermaid 구문 오류 / 시작 라인 비표준
- 컴포넌트/배포 뷰에 `classDiagram` 사용
- 외부 이미지(`![...](...png)`) 참조
- 관점 간 일관성 섹션(§5) 부재 또는 "충돌 없음" 만 기술하고 근거 무
- `viewsIncluded` 가 본문 실제 뷰와 불일치

**점수**
- `score`: 0.0~1.0
- `score_effective = score - 0.5 × |critical|`
- `pass = score_effective >= 0.8`

### Step 3 — 수정 (pass=false)

critical + suggestions 항목만 **국소 수정**. 나머지 원문 유지. 수정 후 Step 2 로.

### Step 4 — 종료 조건
1. pass=true
2. iter 3 완료
3. 같은 critical 이 2회 연속 재발
4. 새 수정이 이전 best 보다 낮은 `score_effective` → 포기

**best-so-far**: 매 iter 의 `(viewsIncluded, document, score_effective)` 기록. 종료 시 최댓값 iter 를 `selectedIter`.

## 출력 형식

반드시 단일 JSON 객체 **하나만** (설명문/마크다운 금지):

```json
{
  "iterations": [
    {
      "iter": 1,
      "score": 0.85,
      "critical": [],
      "suggestions": [],
      "scoreEffective": 0.85,
      "viewsIncluded": ["logical", "process", "deployment"],
      "document": "전체 architecture-integrated.md 본문"
    }
  ],
  "selectedIter": 1,
  "finalViews": ["logical", "process"],
  "finalDocument": "...",
  "stopReason": "pass | max_iter | repeat_critical | regression",
  "unresolvedCritical": []
}
```

**규칙**: `finalViews` / `finalDocument` 는 `selectedIter` 의 값과 동일. document 에 모든 Mermaid 블록은 ```` ```mermaid ```` 로 감싸기. 이슈는 생성/수정하지 않는다 — 문서만 산출.
