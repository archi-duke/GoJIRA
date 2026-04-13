# Use Cases

> 본 문서는 `business/project-description.md`와 VOS를 기반으로 Business Domain 컨텍스트와 Use Case를 분석하여 Feature 도출을 위한 기준 문서입니다.
> 다이어그램은 Mermaid 형식으로 작성합니다.

## 1. Business Domain 분석

> Use Case 도출에 앞서 Business Domain을 먼저 분석합니다. 이 분석 결과가 Use Case 도출을 주도합니다.

### 1.1 도메인 경계와 범위

> 시스템이 책임지는 영역과 책임지지 않는 영역을 구분합니다.

- **책임 범위**: (시스템이 수행하는 업무 영역)
- **책임 외 범위**: (시스템이 위임/외부에 맡기는 영역)

### 1.2 핵심 도메인 개념

> 도메인에서 반복적으로 등장하는 개체/업무 용어.

| 개념 | 설명 |
|------|------|
| (예: 게임) | (플레이 1회 단위의 세션) |
| (예: 라운드) | (게임 내 추측 시도 1회) |

### 1.3 Context Diagram

> 시스템과 외부 Actor/외부 시스템의 관계를 표현.

```mermaid
graph LR
  %% 예시
  %% Actor1[관리자]:::actor --> System((시스템))
  %% System --> ExternalSys[외부 시스템]
  %% classDef actor fill:#fef3c7,stroke:#f59e0b
```

### 1.4 Actor 및 외부 시스템

> 각 Actor의 목표(Goals)는 Use Case 도출의 출발점이 됩니다.

| 요소 | 유형 | 목표/역할 |
|------|------|----------|
| (예: 관리자) | Primary Actor | (계정 관리, 권한 설정) |
| (예: 외부 SSO) | 외부 시스템 | (사용자 인증 위임) |

### 1.5 도메인 규칙 / 제약

> 비즈니스 규칙, 정책, 상태 전이, 불변식(invariant) 등.

- (예: 한 라운드당 추측 시도 횟수는 최대 10회)
- (예: 게임 종료 후 기록은 영구 저장)

## 2. Use Case 분석

### 2.1 User Story 목록

> 각 Primary Actor가 시스템을 사용하는 다양한 스토리를 **정상 / 비정상 / 예외** 3종으로 탐색한 결과.
> 이 목록이 Use Case 도출의 근거가 됩니다.

| ID | Actor | 분류 | Story | 매핑 Use Case |
|----|-------|------|-------|---------------|
| US-1 | (Actor) | 정상 | (예: 게임방을 생성하고 고유 링크로 다른 사용자를 초대한다) | UC-1 |
| US-2 | (Actor) | 비정상 | (예: 초대를 보낸 후 상대가 응답하기 전에 스스로 방을 닫는다) | UC-1 |
| US-3 | (Actor) | 예외 | (예: 초대받은 사용자가 거부하여 방이 비어있게 된다) | UC-1 |
| US-4 | (Actor) | 정상 | (예: 추측을 입력하여 스트라이크/볼 피드백을 받는다) | UC-3 |
| ... | ... | ... | ... | ... |

### 2.2 Use Case Diagram

> Actor와 Use Case의 관계를 표현. include / extend / generalization 관계가 있다면 함께 표현.

```mermaid
graph TB
  %% 예시
  %% Actor1[관리자]:::actor
  %% UC1((사용자 계정 관리))
  %% Actor1 --> UC1
  %% classDef actor fill:#fef3c7,stroke:#f59e0b
```

### 2.3 Feature 후보 Use Case 목록

> Actor와 직접 관계를 가지는 Use Case가 Feature가 됩니다. 보조/하위 Use Case는 Feature 목록에서 제외합니다.

| ID | Use Case | Actor | 관련 VOS | Feature 이슈 키 |
|----|---------|-------|---------|----------------|
| UC-1 | (Use Case 이름) | (Actor) | (VOS-키 목록) | (Feature 이슈 키) |

### 2.4 Use Case 시나리오

> 각 Feature 후보 Use Case에 대해 다음 항목을 작성합니다.

#### UC-1: (Use Case 이름)

- **Actor**: (행위 주체)
- **Precondition**: (진입 시점 상태)
- **Basic Flow**: (기본 작업 흐름)
- **Alternative Flow**: (대체/조건부 흐름)
- **Exception Flow**: (예외 처리 흐름)
- **Postcondition**: (종료 상태)

##### Sequence Diagram

```mermaid
sequenceDiagram
  %% 예시
  %% Actor->>System: 요청
  %% System-->>Actor: 응답
```
