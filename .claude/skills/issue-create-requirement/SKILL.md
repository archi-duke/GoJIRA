---
name: issue-create-requirement
description: Requirement 이슈를 생성합니다. reqType 으로 FR(기능)/NFR(품질속성·QA)/CNST(제약) 을 구분하며, NFR 일 경우 품질속성 시나리오 필드(qa/source/stimulus/environment/artifact/response/measure/responseScenario) 및 평가 점수(qai/qad/qas) 를 함께 입력합니다.
allowed-tools: Bash, Read
disable-model-invocation: true
---

GoJIRA 프로젝트에 새 **Requirement** 이슈를 생성합니다. `reqType` 은 필수이며 NFR 의 경우 품질 속성 시나리오 필드를 최대한 채웁니다. 쓰기 작업이므로 실행 전 사용자 확인이 필요합니다.

## 환경
- `GOJIRA_API` 필수 (launcher 가 주입). 미설정 시 fail-fast
- `X-User-Id` 헤더 필수

## 인자 추출

사용자 메시지에서 다음 값들을 추출해 **먼저** 환경변수로 export 한 뒤 아래 bash 블록들을 실행하라. 누락 시 사용자에게 되물어 확정하라.

- `CODE` — projectCode
- `USER` — user (X-User-Id)
- `REQTYPE` — `FR` | `NFR` | `CNST` (필수)
- `SUMMARY` — 제목
- `DESCRIPTION` — 상세
- `PRIORITY` — (선택) 기본 `Medium`
- `ASSIGNEE` — (선택)
- NFR 전용 (선택): `QA`, `SOURCE`, `STIMULUS`, `ENV`, `ARTIFACT`, `RESPONSE`, `MEASURE`, `RESP_SCENARIO`, `QAI`, `QAD`, `QAS`

예:
```bash
export CODE="GOJIRA3"
export USER="batman"
export REQTYPE="NFR"
export SUMMARY="응답시간 1초 이내"
export DESCRIPTION="..."
# NFR 의 경우:
# export QA="Performance"; export MEASURE="< 1s"
```

## reqType 매핑
- 기능 요구사항(Functional Requirement) → **FR**
- 품질속성/QA(Quality Attribute, 성능/보안/가용성 등) → **NFR**
- 제약사항(Constraint, 기술/규정/환경) → **CNST**

`reqType` 은 `customfield_12555` 에 저장됨.

## Pre-check — 유사 Requirement 조회 (필수)
```bash
curl -s "${GOJIRA_API}/Issue" \
  -H "XBody: {\"prjID\":\"${CODE}\",\"infoType\":\"\"}" \
  -H "X-User-Id: ${USER}" \
  | jq '[.[] | select(.issueType=="Requirement") | {key,subkey,summary,reqType}]'
```

## Step 1 — Requirement 이슈 생성 (POST /Issue)

```bash
GOJIRA_API="${GOJIRA_API:?GOJIRA_API env var required (launcher must inject)}"
SCRIPTS="${GOJIRA_REPO_ROOT:-$(git rev-parse --show-toplevel)}/.claude/skills/_scripts"

RESP=$(cat <<EOF | bash "${SCRIPTS}/issue-write.sh" POST /Issue
{
  "prjID": "${CODE}",
  "issueType": "Requirement",
  "summary": "${SUMMARY}",
  "desc": "${DESCRIPTION}",
  "user": "${USER}"
}
EOF
)
KEY=$(echo "$RESP" | jq -r '.key')
SUBKEY=$(echo "$RESP" | jq -r '.subkey')
```

## Step 2 — reqType + 공통 필드 PUT

```bash
SCRIPTS="${GOJIRA_REPO_ROOT:-$(git rev-parse --show-toplevel)}/.claude/skills/_scripts"
jq -n --arg p "$CODE" --arg k "$KEY" --arg u "$USER" \
    --arg rt "$REQTYPE" --arg pr "$PRIORITY" --arg as "$ASSIGNEE" \
    '{prjID:$p, key:$k, user:$u, data:{reqType:$rt, priority:$pr, assignee:$as}}' \
  | bash "${SCRIPTS}/issue-write.sh" PUT /Issue
```

## Step 3 — NFR 전용: 품질 속성 시나리오 + 평가 점수 PUT

NFR 일 때만 아래 필드를 가능한 한 채웁니다. FR/CNST 는 생략.

| 필드 | customfield | 의미 |
|---|---|---|
| qa | customfield_18217 | 품질속성 이름 (Performance/Security 등) |
| source | customfield_15210 | 자극 발생 주체 |
| stimulus | customfield_15211 | 자극 |
| environment | — | 측정 환경 |
| artifact | customfield_15208 | 자극을 받는 요소 |
| response | customfield_15212 | Artifact 의 반응 |
| measure | customfield_15213 | 측정값/범위 (측정 가능한 수치) |
| responseScenario | customfield_15207 | 시나리오 요약 |
| qai | customfield_15204 | 중요도 점수 |
| qad | customfield_15205 | 난이도 점수 |
| qas | customfield_15206 | 종합 점수 |

```bash
SCRIPTS="${GOJIRA_REPO_ROOT:-$(git rev-parse --show-toplevel)}/.claude/skills/_scripts"
jq -n --arg p "$CODE" --arg k "$KEY" --arg u "$USER" \
    --arg qa "$QA" --arg src "$SOURCE" --arg st "$STIMULUS" \
    --arg env "$ENV" --arg art "$ARTIFACT" --arg rs "$RESPONSE" \
    --arg me "$MEASURE" --arg rsc "$RESP_SCENARIO" \
    --arg qai "$QAI" --arg qad "$QAD" --arg qas "$QAS" \
    '{prjID:$p, key:$k, user:$u,
      data:{qa:$qa, source:$src, stimulus:$st, environment:$env,
            artifact:$art, response:$rs, measure:$me,
            responseScenario:$rsc, qai:$qai, qad:$qad, qas:$qas}}' \
  | bash "${SCRIPTS}/issue-write.sh" PUT /Issue
```

## 규칙
- NFR 평가 점수(qai/qad/qas) 는 별도 평가 단계에서 채워지는 것이 일반적 — 모를 때는 생략
- VOS 로부터 Contains 로 도출되는 것이 보통 (링크는 별도 skill)
- 생성 후 `key` / `subkey` 를 사용자에게 보고
