# Tactics for Skill — GLM 계열 약한 LLM 에서 워크플로우 스킬 안정 운영 전략

## 배경

사내 운영 제약으로 Claude 대신 **GLM-4.x / GLM-5** 를 에이전트 런타임으로 사용해야 함. 현재 워크플로우 스킬(`*2`) 은 Claude 를 암묵적 기준으로 설계되어 있어 GLM 에서 긴 reasoning 후 **스톨(빈 text part 남기고 종료 이벤트 안 찍힘)** 이 반복 발생.

- 관찰된 스톨 패턴:
  - reasoning 10k~17k 자 폭주 후 `text` part 가 빈 상태로 중단
  - terminal 토큰도 안 나옴 → 서버 측 실제 생성 중단
  - `finish=None` 상태로 무한 대기
- 대표 사례: `style-define2` — 세션 `8dc5a7e0`, `185576c1` 에서 동일 패턴

모델 업그레이드(GLM-5) 만으로는 구조적 원인(컨텍스트 한계, 장추론 관리) 을 다 해결 못 함. **스킬 설계 자체를 GLM 친화적으로 재구성** 하는 게 근본 해결.

---

## 개념 정리 — turn / generation / reasoning

리팩터링 전략을 이해하려면 세 단위를 구분해야 함:

- **Turn (대화 턴)**: 사용자 메시지 → 에이전트 응답 → 종료 한 주기. 세션 message history 는 턴을 넘어 누적됨
- **Generation (LLM 호출)**: 모델이 토큰을 생성하는 한 번의 호출. 한 턴에 tool call 이 끼면 여러 generation 이 발생 (Gen 1 → tool 실행 → Gen 2 → …)
- **Reasoning (추론 토큰)**: 각 generation 내부에서 모델이 생성하는 사고 토큰. **Generation 경계에서 리셋** 되며 다음 generation 에 전달되지 않음 (최종 출력 text/tool_use 만 전달)

**GLM 스톨의 실제 원인**: 한 generation 안에서 reasoning 이 내부 한계 (~10k+ 자) 를 넘으면 빈 text part 로 종료. 세션이 길어서도, 턴이 많아서도 아님.

따라서 리팩터링의 핵심은 **"한 generation 당 reasoning 분량 축소"**. Turn 을 나눌 필요는 없고 **tool call 로 generation 경계만 만들면** 충분. 사용자는 1 턴 입력 그대로, 내부적으로 여러 generation 이 이어지는 식으로 자동 진입 가능.

---

## 스킬별 GLM 스톨 위험 프로필

| 스킬 | 줄수 | Step | "에이전트가 직접" 언급 | tool 호출 | self-loop | 리스크 |
|---|---|---|---|---|---|---|
| `vos-collect2` | 211 | 7 | 3 | 3 | 없음 | 중 |
| `usecase-analyze2` | 346 | 7 | 0 | 5 | 3회 | **상** (긴 문서 + Mermaid 3종) |
| `requirement-refine2` | 332 | 7 | 1 | 5 | 3회 | 중 (차집합은 이미 tool화) |
| `driver-select2` | 244 | 7 | 0 | 2 | 없음 | 중 |
| `style-define2` | 285 | 7 | **3** | 2 | 3회 | **최상** — 현재 검증된 문제 |
| `design-decide2` | 271 | 7 | 0 | 3 | 3회 | 상 (대안 비교 reasoning 폭발) |
| `design-integrate2` | 200 | 7 | 0 | 1 | 없음 | 중상 (긴 통합 문서) |
| `satam-analyze2` | 189 | 7 | 1 | 0 | 3회 | 상 (tool 없음 — 전부 reasoning) |

**열 설명**:
- *"에이전트가 직접" 언급*: reasoning 폭주 유발 문구의 직접 신호
- *tool 호출*: `_scripts/` 쉘 호출 건수 — 낮을수록 LLM 부담 큼
- *self-loop*: 한 턴에 누적되는 iter 횟수

---

## 공통 취약 패턴 4 가지

### P1. "에이전트가 직접 [계산/규칙 적용/평가]" 문구
- 해당 스킬: `style-define2`, `satam-analyze2`, `vos-collect2`
- 결과: 결정적 규칙을 매번 reasoning 으로 재생성 — 토큰 낭비 + 오차

