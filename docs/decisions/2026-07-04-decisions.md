# 의사결정 기록 — 2026-07-04

> 세션 중 결정된 사항의 원천 기록. 추후 설계자료(design-*.md)로 정리하기 위한 재료.
> 형식: 배경 → 결정 → 근거 → 구현 위치.

---

## D1. branchDiff 판정 — docs/{user} 브랜치 미존재 = "diff 없음"

- **배경**: 사내에서 docs/{user} 브랜치가 없는 프로젝트에 과제수정 마크(alert-diff.svg)가 잘못 표시됨. Platform-API `/tree` 는 미존재 ref 에 404 가 아니라 200+빈 트리(`notFound: true`)로 응답(클라이언트 콘솔 noise 제거 목적의 의도적 설계)하는데, GoJIRA-API `checkTreeDiff` 가 빈 트리를 정상 트리로 간주해 파일 수 불일치 → diff 로 오판.
- **결정**: 사용자 트리가 비어 있으면 "브랜치 없음 = diff 없음" 으로 판정.
- **구현**: `GoJIRA-API/handlers/project_alerts.go` (이후 D7 에서 플래그 방식으로 대체되며 이 가드 자체는 제거됨 — 판정 의미는 D7 에 승계).

## D2. 알림 발송 모듈화 — 표준 인터페이스 + 기동 시 provider 로딩

- **배경**: PR 생성/머지가능 메일이 SMTP 직발송으로 하드코딩되어 있었음. 사내망은 별도 제공되는 사내 알림 API 를 써야 함.
- **결정**:
  - `Platform-API/notify` 패키지 신설. **채널 중립** 설계 — 메일 전용 시그니처가 아니라 `Message{Kind, To, Title, Body, Link, Meta}` + `Notifier{Name, Send}` 인터페이스. 메신저 등 채널 확장을 감안 (제목 없는 채널은 Title 을 본문 첫 줄로 재해석하는 식으로 각 구현체가 책임).
  - 서비스 기동 시 `NOTIFY_PROVIDER` 설정으로 구현체 선택: `smtp`(사외/dev) | `internal`(사내 API) | `none` | 미설정(자동: SMTP_HOST 있으면 smtp). 잘못된 명시 설정은 fail-fast(log.Fatal) — config 철학과 동일.
  - **사내(internal) 어댑터는 스펙 확정 전까지 빈 스텁** — 호출 로그만 남김. 스펙 확정 시 `notify/internal.go` 한 파일만 구현하면 되고 발송처/인터페이스 무변경.
  - 수신자 ID(knoxId)→채널 주소(이메일 등) 변환은 채널별 구현체 책임 (smtp 가 DB 조회).
- **구현**: `Platform-API/notify/{notify,smtp,internal}.go`, `handlers/mail.go`(Message 조립만), `config.go`(NOTIFY_PROVIDER/NOTIFY_API_URL/NOTIFY_API_TOKEN — 토큰은 ENC: 복호화), main.go 기동 배너에 provider 표시, compose/.env.example 배선.

## D3. LLM 검증용 chat API 추가

- **배경**: llm.json(사외 OpenAI ↔ 사내 gpt-oss gateway 스왑 구조)이 실제로 동작하는지 확인할 수단이 없었음. 사내 반영 시 인증 헤더/프록시 문제를 즉시 식별할 필요.
- **결정**: `GET|POST /api/v2/LLM/chat?q=...` — 질의를 llm.json 설정 경로로 실호출하고 `answer` + 진단정보(`endpoint`/`model`/`elapsedMs`)를 반환. 실패 시에도 같은 진단 필드와 함께 502. 기존 `/LLM/call` 게이트웨이와 같은 `llm.Call()` 경로를 타므로 이 API 성공 = LLM 설정 전체 검증.
- **구현**: `Platform-API/handlers/llm.go` `LLMChatHandler`. 검증: gpt-4o-mini 실응답 확인 (OPENAI_API_KEY 는 Platform-API/.env, git-ignored).

## D4. DocGen(설계문서 생성) 기능 제거 — LLM 인프라는 유지

