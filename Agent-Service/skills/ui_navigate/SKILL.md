---
name: ui_navigate
description: "GoJIRA 화면을 특정 페이지로 이동시킵니다. 프로젝트의 Relation Map, SATAM, Issues, Docs 등을 열 수 있습니다."
---

## Parameters

```yaml
projectCode:
  type: string
  description: "프로젝트 코드 (예: GOJIRA)"
  required: true
page:
  type: string
  description: "이동할 페이지"
  required: true
  enum:
    - relationMap
    - satam
    - issues
    - docs
```

# 규칙

- 사용자가 "Relation Map 보여줘", "SATAM 보여줘", "이슈 목록 열어줘", "문서 보여줘" 등을 요청하면 이 도구를 사용합니다.
- projectCode는 반드시 project_query로 확인한 정확한 코드를 사용하세요.
- page 값: relationMap, satam, issues, docs
- 이 도구는 사용자의 브라우저 화면을 전환합니다.
- 도구 실행 후 "Relation Map을 열었습니다" 등으로 안내하세요.
