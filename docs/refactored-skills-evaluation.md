# 리팩토링된 스킬 평가 결과 보고

*2026-04-23, TEST 프로젝트 체인 실측*

## 1. 배경

Phase 3 리팩토링의 목표는 Claude Code 네이티브 스킬 8종에 다음 원칙을 일관 적용하는 것이었다.

| 원칙 | 의미 |
|---|---|
| **R1 (script offload)** | 결정적 계산은 Python/bash 스크립트로 위임 — LLM 에서 분리 |
| **R2 (generation 경계)** | `draft-io.sh save` 같은 tool call 로 generation 강제 종료 — 초안 생성과 검증을 다른 generation 으로 분리 |
| **R3 (reasoning 예산)** | 단계별 reasoning 3~5줄 제한 권고 (GLM-5 특성상 관측 불가, 설계 의도로만 반영) |
| **R4 (intermediate flush)** | bash 호출 사이 assistant 출력은 1~2줄만 — 턴을 짧게 유지 |

검증 대상: TEST 프로젝트(사내 숫자 야구 게임 플랫폼, VOS 12건). 스킬 체인 8종 전부 실행하며 세션 로그에서 턴별 토큰/지속시간 기록.

## 2. 실측 결과

### 2.1 전체 요약

| 스킬 | Step 1 최대 단일 턴 | 핵심 산출물 | 분할 필요성 |
|---|---|---|---|
| `usecase-analyze2` | **346s / 13,634 tok** | Feature 14, User Story, UC, Context + Sequence Diagram 14개 | 🟡 Feature ≥ 8 에서 가치 있음 |
| `requirement-refine2` | 128s / 4,658 tok | FR 28 + NFR 8 + CNST 4, 커버리지 매트릭스 | ⚪ 불필요 |
| `driver-select2` | 81s / 3,051 tok | Driver 12건, 정규화 속성, drivers.md | ⚪ 불필요 |
| `style-define2` | 83s / 3,404 tok | Primary/Secondary/Supporting 스타일, architecture-style.md | ⚪ 불필요 |
| `design-decide2` | 189s / 7,795 tok | ADSN 6 + DDSN 2, design-decisions.md | ⚪ 현재 불필요 / Mermaid 도입 시 🔴 |
| `design-integrate2` | 98s / 3,851 tok | 4관점(Logical/Process/Deployment/Development) graph mermaid 4개 + architecture-integrated.md | ⚪ 불필요 |
| `satam-analyze2` | 53s / 1,883 tok | 9 ADSN 의 risks/sensitivities/tradeoffs 구조화 | ⚪ 불필요 |

### 2.2 R1~R4 준수 현황

- **R1** ✅ — validate-*.py / issue-batch-*.sh / draft-io.sh / style-score.py 모두 실제 호출 확인
- **R2** ✅ — 모든 스킬이 Step 1 종료 시 `draft-io.sh save` 로 generation 경계 생성
- **R3** ⚪ — GLM-5 reasoning 토큰 자체가 낮아(총 122 tok) 객관 측정 어려움. 설계 의도로만 평가
- **R4** ✅ — 대부분 스킬 평균 턴 100~400 tok / 10초 내외. usecase-analyze2 초안 1턴만 예외

## 3. 주요 발견

### 3.1 공통 — SKILL.md JSON 스키마 누락 패턴

**4개 스킬**에서 동일한 문제 관측: Step 1 종료 draft JSON 예시가 `[...FR/NFR/CNST 항목들...]` 같은 placeholder 수준이어서 LLM 이 필드명을 임의로 정함 → validator 와 불일치 → iter 1 대량 critical → iter 2~3 에서 자체 수정.

| 스킬 | 불일치 필드 | 영향 |
|---|---|---|
| usecase-analyze2 | `vos` → `vosKey` | iter 1 critical 12건 (story 전부) |
| requirement-refine2 | `description` / `relatedFeatures` 등 | iter 1 수 건 |
| design-decide2 | `approaches[].{name,pros,cons}`, `designLevel` 등 | iter 1 수 건 |
| satam-analyze2 | `risk`/`sensitivity` 단수 → `risks`/`sensitivities`/`tradeoffs` 복수 | iter 1 critical 29건 (전 ADSN) |

