---
name: issue-create-vos
description: VOS(이해관계자 요구) 이슈를 생성합니다. 이해관계자의 원시 요구사항(Voice of Stakeholder)을 가공 없이 기록할 때 사용합니다. 요청 주체(client knoxID) 와 사용자 관점 어투의 summary/description 이 핵심.
allowed-tools: Bash, Read
disable-model-invocation: true
---

GoJIRA 프로젝트에 새 **VOS** 이슈를 생성합니다. VOS 는 가공되지 않은 이해관계자의 목소리를 그대로 담는 이슈이며, 이후 Requirement(FR/NFR/CNST) 로 정제되어 Contains 관계로 연결됩니다. 쓰기 작업이므로 실행 전 사용자 확인이 필요합니다.

## 환경
- `GOJIRA_API` 기본값: `http://10.211.55.3:44360/api/v2`
- `X-User-Id` 헤더 필수 (누락 시 403)

## 인자 추출

사용자 메시지에서 다음 값들을 추출해 **먼저** 환경변수로 export 한 뒤 아래 bash 블록들을 실행하라. 누락 시 사용자에게 되물어 확정하라.

- `CODE` — projectCode
- `USER` — user (X-User-Id)
- `SUMMARY` — 요구사항 제목 (사용자 관점 어투)
- `DESCRIPTION` — 요구사항 상세 (사용자 관점 어투)
- `CLIENT` — 요구를 제기한 이해관계자 knoxID
- `PRIORITY` — (선택) 기본 `Medium`
- `ASSIGNEE` — (선택) 담당자 knoxID

예:
```bash
export CODE="GOJIRA3"
export USER="batman"
export SUMMARY="로그인 상태를 오래 유지하면 좋겠어요"
export DESCRIPTION="자주 쓰는 기기에서 반복 로그인하지 않게 30일 유지가 필요해요"
export CLIENT="alice.kim"
export PRIORITY="Medium"
# export ASSIGNEE=""
```

## Pre-check — 유사 VOS 조회 (필수)
생성 전에 반드시 `issue-query` 또는 아래 curl 로 동일 프로젝트의 기존 VOS 를 조회하여 중복/유사 이슈 존재 여부를 확인합니다. 유사 이슈가 있으면 생성 대신 **재사용 / 수정 / 삭제마킹** 으로 처리.

```bash
curl -s "${GOJIRA_API}/Issue" \
  -H "XBody: {\"prjID\":\"${CODE}\",\"infoType\":\"\"}" \
  -H "X-User-Id: ${USER}" \
  | jq '[.[] | select(.issueType=="VOS") | {key,subkey,summary,client}]'
```

## Step 1 — VOS 이슈 생성 (POST /Issue)

```bash
GOJIRA_API="${GOJIRA_API:-http://10.211.55.3:44360/api/v2}"
SCRIPTS="${GOJIRA_REPO_ROOT:-$(git rev-parse --show-toplevel)}/.claude/skills/_scripts"

RESP=$(cat <<EOF | bash "${SCRIPTS}/issue-write.sh" POST /Issue
{
  "prjID": "${CODE}",
  "issueType": "VOS",
  "summary": "${SUMMARY}",
  "desc": "${DESCRIPTION}",
  "user": "${USER}"
}
EOF
)
KEY=$(echo "$RESP" | jq -r '.key')
SUBKEY=$(echo "$RESP" | jq -r '.subkey')
echo "Created: $KEY ($SUBKEY)"
```

## Step 2 — 추가 필드 업데이트 (PUT /Issue)
`client` / `priority` / `assignee` 는 POST 가 직접 받지 않으므로 생성 후 PUT 으로 설정:

```bash
SCRIPTS="${GOJIRA_REPO_ROOT:-$(git rev-parse --show-toplevel)}/.claude/skills/_scripts"
jq -n --arg p "$CODE" --arg k "$KEY" --arg u "$USER" \
    --arg cl "$CLIENT" --arg pr "$PRIORITY" --arg as "$ASSIGNEE" \
    '{prjID:$p, key:$k, user:$u, data:{client:$cl, priority:$pr, assignee:$as}}' \
  | bash "${SCRIPTS}/issue-write.sh" PUT /Issue
```

## 규칙
- VOS 는 **사용자 관점 어투** ("~가 필요해요 / ~하면 좋겠어요") 로 작성
- `client` 에는 요구를 제기한 이해관계자의 knoxID 기록 (customfield_15235 매핑)
- 생성 후 반환된 `key` 와 `subkey` 를 사용자에게 보고
- 링크(Contains 등) 는 별도 skill 에서 처리
