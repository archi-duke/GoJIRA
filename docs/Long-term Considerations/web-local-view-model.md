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

### Phase 0 — ingest 검증 ✅ 완료 (2026-07-07, 커밋 089d449)
- `/Issue/ingest` · `/Project/{code}/issues/ingest` 저장 직전에 구조 검증:
  issueLinks 는 반드시 `{id, type:{name}, outwardIssue|inwardIssue:{key}}` shape 의 객체 배열,
  key/issueType 필수 등. 위반 시 400 + 로그 (NBBALL 류 오염 재발 차단).
- 현행 dual-write 는 아직 유지 — 이 단계는 안전망만 추가.

### Phase 1 — 읽기 로컬뷰 ✅ 완료 (2026-07-07, 런처 v0.1.17)

> **구현 중 근본 원인 발견**: NBBALL 오염의 진원지는 GoJIRA-API 의 YAML 재생성이었다 —
> Mongo 에서 읽은 문서의 **중첩 bson.D 를 yaml.Marshal 에 그대로 넣으면 `[{key,value}]` 리스트로
> 직렬화**된다 (saveLinkYamlFor / IssueUpdateHandler). 이 오염 YAML 이 localWrites 로 로컬 클론에
> 기록되고 재-ingest 로 Mongo 까지 오염시키는 순환. `handlers/bson_normalize.go` 로 수정(3개
> Marshal 지점 normalize + 회귀 테스트). 로컬 클론의 오염 YAML 49파일도 복구 완료.
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

### Phase 2 — 쓰기 시점 이동 ✅ 완료 (2026-07-07, 런처 v0.1.18)

> **착수 검토에서 설계 보정 (D11)**: dual-write 의 실체는 "CLI 가 ingest 를 따로 호출"이
> 아니라 **GoJIRA-API /Issue 핸들러가 Mongo 를 병합 base 로 읽고 캐시로 쓰는** 구조였다.
> 단순히 Mongo 쓰기만 제거하면 연속 로컬 편집이 깨진다(두 번째 편집이 공식본 base 로
> 병합돼 첫 편집 유실). 그래서 **base-from-client 프로토콜**로 확정: 로컬 쓰기 요청이
> 자기 로컬 YAML(진실)을 동봉하면 서버는 순수 "작성 두뇌"(병합/링크/키할당)만 수행하고
> Mongo 무접촉으로 localWrites 를 반환한다.

1. **GoJIRA-API 새 프로토콜** (`handlers/issue_local.go`, 동봉 필드 없으면 기존 경로 —
   전환기 호환):
   - `PUT /Issue` + `baseYaml` / `DELETE /Issue` + `baseYaml` / `POST /Issue/link` +
     `inwardYaml`·`outwardYaml`(한쪽만 허용 — 타 과제 이슈는 로컬에 없음) /
     `DELETE /Issue/link*` + `baseFiles` / `POST /Issue` + `localOnly:true`.
   - 모든 결과 doc 에 validateIssueDoc 적용(오염 원천 차단).
   - **키 할당 원자화** (`db/counters.go`): 기존 NextIssueNum/NextSubkey 가 issues max
     스캔이라 localOnly 생성 시 같은 키가 중복 발급되는 결함 발견 → counters 컬렉션
     `$inc` 원자 발급으로 교체(첫 발급은 기존 max 스캔 seed). 삭제 후 키 재사용 잠재
     버그도 함께 해소.
2. **Platform-API 머지 시점 ingest** (`handlers/issue_resync.go`): main push 웹훅에서
   commits[] 가 `data/issues/` 를 건드리면(판별 불가 시 안전 측 실행) main 트리의 이슈
   YAML 전량을 읽어 벌크 ingest(reconcile=true) 비동기 호출. 부분 실패/truncated 트리면
   ingest 자체를 중단(부분 집합 reconcile 금지). 수동 `POST /DocRepo/{code}/issues/resync`
   병설(웹훅 유실 대비).
   - **reconcile 삭제 보호**: 검증 실패 파일은 "부재"가 아니라 "상태 불명" — 해당 키는
     캐시 제거에서 보호하고, 키조차 모르는 파싱 실패가 있으면 제거 단계 전체를 생략
     (`reconcileSkipped`). *실사고: NBBALL main 의 오염 YAML 49건이 검증 거부되자
     reconcile 이 정상 캐시 49건을 삭제 — 로컬 복구본 재-ingest 로 즉시 복원 후 보호
     로직 추가.*