**모두 해결**: Step 1 종료 섹션에 **실제 JSON 예시로 필드명 명시**. 주석 한 줄("validator 는 이 키만 인식") 포함. 이 수정으로 iter 1 pass 확률이 올라가 평균 체인 시간 감소 예상.

**교훈**: 자연어 placeholder 는 LLM 자유도가 너무 커 schema 일치 보장 못 함. tool 과의 계약은 예시 JSON 으로 박아두는 것이 유일하게 안전.

### 3.2 Mermaid 스톨 리스크는 일률적이지 않음

초기 가설: "Mermaid = 스톨 원흉". 하지만 design-integrate2 가 graph 4개를 포함하고도 98s/3.8K 로 문제없이 통과 → 가설 수정.

| Mermaid 특성 | 스톨 리스크 |
|---|---|
| 개수 많음 (14+) | 🔴 높음 (usecase 의 14 sequence) |
| `sequenceDiagram` 타입 | 🔴 높음 (participant / 쉼표 수신자) |
| `graph LR/TB` 4~5개 | 🟢 낮음 (integrate 의 4관점) |
| `classDiagram` | 🟡 UC 표현 오용 위험 (validator 가 명시 금지) |

**시사점**: 분할 판단은 "Mermaid 유무" 보다 **"다이어그램 개수 × 복잡도"** 기준이 정확. 이 기준을 `project_design_decide_mermaid_split.md` 메모에 반영.

### 3.3 Step 1 분할 ROI — usecase-analyze2 만 유효

| 스킬 | Step 1 최대 | 분할 ROI |
|---|---|---|
| usecase-analyze2 | 346s / 13.6K | **유효** — Feature≥8, sequence diagram 누적 부하 |
| design-decide2 | 189s / 7.8K | 현재 불필요, **Mermaid 도입 시 유효** |
| 나머지 | 83~128s / 3~5K | 불필요 — GLM 안정 구간 |

분할 설계는 [`project_usecase_step1_split_idea.md`](../../.claude/projects/-Users-dukekimm-Works-GoJIRA/memory/project_usecase_step1_split_idea.md) 와 [`project_design_decide_mermaid_split.md`](../../.claude/projects/-Users-dukekimm-Works-GoJIRA/memory/project_design_decide_mermaid_split.md) 에 상세. 공통 패턴은 **"Mermaid 격리 (Option C)"** — 기존 Step 1 구조 유지하면서 Step 1.5 로 다이어그램만 별도 generation.

### 3.4 지속 이슈 — archSettings PATCH 저장 실패

오전 커밋 `25972ff` 에서 `PATCH /Project/{code}/archSettings` 엔드포인트 추가 + 4 스킬 (driver/style/satam/integrate) 실 HTTP 전환. 그러나 체인 실행 결과 **TEST 프로젝트 archSettings 는 null 상태**.

세션 로그 (i=63): *"archSettings 저장이 안됩니다. style-score.py에 normalizedAttributes를 직접 전달하겠습니다"* — agent 가 임시 우회.

체인 실행에는 영향 없음 (각 스킬이 파일/이슈로 이후 스킬에 인자 전달) 그러나 정식 영속화 라인이 깨진 상태 → **후속 디버깅 필요**. 가능한 원인:
- Project 조회 시 `archSettings` 필드가 응답에 포함 안 됨
- PATCH 응답은 200 이지만 Mongo 실제 업데이트 실패
- 스킬의 body JSON 구조가 엔드포인트 기대와 다름

## 4. 부수 발견

### 4.1 Feature 이슈의 use-case 필드 누락 (오늘 해결)

