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
