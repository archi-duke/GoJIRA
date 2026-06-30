---
name: issue-create-design
description: 설계 이슈(Architectural Design / Detailed Design) 를 생성합니다. designLevel=Architectural 이면 ADSN(아키텍처 설계+평가+SATAM), Detailed 면 DDSN(상세 설계) 으로 생성하며, ADSN 은 adiagram/bdiagram/dmodel/archdiagram/objectives/pros/cons/score/eval/sensitivity/mitigation/risk/decision 필드를 함께 입력합니다.
allowed-tools: Bash, Read
disable-model-invocation: true
---

GoJIRA 프로젝트에 새 **설계 이슈** 를 생성합니다. `designLevel` 에 따라 issueType 이 결정됩니다:

- `Architectural` → **"Architectural Design"** (ADSN)
- `Detailed` → **"Detailed Design"** (DDSN)

쓰기 작업이므로 실행 전 사용자 확인이 필요합니다.

## 환경
- `GOJIRA_API` 필수 (launcher 가 주입). 미설정 시 fail-fast
- `X-User-Id` 헤더 필수

## 인자 추출

사용자 메시지에서 다음 값들을 추출해 **먼저** 환경변수로 export 한 뒤 아래 bash 블록들을 실행하라. 누락 시 사용자에게 되물어 확정하라.

