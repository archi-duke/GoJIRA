---
name: project_query
description: 등록된 프로젝트 목록을 조회합니다. 프로젝트 수, 이름, 상태 등을 확인할 수 있습니다.
---

## Parameters

```yaml
projectCode:
  type: string
  description: 특정 프로젝트 코드로 필터링 (생략하면 전체 조회)
```

## 프로젝트 조회 규칙

- projectCode가 없으면 전체 프로젝트를 조회합니다
- status가 "Deleted"인 프로젝트는 결과에서 제외합니다
- 결과를 마크다운 표로 정리합니다
- 프로젝트 수를 먼저 알려줍니다
