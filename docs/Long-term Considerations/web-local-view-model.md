# 웹 이슈 뷰 이원화 — 공식본(main/Mongo) + 내 작업본(로컬 YAML)

> 2026-07-07 논의 확정. local-agent-issue-model.md §3.3(웹 조회 전용)의 **개정 설계**.
> 상태: **설계 확정, 구현 착수 전** (Phase 0 만 즉시 실행 대상).

## 1. 배경 — 무엇이 문제였나

- §3.3 으로 웹 이슈 UI(RelationMap/SATAM/IssueList)는 조회 전용이 됐고, 편집은 로컬(YAML=진실)로
  이전됐다. 그런데 웹 조회는 Mongo 캐시를 보는데, 이 캐시는 로컬 편집 시마다 **즉시 dual-write**
  (YAML write + `/Issue/ingest`) 로 갱신된다.
- 이 구조의 두 가지 문제가 같은 날 함께 드러났다:
  1. **동시편집 취약**: 여러 사용자가 각자 로컬에서 작업한 내용이 검증 없이 곧바로 공유 캐시(Mongo)에
     덮어써진다(`$set`). 편집 브릿지(웹 편집 부활)를 하려 해도 같은 지점이 병목.
  2. **캐시 오염 실증**: NBBALL 과제의 issueLinks 128건이 잘못된 shape([{key,value}] 배열)로
     ingest 되어 Relation Map 연결선이 통째로 사라졌다 — 검증 없는 쓰기 경로의 실제 사고 사례.
     (deploy/scripts/repair-nbball-issuelinks.js 로 복구)

## 2. 확정 모델

**"원격 main 으로 발행된 것만 공식 공유 버전이다. 편집 중에는 각자 자기 로컬 작업본을 본다.
작업 완료 시 등록(commit/push) → PR → 머지로 공식화한다."** (문서 편집과 동일한 모델을 이슈에도)

| 뷰 | 데이터 소스 | 누가 보나 | 갱신 시점 |
|---|---|---|---|
| **공식본** (기본) | Mongo 캐시 (= main 반영본) | 모두 — 런처 없어도 됨 | **PR 머지 시점에만** (웹훅 → 재-ingest) |
| **내 작업본** | 로컬 런처 → 로컬 클론의 이슈 YAML | 그 과제를 로컬에서 작업 중인 본인 | 로컬 파일이 진실 — 실시간 |

- 동시편집 문제 소멸: 편집은 각자 자기 로컬본에서만, 공유본 갱신은 PR 머지 단일 경로.
  경합은 git 이 PR/push 충돌로 잡는다 (문서와 동일).
- Mongo 쓰기 지점이 "편집 때마다 각 클라이언트" → "머지 웹훅 1곳" 으로 줄어 검증을 한 곳에서
  빡빡하게 걸 수 있다.
- 편집 브릿지(웹에서 드래그 연결 등) 부활 가능: 로컬뷰에서만 활성화, launcher /write 로 자기
  로컬 YAML 만 갱신 — 공유 상태를 건드리지 않으므로 안전.

### 기각한 대안
- 읽기 전체를 런처 경유로 (Mongo 캐시 폐지): 공식 공유 뷰마저 각자 로컬 클론 상태에 의존하게 되고
  (남의 변경이 pull 전이면 안 보임), 런처 미기동/부재 환경에서 화면 자체가 안 뜬다. 기각.
- 웹 편집을 Mongo 직접 쓰기로 원복: YAML=진실 붕괴, 로컬 동기화가 웹 변경을 덮어씀. 기각.

## 3. 단계별 계획

### Phase 0 — ingest 검증 (즉시, 독립적, ~반나절)
- `/Issue/ingest` · `/Project/{code}/issues/ingest` 저장 직전에 구조 검증:
  issueLinks 는 반드시 `{id, type:{name}, outwardIssue|inwardIssue:{key}}` shape 의 객체 배열,
  key/issueType 필수 등. 위반 시 400 + 로그 (NBBALL 류 오염 재발 차단).
- 현행 dual-write 는 아직 유지 — 이 단계는 안전망만 추가.

