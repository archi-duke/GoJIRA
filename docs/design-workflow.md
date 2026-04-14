# 워크플로우 정책 설계 문서

본 문서는 Agent-Service의 워크플로우(VOS → Use Case → Requirement → 구조설계 …)에서 각 생성 단계의 **검증 정책(critique loop)**을 어떻게 정의하고 실행할지를 정의한다.

---

## 1. 배경

Agent-Service의 LLM 오케스트레이션 스킬(`vos_collect`, `usecase_analyze`, `requirement_refine`, 그리고 향후 추가될 구조설계 스킬들)은 모두 **생성 → 비판적 평가 → 수정** 루프(critique loop)를 거친다.

기존 구현은 평가 기준과 정책이 다음 두 곳에 분산되어 있었다:
- **워크플로우 문서** (`prompts/workflow-*.md`) — 자연어로 평가 기준 명시
- **각 스킬의 `prompts/critique.txt`** — 자체 critical/suggestions 기준 명시
- **`skills/critique.py`** — 최대 반복 횟수 등 정책을 코드 상수로 보유

이 분산은 (1) 기준 동기화 부담, (2) 도메인 전문가가 정책을 조정하기 어려운 구조, (3) 동일 정책의 중복 표현이라는 문제를 만들었다.

---

## 2. 설계 원칙

### 2.1 단일 진실 출처 (Single Source of Truth)

**워크플로우 문서를 검증 기준과 정책의 단일 진실 출처로 삼는다.**

- 검증 기준 (criteria) — 무엇을 평가할 것인가
- 검증 정책 (policy) — 어떻게 평가하고 언제 멈출 것인가

코드는 워크플로우 문서를 읽어 그 내용을 critique LLM 프롬프트에 그대로 주입한다. 같은 텍스트를 사람과 LLM이 본다.

### 2.2 자연어 우선

검증 기준과 정책은 **자연어 마크다운 불릿**으로 표현한다. YAML/JSON 블록을 마크다운에 끼워 넣지 않는다.

이유:
- LLM은 자연어 기준으로 평가하므로 구조화 변환이 불필요
- 도메인 전문가가 일반 마크다운처럼 편집 가능
- 워크플로우 문서가 "반-실행파일"화되어 가독성이 손상되는 것을 방지

### 2.3 글로벌 기본값 + 단계별 예외

대부분의 단계가 동일한 검증 정책을 사용하므로:
- **글로벌 기본 정책**은 `prompts/system.txt`에 한 번만 명시
- 워크플로우 문서의 단계별 정책 섹션은 **예외만 명시**, 없으면 기본값 적용

---

## 3. 검증 정책 표준 형식

### 3.1 워크플로우 문서의 단계 구조

각 생성 단계는 다음 구조를 가진다:

```markdown
### ① VOS 도출

**목적**: ...
**입력**: ...
**산출물**: ...

**수행 방법**:
- ...
- ...

**검증 기준**:
- 주요 이해관계자가 모두 1개 이상 식별되었는가
- 의미적 중복 VOS가 없는가
- ...

**검증 정책**: 기본 정책 적용
(또는 예외 명시: "검증 점수 0.85 이상까지 반복 (최대 3회)")

**완료 기준**: ...
```

**섹션 이름은 고정**: `**검증 기준**:`, `**검증 정책**:` — 이 헤딩으로 executor가 추출한다.

### 3.2 글로벌 기본 정책 (`prompts/system.txt`)

```
# 워크플로우 검증 기본 정책

생성 단계의 비판적 평가 루프는 다음 정책을 따른다 (단계별 워크플로우 문서에서 예외를 명시한 경우 그 정책 우선 적용):

검증 기준에 따라 평가 점수가 0.8 이상일 때까지 반복한다 (최대 2회 수정).
최종 실패 시 마지막 생성물을 그대로 사용하되 사용자에게 임계값 미달 사실을 알린다.
평가 자체가 파싱 실패하면 현재 결과로 종료한다 (무한 루프 방지).
```

### 3.3 단계별 예외 표기 예시

```markdown
**검증 정책**:
- 점수 임계값: 0.85 (기본값 0.8보다 엄격)
- 최대 반복: 3회
- 최종 실패 시: 사용자에게 알리고 워크플로우 중단
```

자연어 형태도 허용:
```markdown
**검증 정책**: 검증 점수 0.9 이상까지 반복 (최대 4회), 최종 실패 시 워크플로우 중단
```

---

## 4. critique LLM 프롬프트 표준

각 스킬의 `prompts/critique.txt`는 placeholder만 보유하고, 실제 기준/정책은 executor가 워크플로우 문서에서 추출하여 주입한다:

