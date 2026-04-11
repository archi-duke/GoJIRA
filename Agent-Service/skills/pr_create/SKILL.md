---
name: pr_create
description: 사용자 브랜치에서 main으로 Pull Request를 생성합니다
---

## Parameters

```yaml
projectCode:
  type: string
  description: "프로젝트 코드 (예: GOJIRA)"
  required: true
title:
  type: string
  description: "PR 제목"
body:
  type: string
  description: "PR 본문 설명"
```

## PR 생성 규칙

- projectCode는 필수입니다
- title 생략 시 브랜치명 기반으로 자동 생성됩니다
- 사용자 브랜치에서 main 브랜치로의 PR을 생성합니다
- 생성된 PR 번호와 URL을 사용자에게 알려줍니다
- 쓰기 작업이므로 사용자 확인이 필요합니다