### Phase 1 — 읽기 로컬뷰 (런처 v0.1.17 + 웹)
1. **런처**: `GET /issues?code&user` 신설 — 로컬 클론의 이슈 YAML 을 raw 로 일괄 반환
   (`[{path, content}]` 한 번의 응답). YAML 파싱은 웹에서(js-yaml) — 런처는 얇게 유지.
   기존 /files + /file 반복 호출로도 가능하지만 이슈 수십 개 × HTTP 왕복이라 벌크가 낫다.
2. **웹**: RelationMap/SATAM/IssueList 상단에 뷰 토글 `[공식본(main) | 내 작업본]`.
   - 토글 노출 조건: 런처 감지 성공 + 그 과제 로컬 클론 존재.
   - 기본값: 공식본. 단, 그 과제의 에이전트 세션이 살아있으면(런처 /sessions) 내 작업본.
   - 내 작업본 선택 시: 런처 /issues → YAML 파싱 → 기존 이슈 객체 스키마로 매핑 → 동일 렌더 경로.
   - 화면에 현재 뷰 출처 명시 (예: "내 작업본 — docs/duke.kimm, 미발행 변경 포함").
3. **선행 검증**: 로컬 이슈 YAML 스키마 ↔ 웹 기대 필드(key/subkey/issueType/labels/issueLinks/
   reqtype) 매핑 확인. 불일치 시 여기서 변환 계층 확정.

### Phase 2 — 쓰기 시점 이동 (dual-write → 머지 시점 ingest)
1. Platform-API 웹훅(이미 PR merged 이벤트 수신 중)에서 이슈 YAML 변경 파일을 감지해
   GoJIRA-API `POST /Project/{code}/issues/ingest` (기존 벌크 엔드포인트 재사용, Phase 0 검증 적용)
   호출 → Mongo 를 main 기준으로 갱신.
2. 로컬 편집 도구(gojira-cli/이슈 스킬)의 즉시 ingest 호출 제거 — YAML write 만.
   JIRA sync 도 머지 시점 흐름으로 이동.
3. 전환기 호환: 구버전 CLI 가 ingest 를 계속 쏴도 검증만 통과하면 무해(멱등) — 웹훅 ingest 가
   머지 시점에 어차피 main 기준으로 재기록. fleet 업데이트 후 엔드포인트 정리.
4. 웹훅 유실 대비: 기존 수동 Pull(`/Issue/fetch`)/reconcile 경로 보존.

### Phase 3 — 편집 브릿지 부활 (§3.3 개정)
- 로컬뷰에서만 editMode 활성: 드래그 연결/해제, (범위 협의) IssueDetailDialog 필드 편집.
- 쓰기는 전부 launcher `/write`(로컬 YAML 갱신) — 서버/Mongo 무접촉. 이후 등록→PR 은 기존 흐름.
- YAML 갱신 로직(링크 추가/삭제를 YAML 문서에 반영)은 웹 또는 런처 중 어디서 할지 착수 시 결정.

## 4. 열린 결정 (착수 전 확정 필요)
1. 로컬뷰 자동 진입 판별 — 에이전트 세션 기준(제안) 외에 "클론만 있고 세션 없음"을 어떻게 볼지.
2. SATAM 의 파생 데이터(usecase_artifacts 컬렉션)는 Mongo 에만 있음 — 로컬뷰에서 SATAM 을
   어디까지 지원할지(이슈 기반 부분만? 아티팩트는 공식본 고정?). **착수 전 확인 필수.**
3. Phase 3 범위: RelationMap 연결만 vs SATAM 셀 편집 vs IssueDetailDialog 전체.
4. 대형 과제(이슈 수백 개)에서 /issues 벌크 응답 크기 — 필요 시 압축/페이지네이션.

## 5. 리스크
- 로컬 YAML 스키마 매핑 불일치 (Phase 1 선행 검증으로 조기 발견).
- 머지 웹훅 누락 → 공식본 스테일 (수동 Pull 보존 + 주기 reconcile 로 완화).
- 런처 fleet 버전 혼재기 — 토글은 런처 버전/기능 감지로 조건 노출이라 구버전에서 자연 비노출.
