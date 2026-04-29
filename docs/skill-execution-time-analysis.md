# 스킬별 수행 시간 분석

> **상태**: 초안 (2026-04-29) / **측정 환경**: TEST 프로젝트, GLM-5.1 (zai-coding-plan), batman 호출 / **목적**: 워크플로우 스킬 호출의 시간 비용 분포 측정 + fast path 절감 여지 정량화

본 문서는 워크플로우 스킬을 실측하면서 어느 단계가 시간을 가장 많이 소모하는지 식별하고, fast path / 결정적 조립이 절감하는 부분을 정량화한다.

---

## 1. 측정 방법

세 가지 로그 소스를 시각으로 정렬해 단계 경계를 식별:

| 소스 | 가시성 |
|---|---|
| `GoJIRA-API` 로그 (`/tmp/gojira-api.log`) | API 호출 시각 + 메소드 + path + duration |
| `Platform-API` 로그 (`/tmp/platform-api.log`) | DocRepo 호출 시각 |
| OpenCode 세션 로그 (`~/.local/share/opencode/log/<date>.log`) | LLM stream start/end, tool permission 호출, sub-step bash 명령 |

API 호출이 0건인 구간 = **순수 LLM generation 또는 local script 실행**.

특히 OpenCode 의 다음 이벤트로 LLM generation 경계 식별:
- `service=session.prompt sessionID=... step=N` — 새 LLM generation 시작
- `service=llm providerID=... stream` — stream 시작
- `service=bus type=message.part.updated +<ms>` — stream 진행 (큰 ms 값 = 누적 출력 시간)

---

## 2. requirement-refine2 — 첫 측정 (2026-04-29)

### 2.1 입력 / 산출

- 입력: TEST 프로젝트 VOS 14 + Feature 12 + 템플릿 use-cases.md (full mode)
- 산출: Requirement 41건 (FR 25 / NFR 8 / CNST 4) + business/requirements.md
- self-loop: 2 iter (1차 critical → 1차 fix → 2차 검증 → 통과)
- 전체 시간: **약 16분**

### 2.2 단계별 시간 분포

| 단계 | 시각 (KST) | 시간 | 비율 | 동작 |
|---|---|---|---|---|
| Step 0 | 11:14:13 ~ 11:14:50 | ~37초 | 4% | drift-check + GET Project + GET Issue |
| **Step 1** | **11:14:50 ~ 11:18:25** | **3분 35초** | **22%** | LLM 으로 FR/NFR/CNST 본문 도출 (37 항목) |
| Step 2 (1차) | 11:18:25 | <1초 | — | `validate-requirements.py` 실행 |
| **Step 3 (fix iter)** | **11:18:25 ~ 11:21:06** | **2분 41초** | **17%** | LLM 으로 critical 항목 수정 |
| Step 2 (2차) | 11:21:06 | <1초 | — | 재검증 |
| **Step 3 추가 또는 Step 5 진입 준비** | **11:21:06 ~ 11:23:13** | **2분 7초** | **13%** | LLM 활동 (정확한 분리 미상) |
| Step 5.1 (이슈 batch) | 11:23:13 ~ 11:23:26 | ~13초 | 1% | POST 41건 + PUT 8건 (NFR QAS) |
| Step 5.2 (링크 batch) | 11:23:26 ~ 11:25:24 | ~1분 58초 | 12% | POST /Issue/link 52건 + LLM 차집합 계산 |
| **Step 5.3+5.4 (DOCUMENT echo + doc-save)** | **11:25:24 ~ 11:30:33** | **5분 9초** | **32%** | 단일 LLM generation 에서 DOCUMENT 본문 재출력 + doc-save.sh |
| Step 6 (baseline 갱신) | 11:30:33 ~ 11:30:43 | ~10초 | 1% | doc-save 완료 + drift-check set-baseline + PATCH archSettings |

### 2.3 핵심 발견 1 — critique 가 큰 비중

**critique (Step 2 + Step 3) 합산 약 4분 50초 (30%)** 이 Step 1 (22%) 보다 큼.

- validate-requirements.py 자체는 1초 미만
- 시간 잡아먹는 건 Step 3 의 **LLM critical 수정**
- 1 iter 당 약 2~3분
- 이번엔 2 iter 돌고 통과
- 첫 generation 의 critical 률을 낮추거나, 결정적 수정 가능한 패턴을 스크립트화하면 절감 가능

### 2.4 핵심 발견 2 — `echo "$DOCUMENT"` 가 5분

11:25:24 ~ 11:30:33 (**5분 9초**) 는 **단일 LLM generation** 으로 다음 처리:
- Step 5.3 docId → subkey sed 치환 (즉시)
- Step 5.4 `echo "$DOCUMENT" | doc-save.sh` (DOCUMENT 출력)
- Step 6.1 assistant 보고

대부분 시간은 **`$DOCUMENT` 변수 내용을 LLM 이 token 으로 재출력**하는 시간:
- DOCUMENT 분량: requirements.md ~25 KB / ~12K 토큰
- LLM 출력 속도: GLM-5.1 ~50 토큰/초
- 예상 시간: 12000 / 50 = 240초 = **4분** (실측 5분과 부합)

