# 의사결정 기록 — 2026-07-05

> 형식: 배경 → 결정 → 근거 → 구현 위치. (관행: docs/decisions/YYYY-MM-DD-decisions.md)

---

## D1. MongoDB 를 shared-infra(RAGaaS 공유)로 전환 — 완료

- **배경**: RAGaaS 와 같은 호스트 공존 + 폐쇄망 반입 일원화를 위해 MongoDB 인스턴스 공유 결정 (`docs/shared-infra-handoff.md`). 인스턴스 공유·DB 분리(gojira/ragaas), root 미배포, 연계는 REST API 원칙.
- **결정/수행** (2026-07-04 밤 ~ 05):
  - deploy compose: platform-api/gojira-api `shared-net`(external) 조인
  - 데이터: Mac Mongo(.109) gojira DB → shared-mongo 로 dump/restore(`--drop`, 230 docs, 171 issues 검증). RAGaaS 세션과 병행 수행됐으나 `--drop` 으로 최종 일관
  - deploy/.env + **Windows dev exe 용 .env 2개**(localhost:27017) 모두 `gojira_app`/`authSource=gojira` 전환·재기동. **Mac .109 의 gojira DB 는 stale — 사용 금지**
  - JIRA Mock 호스트 포트 8000→44380 리매핑 (RAGaaS Backend 에 8000 양보)
- **함정 기록**: nginx gateway 가 백엔드 재생성 후 구 IP 캐시 → 502, gateway 재시작 필요. wsl.exe 인라인 bash 인용부호 깨짐 → 스크립트 파일 + `bash -c "bash /mnt/..."` 로 우회 (MSYS 경로변환).
- **구현**: deploy/docker-compose.yml, deploy/.env(.example), deploy/DEPLOY.md, docs/ports.md, docs/shared-infra-handoff.md

## D2. Gitea 이관 — 정의는 shared-infra 로, 활성화는 데이터 이관과 동시

- **배경**: 2차 결정(인프라 전면 통합)에서 GoJIRA 루트 docker-compose.yml 의 gitea 를 shared-infra 로 옮기자는 요청. RAGaaS 측은 "WSL 에 볼륨 없어 데이터 승계 이슈 없음"으로 판단.
- **결정**:
  - GoJIRA 루트 `docker-compose.yml` **삭제** (gitea 단독 정의 — 분리 기준상 인프라는 shared-infra 몫, 동일 설정이 shared-infra 주석 스텁에 보존)
  - **shared-gitea 활성화는 보류** — 실사용 Gitea 데이터(레포/계정/PAT/웹훅)는 Mac(.109:3300) 에 있어, 지금 활성화하면 빈 Gitea 가 떠 혼동만 유발. Mac gitea-data 이관(Mac 접근 필요)과 동시에 활성화
  - 이관 후에도 접속은 `GITEA_SERVER=<호스트>:3300` 방식 유지 (런처/로컬 clone/ROOT_URL 일관성 — shared-net DNS 직결로 바꾸면 브라우저·런처 경로와 이원화)
- **활성화 시 체크리스트**: GITEA_SERVER, launcher.json git.base, 로컬 clone origin URL, projects.documents.repoUrl, PAT 유효성, **웹훅 재등록**(branchDiff 마크가 push 웹훅 의존 — 2026-07-04 D7)

## D3. 인프라 배치 원칙 (RAGaaS 와 합의)

- **분리 기준**: 서드파티 기성 이미지(상태 보유) = shared-infra / 자체 빌드 앱 = 각 프로젝트 compose. GoJIRA 에 Redis/그래프DB 필요 시 앱 compose 에 추가 금지 — shared-redis 공용 또는 shared-infra 에 추가.
- **포트**: shared-infra 점유 포트표를 docs/ports.md 에 편입. 발견 사항 — **MinIO 9000/9001 이 Windows dev exe(platform-api/gojira-api) 와 localhost 에서 겹침** (WSL2 포워딩): dev exe 미기동 시 localhost:9000 → MinIO 로 연결되는 혼동 가능성 문서화.
- **기동 순서**: shared-infra → 앱 스택들 (불변).

## D4. 사내 데이터 이전 방침 (재확인)

- Mongo/Gitea 의 사내 이전은 **사내→사내**로 수행 — 로컬 dev 데이터를 사내로 반입하지 않는다. 로컬은 로컬끼리(D1 Mongo 완료, D2 Gitea 예정)만 정리.
