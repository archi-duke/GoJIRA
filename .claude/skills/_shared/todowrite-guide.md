# `todowrite` 진행 상태 가시화 가이드 (선택)

OpenCode 환경에서 실행될 때, 내장 `todowrite` tool 을 각 Step 진입/완료에 맞춰 호출하면 사용자가 "지금 어디까지 왔고 남은 게 뭔지" 를 한눈에 볼 수 있다. TPRJ 실증 중 `design-integrate2` 세션 (`ses_24b74df38ffegNWOWXuMm1uqWX`) 에서 GLM-5 가 자발적으로 호출한 사례가 관찰되어 리팩터링 5 스킬에 공식 가이드로 편입.

## 원칙

1. **선택 사항** — 호출 안 해도 스킬 자체는 정상 완주. 강제 아님.
2. **짧게** — todowrite 내부 설명을 장황하게 쓰지 말 것. Step 번호 + 한 줄 요약.
3. **OpenCode 전용** — Claude Code 환경에서는 무시됨 (내장 `TaskCreate/TaskUpdate` 이미 활용).
4. **reasoning 예산 침식 주의** — "한 generation 당 제약" 의 3~5 줄 reasoning 한도 안에서만 사용.

## 호출 타이밍

- **Step 0 진입 직후**: 전체 Step 체크리스트를 초기 상태(`[ ]`) 로 한 번 세팅
- **Step N 시작**: 해당 항목을 `[•]` (in_progress) 로 업데이트
- **Step N 완료 직후**: `[✓]` 로 업데이트
- **종료**: 모두 `[✓]` 또는 스킵된 항목은 `[-]`

## OpenCode `todowrite` 포맷

OpenCode 가 기대하는 입력은 다음과 같은 마크다운:

```
# Todos
[•] Step 1 — 초안 생성 (iter 1)
[ ] Step 2 — 자가검증
[ ] Step 3 — critical 수정
[ ] Step 5 — 문서 저장
```

기호 컨벤션:
- `[ ]` pending
- `[•]` in_progress (현재 작업 중)
- `[✓]` completed
- `[-]` skipped (예: MODE=relink 에서 Step 5.1 이슈 생성 건너뜀)

## 사이즈 가이드

- 스킬 전체 Step 수는 보통 5~7 개 — todowrite 한 번 호출당 전체 리스트 재출력이 비용 효과적
- 항목당 한 줄 (Step 번호 + 짧은 동사). 세부 설명은 SKILL.md 본문에 있으므로 중복 금지
- self-loop 반복(Step 2 ↔ Step 3) 은 단일 항목으로 처리하고 `iter N/3` 식 접미사로 진행도 표시

## 유의사항

- todowrite 는 prompt 토큰을 증가시킴 → 매우 긴 description 금지
- Step 내부 세부 작업(5.1/5.2/5.3 등) 까지 나눌 필요는 없음. 최상위 Step 만으로 충분
- 호출 실패해도 스킬 중단 금지 — try/catch 식 방어 불필요, OpenCode 가 자체 처리
