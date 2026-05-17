---
name: projdesc-generate
description: 이미 수집된 답변(호출 컨텍스트로 전달) 또는 기존 부분 문서를 기반으로 project-description.md 를 비대화식으로 생성/보강하여 docs/{user} 브랜치에 저장합니다. 인터뷰 없음 — 컨텍스트가 선행 수집되어 있어야 합니다. Claude Code 네이티브 (curl 기반).
allowed-tools: Bash, Read
---

## 환경

- `PLATFORM_API` 기본값: `http://10.211.55.3:44350/api/v2`
- 저장 경로: `business/project-description.md`

## 인자 추출

사용자 메시지에서 다음 값들을 추출해 **먼저** 환경변수로 export 한 뒤 아래 bash 블록들을 실행하라. 누락 시 사용자에게 되물어 확정하라.

- `CODE` — projectCode
- `USER` — user — `docs/{user}` 브랜치, `X-User-Id` 헤더
- `MODE` — (선택) `generate` 또는 `enrich` (미지정 시 Step 0 결과로 판정)

예:
```bash
export CODE="GOJIRA3"
export USER="batman"
# export MODE="enrich"
```

## 역할

노련한 시니어 기획자 관점으로 **이미 주어진 입력**을 문서화한다. 대화/질문 없음. 입력 소스는 두 가지:

1. **answers 모드**: 호출 측이 5개 항목(background/purpose/stakeholders/requirements/constraints) 답변을 이미 가지고 있음 → 신규 생성
2. **enrich 모드**: 기존 문서가 있고 보강 필요 → 기존 내용 보존 + 누락 섹션 보강

입력은 호출 컨텍스트(상위 에이전트 프롬프트, Read 한 로컬 파일, 또는 Step 0 에서 curl 로 가져온 기존 문서)에서 확보한다.

---

## Step 0 — 입력 확보

### 기존 문서 조회 (enrich 판단용)

```bash
PLATFORM_API="${PLATFORM_API:-http://10.211.55.3:44350/api/v2}"

# docs/{user} 우선, fallback main (스크립트 자동 처리)
EXISTING=$(bash "${PROJECT_ROOT}/.claude/skills/_scripts/doc-read.sh" "${CODE}" "${USER}" "business/project-description.md" 2>/dev/null || true)
echo "---EXISTING---"; echo "$EXISTING"
```

### 모드 판정
- 호출 컨텍스트에 5개 답변이 모두 제공됨 → **generate**
- 답변은 없고 기존 문서가 있음 → **enrich**
- 둘 다 없음 → **중단**, 사용자에게 "projdesc-interview 를 먼저 실행하세요" 보고

---

## Step 1 — 문서 작성 (에이전트 추론)

### 공통 템플릿

```markdown
# Project Description

## 과제 배경
...

## 과제 목적
...

## 주요 이해관계자
...

## 주요 사항
...

> VOS 자동생성 시 이 문서의 내용을 참조합니다.
```

### generate 모드 지침
- 제공된 answers 기반으로 각 섹션 서술
- `background` → 과제 배경, `purpose` → 과제 목적, `stakeholders` → 주요 이해관계자, `requirements`+`constraints` → 주요 사항
- 누락된 맥락은 시니어 관점으로 합리적 보강하되, 추정임을 자연스럽게 드러냄
- 간접 이해관계자(운영, 보안, 감사, 파트너) 추가 검토
- 품질 속성(성능/보안/안정성) 암묵 요구 반영

### enrich 모드 지침
- 기존 내용의 **의미와 사실은 변경 금지** — 보강/명확화만 허용
- 누락 섹션 추가
- 간략 섹션은 유사 사례 경험으로 확장
- 문장 전문적으로 다듬기

마크다운 원문만 생성. 코드블록 감싸지 말 것.

---

## Step 2 — 저장 (curl PUT)

완성된 마크다운을 `/tmp/projdesc.md` 로 기록(Write 도구)한 뒤:

```bash
cat /tmp/projdesc.md | bash "${PROJECT_ROOT}/.claude/skills/_scripts/doc-save.sh" "${CODE}" "${USER}" "business/project-description.md"
```

---

## Step 3 — 최종 보고

```
## 과제 설명 문서 생성 완료 (projdesc-generate)

- 모드: generate | enrich
- 경로: business/project-description.md
- 브랜치: docs/{user}
- 저장 결과: 성공/실패 + commit 정보
- 다음 단계: vos-collect2
```

---

## 주의

- **대화/질문 없음** — 입력이 부족하면 중단하고 projdesc-interview 안내
- `X-User-Id: ${USER}` 헤더 필수
- body.content 는 **base64 인코딩**
- Platform-API 응답 content 디코딩은 `| base64 -d`
- Self-loop 없음 — 단일 패스 산출
- 한국어로만 응답
