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

1. **answers 모드**: 호출 측이 6개 항목(background / purpose / stakeholders / user_stories / qualities / constraints) 답변을 이미 가지고 있음 → 신규 생성
2. **enrich 모드**: 기존 문서가 있고 보강 필요 → 기존 내용 보존 + 누락 섹션 보강

입력은 호출 컨텍스트(상위 에이전트 프롬프트, Read 한 로컬 파일, 또는 Step 0 에서 curl 로 가져온 기존 문서)에서 확보한다.

### 범위 원칙 — **무엇을 만들지가 아니라 누가 어떤 가치를 위해 사용하는지**

project-description.md 는 요구사항 정의 이전 단계의 문서다. 후속 단계(driver-select → style-define → design-decide)에서 아키텍처 결정을 내리는 입력이 되므로, 여기서 설계 결정을 미리 박아두면 후속 단계의 자유도가 사라지고 추적성도 깨진다. 본 스킬은 다음 원칙을 강제한다.

- ✅ user story 관점으로 기능을 서술 — `[역할]로서 [무엇을 할 수 있길 원한다] — [왜/가치]`
- ✅ 품질 속성(NFR)은 사용자가 체감하는 결과로 서술 ("응답을 토큰 단위로 즉시 본다" O / "SSE 로 스트리밍" X)
- ✅ 제약은 조직/정책/외부 환경에서 오는 것만 (보안 정책, 규제, 기존 자산, 벤더 종속 회피 등)
- ❌ "기술 스택", "확정 기술" 절을 만들지 않는다
- ❌ "핵심 아키텍처 결정", "컴포넌트 구성", "통신 방식" 절을 만들지 않는다
- ❌ user story 안에 특정 컴포넌트명/포트/프로토콜/DB명을 박지 않는다 — 외부 시스템명(JIRA, ADFS 등)은 OK
- ❌ 호출 측이 기술 결정을 답으로 줬어도 기능 요구사항으로 옮기지 말 것 — 한 단계 위 가치로 재진술하거나 `constraints` 의 "벤더/플랫폼 제약" 으로만 기록

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
(현재 무엇이 잘 안 되고 있는지 + 왜 지금 시작하는지)

## 과제 목적
(시스템이 무엇을 가능하게 하는가 — "어떻게" 가 아니라)

## 주요 이해관계자
(직접/간접 이해관계자와 관심사 — 표 권장)

## 주요 사항

> 이 섹션은 **무엇을 만들지가 아니라 누가 어떤 가치를 위해 사용하는지** 를 기술합니다.
> 기술 스택 / 컴포넌트 분할 / 통신 방식 / 데이터 모델 등 시스템 구조에 관한 결정은
> 후속 설계 단계(driver-select → style-define → design-decide) 의 책임입니다.

### 핵심 기능 요구사항 (User Story 관점)

- **[역할]로서**, [무엇을 할 수 있길 원하는지] — [왜/어떤 가치를 위해].
- (각 항목은 한 단락. 시스템 구조 표현 금지)

### 핵심 품질 속성 (NFR)

| 속성 | 사용자 관점의 목표 |
|---|---|
| ... | ... |

### 제약사항 (조직/정책/외부 환경)

- (조직 정책/규제/기존 자산 호환/벤더 종속 회피 등 — 기술 선택이 아님)
```

### generate 모드 지침
- 제공된 answers 기반으로 각 섹션 서술. 항목 매핑:
  - `background` → 과제 배경
  - `purpose` → 과제 목적
  - `stakeholders` → 주요 이해관계자
  - `user_stories` → 주요 사항 > 핵심 기능 요구사항 (반드시 user story 형식으로 재진술)
  - `qualities` → 주요 사항 > 핵심 품질 속성
  - `constraints` → 주요 사항 > 제약사항
- 답변이 기술 결정(예: "Mongo 사용", "React SPA") 으로 와도 그대로 옮기지 말 것 — 한 단계 위 사용자 가치로 재진술하거나, 강한 외부 제약이면 `constraints` 로 분류
- 누락된 맥락은 시니어 관점으로 합리적 보강하되, 추정임을 자연스럽게 드러냄
- 간접 이해관계자(운영, 보안, 감사, 파트너) 추가 검토
- 품질 속성 암묵 요구 반영

### enrich 모드 지침
- 기존 내용의 **의미와 사실은 변경 금지** — 보강/명확화만 허용
- 누락 섹션 추가
- 간략 섹션은 유사 사례 경험으로 확장
- 문장 전문적으로 다듬기
- **단, 기존 문서에 "기술 스택" / "아키텍처 결정" 같은 설계 결정 섹션이 있으면 제거하거나
  `제약사항`(외부 제약) / user story 의 가치 표현으로 재진술 후 옮긴다** — 본 스킬의 범위
  원칙에 위배되는 섹션은 보존 대상이 아님

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