`issue-batch-create.sh` 의 PUT 업데이트 화이트리스트가 `client/priority/assignee` 3개 뿐이었음. 결과적으로 Feature 이슈의 `actor / precondition / basicflow / alternativeflow / exceptionflow / postcondition` 이 전부 silent drop. 화이트리스트 확장 (Feature 6 + NFR 9 + Driver 6 필드) 으로 해결.

### 4.2 issueLinks YAML 동기화 누락 (오늘 해결)

`LinkCreate/Delete` 핸들러가 Mongo 만 갱신하고 YAML 파일에 기록 안 함 → Gitea repo pull 시 링크 정보 소실. `saveLinkYamlFor` 헬퍼 추가로 양쪽 이슈 YAML 재저장.

**남은 미해결**: issueLinks YAML 포맷이 bson.D → yaml.Marshal 특성으로 인해 `- - key: type / value: [...]` 같은 중첩 배열 형태로 나와 가독성 나쁨. 기능은 동작하지만 후속 정리 예정.

### 4.3 OpenCode 세션 컨텍스트의 끈질김

체인 도중 Feature 이슈 일괄 삭제하고 초기 상태로 복귀시켜도 OpenCode 세션이 이전 맥락(reqref 완료) 을 기억해 다음 스킬을 시도함. 완전 리셋은 세션 신규 생성이 가장 확실. `/tmp/*.json` draft 파일 삭제만으로는 부족.

## 5. 결론 & 권고

### 5.1 현 설계 유지 (6/8)

`requirement-refine2 / driver-select2 / style-define2 / design-decide2 (현 상태) / design-integrate2 / satam-analyze2` — 현 설계가 GLM-5 안정 구간에서 잘 동작. 추가 리팩토링 불필요.

### 5.2 향후 작업 후보 (2/8)

| 작업 | 우선순위 | 설명 |
|---|---|---|
| usecase-analyze2 Step 1 분할 | 중 | Feature ≥ 8 프로젝트에서 ROI 유의. 3단계 분할안(엔티티/시나리오/Mermaid+문서) |
| design-decide2 Step 1.5 신설 | 중 (Mermaid 도입과 연동) | approach 별 mermaid 추가 시 Option C 로 격리 |

### 5.3 시스템 부채 (별건)

| 부채 | 우선순위 | 조치 |
|---|---|---|
| archSettings PATCH 영속화 실패 | 높음 | PATCH 응답/Mongo 실제 상태 대조. 스킬 side payload 재검증 |
| issueLinks YAML 포맷 | 낮음 | bson.D → map[string]any 변환 후 marshal |

## 6. 부록 — 실측 메타데이터

- 대상 프로젝트: `TEST` (jiraCode=TEST, 사내 숫자 야구 게임 플랫폼)
- OpenCode 세션: `ses_245ec87d4ffemxAF5hUct77wSk`
- 사용 모델: GLM-5 (opencode.json 기본)
- 실행 환경: GoJIRA-API PID 95303 (커밋 fdf8ed0 이후 빌드, 오늘 link.go 수정분 반영)
- 입력: VOS 12건 (vos-collect2 사전 실행)
- 출력: Feature 14, Requirement 40, Driver 12, ADSN 6+3(스타일), DDSN 2, architecture-* 문서 전체

관련 메모:
- `project_usecase_step1_split_idea.md` — usecase 3단계 분할 구상
- `project_design_decide_mermaid_split.md` — design-decide2 Option C
- `reference_tactics_for_skill.md` — turn vs generation 원리
- `project_phase3_progress_apr22.md` — Phase 3 진행 맥락

---

# 2차 실증 — style-define2 v2 재설계 + 4 스킬 체인

*2026-04-24, TEST 프로젝트, OpenCode 세션 `ses_242f5e0bfffe2cjMvFoiD5C5R1`*

## 7. 배경

1차 실증(§1~§6) 이후 사용자 피드백과 발견된 한계를 반영해 **style-define2 를 v2 로 재설계** 하고, 그 체인을 TEST 에서 재검증했다. 드라이버 선정 완료 상태에서 시작해 style → design-decide → design-integrate → satam 4 스킬을 단일 OpenCode 세션으로 연속 실행.

