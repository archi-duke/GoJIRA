---
name: doc-diff
description: main 브랜치와 사용자 브랜치 간의 문서 차이를 분석합니다
allowed-tools: Bash
---

main 브랜치와 사용자 브랜치 간의 문서 차이를 분석합니다.

## 인자 추출

사용자 메시지에서 다음 값들을 추출해 **먼저** 환경변수로 export 한 뒤 아래 bash 블록을 실행하라. 누락 시 사용자에게 되물어 확정하라.

- `CODE` — projectCode (예: GOJIRA)
- `FILE` — 비교할 문서 경로 (예: `business/project-description.md`)

예:
```bash
export CODE="GOJIRA3"
export FILE="business/project-description.md"
```

## 문서 비교

```bash
gojira-cli doc diff --project "$CODE" --file "$FILE"
```

결과를 한국어로 정리합니다:
- 추가/삭제/수정된 내용을 구분하여 보여줍니다
- 변경 요약을 제공합니다
