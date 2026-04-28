---
name: requirement_refine
description: Feature와 use-cases.md를 분석하여 FR(User Story)/NFR(QAS)/CNST(제약) Requirement를 도출, 이슈로 등록하고 Feature와 contains 관계를 연결하며 business/requirements.md를 작성합니다. 자가검증 루프(최대 3회)로 품질 보증.
---

## Parameters

```yaml
projectCode:
  type: string
  description: "프로젝트 코드 (예: GOJIRA)"
  required: true
```

## 입력 (Adapter 주입)

- `project_description`: business/project-description.md 본문 (main → docs/{user} 순서)
- `use_cases`: business/use-cases.md 본문
- `feat_list`: Feature 이슈 리스트 (key/summary/description/actor/precondition/basicflow/alternativeflow/exceptionflow/postcondition)
- `vos_list`: VOS 이슈 리스트 (key/summary)
- `existing_reqs`: 기존 Requirement 이슈 리스트 (key/subkey/summary/description/reqtype/qa/measure)
- `template`: requirements.md 양식
- `criteria`: 워크플로우 검증 기준
- `policy`: 검증 정책 텍스트

## 역할

소프트웨어 아키텍트. Feature 와 Use Case 시나리오에서 **User Story(FR) / QAS(NFR) / 제약(CNST)** 을 도출하고 모든 Feature 가 최소 1 개의 FR 에 의해 다루어지도록 책임진다. User Story 형식과 QAS 6 요소(Source/Stimulus/Environment/Artifact/Response/Measure) 엄수.

## 실행 흐름 (self-loop, 최대 3회)

아래를 단일 응답 내에서 최대 3 iteration 수행한다.

### Step 1 — 초안 생성 (iter 1)

각 Feature / Use Case 시나리오에서 FR / NFR / CNST 를 **다양한 관점에서 충분히** 도출한다.

**다양성 가이드**
- 각 Feature 는 보통 **2~5 개의 FR** 로 세분화. 단일 FR 은 Feature 가 단순하거나 세분화 부족 신호
- FR 관점: 기본 동작, 입력 검증, 조회/검색, CRUD, 권한/접근제어, 알림/피드백, 오류 처리
- NFR 은 **품질 속성(QA)별로 다양**: Performance, Security, Availability, Usability, Maintainability, Scalability 등
- CNST 는 과제 설명 / use-case 에서 **명확히 드러난 제약만** (기술 스택, 규정, 플랫폼, 표준). 추측성 제약 금지

**FR 규칙**
- User Story 형식 필수: `"{역할}은(는) {목적}을 위해 {기능}을 할 수 있다"`
- 한 Feature 의 여러 사용자 행위(생성/조회/수정/삭제/검색 등)는 개별 FR 로 분리

**NFR 규칙 (QAS 필수)**
- 필드: qa, source, stimulus, environment, artifact, response, measure, responseScenario
- measure 가 원본에 없으면 합리적 수준으로 결정 후 `measureAuto: true`
- 동일 qa 에 여러 시나리오면 각각 별도 NFR

**CNST 규칙**
- 반드시 지켜야 하는 사양/규제/표준만

**수량 가이드**: FR 10~30, NFR 3~10, CNST 0~5 수준. 한 Feature 에 FR 1 건만 나오면 세분화 부족 — 재검토.

**중복 처리 (mode 판정)** — 각 Requirement 에 `mode`:
- `create`: 기존과 의미적으로 다른 새 항목
- `reuse`: 기존과 동일 (existingKey 필수)
- `modify`: 기존 보강 필요 (existingKey + modifyReason)
- `delete_mark`: 기존 폐기 (existingKey + deleteReason)

**docId 부여**
- FR → `FR-1`, `FR-2`, ... (1부터 순차)
- NFR → `NFR-1`, `NFR-2`, ...
- CNST → `CNST-1`, `CNST-2`, ...
- document 본문의 모든 Requirement 표기(표의 "ID", "(이슈 키)", 헤딩, 관련 Feature 매핑)에 **동일 docId 사용**. "(이슈 키)" 빈칸 금지 — docId 그대로 기록 (adapter 가 실제 subkey FR_NNN / QA_NNN / CNST_NNN 로 치환)
- `relatedFeatures` 에는 실제 Feature JIRA 키(`{code}-N`) 사용 (docId 아님)
- `reuse`/`modify` 항목도 docId 부여하고 `existingKey` 별도 제공

### Step 2 — 자가검증

