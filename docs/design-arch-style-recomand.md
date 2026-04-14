# 아키텍처 스타일 추천 스킬 설계 논의

> 아키텍처 설계 에이전트에서 요구사양에 맞는 아키텍처 스타일을 추천하는 스킬 설계에 관한 논의 정리

---

## 1. 핵심 문제 정의

아키텍처 스타일 선택은 단순 검색이 아니라 **추론**이 필요한 작업이다.

- "이 요구사항에 어떤 스타일이 맞나?" → 조건 매핑
- "Layered + Event-Driven을 조합하면?" → 관계 추론
- "마이크로서비스의 트레이드오프는?" → 맥락적 판단

---

## 2. 참고 자료 제공 방식 비교

### 2-1. 일반 RAG (벡터 검색)

```
요구사항 텍스트 → 임베딩 → 유사 문서 검색 → LLM에 컨텍스트 제공
```

**적합한 경우**
- 아키텍처 설명, 사례, 장단점 같은 자연어 문서 검색
- "마이크로서비스란 무엇인가" 류의 정보 조회

**한계**
- 스타일 간 관계(조합 가능성, 충돌 여부)를 표현 못함
- "A 스타일이 B 요구사항에 왜 맞는지"의 인과 추론이 약함
- 여러 스타일 비교·조합 시 검색 결과가 단편적

---

### 2-2. Graph RAG (그래프 + 추론 검색)

```
아키텍처 스타일 노드 → 요구사항/특성/트레이드오프 엣지 → 추론 검색
```

**적합한 경우**
- 스타일 간 관계 추론 ("Hexagonal은 Layered의 변형")
- 요구사항 조건 매핑 ("고가용성 필요 → Event-Driven / CQRS")
- 조합 패턴 탐색 ("A + B 조합 시 주의사항")

**장점**
- 구조화된 지식으로 일관된 추론 가능
- 요구사항 속성과 스타일 특성을 노드-엣지로 명확히 연결
- 에이전트가 "왜 이 스타일인가"를 근거와 함께 제시 가능

**한계**
- 초기 그래프 구축 비용이 높음
- 새 스타일 추가 시 관계 재정의 필요

---

### 2-3. 구조화된 파일 (YAML/JSON + 검색)

```
스타일별 구조화 파일 → 조건 기반 필터링 → LLM 판단
```

**예시 구조**

```yaml
style: Microservices
when_to_use:
  - 팀이 독립적으로 배포해야 할 때
  - 서비스별 기술 스택이 다를 때
avoid_when:
  - 소규모 팀 (< 5명)
  - 네트워크 레이턴시 민감
combines_well_with: [Event-Driven, CQRS, API Gateway]
conflicts_with: [Monolithic]
tradeoffs:
  pros: [독립 배포, 기술 다양성, 확장성]
  cons: [운영 복잡도, 분산 트랜잭션, 네트워크 오버헤드]
```

**적합한 경우**
- 에이전트가 직접 파일을 읽고 조건 매칭
- 빠른 프로토타이핑, 유지보수 단순
- 스타일 수가 20~30개 이하

---

## 3. 권장 전략: 단계적 접근

```
Phase 1 (지금 당장)
└── 구조화된 YAML 파일
    └── 빠른 구축, LLM이 직접 읽기 좋음

Phase 2 (스타일 30개+ 또는 정교한 추론 필요 시)
└── Graph RAG
    └── 요구사항 → 스타일 매핑의 정확도 향상

일반 RAG → 자연어 문서 보완용으로만 활용 (추천의 근간 X)
```

---

## 4. Phase 1 상세 설계

### 4-1. 설계 원칙

> "스타일 지식은 구조화하고, 선택은 규칙/점수로 하고, LLM은 파싱과 설명에 쓴다."

- **LLM은 해설자, 추천 로직은 심판**
- RAG 없이도 강한 시스템 구성 가능
- 결과가 흔들리지 않고, 설명 가능하며, 테스트 가능

---

### 4-2. 전체 구성 요소

```
A. 구조화된 스타일 카탈로그   — 사람이 검토한 정제된 지식
B. 요구사항 정규화 스키마     — 자연어 입력을 표준 필드로 변환
C. 추천 엔진                  — 가중치 + 룰 + 조합 규칙
D. LLM 설명 계층              — 이유 설명, 대안 비교, 구조 초안 생성
```

---

### 4-3. 요구사항 정규화

자연어 요구사항을 구조화 필드로 변환하는 중간 모델

```yaml
scalability: high
modifiability: high
strong_consistency: required
team_size: small
ops_maturity: low
release_frequency: high
real_time: medium
auditability: high
```

**주의**: 명시되지 않은 속성을 추론해야 하는 경우가 많으므로, 파싱 후 **사용자 확인 단계** 필요

