---
name: driver_select
description: Requirement(FR/NFR/CNST) 중 아키텍처 결정에 큰 영향을 주는 항목을 Architecture Driver 로 선정하고 QAD/QAI/Priority/risk/sensitivity 를 평가합니다. 해당 Requirement 이슈를 업데이트하고 ASR 라벨을 부여하며, architecture/drivers.md 문서와 정규화된 시스템 속성(style_define 입력) 을 산출합니다. 자가검증 루프(최대 3회).
---

# driver_select

Phase 2 ⑤ Architecture Driver 선정 — 요구사항(FR/NFR/CNST)에서 아키텍처 결정에 영향이 큰 항목을 선별하고 QAD/QAI/Priority 를 부여한다. 부산물로 **정규화된 요구사항 속성**(style_define 입력)을 함께 산출한다.

## Parameters

```yaml
projectCode:
  type: string
  description: "프로젝트 코드 (예: GOJIRA3)"
  required: true
priorityWeight:
  type: object
  description: "{qad: 0.5, qai: 0.5} 기본. 지정 시 projects.archSettings.priorityWeight 에 영속화"
```

## 입력 (Adapter 주입)

- `project_description`: business/project-description.md 본문 (docs/{user} 우선, 없으면 main)
- `requirements`: 기존 Requirement 이슈 리스트 (key/subkey/summary/description/reqtype/qa/measure)
- `features`: Feature 이슈 리스트 (참고용)
- `priorityWeight`: `{qad, qai}` 가중치
- `criteria`: 워크플로우 검증 기준
- `policy`: 검증 정책 텍스트

## 역할

노련한 소프트웨어 아키텍트. 요구사항 중 **아키텍처 결정에 영향이 큰 것(ASR)** 을 선별하고 각 속성에 대한 근거·위험·민감도를 평가한다. 허구 key 금지 — 반드시 입력 `requirements` 의 실존 key 만 사용.

## 실행 흐름 (self-loop, 최대 3회)

### Step 1 — 초안 생성 (iter 1)

**1.1 Driver 후보 검토**
- 모든 NFR 는 기본적으로 Driver 후보
- FR 중 **구조적 복잡도가 큰 기능**(분산/이벤트/대량 처리 등) 포함
- CNST 중 **아키텍처 선택을 제한하는 제약**(기술/규정/플랫폼) 포함

**1.2 Driver 카테고리 태깅** — 각 driver 의 `category` 지정:
- `Functional` — FR 기반 구조적 드라이버
- `QualityAttribute` — NFR (성능/가용성/보안 등)
- `Constraint` — CNST (규정/기술/플랫폼 제약)
- `Business` — 사업 목표·비용·시장 제약

**1.3 QAD / QAI 평가 (1~5 정수)**
- `QAD` (Quality Attribute Demand): 비즈니스/품질 관점 영향도
- `QAI` (Quality Attribute Impact): 기술/구조적 난이도
- 각 점수 근거(`rationale`) 명시 — 단순 추측/임의값 금지

**1.4 Priority 산정** — `priority = qad * w_qad + qai * w_qai` (가중치는 주입값 사용)

**1.5 risk / sensitivity**
- `risk`: 아키텍처 결정 시 유발 위험 (예: "분산 트랜잭션 일관성 난이도")
- `sensitivity`: 설계 결정의 민감도 (예: "파티셔닝 전략에 따라 응답시간 2~5배 편차")

**1.6 Driver 선정 수량** — Priority 상위 **5~10건 내외**. 한 품질속성/한 카테고리 편향 금지.

**1.7 정규화 속성 추출 (style_define 입력)**

`normalizedAttributes` 에 기록. 아래 허용 어휘만 사용, 관련 없는 속성은 생략 (추론 금지).

**품질속성**: `scalability, modifiability, availability, performance, security, consistency, latency, auditability, debuggability, testability, simplicity, portability, decoupling`

**요구 패턴 / 워크로드**: `event_volume, async_processing_need, synchronous_low_latency, real_time_processing, read_write_ratio_skewed, sporadic_workload, simple_crud, data_volume, domain_complexity, long_running_job, stateful_heavy, short_lifecycle, long_term_maintenance`

**프론트엔드**: `large_ui_codebase, polyglot_frontend_tech, strict_ui_consistency, ui_team_independence`

**인프라 / 규모**: `service_count, multi_cluster, inter_service_traffic_control, observability_requirement, strong_vendor_independence`

