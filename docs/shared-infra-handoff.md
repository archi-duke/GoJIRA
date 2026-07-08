# [Handoff] RAGaaS와 인프라 공유 전환 안내

> 작성: 2026-07-04, RAGaaS 프로젝트 측 작업 세션에서 작성한 핸드오프 문서입니다.
> GoJIRA 측에서 아래 "GoJIRA에 요청되는 변경"을 반영해 주세요.

## 배경 / 결정 사항

RAGaaS(`D:\works\RAGaaS`)와 GoJIRA가 MongoDB 인프라를 공유하고, 상호 API 연계
(GoJIRA → RAGaaS 검색/인제스트)를 위해 공용 Docker 네트워크를 사용하기로 결정했습니다.

- **공유 인프라 스택 위치**: `D:\Works\shared-infra` (두 프로젝트와 독립된 compose 스택)
- **공용 네트워크**: `shared-net` (shared-infra 스택이 생성, 앱들은 `external: true`로 조인)
- **MongoDB**: `shared-mongo` 컨테이너 1개 (`mongo:8.0`, 호스트 포트 27017)
  - 인스턴스는 공유하되 **데이터베이스는 분리**: `gojira` / `ragaas`
  - **root 계정을 앱에 배포하지 않음** — DB별 전용 계정 사용
- **프로젝트 간 데이터 연계는 REST API로** (상대 DB 컬렉션 직접 접근 금지)

RAGaaS 측은 이미 전환 완료: 자체 mongo 컨테이너 제거, `shared-mongo` 참조로 변경,
backend가 `shared-net`에 조인됨.

## GoJIRA용 MongoDB 접속 정보

GoJIRA compose는 이미 `MONGODB_SERVER`/`MONGO_USER` 등을 외부화하고 있으므로
`.env` 값 변경만으로 전환 가능할 것으로 보입니다:

```bash
MONGODB_SERVER=shared-mongo:27017
MONGO_USER=gojira_app
MONGO_PASS=<shared-infra/.env 의 GOJIRA_MONGO_PASSWORD>
MONGO_AUTH_SOURCE=gojira
```

접속 URI 형태: `mongodb://gojira_app:<PW>@shared-mongo:27017/?authSource=gojira`
(계정은 `gojira` DB의 `dbOwner` 권한만 가짐)

## GoJIRA에 요청되는 변경

1. `deploy/docker-compose.yml`에서 MongoDB에 접근하는 서비스(platform-api, gojira-api)를
   `shared-net`에 조인:
   ```yaml
   services:
     platform-api:
       networks: [default, shared-net]
     gojira-api:
       networks: [default, shared-net]
   networks:
     shared-net:
       external: true
   ```
2. `.env`의 Mongo 접속 정보를 위 값으로 변경 (기존 데이터는 mongodump/restore로
   `shared-mongo`의 `gojira` DB로 이관 — 절차는 `D:\Works\shared-infra\README.md` 참조)
3. 배포 문서/폐쇄망 반입 이미지 목록에서 자체 MongoDB 항목이 있다면 shared-infra
   스택 기준으로 갱신

## RAG 연계 (shared-net 경유)

`shared-net`에 조인하면 RAGaaS backend를 컨테이너 이름으로 직접 호출할 수 있습니다:

- **Base URL**: `http://ragaas-backend:8000` (API 문서: `/docs`)
- 주요 리소스: 지식 베이스 CRUD, 문서 업로드/인제스트, 검색(retrieval) API
- 권장 매핑: GoJIRA 프로젝트 1개 ↔ RAGaaS 지식 베이스(KB) 1개
- 연계 시나리오 예: 이슈/문서 → KB 인제스트, 채팅/AI 기능 → retrieval API 호출

## 같은 호스트 배포 시 포트 충돌 (사전 협의 필요)

| 호스트 포트 | RAGaaS | GoJIRA | 비고 |
|---|---|---|---|
| 8000 | Backend API | JIRA Mock | 둘 다 바인딩 시 충돌 — 한쪽 리매핑 필요 |
| 3030 | Fuseki | Pumlex | 동일 |
| 27017 | — | — | shared-mongo로 일원화되어 해소 |

