---
name: issue-query
description: GoJIRA 프로젝트의 이슈 목록을 조회합니다. 타입별 / Requirement 세부타입(FR/NFR/CNST) 필터링 지원.
allowed-tools: Bash
---

# 🚨 실행 규칙 (반드시 준수)

1. **아래 단일 bash 블록을 그대로 복사해서 1회 실행**하라. 여러 번 쪼개거나 재작성 금지.
2. **절대 금지**: `RESP=$(curl ...)` 같은 command substitution 으로 응답을 변수에 담기 — bash 가 내부 바이너리를 손상시켜 JSON 이 깨진다 (실제로 30KB → 30.6KB 가 되면서 jq parse error 발생).
3. 이슈 조회는 **`issue-query.sh` 로 로컬 clone 에서 읽는다** — `curl ${GOJIRA_API}/Issue` 로 직접 조회하지 말 것. 이슈 쓰기(POST/PUT /Issue)는 Mongo 를 건드리지 않고 로컬 clone 에만 기록되고, Mongo 는 PR 머지 시점에만 갱신되므로 세션 중에는 방금 만든 이슈가 API 응답에 안 보인다.
4. jq 파싱이 실패하면 기본적으로 위 규칙을 어긴 것이다 — python/grep 으로 우회하지 말고 파일-기반 jq 로 재시도하라.

# 인자

사용자 메시지에서 추출:
- `CODE` — projectCode (참고용, 로컬 clone 자체가 이미 단일 프로젝트라 필터링에 쓰이지 않음)
- `TYPE` — (선택) `VOS` | `Feature` | `Requirement` | `Architectural Design` | 빈 문자열(전체)
- `REQTYPE` — (선택, TYPE=Requirement 일 때) `FR` | `NFR` | `CNST` | 빈 문자열(전체)

# 실행 — 이 bash 블록을 그대로 사용

```bash
export TYPE=""                   # 필요 시 타입 지정
export REQTYPE=""                # 필요 시 reqtype 지정

# 1) 반드시 파일로 저장 ($() 캡처 금지) — 로컬 clone data/issues/*.yaml 에서 읽음
bash "${PROJECT_ROOT}/.claude/skills/_scripts/issue-query.sh" > /tmp/gojira_issues.json

# 2) 개수
echo "total: $(jq 'length' /tmp/gojira_issues.json)"

# 3) 타입별 집계
jq 'group_by(.issueType) | map({type: .[0].issueType, count: length})' /tmp/gojira_issues.json

# 4) 필터링된 상세 목록 (reqtype 은 소문자 필드 — YAML/API 응답 그대로)
jq --arg t "$TYPE" --arg rt "$REQTYPE" '
  [ .[]
    | select($t=="" or .issueType==$t)
    | select($rt=="" or (.reqtype // "")==$rt)
    | {key, subkey, issueType, reqtype, summary, status}
  ]' /tmp/gojira_issues.json
```

# 결과 보고 (한국어)
- 전체 이슈 수 + 타입별 집계를 먼저
- 필요 시 key / 타입 / reqType / 요약 / 상태 마크다운 표
