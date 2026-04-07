---
name: doc-manage
description: GoJIRA 프로젝트의 설계문서를 조회, 요약, 비교합니다.
argument-hint: "<list|get|summarize|diff> [options]"
allowed-tools: Bash
---

프로젝트의 설계문서를 관리합니다.

## 파일 목록 조회
!`gojira-cli doc list --project $0`

## 파일 내용 조회
```
gojira-cli doc get --project PROJECT --file FILENAME
gojira-cli doc get --project PROJECT --file FILENAME --ref "docs/duke.kimm"
```

## 문서 요약 (LLM 기반)
```
gojira-cli doc summarize --project PROJECT --file FILENAME
```

## 브랜치 간 Diff 분석 (LLM 기반)
```
gojira-cli doc diff --project PROJECT --file FILENAME
```

### 필수 파일
다음 파일은 프로젝트 생성 시 자동 생성되며 삭제/이름변경 불가:
- `business.md` — 비즈니스 배경, 목표, 스테이크홀더
- `srs.md` — 요구사양서 (VOS → Feature → Requirement)
- `architecture.md` — 구조설계서 (ADSN, 후보안 포함)

결과를 한국어로 정리합니다.