```
당신은 현실적인 reviewer입니다. 아래 검증 기준에 따라 산출물을 평가합니다.

## 검증 기준
{criteria}

## 검증 정책
{policy}

## 컨텍스트
{context_block}

## 생성 결과
{draft}

## 출력 (반드시 JSON 객체로만 응답)
{
  "score": 0.0~1.0,
  "pass": true/false,
  "critical": ["치명적 결함 (없으면 빈 배열)"],
  "suggestions": ["개선 제안 (있을 때만)"],
  "reasoning": "판단 근거"
}
```

### 4.1 score 산출 방식

**단일 종합 점수**를 LLM이 직접 결정한다 (항목별 가중평균 아님).

이유:
- 단순함 — 항목별 가중치 정의/관리 부담 없음
- LLM이 컨텍스트(중요도, 영향도)를 고려해 판단
- 임계값 조정만으로 엄격도 변경 가능

### 4.2 pass 판정 우선순위

1. `critical`이 비어있지 않으면 → `pass = false` (점수 무관)
2. `critical`이 비어있고 `score >= threshold`면 → `pass = true`
3. 그 외 → `pass = false` (수정 반복)

`critical`은 점수 무관 결격 사유. `score`는 정도 평가.

---

## 5. 실행 흐름

```
┌─ executor 시작
│
├─ workflow_md 로드 (e.g., workflow-requirement-analysis.md)
│
├─ 단계 헤딩(### ① VOS 도출)에서 다음 추출:
│    - 검증 기준 (criteria 텍스트)
│    - 검증 정책 (policy 텍스트, 없으면 system.txt 기본값)
│
├─ critique.parse_policy(policy_text) → {threshold, max_iter, on_fail}
│
├─ iterate_generate_critique(
│    generate_prompt,
│    critique_prompt(criteria, policy),
│    revise_prompt,
│    threshold=0.8,
│    max_iter=2,
│  )
│
└─ trail 반환 (사용자 보고용 요약 포함)
```

### 5.1 정책 파싱 규칙

`critique.parse_policy(text)`는 자연어 정책에서 다음 값을 추출한다:

| 추출 대상 | 정규식 힌트 | 기본값 |
|----------|------------|--------|
| threshold | `점수\s*([0-9.]+)` 또는 `임계값[:\s]*([0-9.]+)` | 0.8 |
| max_iter | `최대\s*(\d+)\s*회` | 2 |
| on_fail | "중단" 키워드 → `abort`, 그 외 → `continue` | `continue` |

파싱 실패 항목은 기본값으로 fallback. 자연어 표현 자체도 critique LLM에 그대로 전달되므로 LLM은 풀 텍스트를 본다.

### 5.2 안전장치

- **하드 상한**: 정책의 `max_iter`가 5 초과면 5로 클램프 (executor 코드)
- **파싱 실패**: critique LLM이 JSON을 못 만들면 현재 결과로 종료
- **score 미반환**: LLM이 score를 빼먹으면 critical 유무로만 판정 (legacy fallback)

---

## 6. 사용자 보고 (trail summary)

`trail_summary()`는 다음 형태로 사용자에게 전달한다:

| 결과 | 메시지 예시 |
|------|------------|
| 1회차 통과 | `검증 통과 (1회차, score 0.92)` |
| N회 수정 후 통과 | `검증 통과 (3회차, 2회 수정, 최종 score 0.83)` |
| 임계값 미달 | `검증 미통과 (최대 3회 시도, 최종 score 0.72 — 임계값 0.8 미달, 마지막 결과 사용)` |
| 정책상 중단 | `검증 미통과로 워크플로우 중단 (정책: 최종 실패 시 중단)` |

---

## 7. 변경 영향

### 7.1 코드 변경

| 파일 | 변경 내용 |
|------|----------|
| `Agent-Service/skills/critique.py` | score 필드 처리, threshold 인자, 워크플로우 문서 파싱 헬퍼 추가 |
| `Agent-Service/skills/vos_collect/executor.py` | 워크플로우에서 기준/정책 로드 후 주입 |
| `Agent-Service/skills/vos_collect/prompts/critique.txt` | 자체 기준 제거, placeholder만 유지 |
| `Agent-Service/skills/usecase_analyze/executor.py` | 동상 |
| `Agent-Service/skills/usecase_analyze/prompts/critique.txt` | 동상 |
| `Agent-Service/skills/requirement_refine/executor.py` | 동상 |
| `Agent-Service/skills/requirement_refine/prompts/critique.txt` | 동상 |

### 7.2 문서 변경

| 파일 | 변경 내용 |
|------|----------|
| `Agent-Service/prompts/system.txt` | "워크플로우 검증 기본 정책" 섹션 추가 |
| `Agent-Service/prompts/workflow-requirement-analysis.md` | 단계별 "검증 기준"/"검증 정책" 섹션 표준화 |
| `Agent-Service/prompts/workflow-architecture-design.md` | 동상 (Phase 2~4 단계도 추가 시) |

### 7.3 하위 호환

- 기존 `critiqueTrail` 스키마에 `score` 필드만 추가, 나머지 유지
- 워크플로우 문서에 "검증 정책" 섹션이 없으면 기본 정책 적용

