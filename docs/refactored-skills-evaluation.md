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