### P2. Self-loop 3회 를 한 턴에 몰아서
- 해당 스킬: `usecase-analyze2`, `requirement-refine2`, `style-define2`, `design-decide2`, `satam-analyze2`
- 결과: iter 마다 이전 context 누적 → 컨텍스트 폭발 → 스톨

### P3. Tool 호출 1~2회 + 나머지는 LLM 내부
- 해당 스킬: `style-define2` (tool 2회), `driver-select2` (2회), `satam-analyze2` (0회)
- 결과: 계산/판단을 코드로 offload 안 하고 전부 LLM 어깨에

### P4. 긴 문서 + 다이어그램 출력 요구
- 해당 스킬: `usecase-analyze2` (Mermaid 3종), `design-integrate2` (4관점 통합)
- 결과: 한 번에 수천 자 output 요구 → 긴 reasoning 후 출력 단계 전환 실패

---

## 계산의 3 가지 유형 — 스크립트화 가능성

| 유형 | 특성 | 스크립트화 | 예 |
|---|---|---|---|
| **A. 결정적 규칙 적용** | 입력 → 규칙 → 숫자/bool | 100% | 카탈로그 점수, 필드 존재, ID 중복, 집합 교차 |
| **B. 구조 검증 + 의미 판단 혼합** | 일부 mechanical, 일부 judgment | 분리 가능 | "relatedFeatures 키 존재" (A) + "이 FR 이 Feature 에 의미적으로 맞는가" (C) |
| **C. 순수 판단** | 의미 해석, 새 개념 도출 | 불가 — LLM 필수 | VOS→Story 파생, 도메인 분류, CNST "추측성" 판단 |

**핵심 통찰**: 복잡해 보이는 점수도 대부분 **결정적 함수**. LLM 이 매번 같은 계산을 재생성하는 것 자체가 낭비 + 오차.

### 스킬별 분해 가능성 예시

**style-define2** — A 중심
```
Step 1. 점수 계산 (fit/avoid/QA/decision_rules/조합 가감)  → A 100%
        ※ Agent-Service/engine.py 이미 구현됨
Step 2. Primary/Secondary/Supporting 선정 (tie_threshold/scope) → A 100%
Step 3. 문서 narrative 작성                                  → C (LLM)
```
**약 85% 스크립트화 가능**. LLM 은 최종 문서 자연어만.

**requirement-refine2 critical** — A 대부분
```
- 필수 필드 누락 / docId 중복 / relatedFeatures hallucination → A
- User Story 형식 정규식 "은(는) .* 을 위해 .* 할 수 있다"  → A
- QAS 6 요소 필드 존재                                        → A
- Feature 커버리지 set diff                                   → A
- mode=reuse인데 existingKey 미존재                           → A
- CNST "추측성 제약"                                          → C
- FR "세분화 부족"                                            → B
```
critical 10 개 중 **7~8 개 완전 스크립트**.

**usecase-analyze2** — 혼합
```
검증 (Type A):
- VOS→Feature set diff / Mermaid classDiagram 사용 검출
- Mermaid 쉼표 수신자 / UC 다이어그램 완전성 / 과도 통합
도출 (Type C):
- Use Case 그룹핑 / Actor 식별 / Feature 선정
```
**검증 70% 스크립트, 도출은 LLM**.

---

## 리팩터링 프레임워크 R1 ~ R4

### R1. 계산/규칙 적용을 스크립트로
- `_scripts/` 에 재사용 유틸 추가:
  - `style-score.sh` — Agent-Service 의 기존 `engine.py` 얇은 wrapper
  - `validate-requirements.py` — FR/NFR/CNST 필수 필드 + 형식 검증
  - `validate-usecases.py` — VOS 커버리지 + Mermaid 구문 검증
  - `coverage-diff.py` — 모든 스킬 공통 set diff 계산
- SKILL.md 에서 "에이전트가 직접 ~" → "스크립트 실행 → 결과 해석"