**조직 / 운영**:
- `team_size`: small / medium / large
- `ops_maturity`: low / medium / high
- `release_frequency`: low / medium / high
- `team_independent_deploy`: low / medium / high
- `strong_consistency`: low / medium / high

값 체계: 3단계 `low`/`medium`/`high`, 요구 강제 `required`, 없음/불명 **생략**.

**1.8 문서 작성 — `architecture/drivers.md`**

양식:

```
# Architecture Drivers

## 1. 선정 Driver 목록
| # | Requirement | 타입 | 카테고리 | 이슈 키 | QAD | QAI | Priority | risk | sensitivity |
|---|-------------|------|----------|--------|-----|-----|----------|------|-------------|
| 1 | (요약) | NFR | QualityAttribute | QA_001 | 4 | 5 | 4.5 | ... | ... |

## 2. Driver 별 상세 근거
### QA_001 — (이슈 요약)
- **QAD 4 근거**: ...
- **QAI 5 근거**: ...
- **risk**: ...
- **sensitivity**: ...

## 3. 정규화된 시스템 속성 (style_define 입력)
| 속성 | 값 | 근거 Requirement |
|------|----|------------------|
| scalability | high | QA_001, QA_003 |

## 4. 선정 편향 점검
- 대표 품질속성 집합
- 의도적으로 제외한 후보와 이유
```

문서 본문의 "이슈 키" 열에는 `subkey`(FR_001/QA_001/CNST_001) 사용.

**1.9 Mermaid (해당 시)** — Driver 간 상충/의존 관계가 중요하면 `graph LR` 로 간단 관계도 추가 가능 (선택). `classDiagram` 금지.

### Step 2 — 자가검증

**critical 기준**
- `criteria` / `policy` 위반
- 필수 필드(key, qad, qai, priority, category, rationale) 누락
- `key` 가 입력 `requirements` 에 없음 (hallucination)
- `qad`/`qai` 가 1~5 범위 밖, 또는 근거 없이 모두 동일 값
- 카테고리 편향: 선정 드라이버 중 `QualityAttribute` 가 0건, 또는 단일 카테고리만 존재
- NFR 인데 드라이버 후보에서 누락 (NFR 전건이 Driver 에 포함되지 않음)
- `normalizedAttributes` 가 허용 어휘 밖의 키/값 사용
- document 에서 `subkey` 가 아닌 이슈 key 나 docId placeholder 사용
- Mermaid 를 추가한 경우 `classDiagram` 사용 / 구문 오류
- 외부 이미지(`![..](..png)`) 참조

**점수**
- `score`: 0.0~1.0
- `score_effective = score - 0.5 × |critical|`
- `pass`: `score_effective >= 0.8`

### Step 3 — 수정 (pass=false)

critical + suggestions 항목만 **국소 수정**. 나머지 원문 유지. 수정 후 Step 2.

### Step 4 — 종료 조건

1. pass=true
2. iter 3 완료
3. 같은 critical 항목 2회 연속 재발
4. 새 수정본이 이전 best 보다 낮은 score_effective → 포기

**best-so-far**: 매 iter 의 `(drivers, normalizedAttributes, document, score_effective)` 기록. 종료 시 최댓값 iter 를 selected.

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
      "drivers": [
        {
          "key": "GOJIRA3-27",
          "subkey": "QA_001",
          "category": "QualityAttribute",
          "qad": 4,
          "qai": 5,
          "priority": 4.5,
          "risk": "...",
          "sensitivity": "...",
          "rationale": "..."
        }
      ],
      "normalizedAttributes": {
        "scalability": "high",
        "team_size": "small"
      },
      "document": "전체 drivers.md 본문"
    }
  ],
  "selectedIter": 1,
  "finalItems": [],
  "finalDocument": "",
  "finalNormalizedAttributes": {},
  "stopReason": "pass | max_iter | repeat_critical | regression",
  "unresolvedCritical": []
}
```

**규칙**
- `finalItems` / `finalDocument` / `finalNormalizedAttributes` 는 `selectedIter` 의 것과 동일
- `drivers[].key` 는 입력 `requirements` 의 실존 key 만
- `drivers[].category` 는 Functional / QualityAttribute / Constraint / Business 중 하나
- `unresolvedCritical` 은 stop 시점 남은 critical (pass 시 빈 배열)