3. **클라이언트**: gojira-cli(`findIssueYamlByKey`)·`issue-write.sh`/batch 스크립트
   (`gj_find_issue_yaml`/`gj_find_link_files`)가 로컬 YAML 을 찾아 동봉. batch-create 의
   생성 후 PUT 은 생성 응답의 localWrites content 를 baseYaml 로 사용(localOnly 생성이라
   Mongo 에 없음 — 없으면 404 로 필드 유실). 런처 `/open`·`/edit` 콜드스타트 ingest 제거.
4. 전환기 호환: 동봉 필드 없는 구버전 CLI/스킬/런처는 기존 경로 그대로 — 머지 웹훅이
   어차피 main 기준으로 재수렴. JIRA sync 는 새 경로에서 호출 제거(레거시 경로 잔존,
   머지 시점 이동은 JIRA 연동 실사용 확인 후).
5. 웹훅 유실 대비: 수동 resync + 기존 Pull(`/Issue/fetch`) 보존.
6. **잔여**: NBBALL main 의 오염 YAML 49건 — 로컬 복구본 등록(PR 머지) 시 웹훅 resync 가
   공식본을 복구본 기준으로 갱신(웹훅 경로의 자연 E2E).

### Phase 3 — 편집 브릿지 부활 (§3.3 개정)
- 로컬뷰에서만 editMode 활성. **범위는 편집만이 아니라 생성/삭제까지** — 링크 드래그 연결/해제,
  이슈 생성(+Add 버튼류), 이슈 삭제, (범위 협의) IssueDetailDialog 필드 편집.
- 쓰기는 전부 launcher `/write`/`/delete`(로컬 YAML 갱신) — 서버/Mongo 무접촉. 이후 등록→PR 은
  기존 흐름.
- YAML 갱신 로직(링크 추가/삭제·이슈 생성/삭제를 YAML 문서에 반영)은 웹 또는 런처 중 어디서
  할지 착수 시 결정.
- **에이전트 변경 전달 (2026-07-07 추가 요구)**: 웹에서 발생한 변경(생성/삭제/링크/필드)은 그
  과제의 에이전트에게 전달되어, 에이전트가 **설계 내용이나 관련 문서의 수정 필요성을 판단**하게
  해야 한다. 웹 편집은 "YAML 한 줄 고치기"로 끝나지 않고 설계 일관성(SATAM 매트릭스, 유스케이스
  산출물, 설계 문서)에 파급될 수 있기 때문. 구현 옵션(착수 시 결정):
  - (a) 변경 저널: 런처가 웹발 변경을 클론 내 저널 파일(예: `.gojira/pending-changes.yaml`)에
    누적 기록 → 에이전트 스킬이 세션 시작/작업 중 이를 읽고 영향 분석. 세션이 안 떠 있어도 유실 없음.
  - (b) 능동 통지: 변경 시 런처가 해당 과제 에이전트 세션에 검토 프롬프트를 전달(세션 없으면
    spawn — 기존 /chat 재사용). 즉각적이나 세션 lifecycle 관리 필요.
  - 권장 방향: (a) 를 기본으로 하고, 웹 UI 에 "에이전트 검토 요청" 버튼으로 (b) 를 선택 제공.

## 4. 열린 결정 (착수 전 확정 필요)
1. 로컬뷰 자동 진입 판별 — 에이전트 세션 기준(제안) 외에 "클론만 있고 세션 없음"을 어떻게 볼지.
2. ~~SATAM 의 usecase_artifacts 의존~~ → **해소(2026-07-07 확인)**: usecase_artifacts 는 웹이
   전혀 읽지 않는다 — 소비자는 에이전트 스킬뿐(usecase-analyze2 가 PUT, usecase-assemble.py 가
   결정적 재조립에 GET). SATAM 은 이슈만으로 렌더(IssueDetailDialog 의 'Artifact' 탭은
   품질시나리오 필드로 무관). 로컬뷰에 제약 없음.
3. Phase 3 범위: RelationMap 연결/생성/삭제는 확정, SATAM 셀 편집·IssueDetailDialog 필드 편집
   포함 여부.
4. 대형 과제(이슈 수백 개)에서 /issues 벌크 응답 크기 — 필요 시 압축/페이지네이션.
5. 에이전트 변경 전달 방식 — 위 Phase 3 의 (a)/(b) 조합 확정 + 저널 포맷/스킬 계약.

## 5. 리스크
- 로컬 YAML 스키마 매핑 불일치 (Phase 1 선행 검증으로 조기 발견).
- 머지 웹훅 누락 → 공식본 스테일 (수동 Pull 보존 + 주기 reconcile 로 완화).
- 런처 fleet 버전 혼재기 — 토글은 런처 버전/기능 감지로 조건 노출이라 구버전에서 자연 비노출.
