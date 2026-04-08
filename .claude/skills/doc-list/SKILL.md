---
name: doc-list
description: GoJIRA 프로젝트의 설계문서 파일 목록을 조회합니다
argument-hint: "[project-code]"
allowed-tools: Bash
---

프로젝트의 설계문서 파일 목록을 조회합니다.

## 문서 목록 조회
!`gojira-cli doc list --project $ARGUMENTS`

결과를 한국어로 정리합니다:
- 문서 수를 먼저 알려줍니다
- 파일 경로, 파일명, 최종 수정일을 마크다운 표로 정리합니다
