# 스킬 인프라 v3 — drift-check + 결정적 자동화 + LLM 분할 패턴

작업 기간: 2026-04-25 ~ 2026-04-27 (브랜치 `plantuml`)

본 문서는 Phase 3 8 스킬 리팩터링 (`docs/refactored-skills-evaluation.md`) 이후 운영하면서 드러난 한계를 해결하기 위해 도입한 **인프라 레이어** 를 정리합니다. 개별 스킬 평가/측정 데이터는 기존 문서가 담당하고, 본 문서는 "왜 이 인프라가 필요한가 / 어떻게 동작하는가 / 운영 시 무엇을 지켜야 하는가" 에 집중합니다.

---

## 1. 배경

Phase 3 평가 결과 (`refactored-skills-evaluation.md` §1~11) 8 스킬 모두 체인 완주에는 성공했으나, 다음 운영 이슈가 누적적으로 발견됐습니다.

1. **재호출 시 "할 일이 있는지" 판단이 LLM 책임**
   - 사용자가 같은 스킬을 다시 부르면 매번 모델이 이슈를 다 읽고 "변경이 있는지" 판단해야 했습니다. 평소 운영은 *변경 없으면 즉시 종료* 가 정답인데, 모델은 일단 작업을 시작하는 경향이 있어 비용 누수 발생.

2. **단일 큰 LLM generation 의 부담**
   - `usecase-analyze2` Step 1: 357s / 13.6K 단일 generation, 스톨 위험.
   - `design-integrate2` 단일 통합 문서: 156s / 5.8K + PlantUML escape 오류 21건.
   - 큰 출력일수록 quoted label 안 raw newline, sequence 키워드 오용 등 **모델 가변 실수** 가 누적됨.

3. **PlantUML syntax 결함의 반복**
   - 모델/세션마다 다른 패턴으로 같은 부류 실수 (destroy 화살표, 미정의 participant, `note over` component 오용 등) 가 재발. LLM 으로 후처리하면 비용이 큼.

4. **archSettings PATCH 의 silent fail**
   - `driver-select2` 의 `normalizedAttributes` 영속화가 응답 status 검증 없이 silent fail 시, 하류 스킬은 매번 `drivers.md` fallback 파싱 (~70초) 으로 빠짐.

본 v3 인프라는 위 4 항목을 결정적 도구 + 표준 진입 점검 + LLM 분할 패턴으로 해소합니다.

---

## 2. 핵심 인프라 — drift-check + deferredImpacts

### 2.1 workflow-graph.json — 9 stage DAG

`/.claude/skills/_scripts/workflow-graph.json` 는 9 stage 의 의존 관계와 각 stage 가 관심하는 이슈 필드를 선언합니다.

```
projdesc → vos → usecase → requirement → driver → style
                                                      ↓
                                                design-decide
                                                  ↙        ↘
                                       design-integrate    satam
```

각 stage 정의는 다음 키를 가집니다 (`workflow-graph.json:3-83`).

| 키 | 역할 |
|---|---|
| `issueTypes` | hash 입력에 포함할 이슈 타입 (예: `["Requirement"]`) |
| `filter` / `filterNot` | 같은 이슈 타입 내 필터 (예: `driverSelected: true`) |
| `fields` | hash 에 들어가는 정렬된 필드 |
| `linkFields` | 정규화 후 hash 에 포함할 링크 필드 |
| `archSettingsKeys` | `archSettings` 에서 hash 에 포함할 키 |
| `doc` | 산출물 경로 (단일 파일 또는 배열) |

### 2.2 stage hash 계산

`drift-check.py:122-148` 의 `compute_stage_hash`:

1. `issueTypes` + `filter`/`filterNot` 로 이슈 부분집합 추출
2. 각 이슈에서 `fields` 만 뽑고 `linkFields` 는 정규화 (in/out/type 정렬)
3. `key` 기준 정렬 → JSON canonical (sort_keys=True)
4. `archSettingsKeys` 부분과 함께 SHA256

같은 이슈/설정이면 항상 같은 hash. 모델이 텍스트만 다르게 풀어 써도 hash 는 안정.

### 2.3 archSettings 영속화 — lastRunHash / lastRunAt / deferredImpacts