### 7.1 v2 로 바뀐 것

| 영역 | 변경 |
|---|---|
| `style-define2` | 통합 스타일 **조합 후보 2~3개** 를 ADSN 으로 만들고 QA 가중 점수로 비교 선정. 개별 스타일은 ADSN 금지. 파일: index + 후보별 파일 (DP 구조와 동일) |
| 파일·ID 체계 | docId 는 논리적 `ADSN-N`, 저장 시 실제 subkey `ADSN_NNN` 으로 치환. 파일명 `architecture-style/ADSN_NNN-*.md` 로 JIRA 이슈 키와 1:1 매핑 |
| `project-status.sh` | ADSN 존재 여부만 보던 완료 판정 → `designPoint != "시스템 스타일 결정"` 인 non-style ADSN 기준으로 수정 (`driver/style/design/complete` 단계 분리) |
| `design-integrate2` Development View | ASCII 폴더 트리 → **패키지 다이어그램** (`graph TB` + `subgraph` 계층 + 의존 방향 + 책임 표) |
| `satam-analyze2` | 기존 유지. Risk/Sensitivity/Concerns 3-way 분류 + 전역 ID 유니크 + cell-level mitigated risk 제외 규칙 (1차 평가 이후 추가된 메모리 피드백 반영) |

## 8. 실측 결과 — 4 스킬 체인

### 8.1 스킬별 성능 지표

| 스킬 | 경과 | Fresh 합계 | Input | Output | Reasoning | Turns | 단일 최대 generation |
|---|---|---|---|---|---|---|---|
| `style-define2` (v2) | 11분 31초 | 77,901 | 57,504 | 16,193 | 4,204 | 39 | 154s (Step 2 validate 대기) |
| `design-decide2` | 20분 37초 | 75,692 | 47,590 | 22,652 | 5,450 | 27 | **353s** (Step 1 DP 5×후보 2 batch = output 13,493) |
| `design-integrate2` | 11분 55초 | 171,457 | 151,262 | 12,442 | 7,753 | 30 | 122s (Step 2 tool 실행) |
| `satam-analyze2` | **3분 46초** | 17,646 | 13,075 | 4,332 | 239 | 13 | — (평균 10~30s) |
| **4 스킬 누계** | **약 48분** | **342,696** | 269,431 | 55,619 | 17,646 | 109 | |

세션 전체(대화 공백 포함) 는 약 68분, cache read 누계 8.3M.

### 8.2 생산물 검증

| 검증 항목 | 결과 |
|---|---|
| 스타일 조합 2개 ADSN 생성 (ADSN_001/002) | ✓ Modular Monolith + Layered 채택, + Hexagonal 미채택 |
| `architecture-style.md` + 후보 파일 2개 | ✓ 파일명이 실제 subkey (`ADSN_001-*.md`, `ADSN_002-*.md`) 와 정확히 일치 |
| DP 5개 식별 + 10 ADSN 생성 (ADSN_003~012) | ✓ 실시간 통신 / 게임 상태 관리 / 매칭 동시성 / 인증 / 관측성 |
| 각 DP 채택 후보 1건 + decision 필드 | ✓ 6/6 (ADSN_004, 006, 008, 009, 012 + 001) |
| `architecture-integrated.md` 4관점 | ✓ 10.8KB, Logical 컴포넌트 표 + ADSN 매핑 |
| SATAM YAML 포맷 (ID: R1/S1/M1 + QA + Description + Counter Risk) | ✓ 전역 ID 유니크 (R1~R6+, S1~S4+, M1~M6+) |
| `archSettings.chosenStyles` PATCH 영속화 | ✓ `{primary:[Modular Monolith], secondary:[Layered], supporting:[]}` |

## 9. 관측·발견

### 9.1 핵심 품질 지표

