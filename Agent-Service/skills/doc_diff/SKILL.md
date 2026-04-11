---
name: doc_diff
description: main 브랜치와 사용자 브랜치 간의 문서 차이를 분석합니다
---

## Parameters

```yaml
projectCode:
  type: string
  description: "프로젝트 코드 (예: GOJIRA)"
  required: true
filePath:
  type: string
  description: "비교할 문서 파일 경로"
  required: true
```

## 문서 비교 규칙

- projectCode와 filePath 모두 필수입니다
- main 브랜치와 사용자 브랜치 간 차이를 비교합니다
- 추가/삭제/수정된 내용을 구분하여 보여줍니다
- 변경 요약을 한국어로 정리합니다