**Feature 커버리지 매트릭스 (필수)** — `feat_list` 의 **모든 Feature 키**가 최소 1 개 FR 의 `relatedFeatures` 에 포함되는지 점검.
- 미커버 Feature 가 있으면: **FR 추가 생성** 또는 `coverageMatrix[feat_key] = ["(CNST/NFR 전역)"]` 표기 (전역 품질 제약으로 간접 커버)
- 억지 매핑 금지

**critical 기준**
- `criteria` / `policy` 위반
- 필수 필드 누락: FR(summary, description, docId, relatedFeatures, mode), NFR(+ qa/source/stimulus/environment/artifact/response/measure/responseScenario), CNST(summary, description, docId, mode)
- FR description 이 User Story 형식(`~은(는) ~을 위해 ~할 수 있다`) 아님
- NFR 인데 QAS 6 요소 중 하나라도 누락
- `relatedFeatures` 가 `feat_list` 에 없는 키 참조 (hallucination)
- `mode=reuse/modify/delete_mark` 인데 `existingKey` 가 `existing_reqs` 에 없음
- Feature 1 건이라도 FR 미매핑 (전역 표기 시 면제)
- Feature 평균 FR < 1.5 건 (세분화 부족)
- QA 품질 속성 다양성 부족 (Performance/Security/Availability 중 2 개 이상 누락인데 NFR 은 존재)
- CNST 에 추측성 제약 포함 (원문 근거 없음)
- docId 중복/누락 또는 reqType 과 docId 접두 불일치

**점수 계산**
- `score`: 0.0~1.0
- `score_effective = score - 0.5 × len(critical)`
- `pass = score_effective >= 0.8`

### Step 3 — 수정 (pass=false 일 때)

critical + suggestions 항목만 **국소 수정**. 다른 항목은 원문 그대로. 수정 후 다시 Step 2.

### Step 4 — 종료 조건

1. pass=true
2. iter 3 완료
3. 같은 critical 2회 연속 재발
4. 새 수정이 이전 best 보다 낮은 score_effective → 포기

**best-so-far**: 매 iter 의 `(requirements, document, coverageMatrix, score_effective)` 기록. 종료 시 score_effective 최댓값 iter 를 `selectedIter` 로 채택.

## 출력 형식

단일 JSON 객체 **하나만** (설명문/마크다운 금지):

```json
{
  "iterations": [
    {
      "iter": 1,
      "score": 0.85,
      "critical": [],
      "suggestions": [],
      "scoreEffective": 0.85,
      "requirements": [
        {
          "docId": "FR-1",
          "reqType": "FR",
          "summary": "요구사항 제목",
          "description": "{역할}은(는) {목적}을 위해 {기능}을 할 수 있다",
          "relatedFeatures": ["GOJIRA-5"],
          "mode": "create",
          "existingKey": "",
          "modifyReason": "",
          "deleteReason": ""
        },
        {
          "docId": "NFR-1",
          "reqType": "NFR",
          "summary": "...",
          "description": "...",
          "qa": "Performance",
          "source": "...",
          "stimulus": "...",
          "environment": "...",
          "artifact": "...",
          "response": "...",
          "measure": "P95 < 500ms",
          "responseScenario": "...",
          "measureAuto": true,
          "relatedFeatures": ["GOJIRA-5"],
          "mode": "create",
          "existingKey": "",
          "modifyReason": "",
          "deleteReason": ""
        },
        {
          "docId": "CNST-1",
          "reqType": "CNST",
          "summary": "...",
          "description": "...",
          "relatedFeatures": [],
          "mode": "create",
          "existingKey": "",
          "modifyReason": "",
          "deleteReason": ""
        }
      ],
      "document": "전체 requirements.md 본문 (Markdown, 양식 준수, [자동결정] 표기 포함)",
      "coverageMatrix": {
        "GOJIRA-5": ["FR-1", "FR-2"],
        "GOJIRA-6": ["(CNST 전역)"]
      }
    }
  ],
  "selectedIter": 1,
  "finalRequirements": [],
  "finalDocument": "",
  "finalCoverage": {},
  "stopReason": "pass | max_iter | repeat_critical | regression",
  "unresolvedCritical": []
}
```

**규칙**
- `finalRequirements` / `finalDocument` / `finalCoverage` 는 `selectedIter` 와 동일해야 함
- `iterations` 는 실제 수행한 iter 만 포함 (pass 시 1개면 됨)
- `coverageMatrix` 에 `feat_list` 모든 키 포함 필수
- document 는 requirements.md 전체 본문 (template 양식 준수, 자동결정 NFR 비고에 `[자동결정]`)
- `unresolvedCritical` 은 stop 시점 잔여 critical (pass 시 빈 배열)