baseline 은 `archSettings` 의 3 키에 저장됩니다.

- `lastRunHash`: `{stageId: sha256}`
- `lastRunAt`: `{stageId: ISO8601Z}`
- `deferredImpacts`: `[{stage, trigger, since, reason, resolvedAt?}]`

`GoJIRA-API/handlers/project.go:19-30` 의 `archSettingsAllowedKeys` 화이트리스트에 3 키를 추가해 PATCH 거부 문제를 해소했습니다 (commit `c866ff9`).

### 2.4 drift-check.py 명령

| 명령 | 역할 |
|---|---|
| `check` | 모든 stage hash 계산 → baseline 과 비교, drift 시 하류 영향 추적 |
| `set-baseline [--stage <id>]` | 현재 hash 를 baseline 으로 저장 |
| `mark-deferred <stage> <reason>` | 영향 단계를 의식적으로 보류 (재실행 안 함) |
| `resolve-deferred <stage>` | 보류 해제 (resolvedAt 기록) |

### 2.5 분기 테이블 — Step 0 의 4 케이스

| 본 단계 | 상위 단계 | 동작 |
|---|---|---|
| **NO-BASELINE** | - | 첫 실행 — 데이터 수집 + 본 작업 진행 |
| **DRIFT** | (무관) | 본 작업 진행 |
| **CLEAN** | **DRIFT** | 사용자 통지 — (a) 지금 진행 (b) `mark-deferred` 보류 |
| **CLEAN** | **CLEAN** | 사용자 질문 "강제 재생성?" — `no` → 즉시 종료 (LLM 0회), `yes` → 강제 재생성 모드 |

`drift-check.py:155-242` 가 화면 출력을 담당하고, 분기 판단은 SKILL.md 가 명시적으로 처리합니다.

---

## 3. 8 스킬 표준 진입 점검 (Step 0)

commit `57b19c6` 에서 8 스킬 (vos-collect2, usecase-analyze2, requirement-refine2, driver-select2, style-define2, design-decide2, design-integrate2, satam-analyze2) 의 `Step 0` 가 **동일 패턴** 으로 통합됐습니다.

### 3.1 표준 흐름

```
Step 0.1 drift-check.py check 호출
Step 0.2 4-케이스 분기 (위 표)
Step 0.3 기존 데이터 수집 (스킬별 본문 보존)
...
Step 6.2 set-baseline --stage <stage_id> — baseline 갱신 의무
```

진입 정책: **평소 운영은 drift CLEAN → 즉시 종료가 정답** 입니다. 예외 (PlantUML 마이그레이션, 템플릿 변경 등) 만 사용자가 명시 `yes` 로 강제 재생성 모드로 진입.

### 3.2 강제 재생성 모드

drift CLEAN + 사용자 `yes` 인 경우의 처리:

- **이슈 생성을 동반하는 스킬** (vos / usecase / requirement / driver / style / design-decide): `relink` 흐름으로 강등 — 기존 이슈 보존, 링크·문서만 재작성, 차집합만 POST.
- `requirement-refine2` SKILL.md 의 "MODE=relink/강제 재생성 축약 규칙" 참조.

### 3.3 특수 케이스

| 스킬 | 특수 처리 |
|---|---|
| `satam-analyze2` | 문서 산출 없음 — 모두 CLEAN 시 yes/no 질문 없이 즉시 종료 |
| `design-integrate2` | 이슈 생성 0 — 강제 재생성 yes 와 일반 진행이 동일 흐름 (`SKILL.md:97`) |
| `usecase-analyze2` | 이번 인프라 PoC 실증 — TEST 강제 재생성 모드 검증 (10.5분) |

### 3.4 Step 6.2 의 baseline 갱신 의무

작업이 정상 완료되면 반드시 `set-baseline --stage <stage_id>` 로 baseline 을 갱신합니다. 갱신 누락 시 다음 호출에서 `DRIFT` 로 오인됨.

---

## 4. 결정적 자동화 — plantuml-fix

### 4.1 구성 (commit `048d9e8` + `a735286`)

| 파일 | 역할 |
|---|---|
| `_scripts/plantuml-fix.py` | 결정적 fix 엔진 (정규식, 7 패턴) |
| `_scripts/doc-save.sh` | `.md` 저장 시 자동 호출, ~30ms, idempotent |
| `plantuml-doc-fix/SKILL.md` | 외부 markdown 명시 호출용 진입점 |

