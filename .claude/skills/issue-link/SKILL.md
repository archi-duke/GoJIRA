---
name: issue-link
description: GoJIRA 두 이슈 간의 관계를 설정합니다 (contain, relate, block 등)
allowed-tools: Bash
disable-model-invocation: true
---

두 이슈 간의 관계를 설정합니다. 쓰기 작업이므로 실행 전 사용자 확인이 필요합니다.

## 인자 추출

사용자 메시지에서 다음 값들을 추출해 **먼저** 환경변수로 export 한 뒤 아래 bash 블록을 실행하라. 누락 시 사용자에게 되물어 확정하라.

- `FROM` — 출발(상위) 이슈 키
- `TO` — 도착(하위) 이슈 키
- `TYPE` — 링크 타입 (`contain` | `relate` | `duplicate` | `block`)

예:
```bash
export FROM="GOJIRA-10"
export TO="GOJIRA-20"
export TYPE="contain"
```

## 이슈 링크 생성

```bash
gojira-cli issue link --from "$FROM" --to "$TO" --type "$TYPE"
```

## 사용 가능한 링크 타입
- contain: 포함 관계 (상위 이슈가 하위 이슈를 포함)
- relate: 관련 관계
- duplicate: 중복 관계
- block: 차단 관계 (선행 이슈가 후행 이슈를 차단)

링크 생성 후 결과를 사용자에게 알려줍니다.