bash 입장에선 단순 echo 지만, **LLM 입장에선 변수 내용을 output token 으로 직렬화**해야 함. 이게 Phase 2 fast path (PR #11) 가 절감하는 정확한 부분이다.

### 2.5 핵심 발견 3 — fast path 절감 시나리오

다음 호출 (강제 재생성 → A② fast path) 시 절감되는 단계:

| 단계 | full mode | A② fast path | 절감 |
|---|---|---|---|
| Step 0 | 37초 | 37초 | — |
| Step 1 | 3분 35초 | **0초 (스킵)** | -3분 35초 |
| Step 2+3 | 4분 50초 | 0초 (스킵) | -4분 50초 |
| Step 5.1 | 13초 | 0초 (스킵) | -13초 |
| Step 5.2 | 1분 58초 | 0초 (스킵) | -1분 58초 |
| **Step 5.3+5.4** | **5분 9초** | **~30초** (assemble.py + doc-save) | **-4분 39초** |
| Step 6 | 10초 | 10초 | — |
| **합계** | **약 16분** | **약 1분 17초** | **약 14분 43초 (92%)** |

→ requirement-refine2 fast path 는 **시간 92% 절감** 이론치. 본 PR #11 의 검증 목표.

---

## 3. 다른 스킬 측정 (TBD)

각 스킬도 같은 패턴으로 측정 예정:

| 스킬 | 측정 상태 | 예상 큰 비중 단계 |
|---|---|---|
| usecase-analyze2 | 부분 측정됨 (어제) | Step 1.4 (PlantUML) — GLM 스톨 단골 |
| requirement-refine2 | ✓ 본 문서 §2 | critique + DOCUMENT echo |
| driver-select2 | TBD | Step 1 (Driver 선정 + 정규화 속성) |
| style-define2 | TBD | Step 1.5 (PlantUML 후보별) |
| design-decide2 | TBD | Step 1 (DP × 후보 본문) + Step 1.5 (PlantUML × 후보) |
| design-integrate2 | 이미 결정적 (4ed3ea3) | LLM 4 view × 짧음, 통합 결정적 |
| satam-analyze2 | TBD | Step 1 (ADSN risk/sensitivity 구조화) |

---

## 4. 일반 패턴 (가설)

requirement-refine2 측정으로부터 도출한 가설:

### 4.1 LLM 출력 시간이 dominant cost

- LLM 입력 (prompt + context) 시간은 짧고 cache 도 가능
- **LLM 출력 (token-by-token)** 은 sequential, 단축 불가, 토큰 수에 비례
- 결과적으로 doc 분량 / 이슈 본문 분량 = 시간

### 4.2 critique 의 보이지 않는 비중

- self-loop iter 당 LLM 시간이 Step 1 과 비슷한 규모
- 첫 generation 에서 critical 0 이면 절반 이상 절감
- SKILL.md prompt 강조 / 결정적 수정 가능한 critical 의 자동화로 점진 개선 여지

### 4.3 fast path 의 진짜 가치

- LLM 0회 = 출력 토큰 0 = **시간 거의 0**
- 30ms 의 결정적 조립이 5분의 LLM echo 를 대체
- DOCUMENT 같이 큰 변수를 LLM 으로 echo 하는 패턴은 모든 워크플로우 스킬에 잠재 — fast path 가 모두 해결

---

## 5. 측정 결과의 활용

### 5.1 PR #11 검증

- A② fast path 가 실제로 92% 절감을 달성하는지 (이론치)
- requirement-refine2 다음 호출 (강제 재생성 yes → A② 선택) 시간 측정 → 비교

### 5.2 후속 개선 우선순위

가장 큰 비중 (DOCUMENT echo) → 가장 작은 비중 (Step 6) 순:

1. **DOCUMENT echo (32%)** — fast path (PR #11) 로 해결 ✓
2. **critique (30%)** — 첫 generation 의 critical 률 낮추기 / critical 자동 수정. 별도 PR
3. **Step 1 LLM 도출 (22%)** — full mode 본질적 활동. 단축 어려움. (단 SKILL prompt 최적화로 토큰 약간 절감 가능)
4. Step 5.2 링크 (12%) — POST 1건당 ~300ms × 52 = 16초인데 LLM 사이사이 generation 이 끼어 2분. LLM 출력 최소화로 단축 가능

### 5.3 측정 도구 자동화 (TBD)

수동으로 로그 파싱은 한계. 자동 측정 스크립트:
- 입력: SKILL 호출 시작/종료 시각
- 출력: 단계별 시간 + 비율 표
- 구현: `_scripts/skill-timing.py` (향후)

---

## 6. 참조

- 실측 로그: `/tmp/gojira-api.log` (2026-04-29 11:14~11:30 구간)
- OpenCode 세션 로그: `~/.local/share/opencode/log/2026-04-29T010020.log`
- PR #11: feat/workflow-fast-path
- skill-infrastructure-v3.md §12 — fast path 인프라
- tactics-for-skill.md — GLM 스톨 회피 가이드 (시간 단축의 다른 측면)