### 4.2 처리 패턴 (7종)

| 패턴 | 다이어그램 | 변환 |
|---|---|---|
| `A x B:` 단독 화살표 | sequence | `A -x B:` (destroy) |
| `state "..." as X` | component/database | `[...] as X` |
| `cloud "..." as X` | sequence | `participant "..." as X` |
| `file "..." as X` | sequence | `participant "..." as X` |
| 미정의 식별자 (메시지에만 등장) | sequence | `@startuml` 직후 declare 추가 |
| **quoted label 안 raw newline** | **모든 종류** | `\n` escape 변환 (가장 흔함) |
| **`note over X, Y :` (multi-target)** | component | floating note (`note as N_auto<idx>` ~ `end note`) |

마지막 패턴은 v3 실증에서 `development.md` 1건 발견되어 `a735286` 으로 추가됐습니다.

### 4.3 detect_kind 의 보수적 분류 (`plantuml-fix.py:26-46`)

actor + package 가 함께 등장하는 component/deployment 다이어그램을 sequence 로 오인하면 자동 declare 가 정상 블록을 깨뜨립니다. 그래서 `package` / `node` / `[Component]` / `database` 신호가 있으면 **component 우선**, 없을 때만 actor/participant 로 sequence 분류.

### 4.4 운영 모드

- **자동**: doc-save.sh 가 `.md` 저장 시 자동 적용. 사용자/모델이 의식할 필요 없음.
- **비활성화**: `DOC_FIX=0` 환경변수.
- **명시 호출**: `plantuml-doc-fix` SKILL — 외부 markdown 검증 / 일회성 정리.

### 4.5 검증된 PoC 결과 (TEST, 2026-04-25)

| 산출물 | 패턴 분포 | 결과 |
|---|---|---|
| design-decide2 / DP-01~05 | seq_destroy 2 + component_state 5 + cloud 4 + file 1 + undefined 1 = **13건** | 20/20 OK |
| design-integrate2 / architecture-integrated.md (단일) | quoted_label_newline **21건** | 6/6 OK |
| design-integrate2 v3 (분할) / development.md | component_note_over **1건** | 8/8 OK |

단일 generation → 분할 전환만으로 PlantUML 오류 21 → 1건 (95% ↓) 효과. 비용 비교: 강제 재생성 9분 / 17K vs 결정적 fix 30ms / 0 토큰.

### 4.6 한계

- **의미 변경이 필요한 오류는 결정적 fix 불가** — 본질적으로 잘못 선택한 다이어그램 종류, 누락된 참여자 의도 등. 이런 케이스는 LLM/수동 처리 필요.
- 본 도구는 **syntax 결함 흡수** 전용. 새 패턴 발견 시: (1) 정규식 fix 가능 확인 (2) `plantuml-fix.py` 패턴 추가 (3) doc-save 통합으로 모든 스킬 자동 보호.

---

## 5. LLM 분할 + 결정적 통합 패턴

### 5.1 design-integrate2 v3 (commit `4ed3ea3`)

단일 generation 156s/5867토큰 → 4 view 분할 + 결정적 index 통합.

```
Step 1.1 Logical view    (component)       — LLM
Step 1.2 Process view    (sequence)        — LLM
Step 1.3 Deployment view (node/artifact)   — LLM
Step 1.4 Development view (package)        — LLM
Step 1.5 결정적 통합 (index 작성)          — LLM 0회
```

산출물도 분할:
- `architecture/architecture-integrated.md` (짧은 index)
- `architecture/architecture-integrated/{logical,process,deployment,development}.md` (4 본문)

cross-view 일관성 보장:
- 각 view step 종료 시 `entity_dictionary` 누적 (id/name/type/appearsIn)
- 다음 view 가 dictionary 입력 받아 동일 식별자 재사용
- `validate-integrate-doc.py` 가 같은 id 의 다른 name 을 `entity_inconsistent` 로 critical 분류

### 5.2 usecase-analyze2 v3 (commit `d4c3e49`)

단일 generation 357s/13.6K → Step 1 의 5 sub-step 분할.