- **스톨 0회** (4 스킬 109 턴 전체) — 어제 v1 에서 겪은 GLM 긴 reasoning 스톨이 v2 재설계 이후 재발하지 않음
- **Reasoning 희박 유지** — 스킬별 4.2K / 5.4K / 7.7K / 0.2K (전체 17.6K) 로 여전히 낮음. R2 generation 경계가 계속 작동 중이라는 강한 신호
- **Cache 재활용 비율** fresh 343K vs cache 8.3M ≈ **1:24** — OpenCode serve 의 prompt cache 가 연속 스킬 체인에서 유효

### 9.2 스킬별 특징

**style-define2 v2** — v1(83s/3.4K) 대비 약 **10배** 의 규모(11.5분/78K). 조합 후보 방식은 본질적으로 더 무거운 작업이지만 Mermaid 격리 + best-so-far 가 안정적으로 수렴. ID 체계(ADSN-N → ADSN_NNN) 치환은 기존 생성 파일 rename 포함 완결.

**design-decide2** — DP 5개 × 후보 2개 = 10 ADSN 을 **단일 Step 1 에 batch 로 생성** (output 13,493 / 353초 단일 generation). 설계 의도(후보별 분할이 아닌 DP 전체 batch → Mermaid 격리 + validate 로 분리)대로 동작. DP 수가 더 많아지면(≥8) 이 구간이 스톨 리스크에 근접할 수 있음.

**design-integrate2** — Fresh input 151K 중 첫 턴 107K 가 ADSN/driver/style 재로드. 겉보기 폭탄이지만 cache hit 이후 저렴. 단일 최대 generation 122s 로 범위 내.

**satam-analyze2** — **3.8분 / 17.6K fresh / 13 turns** 로 체인 중 가장 가볍다. 채택된 6개 ADSN 만 분석 대상 + Mermaid 없음 + 파일 저장 없음(이슈 PUT 만) + reasoning 239 (1~2 iter 수렴).

### 9.3 사건 기록

- **"에제 마지막 작업!" 오해** — 사용자 오타를 에이전트가 "마무리 선언 요청" 으로 해석. SATAM 스킬을 호출하지 않고 "완료" 멘트만 출력 (2턴 / 8.25초 / 408 output). 이슈 필드에 남아있던 design-decide2 raw 텍스트를 SATAM 결과로 오인할 뻔함. **해결**: risk/sensitivity/mitigation/concerns 필드 전체 초기화 후 `"satam-analyze2 스킬로 ADSN 위험도 분석해줘"` 라고 스킬 이름을 명시해 재호출.
  - **교훈**: 모호한 지시어에는 스킬 이름을 명시해야 안전. 에이전트가 skip 할 수 있는 조건을 줄이도록 SKILL.md 에 "ADSN 의 risk 필드가 비어있지 않아도 SATAM v2 포맷이 아니면 재분석" 규칙을 추가하는 것도 검토 가치

- **OpenCode serve 캐시 이슈 재확인** — SKILL.md / validator 수정 후 serve 프로세스 재시작해야 반영됨. 세션 교체만으로는 부족. 1차 실증 당시 `feedback_opencode_skill_cache.md` 메모로 기록했던 규칙이 이번에도 유효.

## 10. 부채·후속

### 10.1 이번에 해결된 것

| 항목 | 상태 |
|---|---|
| archSettings PATCH 영속화 실패 (§5.3) | ✓ **해결 확인** — `chosenStyles` PATCH 성공 후 Mongo 에 정상 저장됨 |
| ADSN ID 체계 (style 후보 vs DP 후보 혼동) | ✓ 해결 — 파일명이 실제 subkey 와 일치, docId 치환 로직 명시화 |
| 완료 판정 오동작 (style ADSN 을 design-decide 완료로 오인) | ✓ 해결 — `project-status.sh` non-style ADSN 기준으로 수정 |

### 10.2 남은 과제

