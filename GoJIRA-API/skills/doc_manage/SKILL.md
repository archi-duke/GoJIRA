---
name: doc_manage
description: 프로젝트 설계문서를 조회, 요약하거나 main과 사용자 브랜치 간 차이를 분석합니다.
parameters:
  action:
    type: string
    description: 수행할 작업
    enum: [list, summarize, diff]
    required: true
  projectCode:
    type: string
    description: 프로젝트 코드
    required: true
  filePath:
    type: string
    description: "파일 경로 (summarize/diff 시 필수, 예: srs.md)"
actions:
  list:
    method: GET
    path: "/DocRepo/{{.projectCode}}/tree?ref=main"
    platform_api: true
    response_filter_type: file
  summarize:
    method: GET
    path: "/DocRepo/{{.projectCode}}/file?path={{.filePath}}&ref=main"
    platform_api: true
    llm_post_process: true
    prompt_template: skill-doc-summarize.txt
  diff:
    method: GET
    paths:
      - "/DocRepo/{{.projectCode}}/file?path={{.filePath}}&ref=main"
      - "/DocRepo/{{.projectCode}}/file?path={{.filePath}}&ref=docs/{{.user}}"
    platform_api: true
    llm_post_process: true
    prompt_template: skill-diff-explain.txt
protected_files:
  - business.md
  - srs.md
  - architecture.md
---

## 문서 관리 규칙

### list
- 프로젝트의 main 브랜치 파일 목록을 반환합니다
- type이 "tree"인 항목(디렉토리)은 제외합니다

### summarize
- 파일 내용을 조회한 뒤 LLM으로 요약합니다
- main 브랜치에 없으면 사용자 브랜치(docs/{user})에서 조회합니다
- 빈 파일이면 "파일이 비어 있습니다"를 반환합니다

### diff
- main 브랜치와 사용자 브랜치의 파일을 각각 조회합니다
- 내용이 동일하면 "차이가 없습니다"를 반환합니다
- 차이가 있으면 LLM으로 변경사항을 자연어로 해석합니다

### 필수 파일
- business.md, srs.md, architecture.md는 삭제/이름변경 불가
- 내용 편집은 자유
