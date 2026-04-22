# SKILL.md 의 bash/curl 인라인 — Claude vs ChatGPT 관점 비교

## 발단

> "에이전트 스킬에서 bash 또는 curl 스크립트를 SKILL.md 에 정리하는 게 일반적인가?"

ChatGPT 의 답변은 **스크립트를 분리하라**는 것. SKILL.md 에는 요약 + 핵심 예제만 두고, 실행 가능한 bash 는 `scripts/` 하위 파일로 빼는 구조를 추천.

```
skills/
 ├─ jira-create/
 │   ├─ skill.md         (요약·입력·실패 복구·위험도)
 │   ├─ scripts/create_issue.sh
 │   └─ schema.json
```

논지: "curl 만 잔뜩 넣은 SKILL.md 는 문서가 아니라 스크립트 저장소다. 좋은 스킬 문서는 언제 써야 하는가 / 입력값 / 실패 복구 / 승인 / 위험도 가 먼저이고 bash/curl 은 보조여야 한다."

## ChatGPT 주장에서 맞는 부분

- **재사용되는 공통 동작은 스크립트로 빼는 게 맞다** — GoJIRA repo 는 이미 이 패턴을 쓰고 있음:
  - `.claude/skills/_scripts/doc-read.sh`, `doc-save.sh`
  - `issue-batch-create.sh`, `issue-batch-link.sh`
  - 여러 스킬이 공유하는 "이슈 생성/링크/문서 읽기·저장" 은 모두 외부 스크립트. SKILL.md 에서는 `bash .../doc-save.sh ...` 한 줄로 호출만 한다.
- **메타 정보가 필요하다** 는 지적도 맞다 — "언제 써야 하는가 / 입력값 / 실패 복구" 같은 내용. 이 세션에서 `requirement-refine2/SKILL.md` 에 "이 스킬을 언제 호출하나" 블록을 추가한 것이 정확히 그 필요에 대응.

## 근본적으로 다른 점 — SKILL.md 는 "문서" 가 아니라 "프롬프트"

ChatGPT 는 SKILL.md 를 **사람이 읽는 문서**로 간주했다. 실제로는 **LLM 이 읽고 그 자리에서 실행하는 프롬프트**이다. 두 대상은 최적화 방향이 다르다.

| 사람용 문서 | LLM 프롬프트 |
|---|---|
| 중복 최소화 (DRY) | 필요한 맥락은 같은 문서에 (cache-miss 감소) |
| 구조화된 추상 설명 | **구체 예시**가 hallucination 을 줄임 |
| 외부 파일 참조 자유 | 참조마다 LLM 이 "뭘 해야 하지?" 다시 추론 |

Bash 를 SKILL.md 밖으로 빼면:
- 에이전트가 `bash ./scripts/create_issue.sh` 를 **무슨 인자로 호출할지** 를 다시 추론해야 한다.
- 인자 실수 → 실패 → 재시도 루프 → 토큰 낭비 + 디버깅 난이도 상승.
- curl body 의 `jq -n --arg ... '{...}'` 같은 미묘한 형식은 **보는 즉시** 따라 할 때 가장 안전하다.

반면 SKILL.md 안에 구체 bash 블록이 있으면 LLM 이 그 패턴을 그대로 카피/수정해서 실행. 실패율이 낮다.

## GoJIRA repo 의 현재 구조는 이미 제대로 나뉘어 있음

```
.claude/skills/
├── _scripts/                        ← 공통 유틸 (여러 스킬 공유)
│   ├── doc-read.sh / doc-save.sh
│   ├── issue-batch-create.sh
│   └── issue-batch-link.sh
├── requirement-refine2/SKILL.md     ← 스킬별 오케스트레이션 (bash 인라인)
├── usecase-analyze2/SKILL.md
└── vos-collect2/SKILL.md
```

**공통 동작은 스크립트, 스킬별 Step 0~6 흐름은 SKILL.md 안에 구체 bash 로**. ChatGPT 가 말한 "분리 패턴" 이 이미 실질적으로 적용돼 있다.

## 진짜 개선 여지 (ChatGPT 가 놓친 부분)

이번 세션에서 실제로 부족했던 건 "스크립트 분리" 가 아니라:

1. **메타 정보 블록 강화** — "언제 호출하나 / MODE 선택 / 실패 복구".  
   → requirement-refine2 / usecase-analyze2 에 "이 스킬을 언제 호출하나" 블록 추가 (2026-04-22).

2. **불변 규칙을 스크립트에 옮겨 하드닝** — 예: `issue-batch-link.sh` 에 `--skip-existing` 플래그를 추가해 중복 POST 를 **원천 차단**. SKILL.md 가 "차집합 계산해서 넘겨" 라고 지시해도 LLM 이 실수로 안 지키면 중복 발생. 스크립트 가드가 있으면 안전. (부분적 TODO)

3. **Agent 의 판단 가이드** — "승인 필요 / 위험도". 이 repo 는 내부 API 위주라 대부분 불필요하지만, 파괴적 액션(이슈 삭제, 브랜치 강제 푸시 등) 이 들어갈 때 필요.

## 결론

ChatGPT 의 방향성은 타당하지만, "SKILL.md 에 bash 가 많은 건 문서가 아니라 스크립트 저장소다" 라는 비판은 **LLM 프롬프트로서의 SKILL.md 를 사람 문서 기준으로 오해한 것**이다.

실무적으로는:
- **공통 유틸** → `_scripts/` (✓ 이미 반영)
- **스킬별 오케스트레이션** → SKILL.md 안에 구체 bash 유지 (LLM 신뢰성 확보)
- **메타 정보(when/why/how-to-recover)** → SKILL.md 상단에 블록 형태로 보강 (✓ 이번에 보강)
- **불변 규칙 강제** → 스크립트 측에 가드 추가 (TODO)

SKILL.md 가 500 줄을 넘기 시작하면 분리 고민을 시작해도 된다. 현재 `requirement-refine2` 가 330 줄 정도라 아직 밀도 적정.

## 참고 문맥

- 관련 커밋: `b959abb` Feature-Requirement/VOS-Feature 링크 재정리 (MODE=relink) + 중복 렌더 수정
- 관련 스킬: `.claude/skills/requirement-refine2/SKILL.md`, `.claude/skills/usecase-analyze2/SKILL.md`
- 공통 스크립트: `.claude/skills/_scripts/issue-batch-{create,link}.sh`, `doc-{read,save}.sh`
