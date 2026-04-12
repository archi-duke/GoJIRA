---
name: reset_project
description: "[개발/테스트 전용] 프로젝트의 모든 이슈(DB+JIRA)와 business/architecture 문서를 초기 템플릿 상태로 리셋합니다. 파괴적 작업이므로 confirm=true 없이 호출하면 영향 범위만 dry-run으로 반환합니다."
---

## Parameters

```yaml
projectCode:
  type: string
  description: "프로젝트 코드 (예: GOJIRA)"
  required: true
confirm:
  type: boolean
  description: "true로 설정해야 실제 실행. false 또는 미지정 시 dry-run으로 영향 범위만 반환."
user:
  type: string
  description: "사용자 ID (사용자 브랜치 docs/{user} 처리용)"
resetIssues:
  type: boolean
  description: "이슈(DB+JIRA) 초기화 여부. 기본 true."
resetDocs:
  type: boolean
  description: "문서 초기화 여부. 기본 true."
```

## 동작

### 1. 이슈 초기화 (`resetIssues=true`)
- 프로젝트 코드 prefix(`{code}-`)에 해당하는 모든 이슈를 GoJIRA-API `DELETE /Issue`로 삭제
- 내부에서 MongoDB 삭제 + JIRA 비동기 삭제 + 모든 프로젝트의 issues 배열에서 키 제거
- 삭제 결과(성공/실패 키 목록) 반환

### 2. 문서 초기화 (`resetDocs=true`)
- 대상 파일:
  - `business/project-description.md`
  - `business/use-cases.md`
  - `business/requirements.md`
  - `architecture/architecture.md`
- 사용자 브랜치(`docs/{user}`): 각 파일을 초기 템플릿으로 덮어쓰기 (PUT)
- main 브랜치: `POST /DocRepo/{code}/init-files` 호출로 누락 파일만 템플릿 생성
  - **참고**: main에 이미 존재하는 파일은 init-files가 덮어쓰지 않습니다 (현 API 한계)

## 안전장치

- `confirm=true`가 없으면 **dry-run** 모드: 영향받는 이슈/파일 목록만 반환, 실제 변경 없음
- system.txt에 "사용자 명시 동의 없이 호출 금지" 정책 있음
- KEYWORD_MAP에 등록하지 않음 (자동 선별 대상에서 제외, 명시 호출만 가능)

## 사용 예

dry-run:
```
{ "projectCode": "GOJIRA" }
→ 영향 범위: 이슈 N건, 파일 4개
```

실행:
```
{ "projectCode": "GOJIRA", "confirm": true, "user": "dukekimm" }
→ 이슈 N건 삭제, 사용자 브랜치 4파일 덮어쓰기, main init-files 호출
```