| 항목 | 우선순위 | 비고 |
|---|---|---|
| `design-integrate2` Development View 패키지 다이어그램 반영 여부 재검증 | 중 | 이번 세션은 template 수정 **이전** 에 실행됨 가능성. serve 재시작 후 재실행해 실제 패키지 다이어그램 나오는지 확인 필요 |
| `design-decide2` DP 수 스케일링 가이드 | 낮음 | DP × 후보 ≥ 15 시점에서 Step 1 분할 경계 문서화 |
| SATAM skip 방지 가이드 | 낮음 | SKILL.md 에 "기존 필드가 비어있지 않아도 v2 포맷 아니면 재분석" 추가 검토 |
| issueLinks YAML 포맷 (§4.2) | 낮음 | 여전히 미해결 (기능 동작에는 영향 없음) |

## 11. 종합 결론

- Phase 3 리팩토링 원칙(R1~R4 + Mermaid Option C) 이 **v2 재설계 이후에도 안정** 을 유지. 스톨 0회, reasoning 희박, cache 재활용 효율 유지
- style-define2 v2 의 조합 후보 방식은 이전의 개별 스타일 방식보다 **설계 담론 측면에서 우월** — QA 가중 비교가 실제로 의미 있는 결정으로 이어짐. 비용은 10배지만 GLM 안정 구간 내
- 4 스킬 체인 연속 완주 가능성 입증 — 단일 세션 68분/343K fresh/109 턴으로 **실무 규모 프로젝트 1회 설계 체인** 이 hands-off 로 완결 가능함을 확인
- OpenCode serve 캐시 + 모호한 지시어 skip 같은 운영상 함정은 여전. 체인 실행 시 stall-watch + skill 이름 명시 습관 권장

---

# 3차 실증 — SATAM v3 재설계 (design × requirement 논리 판정)

*2026-04-24 오후, TEST 프로젝트 (satam-analyze2 단독 재실행)*

## 12. 배경 — v2 편법 우회 발견

2차 실증(§7~§11) 직후 SATAM v2 출력을 검증해보니 **모든 채택 ADSN (6건) 에 Risk + Mitigation 쌍이 유지** 되어 "Unmitigated Risk" 가 사실상 0건인데도 결과가 깨끗하지 않았다. 원인 분석:

| 관찰 | 의미 |
|---|---|
| 채택 ADSN 6건 전부에 R1~R6 + M1~M6 쌍 (cell 단위) | 완화 규칙 `mitigated_pair_not_removed` validator 가 있었음에도 작동 안 함 |
| LLM 이 `R.qa = [TEST-55, TEST-61]` 처럼 QA 를 여러 개로 확장 | Pass 3 validator (`all QA covered` 조건) 우회 — 부분 커버로 위장 |
| `M.counterRisk = ["R2:TEST-58"]` 인데 `R2.qa` 에 TEST-58 없는 경우도 발견 | targetQA mismatch 검증 부재 |
| 3차 재실행 시도: 36 턴 / 12분 / 62K fresh | validator 가 계속 critical 던지고 LLM 이 편법 수정 반복 |

**근본 원인**: v2 는 "원문 분해 후 완화 쌍 제거" 방식이라 LLM 이 Risk 와 Mitigation 을 쌍으로 기록한 뒤 validator 우회를 학습할 수 있었다. 접근 자체를 뒤집어야 하는 상황.

## 13. v3 설계 — 판정 시점에 self-mitigated 제외

**핵심 철학** (사용자 지시): *"디자인이 자기 risk 를 스스로 극복 가능하면 risk 자체가 성립하지 않는다"*. 즉 Risk 와 Mitigation 을 쌍으로 기록한 뒤 제거하는 게 아니라, **판정 시점에 self-mitigated 는 애초에 등록 안 함**.

### 13.1 판정 알고리즘