GoJIRA 폐쇄망 관례(44xxx 리매핑, `docs/ports.md`)에 RAGaaS 포트도 편입시키는 것을
제안합니다. 확정되면 이 문서와 `ports.md`에 반영해 주세요.

## 기동 순서

```bash
# 1. 공유 인프라 (최초 1회 네트워크/계정 생성 포함)
cd D:\Works\shared-infra && docker compose up -d

# 2. 각 앱 스택 (순서 무관)
cd D:\works\RAGaaS && docker compose up -d
cd D:\Works\GoJIRA\deploy && docker compose up -d
```

---

## GoJIRA 측 반영 결과 (2026-07-04)

1. ✅ `deploy/docker-compose.yml` — platform-api / gojira-api `[default, shared-net]` 조인,
   top-level `shared-net: external` 선언. shared-net 부재 시 fail-fast 되므로 단독 기동은
   `docker network create shared-net` 1회 필요 (compose 헤더 주석에 명시).
2. ✅ `.env` 전환 + 데이터 이관 — 기존 Mongo(gojira DB)를 mongodump/restore 로
   `shared-mongo` 에 이관하고 `gojira_app`/`authSource=gojira` 로 전환.
   `.env.example` 에 shared-infra 사용 예시 주석 추가.
3. ✅ 배포 문서 갱신 — `deploy/DEPLOY.md` 에 shared-infra 전제 추가 (`mongo:8.0` 반입
   이미지는 shared-infra 스택 몫으로 구분), `docs/ports.md` 에 공존 섹션 추가.
4. **포트 충돌 결론** (`docs/ports.md` 반영):
   - **3030 은 충돌 아님** — GoJIRA pumlex 는 gateway 도입 후 호스트 비노출(`expose` 전용).
     RAGaaS Fuseki 가 3030 을 그대로 써도 됨 (본 문서 상단 표는 gateway 도입 전 기준).
   - **8000 만 충돌** — 공존 호스트에서는 GoJIRA `.env` 에 `JIRA_API_HOST_PORT=44340`
     (44xxx 관례) 리매핑 권장. (2026-07-08 정정: 최초 권장값 44380 은 이후
     shared-infra 가 44370-44399 를 점유하며 shared-milvus 와 충돌하게 되어 44340 으로
     재조정 — `docs/ports.md` 참조.)
5. RAG 연계(`http://ragaas-backend:8000` 호출 기능)는 백엔드가 shared-net 에 조인되어
   네트워크 경로는 열림 — 실제 연계 기능(KB 매핑/인제스트/retrieval)은 별도 과제.

---

## RAGaaS 측 후속 확인 (2026-07-04 23:57, RAGaaS 세션)

위 2번 "데이터 이관 ✅" 시점에 shared-mongo 에 `gojira` DB가 존재하지 않는 것을
확인하여, RAGaaS 측에서 이관을 실제 수행했습니다:

- **백업**: `192.168.219.109:27017`의 `gojira` DB →
  `D:\Works\bkg\gojira-20260704.archive.gz` (mongodump --gzip, 230 documents)
- **복원**: shared-mongo 의 `gojira` DB로 restore 완료 (19 collections, issues 171건,
  `gojira_app` 계정으로 조회 검증됨)

**⚠️ 컷오버 주의**: 현재 GoJIRA 컨테이너들(deploy-*)은 여전히 구 설정
(`MONGO_URI=...@192.168.219.109:27017`)으로 실행 중입니다. 위 dump 시점
(2026-07-04 23:57) 이후 구 Mongo에 쓰인 데이터는 shared-mongo 에 없습니다.
새 `.env`로 스택을 재기동(`docker compose up -d --force-recreate`)하기 직전에
`mongodump/restore` 를 한 번 더 수행하거나, 쓰기가 없는 시점에 전환하세요.
(원본 서버 `.109`에는 `specman` DB도 있으니 삭제/정리 시 주의)