- **배경**: DocGen(프로젝트 우클릭 "설계문서 생성")은 이슈 DB 를 타입별 마크다운으로 덤프하는 기능. ① 스킬 생성 문서(business/, architecture/)와 중복이며 정보가 더 적음 ② 루트 README.md/requirement.md 를 통째로 덮어써 정성 들인 문서 훼손 위험 ③ 편집 로컬이관 때 제거된 `/git/publish` 를 여전히 호출해 이미 404 로 죽어 있었음 (removal-inventory 조사 누락 — 마지막 잔여 호출자).
- **결정**: 기능 자체를 제거. 단 **LLM 호출 인프라는 재사용 예정이므로 유지** — `callLLM`(GoJIRA-API→Platform /LLM/call 단일 진입), `callPlatform`(→ `handlers/platform.go` 로 분리), `config/prompt.go` LoadPrompt, Platform-API LLM 게이트웨이 전부.
- **경과**: 제거 전에 런처 위임 방식(서버는 파일+LLM 커밋메시지 반환, 웹이 런처 /write+/sync 로 저장)으로 이관해 e2e 성공까지 확인한 뒤, 효용 판단으로 제거를 결정.
- **구현**: `GoJIRA-API/handlers/docgen.go` 삭제, `/Issue/docgen` 라우트/ContextMenu 메뉴/ProjectMap.handleDocGen 제거.

## D5. git 책임 경계 재확인 — "서버는 절대 commit/push 하지 않는다"

- **배경**: DocGen 이관 논의 중 서버 push 부활(임시방편) vs 런처 위임 중 선택 필요.
- **결정**: 런처 위임 유지. 서버는 콘텐츠 생성만, **위임의 중개자는 웹(브라우저)** — 서버는 런처(127.0.0.1 데몬)에 접근할 방법 자체가 없음. 흐름: 웹→서버(생성 요청)→웹(산출물 수신)→런처 /write(로컬 저장)→/sync(사용자 인증으로 commit+push). 커밋이 사용자 이름으로 남고, main 반영은 항상 PR 리뷰 절차 경유.
- **파일 저장 단계의 의미**: /write 는 사용자 PC 로컬 clone 의 working tree 디스크 쓰기(네트워크 없음). /sync 가 원격 docs/{user} 까지. main 은 발행(PR)→승인→머지로만.

## D6. 사내 런처 연동 장애 — 원인 확정과 이중 수정

- **배경**: 사내에서 "과제 문서 등록 클릭 시 로컬 런처 감지 안 됨" 중단. opencode 의 진단(launcher.json 에 token 필드 부재 → 빈 토큰)은 코드 검증 결과 **불성립** — 설치 토큰은 launcher.json 에 없으면 `~/.gojira/launcher-token` 에서 자동 생성/로드되어 빈 값이 불가능하고, /health 는 Origin 불일치 시 token 을 생략(또는 secure 미들웨어가 403)함.
- **실측 확정**: DevTools 에서 `/sync` preflight(OPTIONS) 204 + 본 POST "canceled" → **웹의 30s AbortController 타임아웃**이 원인. /sync(최초 clone+push)가 사내망에서 30s 초과 → abort → catch 가 '런처 미기동' 으로 **오진 표시**한 것.
- **결정/수정 2건**:
  1. **웹**: 런처 git 작업(POST — sync/push/pull/chat 등) 타임아웃 30s→**180s** (읽기 GET 은 30s 유지). AbortError 는 "응답 대기 시간 초과 — git 작업 진행 중일 수 있음" 으로 미기동과 구분. 웹이 abort 해도 런처 git 작업은 계속 진행됨(실제로는 성공했을 수 있음)을 안내.
  2. **런처 v0.1.15 하드닝**: ① Origin 비교 대소문자 무관(EqualFold — 브라우저는 Origin 호스트를 항상 소문자로 보내므로, 설정에 대문자 호스트가 있으면 curl 은 통과/브라우저만 403 나는 함정) ② Chrome PNA preflight (`Access-Control-Request-Private-Network` → `Access-Control-Allow-Private-Network: true`) — HTTPS 페이지→127.0.0.1 차단 대응.
- **구현**: `GoJIRA-App·Platform-App/src/utils/launcher.js`, `gojira-launcher/security.go`. PR #66.

## D7. diff 알림 = DB 플래그(branchAlerts) 기반으로 전환 (라이브 스캔 폐지)