```
에이전트: "다음과 같이 이해했어요. 맞나요?
  - 확장성: 높음
  - 팀 규모: 소규모
  - 운영 성숙도: 낮음 (추론)  ← 명시되지 않은 속성
  - 강한 일관성: 불필요"
```

---

### 4-4. index.yaml 구조

```yaml
requirement_mapping:

  high_scalability:
    primary: [microservices, event-driven]
    secondary: [cqrs, serverless]

  low_latency:
    avoid: [microservices, layered]
    prefer: [in-process, monolithic]

  team_independent_deploy:
    primary: [microservices, modular-monolith]

  complex_domain:
    primary: [ddd-layered, hexagonal, clean]
    secondary: [cqrs, event-sourcing]

  simple_crud:
    primary: [layered, mvc]
    avoid: [microservices, event-sourcing]

combinations:
  microservices + event-driven:
    compatible: true
    note: "비동기 서비스 간 통신에 자연스러운 조합"
  cqrs + event-sourcing:
    compatible: true
    note: "이벤트 기반 상태 추적에 강력한 조합"
```

---

### 4-5. 추천 방법

#### 방법 A: LLM이 직접 파일을 읽고 판단
```
요구사항 입력
    ↓
index.yaml에서 키워드 매핑으로 후보 압축 (5~8개)
    ↓
후보 스타일의 yaml 파일 전체 로드 (컨텍스트에 삽입)
    ↓
LLM이 요구사항 vs 스타일 특성 비교 추론
    ↓
추천 + 근거 출력
```
→ 스타일 수 ~15개, 빠른 프로토타이핑에 적합

#### 방법 B: 조건 매칭 후 LLM 판단
```
요구사항 입력
    ↓
에이전트가 요구사항을 속성으로 구조화
    ↓
각 스타일의 when_to_use / avoid_when 조건과 매칭
    ↓
점수 기반으로 후보 정렬
    ↓
LLM이 최종 판단 + 조합 검토
```
→ 스타일 수 20개+, 일관된 추천 품질 필요 시 적합

---

### 4-6. 점수화 시 주의사항

- 규칙이 너무 단선적이면 현실을 못 담음 → **가중치 + 조건부 룰** 사용
- 스타일을 단일 선택 문제로 보면 틀릴 수 있음 → **Primary / Secondary / Supporting 구조** 권장

```
예시:
Primary:    Modular Monolith
Secondary:  Event-Driven Integration
Supporting: Hexagonal, CQRS for read model
```

- 점수 차이가 근소할 때 하나를 찍으면 신뢰를 잃음 → **상위 N개 후보로 올리고 LLM이 결정적 차이 설명**

```
Modular Monolith  82점
Microservices     79점  ← 3점 차이 = 사실상 동률
Layered           61점
```

---

### 4-7. 전체 흐름 (보완된 구조)

```
A. 스타일 카탈로그
B. 요구사항 정규화 스키마
        ↓
   [사용자 확인 루프]
C. 추천 엔진 (점수 + 룰)
        ↓
   [동점 처리 / 후보군 선정]
D. LLM 설명 계층
```

---

### 4-8. 구현 순서

1. **요구사항 정규화 스키마 설계** — 가장 먼저, 가장 중요
2. **스타일 카탈로그 작성** — 20~30개 스타일, 공통 스키마로
3. **점수 계산 룰 정의** — 가중치 + 조건부 룰
4. **LLM 파싱 + 설명 레이어** — 마지막에 붙이기

---

## 5. Phase 2 상세 설계

### 5-1. Phase 1 → Phase 2 전환 신호

- 스타일이 30개+ 넘어가며 룰 관리가 복잡해짐
- 새로운 스타일 추가 시 기존 룰과 충돌 발생
- "왜 이 조합은 안 되는가"를 룰로 표현하기 어려움
- 도메인별 맥락 차이를 점수로 못 담음 (금융 vs 게임 도메인)

---

### 5-2. 핵심 아이디어

```
Phase 1: 요구사항 속성 → 점수 계산 → 추천   (테이블 기반 매핑)
Phase 2: 요구사항 → 그래프 탐색 + 추론 → 추천 (지식 그래프 위의 추론)
```

---

### 5-3. 그래프 구조 설계

**노드 타입**
```
├── ArchitectureStyle     (Microservices, Layered, ...)
├── QualityAttribute      (Scalability, Modifiability, ...)
├── RequirementPattern    (HighTraffic, FrequentRelease, ...)
├── OrganizationContext   (SmallTeam, LowOpMaturity, ...)
├── DomainPattern         (FinTech, E-Commerce, IoT, ...)
└── ArchitecturalConcern  (DataConsistency, Latency, ...)
```