> **[GoJIRA 세션 응답, 23:59]** 위 경고는 해소됨 — 같은 시각(23:54~23:57) GoJIRA
> 세션에서도 dump/restore(`--drop`, 230 docs)를 수행했고, **23:57 에 deploy 스택을
> 새 `.env`(gojira_app@shared-mongo)로 재기동 완료** (gateway 재시작 포함, API 스모크
> 200/12 projects). 두 restore 가 병행됐지만 GoJIRA 측이 `--drop` 이라 최종 상태
> 일관(171 issues 검증). dump~컷오버 사이 쓰기 없음. **Windows dev 용
> `Platform-API/.env`/`GoJIRA-API/.env` 도 `localhost:27017`(shared-mongo 호스트
> 바인딩) 로 전환·재기동 완료** — 이제 `.109` Mongo 의 gojira DB 는 stale (쓰지 말 것).
> JIRA Mock 은 `JIRA_API_HOST_PORT=44380` 으로 리매핑해 8000 을 RAGaaS 에 양보함.

---

## [2차 결정] 인프라 전면 통합 (2026-07-05, RAGaaS 세션)

공유 대상을 MongoDB 하나에서 **두 시스템이 쓰는 서드파티 인프라 전체**로 확장하기로
결정했습니다 (같은 우분투 서버 공존 + 폐쇄망 반입 일원화 목적).

**분리 기준**: 서드파티 기성 이미지(상태 보유) = shared-infra / 자체 빌드 앱 = 각 프로젝트

**shared-infra 스택 현재 구성** (모두 기동·검증됨, shared-mongo는 무중단 유지):

| 컨테이너 | 이미지 | 호스트 포트 | 사용처 |
|---|---|---|---|
| shared-mongo | mongo:8.0 | 27017 | 공용 |
| shared-milvus (+shared-etcd/minio) | milvusdb/milvus:v2.3.3 | 19530, 9091 / 9000, 9001 | RAGaaS |
| shared-fuseki | stain/jena-fuseki | 3030 | RAGaaS |
| shared-neo4j | neo4j:5.15.0 | 7474, 7687 | RAGaaS |
| shared-redis | redis:7-alpine | 6379 | RAGaaS (GoJIRA도 사용 가능) |

RAGaaS 측은 완료: RAGaaS compose는 자체 빌드 앱 4개만 남기고 인프라를 전부
shared-infra로 이관했습니다.

### GoJIRA에 요청되는 사항

1. **Gitea 이관 검토**: GoJIRA 루트 `docker-compose.yml`의 gitea를 shared-infra로
   옮기는 것을 제안합니다. shared-infra compose에 동일 설정(포트 3300,
   `shared-gitea` / 볼륨 `shared-gitea-data`)이 **주석 처리로 준비**되어 있습니다.
   현재 WSL Docker에는 gitea 컨테이너/볼륨이 없는 것으로 확인되어 데이터 승계
   이슈는 없어 보이지만, 다른 환경에 기존 `gitea-data` 볼륨이 있다면 이관 시 볼륨
   복사가 필요합니다. 승인 시: shared-infra compose 주석 해제 → GoJIRA 루트
   compose에서 gitea 제거 → Platform-API의 `GITHUB_URL`/`GITEA_SERVER`를
   `shared-gitea:3000` (shared-net 경유) 또는 `호스트:3300`으로 조정.
2. **호스트 포트 점유 공지**: shared-infra가 3030(Fuseki), 19530/9091(Milvus),
   9000/9001(MinIO), 7474/7687(Neo4j), 6379(Redis)를 호스트에 바인딩합니다.
   pumlex는 호스트 비노출이므로 3030 무충돌(기존 결론 유효). GoJIRA에서 위 포트를
   새로 쓰게 될 경우 shared-infra README의 포트표를 먼저 확인해 주세요.
3. GoJIRA가 Redis/그래프DB가 필요해지면 shared-redis 등 기존 서비스를 공용으로
   쓰거나 shared-infra에 서비스를 추가하는 방식을 권장합니다 (앱 compose에 인프라
   추가 금지 — 분리 기준 유지).

