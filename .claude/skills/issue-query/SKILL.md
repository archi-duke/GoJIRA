---
name: issue-query
description: GoJIRA 프로젝트의 이슈 목록을 조회합니다. 타입별 / Requirement 세부타입(FR/NFR/CNST) 필터링 지원.
allowed-tools: Bash
---

# 🚨 실행 규칙 (반드시 준수)

1. **아래 단일 bash 블록을 그대로 복사해서 1회 실행**하라. 여러 번 쪼개거나 재작성 금지.
2. **절대 금지**: `RESP=$(curl ...)` 같은 command substitution 으로 응답을 변수에 담기 — bash 가 내부 바이너리를 손상시켜 JSON 이 깨진다 (실제로 30KB → 30.6KB 가 되면서 jq parse error 발생).
3. 반드시 `curl ... > /tmp/gojira_issues.json` 으로 **파일에 직접 저장**한 뒤 `jq ... /tmp/gojira_issues.json` 으로 읽어라.
4. jq 파싱이 실패하면 기본적으로 위 규칙을 어긴 것이다 — python/grep 으로 우회하지 말고 파일-기반 jq 로 재시도하라.

# 인자

사용자 메시지에서 추출:
- `CODE` — projectCode (필수, 예: `GOJIRA3`)
- `USER` — user = X-User-Id (시스템 컨텍스트의 knoxID)
- `TYPE` — (선택) `VOS` | `Feature` | `Requirement` | `Architectural Design` | 빈 문자열(전체)
- `REQTYPE` — (선택, TYPE=Requirement 일 때) `FR` | `NFR` | `CNST` | 빈 문자열(전체)

# 실행 — 이 bash 블록을 그대로 사용

```bash
export CODE="GOJIRA3"            # 실제 프로젝트 코드로 교체
export USER="batman"             # 시스템 컨텍스트의 knoxID
export TYPE=""                   # 필요 시 타입 지정
export REQTYPE=""                # 필요 시 reqType 지정
export GOJIRA_API="${GOJIRA_API:?GOJIRA_API env var required (launcher must inject)}"

# 1) 반드시 파일로 저장 ($() 캡처 금지)
curl -s "${GOJIRA_API}/Issue" \
  -H "XBody: {\"prjID\":\"${CODE}\",\"infoType\":\"\"}" \
  -H "X-User-Id: ${USER}" \
  > /tmp/gojira_issues.json

# 2) 개수
echo "total: $(jq 'length' /tmp/gojira_issues.json)"

# 3) 타입별 집계
jq 'group_by(.issueType) | map({type: .[0].issueType, count: length})' /tmp/gojira_issues.json

# 4) 필터링된 상세 목록
jq --arg t "$TYPE" --arg rt "$REQTYPE" '
  [ .[]
    | select($t=="" or .issueType==$t)
    | select($rt=="" or (.reqType // "")==$rt)
    | {key, subkey, issueType, reqType, summary, status}
  ]' /tmp/gojira_issues.json
```

# 결과 보고 (한국어)
- 전체 이슈 수 + 타입별 집계를 먼저
- 필요 시 key / 타입 / reqType / 요약 / 상태 마크다운 표
