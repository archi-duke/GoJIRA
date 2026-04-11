---
name: doc_list
description: 프로젝트의 설계문서 파일 목록을 조회합니다
parameters:
  projectCode:
    type: string
    description: "프로젝트 코드 (예: GOJIRA)"
    required: true
api:
  method: GET
  path: /Doc
---

## 문서 목록 조회 규칙

- projectCode는 필수입니다
- 파일 경로, 파일명, 최종 수정일을 포함하여 결과를 정리합니다
- 결과를 마크다운 표로 정리합니다
- 문서 수를 먼저 알려줍니다
