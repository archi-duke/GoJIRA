# Mermaid 다이어그램 작성 규칙

모든 Mermaid 다이어그램은 아래 규칙을 반드시 준수한다. 위반 시 브라우저에서 렌더링 오류 발생.

## 노드 ID 규칙
- **영문/숫자/언더스코어만 허용**. 공백/한글/하이픈/특수문자는 ID로 사용 금지
- 한글/공백이 포함된 표시명은 반드시 `[]`(사각형) 또는 `(())`(원형)로 감싸 **label로 지정**

### 올바른 예
```
User[사용자] --> UC1((로그인))
SecAdmin[보안 담당]:::actor --> UC4((감사 로그 조회))
Order_A --> Payment
```

### 잘못된 예
```
✗ 보안 담당:::actor --> UC4     (공백/한글 ID)
✗ 사용자 --> 로그인             (공백/한글 ID)
✗ user-profile --> login        (하이픈은 문맥에 따라 오류)
```

## 화살표 / 관계
- 실선: `A --> B`
- 굵은 선: `A ==> B`
- 점선: `A -.-> B`
- 레이블 있는 선: `A -->|label| B`
- include/extend: `UC1 -.->|include| UC2`, `UC1 -.->|extend| UC3`

## sequenceDiagram
- participant 별칭 사용: `participant User as 사용자`
- 메시지: `User->>System: 요청`
- 응답: `System-->>User: 응답`

## classDef
- 클래스 이름도 영문만: `classDef actor fill:#fef3c7,stroke:#f59e0b`
- 적용: `User[사용자]:::actor`

## 코드블록
- 반드시 \`\`\`mermaid 로 열고 \`\`\` 로 닫기
- 다이어그램마다 별도 코드블록 사용

## 다이어그램 종류별 시작 라인 (반드시 정확히 사용)

| 용도 | 시작 라인 | 절대 사용 금지 |
| --- | --- | --- |
| Context Diagram | `graph LR` | classDiagram, erDiagram |
| **Use Case Diagram** | **`graph TB`** | **classDiagram (절대 금지)**, useCaseDiagram(존재하지 않음) |
| Sequence Diagram | `sequenceDiagram` | graph |
| ER Diagram | `erDiagram` | classDiagram |

### Use Case Diagram 템플릿 (반드시 graph TB 사용 — classDiagram 금지)
```
graph TB
    User[사용자]:::actor
    Admin[관리자]:::actor
    UC1((로그인))
    UC2((게임 시작))
    User --> UC1
    User --> UC2
    Admin --> UC1
    UC2 -.->|include| UC1
    classDef actor fill:#fef3c7,stroke:#f59e0b
```

### 흔한 치명적 오류 (절대 금지)
```
✗ classDiagram                              (Use Case에 classDiagram 사용 금지 — 렌더 실패)
✗ class user[사용자]{}                      (classDiagram에서 [] 사용 불가)
✗ class user[사용자]{ +game[게임] 시작() }  (메서드명에 한글/공백/대괄호 금지)
```

## classDiagram (꼭 필요할 때만 — Use Case 표현용 아님)
- label에 한글/공백 시 반드시 **큰따옴표**: `class User["사용자"] { }`
- `[]` 대괄호는 classDiagram에서 사용 불가
- 메서드/속성 이름도 영문/숫자만 — 한글 금지
