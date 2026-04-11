---
name: issue_delete
description: 이슈를 삭제합니다
---

## Parameters

```yaml
key:
  type: string
  description: "삭제할 이슈 키 (예: GOJIRA-1)"
  required: true
```

## 이슈 삭제 규칙

- key는 필수입니다
- 삭제는 되돌릴 수 없으므로 반드시 사용자 확인이 필요합니다
- 삭제 완료 후 결과를 사용자에게 알려줍니다
