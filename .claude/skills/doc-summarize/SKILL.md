---
name: doc-summarize
description: GoJIRA 프로젝트의 설계문서를 요약합니다 (LLM 기반)
allowed-tools: Bash
---

프로젝트의 설계문서를 LLM 기반으로 요약합니다.

## 인자 추출

사용자 메시지에서 다음 값들을 추출해 **먼저** 환경변수로 export 한 뒤 아래 bash 블록을 실행하라. 누락 시 사용자에게 되물어 확정하라.

- `CODE` — projectCode
- `FILE` — 요약할 문서 경로

예:
```bash
export CODE="GOJIRA3"
export FILE="architecture/architecture-integrated.md"
```

## 문서 요약

```bash
gojira-cli doc summarize --project "$CODE" --file "$FILE"
```

결과를 한국어로 정리합니다:
- 문서의 주요 내용과 목적을 요약합니다
- 핵심 결정사항을 포함합니다
