# 아키텍처 스타일 카탈로그 스키마 (v2)

> 피드백 반영본. 추천 엔진이 직접 읽을 수 있도록 조건/관계/강도/scope를 명시한다.

## 1. 파일 구조

```
knowledge/arch-styles/
  SCHEMA.md                  # 본 문서
  index.yaml                 # 속성 어휘 + 매핑 + 가중치 + scope 정의
  styles/
    {id}.yaml                # 스타일 1개
```

## 2. 속성 값 체계 (level)

- 3단계: `low | medium | high`
- 요구 강제: `required` (high보다 강한 "반드시")
- 비교 표기: `medium_or_higher`, `medium_or_lower` — fit_signals/avoid_signals에서 허용
- 범위: `none` (해당 없음)

## 3. scope (스타일이 속하는 레벨)

| scope | 의미 | 예 |
|-------|------|----|
| `system_structure` | 시스템 전체 구조 (모놀리스 vs 분산) | microservices, modular-monolith, micro-frontend |
| `integration` | 컴포넌트/서비스 간 통합 방식 | event-driven |
| `interaction` | 요청/응답 상호작용 방식 | (REST, gRPC 스타일이 속함 — 본 카탈로그엔 없음) |
| `deployment` | 실행/배포 모델 | serverless |
| `data` | 데이터 처리/저장 패턴 | cqrs, event-sourcing |
| `module_boundary` | 모듈 내부 구조 규칙 | hexagonal, clean-architecture, layered |
| `infrastructure` | 인프라 레이어 보조 패턴 | service-mesh |

**규칙**: `conflicts_with`는 **같은 scope**에서만 선언. 다른 scope는 `combines_well_with` 또는 직교(언급 없음).

## 4. 스타일 파일 스키마

```yaml
id: <slug>                            # 파일명과 일치
name: "<표기명>"
scope: <scope 값>                     # 단일 값. 다중 필요 시 scopes 배열로
category: <자유 서술 카테고리>        # UI 그룹화용 (선택)
summary: "<1문장>"
description: |
  <2~4문장 설명>

# ── 추천 신호 ──────────────────────────────
fit_signals:                          # 이 스타일이 적합한 신호 (attr → level)
  scalability: high
  event_volume: high
  auditability: medium_or_higher

avoid_signals:                        # 이 스타일을 피해야 하는 신호
  strong_consistency: high
  ops_maturity: low

# ── 전제조건 ──────────────────────────────
prerequisites:
  technical:                          # 인프라/기술 요소
    - message_broker
  design:                             # 설계 원칙/구현 규율
    - idempotency
    - retry_policy
    - dead_letter_queue
  organizational:                     # 조직/운영 역량
    ops_maturity: medium_or_higher

# ── 품질속성 영향 ─────────────────────────
quality_attributes:
  - attribute: scalability
    effect: positive                  # positive | negative
    strength: high                    # low | medium | high
  - attribute: consistency
    effect: negative
    strength: medium

# ── 스타일 간 관계 ────────────────────────
combines_well_with:
  - style: microservices
    reason: "서비스 간 비동기 통합에 자연스러움"
  - style: cqrs
    reason: "이벤트 기반 읽기 모델 갱신"

conflicts_with:                       # 같은 scope 내에서만
  - style: shared-database-centric
    reason: "서비스 자율성과 비동기 이벤트 흐름을 약화"

# ── 평가용 속성 (엔진이 직접 조회) ────────
operational_complexity: high          # low | medium | high
observability_requirement: high
testing_complexity: high
consistency_model: eventual           # strong | eventual | bounded_staleness | n/a
interaction_mode: asynchronous        # synchronous | asynchronous | mixed | n/a
team_maturity_required: medium_or_higher
failure_handling_patterns:
  - retry
  - dead_letter_queue
  - idempotent_consumer

# ── 결정 규칙 (엔진 가산점/감점) ──────────
decision_rules:
  - if:
      strong_consistency: high
    then:
      score: -3
      reason: "즉시 일관성이 핵심인 흐름에 부적합"
  - if:
      event_volume: high
      scalability: high
    then:
      score: +3
      reason: "이벤트 중심 고확장성 요구에 적합"

# ── 흔한 실패 패턴 ─────────────────────────
anti_patterns:
  - distributed_monolith_via_events
  - no_schema_versioning
  - synchronous_through_broker_abuse

# ── 트레이드오프 (설명용) ─────────────────
tradeoffs:
  pros:
    - "..."
  cons:
    - "..."

# ── 메타 ──────────────────────────────────
specializes: []                       # 이 스타일이 구체화한 상위 스타일
generalizes: []                       # 이 스타일의 구체화 스타일
related_patterns: []                  # 참고 관련 패턴
references:
  - "..."
```

## 5. 엔진의 점수 계산 (engine.py 요약)

1. 요구사항 정규화 속성 = `{attr: level}` 딕셔너리
2. 각 스타일에 대해:
   - **base score = 0**
   - fit_signals 매칭: 각 매치당 +`weights.fit_match` (레벨 강도 가중)
   - avoid_signals 매칭: 각 매치당 `weights.avoid_match` (음수)
   - prerequisites 불만족:
     - technical 불명시 → 경고만 (점수 영향 없음, LLM에 근거 전달)
     - design 부족 → 경감 감점
     - organizational 불만족 → 강한 감점
   - quality_attributes: 요구 품질속성 매칭 시 effect×strength 가중 반영
   - decision_rules: 조건 매치 시 `score` 누적 + reason 기록
3. 후보군 상위 N개 선정 → combinations/conflicts 재검토 → 최종 Primary/Secondary/Supporting 구조로 출력

## 6. 검증 규칙 (catalog.py)

- `id`는 파일명과 일치 필수
- `scope`는 §3의 값 중 하나
- `conflicts_with.style`은 **같은 scope의 다른 스타일**만 허용
- `combines_well_with.style`은 scope 상관없음
- level 값은 §2 허용값만 허용
- 참조하는 style id는 styles/ 디렉토리에 실제 존재해야 함