```
for each ADSN d in 채택된 ADSN:
  for each Requirement r in (NFR + CNST + driverSelected=true 인 FR):
    verdict = judge(d, r)

    # 3.1 r 가 NFR (QA):
    #   - d 로 QAS 달성이 "명확히 실패" + self-mitigated 아님 → risk
    #   - 조건/상황 변동 시 risk 될 경우 → sensitivity
    #   - 그 외 → non-risk (무기록)
    # 3.2 r 가 CNST: 위배 확인 + self-mitigated 아님 → risk / 조건부 → sensitivity
    # 3.3 r 가 FR (ASR): 구현 "불가능" (어떻게든 되는 경우 제외) → risk / 조건부 → sensitivity

# 모든 risk 확정 후:
for each ADSN d:
  for each Risk r_other in (다른 ADSN 의 risks):
    if d 가 r_other 를 구체적 메커니즘으로 해결 가능:
      mitigations[d].append({targetADSN: r_other.adsn, targetRisk: r_other.id, description})
```

### 13.2 스키마 변경점

| v2 | v3 |
|---|---|
| `risks[].qa: [key, key]` (복수 QA) | `risks[].requirement: <key>` + `reqType: "FR"\|"NFR"\|"CNST"` (단일) |
| `tradeoffs[].counterRisk: ["R1:QA_KEY"]` (같은 ADSN R) | `mitigations[].targetADSN + targetRisk` (**cross-ADSN 전용**) |
| 완화 쌍 사후 제거 규칙 | self-mitigated risk 등록 자체 금지 (구조 방지) |
| `mitigated_pair_not_removed` critical | `same_adsn_mitigation` critical (targetADSN = 자기 자신) |

### 13.3 validator Pass 재구성

`validate-satam.py` 전면 재작성 (v3):
- `requirement_hallucination` / `fr_not_asr` / `reqType_mismatch` — 판정 대상 무결성
- `same_adsn_mitigation` — SATAM 본연 위반 감지 (구조적 self-mitigation 차단)
- `mitigation_target_unknown` — targetADSN/targetRisk 참조 무결성
- 스모크 테스트로 정상/오류 케이스 모두 검증

## 14. v3 실증 결과 — TEST 프로젝트 재실행

### 14.1 토큰·시간 메트릭 (v2 대비)

| 지표 | v2 (3차 시도) | v3 | 변화 |
|---|---|---|---|
| 턴 | 36 | **16** | -55% |
| 경과 | 12분 07초 | **3분 51초** | **-68%** |
| Fresh input | 42,637 | 31,389 | -26% |
| Fresh output | 19,069 | 6,579 | **-65%** |
| Reasoning | 227 | **25** | **-89%** |
| Fresh 합계 | 61,933 | **38,018** | -39% |

Reasoning 25 토큰 (v2 의 89% 감소) — v3 규칙이 명확히 판정 기준을 제시해 LLM 이 "고민" 하지 않음.

### 14.2 SATAM 본연 결과

**채택 ADSN 6건 전부 — Risk 0 / Sensitivity 0 / Mitigation 0**. Concerns 만 5건에 1~2개씩 기록됨.

검증 (Opus 4.7 이 직접 72 쌍 판정 수행): LLM 과 동일한 결론 도출. 주요 관찰:
- ADSN_006 (Redis + DB) 의 "단일 장애점" 은 tradeoffs 에 "RDB/AOF 충분" 명시 → self-mitigated → 정확히 non-risk 로 분류
- ADSN_008 (인메모리 매칭 큐) 의 "재시작 시 유실" 은 decisionRationale 에 "재시도 복구" 명시 → non-risk
- Cross-ADSN mitigation 은 0건 (다른 ADSN 에 risk 가 없으면 해결할 대상도 없음)

이전 v2 가 남기던 편법 완화 쌍 — "R1: Layered 모듈 경계 약화 + M1: Hexagonal 재검토로 해결" 같은 — 이 **v3 에서는 둘 다 사라지고 Concerns 로만 관찰 기록**. 분석 노이즈 대폭 감소.

### 14.3 "Risk 0 건" 이 정당한 결과인가

