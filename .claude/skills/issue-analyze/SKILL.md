---
name: issue-analyze
description: GoJIRA 프로젝트의 이슈 관계(Contains 체인, 고아 이슈, 타입 통계)를 분석합니다.
allowed-tools: Bash
---

로컬 clone data/issues/*.yaml 에서 전체 이슈를 가져와 에이전트가 직접 관계를 분석합니다.
(이슈 쓰기는 Mongo 를 건드리지 않고 로컬에만 기록되고 Mongo 는 PR 머지 시점에만 갱신되므로,
세션 중 GoJIRA-API `/Issue` 로 조회하면 방금 만든 이슈가 보이지 않는다 — 그래서 로컬에서 읽는다.)

## 환경 & 인자
- `CODE` — projectCode (참고용)

```bash
export CODE="GOJIRA3"
```

## 이슈 전체 조회
🚨 `RESP=$(curl ...)` 같은 command substitution 캡처 금지 — bash 가 응답 바이트를 손상시킴. **파이프** 또는 **파일 저장** 후 jq 사용.

```bash
bash "${PROJECT_ROOT}/.claude/skills/_scripts/issue-query.sh" > /tmp/gojira_issues.json

jq '[.[] | {key, subkey, issueType, reqtype, summary, status, issueLinks}]' /tmp/gojira_issues.json
```

## 분석 항목 (에이전트가 직접 수행)
1. **Contains 체인**: VOS → Feature → Requirement → ADSN 연결 여부
2. **고아 이슈**: 어떤 상위에도 Contains 되지 않는 이슈
3. **타입/관계별 통계**

결과는 한국어 마크다운 표 + 개선 제안으로 정리.
