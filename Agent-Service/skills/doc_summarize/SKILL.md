---
name: doc_summarize
description: 프로젝트의 설계문서를 요약합니다 (LLM 기반)
---

## Parameters

```yaml
projectCode:
  type: string
  description: "프로젝트 코드 (예: GOJIRA)"
  required: true
filePath:
  type: string
  description: "요약할 문서 파일 경로"
  required: true
```

## 문서 요약 규칙

- projectCode와 filePath 모두 필수입니다
- LLM 기반 요약을 제공합니다
- 문서의 주요 내용, 목적, 핵심 결정사항을 포함하여 요약합니다
- 결과를 한국어로 정리합니다
