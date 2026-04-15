---
name: projdesc-interview
description: project-description.md 작성을 위해 에이전트가 직접 사용자와 대화식 인터뷰를 진행합니다. 한 번에 한 질문씩 자연스럽게 물어보고, 충분히 모이면 마크다운 초안을 작성해 docs/{user} 브랜치의 business/project-description.md 에 저장합니다. Claude Code 네이티브 (curl 기반, self-loop 없음).
allowed-tools: Bash, Read
---

## 환경

- `PLATFORM_API` 기본값: `http://localhost:9000/api/v2`
- 저장 경로: `business/project-description.md`

## 인자 추출

사용자 메시지에서 다음 값들을 추출해 **먼저** 환경변수로 export 한 뒤 아래 bash 블록들을 실행하라. 누락 시 사용자에게 되물어 확정하라.

- `CODE` — projectCode (예: GOJIRA3)
- `USER` — user (예: batman) — `docs/{user}` 브랜치로 커밋, `X-User-Id` 헤더로 전송

예:
```bash
export CODE="GOJIRA3"
export USER="batman"
```

## 역할

노련한 시니어 기획자. 이 세션 안에서 **에이전트(Claude) 가 직접** 사용자에게 자연스럽게 질문하고 답변을 채팅으로 수집한다. 별도 Q&A 저장소 없음 — 대화 맥락만으로 충분. 답변이 모호/짧으면 재질문. 모든 항목이 충분히 채워지면 마크다운을 작성해 저장한다.

---

## Step 0 — 기존 문서 확인

```bash
PLATFORM_API="${PLATFORM_API:-http://localhost:9000/api/v2}"

# docs/{user} 우선, 없으면 main
DESC=$(curl -s "${PLATFORM_API}/DocRepo/${CODE}/file?path=business/project-description.md&ref=docs/${USER}" \
  | jq -r '.content // empty' | base64 -d 2>/dev/null)
if [ -z "$DESC" ] || [ "$(echo "$DESC" | wc -c)" -lt 50 ]; then
  DESC=$(curl -s "${PLATFORM_API}/DocRepo/${CODE}/file?path=business/project-description.md&ref=main" \
    | jq -r '.content // empty' | base64 -d 2>/dev/null)
fi
echo "---EXISTING---"; echo "$DESC" | head -80
```

내용이 이미 충분(템플릿 외 실질 30자 이상)이면 사용자에게 "이미 문서가 있습니다. 보강할까요, 새로 작성할까요?" 를 물어본 뒤 선택에 따라 진행. 비었으면 바로 Step 1.

---

## Step 1 — 인터뷰 진행 (에이전트 직접)

### 진행 규칙
1. 먼저 "project-description.md 작성을 위해 몇 가지 여쭙겠습니다" 안내
2. **한 번에 한 질문씩** 자연스러운 말투로 질문
3. 답변이 짧거나 모호하면 **구체적으로 재질문** (예: "조금 더 구체적인 배경이 있을까요?")
4. 답변은 에이전트가 세션 컨텍스트(대화 기록)로만 기억. 중간에 저장 없음.
5. 모든 항목이 충분해지면 사용자에게 "이대로 문서를 생성할까요?" 확인 후 Step 2.

### 질문 항목 (질문 뱅크)

| id | label | 질문 |
|---|---|---|
| background | 과제 배경 | 이 과제를 시작하게 된 배경이나 동기는 무엇인가요? |
| purpose | 과제 목적 | 이 과제가 달성하려는 핵심 목표는 무엇인가요? 성공 지표가 있다면 함께 알려주세요. |
| stakeholders | 주요 이해관계자 | 이 과제의 주요 사용자나 이해관계자는 누구인가요? (직접/간접 모두) |
| requirements | 핵심 요구사항 | 반드시 충족해야 할 주요 요구사항이 있다면 알려주세요. |
| constraints | 제약사항 | 기술적 또는 비즈니스적 제약사항이 있나요? (없으면 '없음') |

### 추가 탐색 (시니어 관점)
- 답변에 누락된 **간접 이해관계자**(운영, 보안, 감사, 파트너) 있으면 "혹시 ~ 관점도 고려해야 할까요?" 로 확인
- 품질 속성(성능/보안/안정성) 암묵 기대가 있으면 질문
- 유사 과제 pitfall 이 의심되면 가볍게 짚어 확인

---

## Step 2 — 마크다운 초안 작성 (에이전트 추론)

수집된 대화 내용을 아래 구조로 정리한다. **사용자 답변 사실은 보존**, 누락 맥락은 시니어 관점으로 합리적 보강하되 해석임을 드러낸다.

### 문서 템플릿

```markdown
# Project Description

## 과제 배경
(배경과 동기를 구체적으로 서술. 대화에 없는 맥락도 시니어 관점에서 보강하되, 추정임을 자연스럽게 드러냄)

## 과제 목적
(핵심 목표를 명확하게 서술. 달성 지표/성공 기준 함께 기술)

## 주요 이해관계자
(직접/간접 이해관계자와 역할. 답변에 누락된 합리적 예상 관련자 포함)

## 주요 사항
(핵심 요구사항, 제약사항, 유사 사례에서 검토되어야 할 잠재 이슈 정리)

> VOS 자동생성 시 이 문서의 내용을 참조합니다.
```

작성 후 사용자에게 **미리보기 제시** 후 수정 요청 받으면 반영.

---

## Step 3 — 저장 (curl PUT)

확정된 마크다운을 `docs/{user}` 브랜치에 저장한다.

```bash
PLATFORM_API="${PLATFORM_API:-http://localhost:9000/api/v2}"

# 마크다운 본문을 /tmp/projdesc.md 에 저장했다고 가정 (에이전트가 Write 로 기록하거나 heredoc 사용)
CONTENT_B64=$(base64 < /tmp/projdesc.md | tr -d '\n')

curl -s -X PUT "${PLATFORM_API}/DocRepo/${CODE}/file" \
  -H "Content-Type: application/json" \
  -H "X-User-Id: ${USER}" \
  -d @- <<EOF
{
  "path": "business/project-description.md",
  "branch": "docs/${USER}",
  "content": "${CONTENT_B64}",
  "message": "[projdesc-interview] 인터뷰 기반 과제 설명 생성",
  "encoding": "base64"
}
EOF
```

저장 응답(status/commit) 확인 후 사용자에게 경로/브랜치 보고.

---

## Step 4 — 최종 보고

```
## 과제 설명 문서 작성 완료

- 경로: business/project-description.md
- 브랜치: docs/{user}
- 수집 항목: background / purpose / stakeholders / requirements / constraints
- 다음 단계: vos-collect2 로 VOS 추출 진행 가능
```

---

## 주의

- **모든 쓰기 호출에 `X-User-Id: ${USER}` 헤더 필수**
- Platform-API 응답 content 는 **base64** → `| base64 -d`
- 저장 시에도 body.content 는 **base64 인코딩** 하여 전송
- 질문은 반드시 **한 번에 하나씩**, 채팅으로 자연스럽게. 다섯 개를 한 번에 쏟지 말 것.
- Self-loop 자가검증 없음 — 문서 품질은 사용자 확인으로 담보
- 한국어로만 대화