- `CODE` — projectCode
- `USER` — user (X-User-Id)
- `LEVEL` — `Architectural` 또는 `Detailed` (필수)
- `SUMMARY` — 제목
- `DESCRIPTION` — 상세
- `ADIAGRAM`, `BDIAGRAM` — (선택) PlantUML 코드 (` ```plantuml ` 블록 포함, `@startuml` / `@enduml` 쌍 필수)
- `PRIORITY` — (선택) 기본 `Medium`
- `ASSIGNEE` — (선택)
- ADSN 전용 (선택): `DMODEL`, `ARCHDIAGRAM`, `OBJECTIVES`, `PROS`, `CONS`, `SCORE`, `EVAL`, `SENSITIVITY`, `MITIGATION`, `RISK`, `DECISION`

예:
```bash
export CODE="GOJIRA3"
export USER="batman"
export LEVEL="Architectural"
export SUMMARY="API Gateway 도입"
export DESCRIPTION="..."
```

## Pre-check — 유사 설계 이슈 조회 (필수)
```bash
curl -s "${GOJIRA_API}/Issue" \
  -H "XBody: {\"prjID\":\"${CODE}\",\"infoType\":\"\"}" \
  -H "X-User-Id: ${USER}" -H "Authorization: Bearer ${GOJIRA_SESSION_TOKEN}" \
  | jq '[.[] | select(.issueType=="Architectural Design" or .issueType=="Detailed Design") | {key,subkey,issueType,summary}]'
```

## PlantUML 주의
`adiagram`(정적 구조), `bdiagram`(동적 동작) 필드에 PlantUML 코드를 작성할 때:
- 반드시 ` ```plantuml ` 코드펜스로 감싸고 내부는 `@startuml` ~ `@enduml` 쌍 포함
- 정적: `component` / `package` / `node` / `artifact` / `database` 키워드 사용
- 동적: `participant` / `actor` 선언 + `->` / `-->` / `->>` / `-->>` 화살표
- Use Case 는 `actor` + `usecase` + `rectangle "..."` 로 (class 다이어그램 키워드 `class`/`interface`/`abstract`/`enum` 사용 금지)
- 공백 있는 이름은 `"..."` 따옴표 또는 `as` 별칭 필수 (예: `participant "게임 서버" as Game` / `participant Game as "게임 서버"`)
- 외부 이미지(`![](.../*.png)`) 참조 금지
- 구문 오류 시 PlantUML 서버 렌더 실패

## Step 1 — 설계 이슈 생성 (POST /Issue)

```bash
GOJIRA_API="${GOJIRA_API:?GOJIRA_API env var required (launcher must inject)}"
SCRIPTS="${GOJIRA_REPO_ROOT:-$(git rev-parse --show-toplevel)}/.claude/skills/_scripts"

# LEVEL="Architectural" | "Detailed"
if [ "$LEVEL" = "Architectural" ]; then
  ISSUE_TYPE="Architectural Design"
else
  ISSUE_TYPE="Detailed Design"
fi

RESP=$(cat <<EOF | bash "${SCRIPTS}/issue-write.sh" POST /Issue
{
  "prjID": "${CODE}",
  "issueType": "${ISSUE_TYPE}",
  "summary": "${SUMMARY}",
  "desc": "${DESCRIPTION}",
  "user": "${USER}"
}
EOF
)
KEY=$(echo "$RESP" | jq -r '.key')
SUBKEY=$(echo "$RESP" | jq -r '.subkey')
```

## Step 2 — 공통 다이어그램 PUT (ADSN + DDSN)

| 필드 | customfield | 의미 |
|---|---|---|
| adiagram | customfield_18204 | 정적 구조 다이어그램 (PlantUML) |
| bdiagram | customfield_18205 | 동적 동작 다이어그램 (PlantUML) |

```bash
SCRIPTS="${GOJIRA_REPO_ROOT:-$(git rev-parse --show-toplevel)}/.claude/skills/_scripts"
jq -n --arg p "$CODE" --arg k "$KEY" --arg u "$USER" \
    --arg ad "$ADIAGRAM" --arg bd "$BDIAGRAM" \
    --arg pr "$PRIORITY" --arg as "$ASSIGNEE" \
    '{prjID:$p, key:$k, user:$u,
      data:{adiagram:$ad, bdiagram:$bd, priority:$pr, assignee:$as}}' \
  | bash "${SCRIPTS}/issue-write.sh" PUT /Issue
```

## Step 3 — ADSN 전용 필드 PUT (Architectural 일 때만)

DDSN 은 adiagram/bdiagram 만 의미 있으므로 이 Step 은 **Architectural 일 때만** 실행. DDSN 에 아래 필드를 넣어도 무시됨.

| 필드 | customfield | 의미 |
|---|---|---|
| dmodel | customfield_18218 | 디자인이 적용된 도메인 모델/시스템 영역 |
| archdiagram | customfield_18215 | 디자인의 통합 구조 |
| objectives | customfield_18206 | 주요 목표 |
| pros | customfield_18207 | 장점 |
| cons | customfield_18208 | 단점 |
| score | customfield_18209 | 평가 점수 |
| eval | customfield_18210 | 평가 내용 |
| sensitivity | customfield_18212 | Sensitivity (YAML-like) |
| mitigation | customfield_18213 | Mitigation (YAML-like) |
| risk | customfield_18216 | Risk (YAML-like) |
| decision | customfield_18211 | 채택 여부 — 비어있지 않으면 최종 선택안 |

```bash
SCRIPTS="${GOJIRA_REPO_ROOT:-$(git rev-parse --show-toplevel)}/.claude/skills/_scripts"
jq -n --arg p "$CODE" --arg k "$KEY" --arg u "$USER" \
    --arg dm "$DMODEL" --arg ar "$ARCHDIAGRAM" --arg ob "$OBJECTIVES" \
    --arg pro "$PROS" --arg con "$CONS" --arg sc "$SCORE" --arg ev "$EVAL" \
    --arg se "$SENSITIVITY" --arg mi "$MITIGATION" --arg ri "$RISK" \
    --arg de "$DECISION" \
    '{prjID:$p, key:$k, user:$u,
      data:{dmodel:$dm, archdiagram:$ar, objectives:$ob,
            pros:$pro, cons:$con, score:$sc, eval:$ev,
            sensitivity:$se, mitigation:$mi, risk:$ri, decision:$de}}' \
  | bash "${SCRIPTS}/issue-write.sh" PUT /Issue
```

## 규칙
- ADSN 은 Requirement 에 Contains 관계로 연결되는 것이 일반적
- DDSN 은 ADSN 하위에 위치
- sensitivity / risk / mitigation 은 `specs/issue_schema.md` YAML-like 양식을 따름
- 생성 후 `key` / `subkey` 를 사용자에게 보고