**엣지 타입**
```
├── SUPPORTS         (Microservices → Scalability)
├── WEAKENS          (Microservices → StrongConsistency)
├── REQUIRES         (Microservices → HighOpMaturity)
├── COMBINES_WITH    (Microservices ↔ EventDriven)
├── CONFLICTS_WITH   (Microservices ↔ Monolithic)
├── PREFERRED_IN     (EventDriven → RealtimeSystem)
├── AVOID_WHEN       (Microservices → SmallTeam)
└── REFINES          (HexagonalArch → LayeredArch)
```

---

### 5-4. 추론 방식 (4단계)

**Step 1. 요구사항 → 그래프 노드 매핑**
```
scalability: high    → QualityAttribute: HighScalability
team_size: small     → OrganizationContext: SmallTeam
ops_maturity: low    → OrganizationContext: LowOpMaturity
real_time: high      → RequirementPattern: RealtimeSystem
```

**Step 2. 후보 스타일 탐색**
```python
candidates = graph.traverse(
    start_nodes=requirement_nodes,
    edge_types=["SUPPORTS", "PREFERRED_IN"],
    direction="inbound"
)
filtered = graph.filter(
    candidates,
    exclude_edges=["AVOID_WHEN", "REQUIRES"],
    context=org_context_nodes
)
```

**Step 3. 경로 기반 근거 수집**
```
"왜 Microservices가 후보에서 제외됐는가?"

탐색 경로:
SmallTeam ──AVOID_WHEN──► Microservices  ✓ 발견
LowOpMaturity → Microservices REQUIRES HighOpMaturity → 충돌  ✓ 발견

근거: 2개의 충돌 경로 존재 → 강한 제외 신호
```

**Step 4. LLM이 경로를 해석해 설명 생성**
```
그래프 탐색 결과:
- ModularMonolith ← SUPPORTS ← FrequentRelease (강)
- ModularMonolith ← COMBINES_WITH → HexagonalArch
- HexagonalArch ← SUPPORTS ← Modifiability (중)

LLM 출력:
"잦은 배포 요구사항과 소규모 팀 조건을 고려할 때
Modular Monolith가 가장 적합합니다. 내부적으로
Hexagonal 구조를 적용하면 향후 분리 가능성도 확보됩니다."
```

---

### 5-5. Phase 1 vs Phase 2 비교

| 항목 | Phase 1 | Phase 2 |
|------|---------|---------|
| 지식 저장소 | YAML 파일 | 그래프 DB |
| 추론 방식 | 점수 합산 | 경로 탐색 |
| 확장성 | 룰 추가 = 코드 수정 | 노드/엣지 추가 |
| 설명 근거 | 점수 분해 | 탐색 경로 |
| 구축 비용 | 낮음 | 높음 |
| 추론 정교함 | 중간 | 높음 |

---

### 5-6. Phase 1 → Phase 2 전환 전략

Phase 1 자산이 Phase 2의 재료가 된다.

```
Phase 1 YAML                    Phase 2 Graph
─────────────────────────────────────────────
when_to_use: [A, B]     →      SUPPORTS 엣지
avoid_when: [C, D]      →      AVOID_WHEN 엣지
combines_well: [E]      →      COMBINES_WITH 엣지
conflicts_with: [F]     →      CONFLICTS_WITH 엣지
quality_attributes: {}  →      QualityAttribute 노드
```

> Phase 1 스키마를 잘 설계하면 그래프 변환이 거의 자동화된다.

---

### 5-7. 기술 스택 옵션

| 단계 | 스택 | 특징 |
|------|------|------|
| 경량 (프로토타입) | NetworkX + JSON | 그래프 탐색 로직 직접 구현 |
| 중간 (실용) | Neo4j + Cypher | 복잡한 경로 탐색 쿼리 가능 |
| 고도화 | Neo4j + LangChain GraphRAG | 자연어 → Cypher 자동 변환 |

---

## 6. RAG의 역할 재정의

RAG를 완전히 버릴 필요는 없다. 역할을 줄이면 된다.

**RAG를 붙여도 되는 곳**
- 스타일별 상세 설명 보강
- 실제 사례나 패턴 예시 제시
- 내부 아키텍처 원칙 문서 참조
- 특정 도메인별 주의사항 보강

> 추천의 근간이 아니라 **설명과 근거 보강용 부속 장치**로 활용

---

## 7. 결론 요약

```
스타일 지식은 구조화하고
선택은 규칙/점수로 하고
LLM은 파싱과 설명에 쓴다
```

| 역할 | 담당 |
|------|------|
| 스타일 지식 관리 | 구조화된 YAML 카탈로그 |
| 요구사항 파싱 | LLM |
| 스타일 추천 판단 | 추천 엔진 (규칙/점수) |
| 추천 설명 생성 | LLM |
| 지식 보강 | RAG (보조) |