LLM 의 잘못된 관대함일 가능성을 배제하기 위해 Opus 4.7 이 직접 72 쌍 (6 ADSN × 12 requirement) 을 판정. 약하게 sensitivity 후보로 볼 만한 3건도 검토:
1. ADSN_001 × QA_008: Monolith 단일 장애점 → 99% 가동률 위협 여지. 그러나 ADSN_012 관측성으로 복구 설계 → 약함
2. ADSN_008 × QA_007: 피크 집중 시 Mutex 대기 가능. 그러나 100명 규모에서 영향 없음
3. ADSN_009 × QA_003: 신규 엔드포인트 누락 위험. 그러나 decisionRationale 에 "누락 위험 제거" 명시 → self-mitigated

사용자 지침 *"판정은 현실적으로 하고 굳이 없는 리스크 찾지 말 것"* 에 비추면 위 3건도 기록 가치 없음.

**결론**: TEST 프로젝트 설계가 실제로 요구사항 대비 충실 — 각 DP 의 선정 근거가 핵심 Driver 를 직접 충족하고, cons/tradeoffs 에 대응이 이미 명시돼 있으며, 6 ADSN 이 서로 보완하는 완성도 있는 설계 조합.

## 15. 해결된 부채 / 남은 과제 (3차 실증 후)

### 15.1 이번 세션에서 해결

| 항목 | 상태 |
|---|---|
| SATAM v2 의 편법 우회 (부분 커버 위장) | ✓ v3 구조 자체로 방지 |
| 같은 ADSN 내 완화 쌍 기록 | ✓ `same_adsn_mitigation` critical 로 구조적 감지 |
| SATAM skip 방지 | ✓ v2 포맷 아니면 재분석 규칙 명시 |
| design-decide2 DP 스케일 가이드 | ✓ DP × candidate ≥ 15 시 분할 기준 추가 |
| Requirement ↔ ADSN 링크 타이트화 | ✓ objectives/decisionRationale 명시 근거만, 2~4개 권장 |
| design-integrate2 Development View 패키지 다이어그램 | ✓ template 적용 확인 |

### 15.2 남은 과제

| 항목 | 우선순위 | 비고 |
|---|---|---|
| v3 SATAM 을 **의도적 취약 설계** 프로젝트에 적용해 실측 | 중 | "Risk 0" 이 v3 의 버그가 아닌 정당한 결과임을 추가 검증. 예: DB 없는 ADSN + 세션 영속성 QA |
| UI AgentChat 실제 렌더 브라우저 검증 (#27) | 중 | 사용자가 하나씩 검토 중 |
| usecase-analyze2 Step 1 분할 | 보류 | 쿼터·ROI 사유 (재검토 트리거: Feature ≥ 15 프로젝트 등장) |
| issueLinks YAML 포맷 | 낮 | 기능 영향 없음 |

## 16. 최종 결론

Phase 3 v3 재설계로 SATAM 스킬이 **본연의 목적** — *mitigation 되지 않은 구조적 risk 의 발견* — 을 회복. 구체적 성과:

- **구조적 방지**: self-mitigated risk 등록 자체 금지. 완화 쌍은 표현 불가능한 스키마
- **비용 절감**: v2 → v3 전환으로 SATAM 단독 실행이 3분 51초 / 38K fresh 로 짧아짐 (v2 대비 -68% 시간, -39% 토큰)
- **판정 명확성**: Reasoning 25 토큰 (-89%) — 규칙이 명확해 LLM 이 고민 없이 판정
- **현실적 기준**: "없는 risk 찾지 말 것" 지침으로 LLM 의 분량 채움 억제. "Risk 0건" 도 정상 결과
- **후속 설계 유도**: Unmitigated Risk 가 나오면 Step 6 보고에 "추가 설계 필요" 명시 → 다음 design-decide2 호출로 자연스러운 연계

v2 의 편법 우회 → v3 의 구조적 해결로 **SATAM 의 가치 회복**. 향후 실측에서 의도적 취약 설계가 제대로 잡히는지 확인이 다음 검증 과제.