```
Step 1.1 Entities (Domain + User Story + Use Case) — JSON only
Step 1.2 Features (선정 + relatedVOS + mode)
Step 1.3 Scenarios (basicflow / alternative / exception)
Step 1.4 Diagrams (Context / Use Case / Sequence × Feature) — PlantUML 격리
Step 1.5 결정적 문서 조립 (LLM 0회)
```

**design-integrate2 v3 와의 차이**: 산출물은 단일 `business/use-cases.md` 그대로. LLM generation 단계만 분할. 이유는 추적성 + 하류 호환성 (Feature 이슈 ↔ docId 매칭 + 기존 단일 파일 의존 흐름 유지).

### 5.3 결정적 조립 인프라

| 도구 | 역할 |
|---|---|
| `_scripts/draft-io.sh` `merge` 모드 | jq deep merge + features 배열 docId 매칭 누적 |
| `_scripts/validate-usecases.py --stage` | `entities|features|scenarios|diagrams|final` 단계별 critical 기준 분리 |
| `_scripts/usecase-assemble.py` | Step 1.5 결정적 조립 헬퍼, Python stdlib 만 |
| `templates/use-cases.md` | 7 marker (`<!-- INSERT business_domain HERE -->` 등) |

`draft-io.sh` 의 jq 머지 (`draft-io.sh:67-88`) 는 features 배열에 대해 docId 키로 항목별 deep merge — 같은 docId 면 필드 누적, 없으면 append.

### 5.4 효과

- **PlantUML 오류**: design-integrate2 단일 21건 → 분할 1건 (95% ↓)
- **단일 큰 generation 부담**: PlantUML 채우기 (가장 위험) 를 별도 sub-step 으로 격리. 다른 step 의 스톨 위험 차단.
- **재시도 비용**: 분할 시 실패한 sub-step 만 재실행 가능.
- **일관성**: 두 스킬에 같은 패턴 적용으로 향후 다른 스킬 (현재는 driver-select2 / style-define2 / design-decide2 / requirement-refine2 가 단일 generation 부담 적음 — 후순위) 적용 시 학습/유지보수 비용 ↓.

---

## 6. archSettings 영속화 보장

### 6.1 driver-select2 Step 5.3 보강 (commit `f463087`)

기존 PATCH 호출이 응답 status 를 체크하지 않아, silent fail 시 하류 스킬 (style-define2 / design-decide2) 이 `drivers.md` fallback 파싱 (~70초) 으로 빠지는 비효율이 있었습니다.

보강 사항:
- HTTP status 2xx 검증 추가
- 사후 GET 으로 `normalizedAttributes` / `priorityWeight` 실제 영속화 확인
- 실패 시 명확한 ⚠ 알림 + 수동 재시도 안내

### 6.2 검증 사실

TEST 실증으로 PATCH 코드 자체는 정상 (HTTP 200, 12 키 영속화) 임을 확인. 이전 archSettings 의 `normalizedAttributes` 누락은 driver-select2 호출이 누락됐거나 어떤 시점 archSettings 부분 reset 때문이었습니다 (메모리 `project_drift_check_improvements.md` 의 DONE 항목).

이로써 silent fail 차단 → 하류 스킬의 ~70초 fallback 비효율 제거.

---

## 7. 운영 정책

### 7.1 평소 운영

| 상황 | 동작 |
|---|---|
| drift CLEAN | 즉시 종료, LLM 호출 0회 |
| drift DRIFT | 본 작업 진행 |
| 본단계 CLEAN + 상위 DRIFT | 사용자 통지 → 진행 또는 mark-deferred |

### 7.2 예외 — 강제 재생성

PlantUML 마이그레이션, 템플릿 변경 등 외부 요인으로 문서를 새로 그려야 할 때만 사용자 명시 `yes`. 이슈 생성을 동반하는 스킬은 자동으로 `relink` 흐름으로 강등.

### 7.3 doc-save 자동 보호

모든 `.md` 저장 시 plantuml-fix 가 자동 적용. 사용자/모델이 의식할 필요 없음. 비활성화는 `DOC_FIX=0`.

### 7.4 새 LLM 실수 패턴 발견 시

1. 정규식으로 결정적 fix 가능한지 확인
2. `_scripts/plantuml-fix.py` 에 패턴 추가
3. doc-save 통합으로 모든 스킬 자동 보호

