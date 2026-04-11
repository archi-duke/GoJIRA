---
name: doc_history
description: "프로젝트 문서 저장소의 커밋 이력을 조회합니다. 브랜치별 커밋 로그와 변경 이력을 확인할 수 있습니다."
---

## Parameters

```yaml
projectCode:
  type: string
  description: "프로젝트 코드 (예: GOJIRA)"
  required: true
branch:
  type: string
  description: "브랜치명 (기본값: main)"
limit:
  type: string
  description: "조회할 커밋 수 (기본값: 20)"
```

# 규칙

- projectCode는 필수입니다
- branch를 지정하지 않으면 main 브랜치의 커밋을 조회합니다
- 사용자 브랜치를 조회하려면 branch에 "docs/사용자ID"를 지정합니다
- 결과에는 커밋 SHA, 메시지, 작성자, 날짜가 포함됩니다
- 커밋 이력을 마크다운 표로 정리하세요