### R2. Self-loop 를 generation 단위로 분할
- **핵심 단위는 "턴" 이 아니라 "generation" (LLM 한 번 호출)**. 한 턴에도 tool call 이 끼면 여러 generation 발생. 각 generation 의 reasoning 은 독립, 이전 generation 의 reasoning 은 다음에 안 보임 (출력만 prompt 로 전달)
- PHASE 모드 도입: `PHASE=draft | validate | save` — 각 PHASE 종료를 **tool call (파일 저장)** 로 끝내면 자동으로 generation 경계 발생
  - `draft`: Step 0~1 → `write_file` 로 종료 → 다음 generation 은 fresh reasoning
  - `validate`: `validate-*.py` tool 호출 → 결과 받아 critical 수정 → `write_file`
  - `save`: 이슈 batch 스크립트 → narrative 작성 → `doc-save.sh`
- **효과**: 한 generation 당 reasoning 분량이 기존 15k~17k → 2~3k 자. GLM 스톨은 "한 generation 안에서 reasoning 내부 한계 초과" 가 원인이므로 이 분할이 직접 효과적
- **사용자 turn 분할 불필요** — tool call 체인만으로 자동 진입 달성 가능. 사용자는 "요구사항 정제해" 한 번만 입력, 내부적으로 6~7 generation 이 이어짐
- **단독 사용 금지** — R4 와 세트 필수. tool call 사이에 긴 assistant text 를 뱉으면 다음 generation 의 prompt 가 불어나 reasoning 한계에 더 빨리 도달
- 추가 이점: 스톨 시 해당 PHASE 만 재실행, 중간 산출물로 복구 가능

### R3. Reasoning 예산 명시
- SKILL.md 상단 한 줄:
  > **GLM 계열 사용 주의**: reasoning 은 각 Step 당 3~5 줄 이내. 상세 분석은 tool output 으로. 장기 추론 불필요.
- 실측상 이 한 줄만으로도 GLM 의 reasoning 폭주가 감소

### R4. 중간 산출물 즉시 flush + assistant 출력 축약
- reasoning 많은 Step 사이마다 임시 파일:
  ```bash
  echo "$DRAFT_JSON" > /tmp/${CODE}-${SKILL}-draft.json
  ```
- **R2 와 필수 세트**: 턴 종료 시 assistant 메시지에 긴 JSON 을 출력하지 말고 "저장 경로 + 다음 PHASE 지시" 한두 줄만. 턴 2 는 `cat /tmp/...` 로 tool output 으로 다시 로드 → session history 누적 최소화
- 스톨 시 재시작 가능. 현재는 스톨 = 처음부터 다시

---

## 재구성된 스킬 흐름 (표준 템플릿)

```
[Gen 1] Step 0 데이터 수집 (curl tool)
   ↓ tool 응답
[Gen 2] Step 1 초안 생성 (Type C)  → draft-io.sh save 로 종료
   ↓ generation 경계 — reasoning 리셋
[Gen 3] validate-*.py 호출 (Type A, tool)
   ↓ critical 리스트
[Gen 4] Step 3 critical 수정 (Type C, 국소 수정) → draft-io.sh save
   ↓ 경계
[Gen 5] 재검증 (Type A, tool) — pass 될 때까지 Gen 4~5 반복 (한 gen 에 1 iter)
   ↓ pass
[Gen 6] Step 5 이슈 batch create/link (Type A, tool)
[Gen 7] Step 5 narrative 작성 (Type C) → doc-save.sh
```

**generation 경계 = tool call 종료**. 에이전트가 tool call 없이 한 generation 에 여러 Step 을 몰아서 수행하지 않도록 SKILL.md 에 명시적 제약 필요.

기대 효과:
- Generation 당 reasoning ~15k → 2~3k 자 (기존 대비 ~85% 감소)
- 스톨 지점이 Gen 2/4/7 의 C 영역만으로 한정 → 예측/복구 용이
- Gen 3/5/6 의 Type A 스크립트는 항상 같은 입력이면 같은 결과 → 결정적 재현성 ↑

---

## 우선순위 로드맵

### Phase 0 (pilot) — `requirement-refine2` 로 R1/R2/R4 통합 실증 (반나절)

가장 리스크 낮은 스킬 하나에 R1+R2+R4 를 함께 적용해 실측 지표 확보. 통과하면 동일 패턴을 Phase 1/2 로 확장.