LLM 의 가변성을 결정적 도구가 흡수 — 한 번 등록한 패턴은 모든 doc-save 흐름에서 자동 보호됩니다.

---

## 8. PoC 실측 데이터 (참고)

| 케이스 | 시간 | output | reasoning | 비고 |
|---|---|---|---|---|
| usecase-analyze2 강제 재생성 (단일) | 10.5분 | 17K | 7K | 이슈 0 변경 — drift PoC |
| style-define2 강제 재생성 | 10.7분 | 16.2K | 3.2K | Step 1.5 PlantUML 채우기 두 번 분할 패턴 |
| design-decide2 강제 재생성 | 9.4분 | 14K | - | 5 DP 분할 패턴, PlantUML 한 번에 채움 |
| design-integrate2 v3 (분할) | (실증 대기) | - | - | development.md PlantUML 1건 (단일 21건 대비 95%↓) |
| usecase-analyze2 v3 분할 | (실증 대기) | - | - | E2E smoke test pass / score 1.0 / 결정적 조립 0.03초 |

> design-integrate2 v3 / usecase-analyze2 v3 의 풀 실증은 다음 세션에서 강제 재생성 호출로 측정 예정 (메모리 `project_design_integrate2_view_split_idea.md` / `project_usecase_step1_split_idea.md`).

---

## 9. 후속 개선 후보

메모리 `project_drift_check_improvements.md` 에 보존된 사항.

| 우선순위 | 항목 | 비고 |
|---|---|---|
| HIGH | Step 1 변환 비용 (Mermaid → PlantUML 결정적 도구 분리) | drift CLEAN 강제 재생성 누적 부담 |
| MEDIUM | 링크 desired set 가이드 강화 | relink/forced-doc-regen 시 desired = existing 디폴트 명시 |
| MEDIUM | reasoning 토큰 두 곳 집중 (#9 / #16) | 분기표 결정성 더 끌어올리기 |
| LOW | `cache_w = 0` (Provider Prompt Cache 미사용) | OpenCode LLM 통합 점검 별건 |
| LOW | 8 스킬 SKILL.md DRY 화 | `_shared/drift-check-step0.md` 분리 + include 지원 검증 필요 |
| LOW | usecase-analyze2 v3 풀 14 Feature 실증 측정 | GLM 토큰 회복 후 |

---

## 10. 참조 commit

| commit | 요약 |
|---|---|
| `c866ff9` | 워크플로우 단계 drift-check + deferredImpacts 인프라 |
| `57b19c6` | 워크플로우 8 스킬 drift-check 표준 진입 점검 통합 |
| `048d9e8` | PlantUML 결정적 자동 수정 도구 + doc-save 통합 |
| `4ed3ea3` | design-integrate2 view 분할 + 결정적 통합으로 재구성 |
| `a735286` | plantuml-fix 에 component_note_over 패턴 추가 |
| `f463087` | driver-select2 Step 5.3 archSettings PATCH status 검증 보강 |
| `d4c3e49` | usecase-analyze2 Step 1 분할 + 결정적 조립으로 재구성 |

---

## 11. 참조 문서

- `docs/refactored-skills-evaluation.md` — Phase 3 8 스킬 평가 (§1~11). 본 문서는 그 다음 단계 인프라.
- `docs/tactics-for-skill.md` — 스킬 설계 일반 가이드. 본 문서가 보강하는 관점.
- `.claude/skills/_scripts/workflow-graph.json` — 9 stage 정의 + 의존.
- `.claude/skills/_scripts/drift-check.py` — workflow-graph 기반 stage hash 비교.
- `.claude/skills/_scripts/plantuml-fix.py` — 7 패턴 결정적 fix.
- `.claude/skills/_scripts/draft-io.sh` — save / load / merge 모드.
- `.claude/skills/_scripts/usecase-assemble.py` — 결정적 조립 헬퍼.
- `.claude/skills/design-integrate2/SKILL.md` — Step 1.1~1.5 분할.
- `.claude/skills/usecase-analyze2/SKILL.md` — Step 1.1~1.5 분할.
- `GoJIRA-API/handlers/project.go` — `archSettingsAllowedKeys` 화이트리스트.