- **배경**: `/Project/alerts` 가 프로젝트마다 main/user 트리를 라이브 비교(프로젝트당 Gitea 왕복 2회 × N 개 직렬) → 프로젝트 많으면 수 초~15s 타임아웃, Project Map 에 마크가 한참 뒤 일괄 표시. 핸들러 주석은 원래 "DB 플래그 기반" 의도였으나 구현이 어긋나 있었음. 프로젝트 문서에 `branchAlerts`(diff 있는 사용자 목록) 필드가 이미 존재.
- **결정**: 변화 시점에 기록하고 조회는 플래그만 읽는다.
  - **갱신 경로 완성**: 웹훅 handlePushEvent 가 main push 만 처리하고 docs/* push 를 무시하던 공백 수리 — docs/{user} push 시 해당 사용자만 `UpdateBranchAlert`(dead code 였음, 배선 + 결과 반환하도록 수정) 후 SSE 방송. main push 는 기존대로 전체 브랜치 재계산.
  - **조회**: GoJIRA-API 는 `branchAlerts` 배열에 사용자 포함 여부만 확인. 라이브 비교(checkTreeDiff) 삭제.
  - **self-healing**: 플래그가 계산된 적 없는(필드 미존재) 프로젝트는 `POST /DocRepo/{code}/alerts/recompute`(신설)를 비동기 fire-and-forget 호출 — 응답은 즉시, 계산 완료 시 SSE 로 반영.
  - **판정 기준 통일**: 전부 ancestor 체크(`HasUserCommits` — 내 브랜치에 main 에 없는 커밋이 있는가). 기존 트리 SHA 비교는 "단순히 main 보다 뒤처진" 브랜치도 diff 로 표시하는 문제가 있었음 (초기로드 vs SSE 기준 불일치도 해소).
  - **정규화**: nil 배열은 빈 배열로 저장 — Mongo null 이 "미계산(필드 없음)"과 구분 안 되어 self-heal 이 무한 반복되는 것 방지.
- **효과**: 응답 수 초 → **3ms** (실측). 
- **제약**: Gitea 웹훅 등록이 전제 (`PLATFORM_WEBHOOK_URL` — repo 생성 시 자동 등록). 그 이전에 만들어진 레포는 웹훅이 없을 수 있어 사내 Gitea 에서 기존 레포 웹훅 확인 필요.
- **구현**: `Platform-API/handlers/webhook.go`(docs/* 분기, UpdateBranchAlert 반환, recompute 핸들러), `main.go` 라우트, `GoJIRA-API/handlers/project_alerts.go`(플래그 읽기 + self-heal).

## D8. 파일 뱃지(DocEditor Local 탭)는 매번 계산 유지 + SSE push 필터

- **배경**: D7 이후 "파일 상태도 매번 확인하나?" 검토.
- **결정**: 파일 뱃지(refreshDocState)는 **매번 계산이 맞는 설계** — 단일 프로젝트 + 대부분 로컬 작업(Gitea 1회 + 런처 /files 로컬 git)이라 저렴하고, 미커밋(-) 등 working tree 실시간 상태는 캐시하면 낡은 정보가 됨. 플래그화 대상 아님.
- **부수 수정**: SSE `push` 이벤트를 레포 구분 없이 받아 남의 프로젝트 push 마다 Gitea 왕복이 생기던 낭비 — 현재 열린 프로젝트의 레포(`documents.repoName`)일 때만 재조회하도록 필터.
- **참고 — 뱃지 체계**: `[부호][문자]`. 문자 = main 과의 차이(A 추가/M 수정/D 삭제, SHA 3종 비교: main/working tree/HEAD). 부호 = `+` 커밋됨(발행 시 PR 포함) / `-` 미커밋(등록 필요). 회색 이탤릭 = main 에만 있는 파일(Pull 필요), 제목 옆 빨간 ⚠ = main-only 파일 존재. 파일 뱃지는 파일 단위·로컬 기준, Project Map 마크는 프로젝트 단위·원격(branchAlerts) 기준 — `-` 상태는 push 전이라 Map 에 안 뜸.

## D9. 기타 확인 사항 (결정 아님, 구조 기록)

- **PR 메일 발신 구조**: PR 생성 알림(notifyPRCreated)은 PR 생성 API 성공 직후 호출(웹훅 아님), 머지가능 알림(notifyMergeable)은 `pull_request_approved` 웹훅 경유 — 대기 리뷰어 0 이 되면 생성자에게. 반려는 메일이 아니라 DB 알림(SaveNotification). SMTP 미설정 시 조용히 생략 → D2 로 모듈화.
- **LLM 설정 구조**: llm.json 활성 필드 = 사외 OpenAI(gpt-4o-mini, bearer). 사내 설정(apigw gpt-oss-120b, X-Dep-Ticket 헤더 인증)은 `_disabled_Internal` unknown 키에 데이터로 보존 — 전환은 활성 필드 수동 교체(deploy 는 llm.json ro bind mount 라 파일 교체만으로 적용). 개선 여지: NOTIFY_PROVIDER 처럼 `LLM_PROVIDER` env 스위치화.
- **발행 PR 제목은 LLM 아님**: 웹 발행(handleLocalPublish)은 고정 문자열 `[코드] 설계문서 배포 요청`. LLM 요약은 DocGen 전용이었고 D4 로 함께 제거됨. 발행 PR 에 LLM 요약 제목/본문을 붙이는 것은 향후 선택지.