---

## 8. 향후 확장

- **단계별 가중치 정책**: 특정 검증 항목에만 더 높은 가중치를 두고 싶다면 워크플로우 문서에 자연어로 명시 (예: "이해관계자 커버리지는 다른 기준보다 2배 가중")
- **사용자 개입 모드**: 임계값 미달 시 `on_fail: ask_user` 정책으로 확장 — 사용자가 수동으로 pass/retry 결정
- **Phase 2~4 적용**: 구조설계 스킬(architecture_driver, design_decision, satam_evaluate)이 추가되면 동일 표준 적용

---

## 9. 문서 ID ↔ subkey 동기화 정책

### 9.1 배경

`usecase_analyze`, `requirement_refine` 스킬은 LLM이 **문서(use-cases.md / requirements.md)와 이슈 목록(features / requirements)을 한 번의 JSON 응답으로 동시 생성**한다. 이 시점에는 실제 이슈가 아직 DB에 없으므로 LLM은 문서 안의 Use Case/Requirement를 자체 placeholder(`UC-1`, `FR-1`, `NFR-1`, `CNST-1`)로 표기한다. 이후 `create_issue()`가 실제 subkey(`UC_001`, `FR_001`, `QA_001`, `CNST_001`)를 부여한다.

결과적으로 문서에 기록된 ID(UC-1 …)와 실제 생성된 이슈의 subkey(UC_001 …) 가 일치하지 않아 **문서-이슈 추적성이 끊겼다**.

### 9.2 대안 비교

| 안 | 방식 | 토큰 비용 | 지연 | 결정성 | 비고 |
|----|------|-----------|------|--------|------|
| A | LLM 응답에 `docId`(UC-1) 포함 → 이슈 생성 후 executor가 문서에서 `docId` → `subkey` 치환 | 동일 | 동일 | 높음 (정규식 치환) | 채택 |
| B | 2-pass: features 생성 → 이슈 create → 실제 subkey를 컨텍스트로 document 재생성 | 약 2배 | 한 번 더 LLM 왕복 | 중간 (LLM 재생성 변동성) | 탈락 |

### 9.3 채택 안 (A)

**원칙**: LLM은 문서-내부 식별자(docId)만 책임지고, 코드가 이슈 생성 후 실제 subkey로 기계적 치환한다.

- 각 features/requirements 항목에 **`docId` 필드 필수** (예: `"docId": "UC-1"`)
- LLM은 문서 본문(표, 헤딩, 시나리오 섹션, 매핑 열 모두)에서 동일 `docId`를 일관되게 사용
- "(Feature 이슈 키)" 등 실제 subkey를 써야 하는 컬럼에도 **LLM은 `docId`를 그대로 기록** (빈칸 금지)
- executor는 `dispatch_by_mode` 완료 후 `docId → subkey` 매핑을 구축:
  - `created`: `create_issue()` 반환값의 `subkey`
  - `reuse`/`modify`: 기존 이슈 조회 결과의 `subkey`
- 매핑을 document에 단어 경계 정규식으로 치환 후 저장

### 9.4 채택 사유

1. **토큰/지연 추가 없음** — 기존 단일 호출 구조 유지
2. **결정성** — 치환은 기계적이므로 LLM 변동성 없음
3. **하위 호환** — 기존 mode 분기/링크 로직에 영향 없음
4. **범위 제한** — use-cases.md / requirements.md 만 대상. VOS(vos_collect)는 문서 산출이 없고, projdesc_generate는 이슈를 만들지 않아 제외

### 9.5 치환 규칙

- 단어 경계(`\b`) 기반 정규식으로 `docId` → `subkey` 전역 치환
- `docId` 접두사는 고정: `UC`, `FR`, `NFR`, `CNST` (대시/밑줄 모두 허용하지 않고 `UC-N` 형식만)
- docId가 매핑에 없으면(LLM 실수/누락) 원문 유지 — 사용자에게 경고 메시지로 보고

---

## 10. 핵심 결정 요약

1. **단일 진실 출처**: 워크플로우 문서가 검증 기준/정책의 권위 출처
2. **자연어 우선**: YAML/JSON 블록 금지, 마크다운 불릿으로 표현
3. **글로벌 기본값 + 단계별 예외**: `system.txt`에 기본 정책, 워크플로우 문서에 예외만
4. **헤딩 매칭으로 추출**: `**검증 기준**:`, `**검증 정책**:` 섹션을 executor가 추출
5. **score 기반 판정**: LLM 단일 종합 점수, critical은 점수 무관 결격
6. **하드 상한으로 안전성 보장**: max_iter 5회 클램프, 파싱 실패 시 현재 결과 종료
7. **문서 ID는 docId로 우회 후 치환**: LLM 응답에 `docId` 필수, executor가 이슈 생성 후 실제 subkey로 기계 치환 (옵션 A 채택)
