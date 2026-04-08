---
name: doc-diff
description: main 브랜치와 사용자 브랜치 간의 문서 차이를 분석합니다
argument-hint: "[project-code] --file FILE"
allowed-tools: Bash
---

main 브랜치와 사용자 브랜치 간의 문서 차이를 분석합니다.

## 문서 비교
!`gojira-cli doc diff --project $0 --file "$1"`

결과를 한국어로 정리합니다:
- 추가/삭제/수정된 내용을 구분하여 보여줍니다
- 변경 요약을 제공합니다
