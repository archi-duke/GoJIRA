# Use Cases

> 본 문서는 `business/project-description.md`와 VOS를 기반으로 Business Domain 컨텍스트와 Use Case를 분석하여 Feature 도출을 위한 기준 문서입니다.
> 다이어그램은 Mermaid 형식으로 작성합니다.

## 1. Business Domain 분석

### Context Diagram

> 시스템과 외부 Actor/외부 시스템의 관계를 표현하는 컨텍스트 다이어그램.

```mermaid
graph LR
  %% 예시
  %% Actor1[관리자]:::actor --> System((시스템))
  %% System --> ExternalSys[외부 시스템]
  %% classDef actor fill:#fef3c7,stroke:#f59e0b
```

### 등장 요소 설명

| 요소 | 유형 | 설명 |
|------|------|------|
| (예: 관리자) | Actor | (예: 사용자 계정 관리 권한을 가진 운영 담당자) |
| (예: 외부 SSO) | 외부 시스템 | (예: knox 인증 서비스) |

## 2. Use Case 분석

### Use Case Diagram

> Actor와 Use Case의 관계를 표현. include / extend / generalization 관계가 있다면 함께 표현.

```mermaid
graph TB
  %% 예시
  %% Actor1[관리자]:::actor
  %% UC1((사용자 계정 관리))
  %% Actor1 --> UC1
  %% classDef actor fill:#fef3c7,stroke:#f59e0b
```

### Feature 후보 Use Case 목록

> Actor와 직접 관계를 가지는 Use Case가 Feature가 됩니다. 보조/하위 Use Case는 Feature 목록에서 제외합니다.

| ID | Use Case | Actor | 관련 VOS | Feature 이슈 키 |
|----|---------|-------|---------|----------------|
| UC-1 | (Use Case 이름) | (Actor) | (VOS-키 목록) | (Feature 이슈 키) |

### Use Case 시나리오

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
