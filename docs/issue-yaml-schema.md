# 이슈 YAML 스키마 — Git 기반 이슈 스토리지

> **상태**: 초안 (2026-04-15) / **원천**: 각 과제 docs repo / **관계**: MongoDB `issues` 컬렉션은 이 YAML의 **파생 캐시**.

## 1. 경로 규칙

```
<project-docs-repo>/
  data/
    issues/
      <SUBKEY>.yaml     # VOS_001.yaml, FR_002.yaml, ADSN_003.yaml, UC_004.yaml, DDSN_005.yaml
```

- 파일명 = 이슈의 `subkey` + `.yaml`. 절대 변하지 않는 식별자.
- JIRA `key`(예: `GOJIRA3-13`) 는 YAML **본문 필드**로만 존재. 파일명에 넣지 않음 — key 재번호 시 rename 비용 회피.
- 브랜치 전략은 기존 docs 와 동일: 사용자 브랜치(`docs/<user>`)에서 작업 → main 머지.

## 2. 공통 필드 (모든 이슈 유형)

```yaml
key: GOJIRA3-13                # JIRA DC 동기화 키
subkey: FR_001                 # 파일명 일치. 프로젝트 내 고유
issueType: Requirement         # VOS | Feature | Requirement | Architectural Design | Detailed Design
summary: 게임방 생성 기능
description: |
  사용자는 게임을 시작하기 위해 게임방을 생성할 수 있다
status: To Do                  # JIRA 상태
priority: Medium
reporter: batman
assignee: ""
labels: []
created: 2026-04-14T18:02:39+09:00
updated: 2026-04-14T18:02:39+09:00

# 관계 링크 — JIRA DC 포맷 유지
issueLinks:
  - id: "10053"
    type: { name: "Contains(WBSGantt)" }
    inwardIssue: { key: GOJIRA3-10 }

# 코멘트
comments: []
```

### 공통 필드 규칙
- 날짜는 ISO-8601 with offset. JIRA 의 `+0900` 은 `+09:00` 로 정규화.
- `description`, `comments[*].body` 등 자유 텍스트는 YAML `|` 리터럴 블록 권장 (머지 라인 단위 깔끔).
- 빈 배열/객체는 명시 (`labels: []`) — 머지 noise 최소화.

## 3. 유형별 추가 필드

### 3.1 VOS (이해관계자 요구)
```yaml
source: 운영팀                 # 요구 출처 (이해관계자)
objectives: ""
```

### 3.2 Feature (Use Case)
```yaml
actor: 사용자
precondition: |
  ...
postcondition: |
  ...
basicflow: |
  ...
alternativeflow: |
  ...
exceptionflow: |
  ...
```

### 3.3 Requirement
```yaml
reqtype: FR                    # FR | NFR | CNST
driverSelected: false          # Architecture Driver 선정 여부

# NFR(QAS) 전용
stimulus: ""
response: ""
responseScenario: ""
environment: ""
artifact: ""
measure: ""
measureAuto: false

# 품질 속성 속성 (normalizedAttributes 집약 대상)
qas: ""      # QA Scenario
qai: ""      # QA Importance
qad: ""      # QA Difficulty
```

### 3.4 Architectural Design (ADSN)
```yaml
designLevel: Architectural     # Architectural | Detailed
designPoint: "데이터 일관성 전략"
decision: "SAGA 선정"          # 비어있지 않으면 최종 선택된 설계안
decisionRationale: |
  ...
approaches: |
  # 대안 비교 — JSON/YAML 리스트
  - name: "SAGA 패턴"
    pros: "..."
    cons: "..."

# 설계 평가
dmodel: ""                     # 디자인이 적용된 도메인 모델(시스템) 영역
objectives: ""                 # 디자인의 주요 목표
pros: ""                       # 디자인의 장점
cons: ""                       # 디자인의 단점
score: ""                      # 디자인의 평가 점수 (5점 또는 10점 기준)
eval: ""                       # 디자인의 평가 내용

# 다이어그램
adiagram: ""                   # 정적 구조 다이어그램
bdiagram: ""                   # 동적 동작 다이어그램

# SATAM 관련 — satam_analyze 스킬이 생성
risk: |
  - ID : R1
    QA : [GOJIRA3-19]
    Description : ...
sensitivity: |
  - ID : S1
    QA : [GOJIRA3-19]
    Description : ...
mitigation: |
  - ID : M1
    QA : [GOJIRA3-19]
    Counter Risk : ["R1:GOJIRA3-19"]
    Description : ...
tradeoffs: ""                  # 원문(analysis 전). satam_analyze 후엔 mitigation 으로 구조화됨
nonrisk: ""                    # 디자인이 좋은 영향을 주는 Requirement과 내용
```

### 3.5 Detailed Design (DDSN)
ADSN 과 동일 필드 + `designLevel: Detailed`. 단, `dmodel`, `archdiagram`, `objectives`, `pros`, `cons`, `score`, `eval` 은 ADSN 전용.

## 4. 예약 / 무시 필드

- `_id` (MongoDB ObjectId) — **저장 금지**.
- `_source` (ex. `gojira-agent`) — 선택. 기록하고 싶으면 `metadata.source`.
- `_syncedAt`, `_dirty`, `_deletedLinks` — **sync 상태는 Mongo 캐시 레이어의 관심사**. YAML 에 쓰지 않음.

## 5. 스키마 검증

`Agent-Service/core/issue_schema.py` 에 pydantic 모델로 강제.
- 미지 필드 허용 (forward compat), 단 경고 로그.
- 필수 필드(`key`, `subkey`, `issueType`, `summary`) 누락 시 load 실패.

## 6. 머지 정책 (요약)

- **이슈 파일 간 충돌**: 같은 이슈를 두 사용자가 동시 수정 시만 발생. 기본 "ours" + UI 충돌 배너.
- **파일 단위 격리**: 파일명이 `subkey` 이므로 이슈 간 교차 충돌 없음.
- 상세는 `design-platform.md §데이터 저장소` 참조.

## 7. Mongo 캐시 재정의

- Mongo `issues` 컬렉션은 **git → Mongo 단방향 동기화 결과**. 직접 쓰지 않음 (예외: sync 훅).
- 손상/유실 시 `git pull` 후 전체 리빌드 가능.
- 쿼리/카운트 성능 보존 목적. 이슈 수 3000 이하는 메모리 인덱스만으로도 충분하지만 기존 코드 변경 최소화 위해 유지.

### 7.1 보조 컬렉션 (PR #11)

| 컬렉션 | 역할 | retention |
|---|---|---|
| `usecase_artifacts` | use-cases.md 의 보조 데이터 (domain / userStories / useCases / diagrams) 영속화. projectId unique | 영구 |
| `issues_trash` | DELETE 시 soft delete 휴지통. `_trashedAt` / `_trashedBy` + 원본 이슈 필드 보존 | 3일 (자동 sweeper) |

## 8. TODO / 열린 이슈

- `attachment` 필드는 base64 로 YAML 에 두기엔 부담. 별도 `data/attachments/` 디렉토리 + 참조로 분리 예정.
- `approaches` 는 현재 Mongo 에 JSON 문자열로 저장됨. YAML 네이티브 리스트로 옮기면서 migration 단계에서 파싱 규칙 확정 필요.
- JIRA DC 재동기화 시 `key` 는 고정이지만 JIRA 에서 재번호 이벤트 발생 시 처리 정책 미정.