**Deliverables**:
1. `.claude/skills/_scripts/draft-io.sh` — stdin JSON → `/tmp/${CODE}-${SKILL}-${PHASE}.json` 저장 / `cat` 중 하나 (R4 helper)
2. `.claude/skills/_scripts/validate-requirements.py` — FR/NFR/CNST 필수 필드·형식·User Story 정규식·docId 중복·relatedFeatures hallucination·mode/existingKey 일관성 검증. 출력 `{pass, score, score_effective, critical[], suggestions[]}` (R1)
3. `requirement-refine2/SKILL.md` 개정:
   - 최상단에 **"한 generation 당 제약"** 블록 추가 — 한 번의 generation 에서 Step 두 개 이상 처리 금지, reasoning 3~5줄 이내, Step 종료는 반드시 tool call 로 마무리
   - Step 1 종료를 `draft-io.sh save` 호출로 강제
   - Step 2 시작을 `validate-requirements.py` 호출로 강제 (에이전트가 직접 검증 금지)
   - critical 수정 루프는 `save → validate → save → validate` 형태로 generation 경계 명시 (최대 3 iter)
   - MODE=full/relink 기존 분기 유지, PHASE 인자는 미도입 (tool 경계만으로 달성)

**검증 지표**:
- OpenCode 세션 JSONL 에서 `message.part.updated` 이벤트로 generation 경계 카운트 → **이전 대비 generation 수 증가, 개별 reasoning 길이 감소** 확인
- GOJIRA3 실제 실행에서 스톨 없이 완주
- 최종 이슈/링크/requirements.md 품질이 기존과 동등 이상

### Phase 1 — 가장 위험한 2 스킬 (1~2일)
- **style-define2**: R1 (engine.py wrapper `style-score.sh`) + R3
  - 검증 지표: reasoning 17k → 2~3k 자 감소, GLM-5 완주율 확인
- **satam-analyze2**: R1 (`validate-qas.py` 신규) + R3

### Phase 2 — Self-loop 3회 탑재 스킬 (3~5일)
- R1+R2+R4 통합 패턴 (Phase 0 에서 확정된 템플릿) 을 `usecase-analyze2`, `design-decide2` 에 확장 적용
- 공통 helper: `draft-io.sh` 재사용, `validate-usecases.py` / `validate-decisions.py` 신규

### Phase 3 — 나머지 + 공통 인프라 (1~2일)
- R4 (flush) 전체 적용
- 리팩터링 패턴을 `docs/skills-refactor-glm.md` 로 문서화
- 각 스킬 실행 리플레이 스크립트 (스톨 재현/회복 테스트용)

---

## 남는 의문과 답변

**Q. 점수 계산이 단계마다 다양한데 스크립트로 가능한가?**
A. 점수 규칙이 **구조화된 입력 + 결정적 함수** 인 한 가능. 프로젝트별 파라미터가 다르면 스크립트를 **파라미터화 (YAML 설정)** 해서 대응. "스크립트 vs LLM" 이 아니라 "스크립트 + 설정 파일 + LLM 판단" 3 요소 조합.

**Q. 그래도 판단이 필요한 영역은?**
A. Type C (의미 해석, 새 개념 도출, 애매한 분류) 는 반드시 LLM. 스크립트가 이 영역을 건드리면 오히려 품질 저하.

**Q. GLM-5 가 나오면 이 리팩터링이 필요 없어지지 않나?**
A. 신 버전이 컨텍스트 한계는 완화해도 **구조적 스킬 설계 문제** 는 그대로. 긴 reasoning 강제하는 스킬은 어떤 모델에서도 취약. 리팩터링 가치는 모델 세대와 무관.

---

## 다음 실행 단계

**Phase 0 pilot 로 `requirement-refine2` 부터 착수** (style-define2 보다 리스크 낮고 tool call 이 이미 많아 템플릿 확정 비용이 낮음):
1. `_scripts/draft-io.sh` + `_scripts/validate-requirements.py` 작성
2. `requirement-refine2/SKILL.md` 에 한 generation 당 제약 + tool 경계 명시
3. GOJIRA3 에서 GLM-5 로 실행 → 세션 JSONL 분석으로 generation 경계/reasoning 길이 측정
4. 품질 동등 이상 확인되면 동일 템플릿을 `style-define2` (R1 core), `satam-analyze2` (R1 core) 로 확장 — Phase 1 진입

참고 커밋:
- `2e321d9`: style-define2 API 경로 교정
- 관련 스킬 (리팩터링 이전 상태 스냅샷): `.claude/skills/*/SKILL.md`
