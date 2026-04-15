---
name: issue-update
description: GoJIRA 기존 이슈의 필드를 수정합니다
allowed-tools: Bash
disable-model-invocation: true
---

기존 이슈의 필드를 수정합니다. 쓰기 작업이므로 실행 전 사용자 확인이 필요합니다.

## 인자 추출

사용자 메시지에서 다음 값들을 추출해 **먼저** 환경변수로 export 한 뒤 아래 bash 블록을 실행하라. 최소 `KEY` + 변경 필드 하나 이상이 필요. 누락 시 사용자에게 되물어 확정하라.

- `KEY` — 이슈 키 (필수, 예: GOJIRA-1)
- `STATUS` — (선택) `To Do` | `In Progress` | `Done`
- `SUMMARY` — (선택) 새 제목
- `PRIORITY` — (선택) `Highest` | `High` | `Medium` | `Low` | `Lowest`

예:
```bash
export KEY="GOJIRA-1"
export STATUS="In Progress"
# export SUMMARY="새 제목"
# export PRIORITY="High"
```

## 상태 변경 (또는 제목/우선순위 부분 업데이트)

변경할 필드만 전달해 실행 (빈 변수는 옵션 자체를 생략):

```bash
ARGS=(--key "$KEY")
[ -n "$STATUS" ] && ARGS+=(--status "$STATUS")
[ -n "$SUMMARY" ] && ARGS+=(--summary "$SUMMARY")
[ -n "$PRIORITY" ] && ARGS+=(--priority "$PRIORITY")
gojira-cli issue update "${ARGS[@]}"
```

## 사용 가능한 상태
- To Do / In Progress / Done

## 사용 가능한 우선순위
- Highest / High / Medium / Low / Lowest

변경할 필드만 전달합니다 (부분 업데이트).
