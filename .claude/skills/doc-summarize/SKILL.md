---
name: doc-summarize
description: GoJIRA 프로젝트의 설계문서를 요약합니다 (LLM 기반)
argument-hint: "[project-code] --file FILE"
allowed-tools: Bash
---

프로젝트의 설계문서를 LLM 기반으로 요약합니다.

## 문서 요약
!`gojira-cli doc summarize --project $0 --file "$1"`

결과를 한국어로 정리합니다:
- 문서의 주요 내용과 목적을 요약합니다
- 핵심 결정사항을 포함합니다
