---
name: issue-create-feature
description: Feature(Use Case) 이슈를 생성합니다. 사용자 가치 단위의 기능 묶음과 Use Case Story(actor/precondition/basicflow/alternativeflow/exceptionflow/postcondition) 를 표현할 때 사용합니다. 일반적으로 여러 Requirement 를 Contains 로 묶습니다.
allowed-tools: Bash, Read
disable-model-invocation: true
---

GoJIRA 프로젝트에 새 **Feature** 이슈를 생성합니다. Feature 는 사용자에게 전달되는 가치 단위(Use Case) 이며, Use Case Story 6 필드로 시나리오를 기술합니다. 쓰기 작업이므로 실행 전 사용자 확인이 필요합니다.

## 환경
- `GOJIRA_API` 기본값: `http://localhost:9001/api/v2`
- `X-User-Id` 헤더 필수

## 인자 추출

사용자 메시지에서 다음 값들을 추출해 **먼저** 환경변수로 export 한 뒤 아래 bash 블록들을 실행하라. 누락 시 사용자에게 되물어 확정하라.

- `CODE` — projectCode
- `USER` — user (X-User-Id)
- `SUMMARY` — Feature 제목
- `DESCRIPTION` — Feature 상세
- `ACTOR` — (선택) 행위 주체
- `PRECOND` — (선택) 진입 조건
- `BASIC` — (선택) Basic Flow
- `ALT` — (선택) Alternative Flow
- `EXC` — (선택) Exception Flow
- `POST` — (선택) 종료 조건
- `PRIORITY` — (선택) 기본 `Medium`
- `ASSIGNEE` — (선택)

예:
```bash
export CODE="GOJIRA3"
export USER="batman"
export SUMMARY="자동 로그인"
export DESCRIPTION="..."
export ACTOR="End User"
# ...
```

## Pre-check — 유사 Feature 조회 (필수)
`issue-query` 또는 아래 curl 로 기존 Feature 조회:

```bash
curl -s "${GOJIRA_API}/Issue" \
  -H "XBody: {\"prjID\":\"${CODE}\",\"infoType\":\"\"}" \
  -H "X-User-Id: ${USER}" \
  | jq '[.[] | select(.issueType=="Feature") | {key,subkey,summary,actor}]'
```

유사 이슈가 있으면 재사용/수정/삭제마킹으로 처리.

## Step 1 — Feature 이슈 생성 (POST /Issue)

```bash
GOJIRA_API="${GOJIRA_API:-http://localhost:9001/api/v2}"

RESP=$(curl -s -X POST "${GOJIRA_API}/Issue" \
  -H "Content-Type: application/json" \
  -H "X-User-Id: ${USER}" \
  -d @- <<EOF
{
  "prjID": "${CODE}",
  "issueType": "Feature",
  "summary": "${SUMMARY}",
  "desc": "${DESCRIPTION}",
  "user": "${USER}"
}
EOF
)
KEY=$(echo "$RESP" | jq -r '.key')
SUBKEY=$(echo "$RESP" | jq -r '.subkey')
```

## Step 2 — Use Case Story 필드 업데이트 (PUT /Issue)

| 필드 | customfield | 의미 |
|---|---|---|
| actor | customfield_15215 | 행위 주체 |
| precondition | customfield_15216 | 진입시점 상태 |
| basicflow | customfield_15217 | 기본 작업 흐름 |
| alternativeflow | customfield_15218 | 대체/조건부 흐름 |
| exceptionflow | customfield_15219 | 예외 처리 흐름 |
| postcondition | customfield_15220 | 종료 상태 |

```bash
curl -s -X PUT "${GOJIRA_API}/Issue" \
  -H "Content-Type: application/json" \
  -H "X-User-Id: ${USER}" \
  -d "$(jq -n --arg p "$CODE" --arg k "$KEY" --arg u "$USER" \
    --arg ac "$ACTOR" --arg pre "$PRECOND" --arg bf "$BASIC" \
    --arg af "$ALT" --arg ef "$EXC" --arg po "$POST" \
    --arg pr "$PRIORITY" --arg as "$ASSIGNEE" \
    '{prjID:$p, key:$k, user:$u,
      data:{actor:$ac, precondition:$pre, basicflow:$bf,
            alternativeflow:$af, exceptionflow:$ef, postcondition:$po,
            priority:$pr, assignee:$as}}')"
```

## 규칙
- Basic Flow 가 5단계를 넘으면 별도 Use Case 로 분해 검토
- VOS 와는 `Contains(WBSGantt)` 로, Requirement 와는 `Contains` 관계로 연결 (링크는 별도 skill)
- 생성 후 `key` / `subkey` 를 사용자에게 보고
