# 통합 설계 뷰 (Integrated Design Views)

## 1. 개요

본 문서는 {프로젝트명} 의 ADSN/DDSN 결정을 **Logical / Process / Deployment / Development** 4가지 관점(Architectural Views)으로 통합해 아키텍처 전체를 서술한다.

- **선정 스타일**: {Primary} / {Secondary} / {Supporting}
- **주요 ADSN 수**: {N}
- **주요 DDSN 수**: {M}

---

## 2. Logical View

**관심**: 시스템이 제공하는 기능적 구성 요소와 그들 사이의 책임 분담

### 2.1 주요 컴포넌트와 책임

| 컴포넌트 | 책임 | 관련 ADSN/DDSN |
|----------|------|----------------|
| {컴포넌트명} | ... | ADSN_001 |
| {컴포넌트명} | ... | ADSN_002, DDSN_001 |
| ... | ... | ... |

### 2.2 의존 관계

```mermaid
graph TB
    UI[UI 레이어]
    BL[비즈니스 로직]
    DAL[데이터 접근]
    DB[(저장소)]
    UI --> BL
    BL --> DAL
    DAL --> DB
```

### 2.3 반영된 ADSN/DDSN

- `ADSN_001`: {Design Point} — ...
- `DDSN_001`: {Design Point} — ...

---

## 3. Process View

**관심**: 런타임 동작 / 프로세스 간 상호작용 / 동시성

### 3.1 런타임 상호작용

```mermaid
sequenceDiagram
    participant C as Client
    participant API as API Gateway
    participant Svc as Service
    participant Q as Queue
    C->>API: 요청
    API->>Svc: 라우팅
    Svc->>Q: 이벤트 발행
    Q-->>Svc: ACK
    Svc-->>API: 응답
    API-->>C: 응답
```

### 3.2 비동기 / 이벤트 흐름

```mermaid
graph LR
    P[생산자] -->|event-A| Q[(메시지 큐)]
    Q -->|consume| C1[소비자 1]
    Q -->|consume| C2[소비자 2]
```

### 3.3 반영된 ADSN/DDSN

- `ADSN_XXX`: ...

---

## 4. (선택) Deployment View

**관심**: 물리적 배포 / 네트워크 토폴로지 / 인프라

```mermaid
graph TB
    subgraph VPC
        ALB[ALB]
        subgraph "AZ-A"
            WebA[Web]
            AppA[App]
        end
        subgraph "AZ-B"
            WebB[Web]
            AppB[App]
        end
        DB[(RDS Multi-AZ)]
    end
    ALB --> WebA
    ALB --> WebB
    WebA --> AppA
    WebB --> AppB
    AppA --> DB
    AppB --> DB
```

- **반영 ADSN/DDSN**: ...

---

## 5. (선택) Development View

**관심**: 논리 패키지 구조 / 계층·의존성 / 빌드 모듈 경계

폴더 트리가 아니라 **패키지 다이어그램** 으로 표현 — 각 패키지의 책임과 **패키지 간 의존 방향** 을 설계 관점에서 드러낸다. Layered/Hexagonal/Modular Monolith 등 선정 스타일의 경계가 패키지 구조에 어떻게 반영되는지가 보여야 한다.

```mermaid
graph TB
    subgraph Presentation["Presentation Layer"]
        API[api.rest]
        WS[api.websocket]
    end
    subgraph Application["Application Layer"]
        UC[application.usecase]
        DTO[application.dto]
    end
    subgraph Domain["Domain Layer (Core)"]
        MODEL[domain.model]
        SERVICE[domain.service]
        PORT[domain.port]
    end
    subgraph Infrastructure["Infrastructure Layer"]
        REPO[infra.persistence]
        EXT[infra.external]
    end
    Presentation --> Application
    Application --> Domain
    Infrastructure -.implements.-> PORT
    Infrastructure --> MODEL
```

**패키지 책임 표**

| 패키지 | 책임 | 의존 대상 | 반영 ADSN/DDSN |
|---|---|---|---|
| `api.rest` | REST 엔드포인트, 요청/응답 변환 | `application.usecase` | ADSN_NNN |
| `application.usecase` | 유스케이스 흐름 조립 | `domain.*` | ADSN_NNN |
| `domain.model` | 엔티티·값객체 (순수) | — | ADSN_NNN |
| `domain.port` | 외부 의존의 추상 (DIP) | — | ADSN_NNN |
| `infra.persistence` | `domain.port` 구현 (DB 어댑터) | `domain.port`, `domain.model` | DDSN_NNN |

- 의존 방향 규칙(예: 도메인은 인프라를 참조하지 않는다) 명시
- **반영 ADSN/DDSN**: ...

---

## 6. 관점 간 일관성 점검

| 체크 항목 | 결과 | 비고 |
|-----------|------|------|
| Logical 의 컴포넌트가 Process sequence 에 등장하는가 | ✓ | |
| Logical 의 모듈이 Development 패키지 구조에 반영되는가 | ✓ | |
| Process 의 요소가 Deployment 토폴로지에 배치 가능한가 | ✓ | |
| 모든 ADSN/DDSN 이 최소 1 개 관점에 반영되었는가 | ✓ | |

⚠️ 하나라도 불일치 있으면 Step 3 critical.

---

## 7. 후속 단계 포인트 (SATAM 집중 점검 후보)

- `ADSN_XXX` — Risk {요약}, 영향 QA: Performance, Availability
- `ADSN_YYY` — Risk {요약}, 영향 QA: Security
- ...

(다음 단계 `satam-analyze2` 에서 각 ADSN 의 risk/sensitivity/mitigation 원문이 구조화됨)