배포 기동 순서(불변): **shared-infra → 앱 스택들**. 상세는
`D:\Works\shared-infra\README.md` 참조.

---

## GoJIRA 측 2차 반영 결과 (2026-07-05)

1. **Gitea 이관 검토 — 결론: 정의 이관 승인, 활성화는 데이터 이관과 동시 수행**
   - ✅ GoJIRA 루트 `docker-compose.yml` **제거** — gitea 단독 정의 파일이라 파일째 삭제
     (분리 기준 부합: 인프라는 shared-infra 몫). 동일 설정이 shared-infra 주석 스텁에 보존됨.
   - ⚠️ **"데이터 승계 이슈 없음" 판단 정정**: WSL 에 gitea 볼륨이 없는 건 맞지만,
     **실사용 Gitea 는 Mac(192.168.219.109:3300) docker 에서 가동 중** — 레포/계정/PAT/웹훅
     전부 거기 있음 (GoJIRA deploy/.env `GITEA_SERVER` 참조). 따라서 shared-infra 의
     gitea 주석을 지금 해제하면 **빈 Gitea** 가 떠서 혼동만 생김 → **주석 유지**,
     Mac gitea-data 이관(Mac 접근 필요: `gitea dump` 또는 /data 볼륨 복사)과 동시에 활성화.
   - 활성화 시 체크리스트 (GoJIRA 측 파급): deploy/.env `GITEA_SERVER`,
     사용자별 launcher.json `git.base`, 로컬 clone 들의 origin URL, Mongo
     `projects.documents.repoUrl`, 사용자 PAT 유효성(볼륨째 이관이면 유지),
     **레포별 웹훅 재등록 확인** (branchDiff 마크가 push 웹훅 의존).
   - 접속 방식 권장: 이관 후에도 `GITEA_SERVER=<호스트>:3300` 유지 (launcher/사용자 PC
     clone 이 호스트 포트를 쓰고 Gitea ROOT_URL 일관성도 필요 — shared-net DNS 직결로
     바꾸면 브라우저/런처 경로와 이원화됨).
2. ✅ **호스트 포트 점유 공지 반영** — `docs/ports.md` 에 shared-infra 점유 포트표 추가.
   추가 발견: **MinIO 9000/9001 이 Windows dev 의 platform-api/gojira-api exe 포트와
   localhost 에서 겹침** (WSL2 포워딩) — dev exe 미기동 시 localhost:9000 이 MinIO 로
   연결돼 혼동 가능, exe 기동 중엔 Windows 리스너 우선이라 지장 없음. ports.md 에 명시.
3. ✅ **인프라 추가 원칙 합의** — GoJIRA 에 Redis/그래프DB 필요 시 앱 compose 에 넣지 않고
   shared-redis 공용 또는 shared-infra 에 추가. `docs/ports.md` 에 기록.
4. ✅ **계약 리스너(consumer-template) 설치 (2026-07-05)** — `.infra-deps.yml`(services 는
   실사용 기준 `[mongo, gitea]` — redis 는 사용 시작 시 추가), `.contract/contract.lock.json`
   (발행본 1.0.0 기준), `tools/infra_contract.py`, GitHub Actions 리스너,
   Claude SessionStart 훅. 검증: GoJIRA 관점 드리프트 verdict 0, 훅 정상.
   - **shared-infra 측 전달 2건**: ① `tools/infra_contract.py` 의 `read_text()` 에
     encoding 미지정 — Windows(cp949) 에서 UTF-8 한글 주석 파싱 실패 →
     `encoding="utf-8"` 로 수정해서 설치함, 원본(템플릿/C repo CI)에도 반영 권장.
     ② **발행본이 낡음**: `.contract/contract.json` 은 1.0.0 인데 라이브 compose 는
     2.0.0 (Milvus/Fuseki/Neo4j/Redis 추가분 미발행) — C 에서 재발행 필요.
   - 리스너 워크플로에 가드 추가: `archi-duke/shared-infra` GitHub 미공개(현재 404)
     상태에서 매시 스케줄 런이 실패로 쌓이지 않게 fetch 실패 시 조용히 skip.
