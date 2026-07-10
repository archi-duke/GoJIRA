# GoJIRA 프로젝트 클론 — 에이전트 세션 메모리

<!--
이 파일은 gojira-launcher 가 각 프로젝트 clone 에 배포하는 프로젝트 메모리다.
원본: gojira-launcher/assets-extra/CLAUDE.md
배포 체인: build-launcher.sh 가 assets/.claude/CLAUDE.md 로 복사 → exe 임베드 →
런처가 ~/.gojira/skills/.claude/CLAUDE.md 로 추출 → ensureAgentAssets 가 clone 의
.claude 로 연결(junction/symlink). GoJIRA 개발 레포 자체에는 적용되지 않는다.
-->

## 환경변수 — 어디서 뭘 확인할지

API 호출이나 이슈/문서 작업이 필요한 세션이면, **다른 걸 하기 전에 먼저 아래 한 줄로
관련 변수를 한 번에 확인하라** — 하나씩 따로 찔러보며 헤매지 말 것:

```bash
env | grep -E '^(GOJIRA_API|GOJIRA_API_URL|PLATFORM_API|GOJIRA_PLATFORM_URL|GOJIRA_USER|GOJIRA_REPO_ROOT|GOJIRA_SESSION_TOKEN|GOJIRA_ISSUE_JOURNAL)='
```

launcher 가 세션 spawn 시 주입하는 값은 아래가 전부다. **이름이 비슷하다고 다른 변수로
대체하지 말 것** (특히 `USER`/`CODE`는 아래처럼 흔히 오독된다):

| 변수 | 의미 | 주의 |
|---|---|---|
| `GOJIRA_REPO_ROOT` | 이 클론의 로컬 경로 | 마지막 디렉토리명이 곧 프로젝트 코드(`CODE`) — 채팅에 코드가 없으면 `basename "$GOJIRA_REPO_ROOT"`로 유추 |
| `GOJIRA_USER` | 실제 GoJIRA 사용자(knox ID) | **맨 `$USER`를 쓰지 말 것** — OS 로그인 계정명이라 knox ID와 다를 수 있다. X-User-Id 등엔 반드시 `GOJIRA_USER` 사용 |
| `GOJIRA_API` (= `GOJIRA_API_URL`) | gojira-api 베이스 URL | 비어있으면 launcher.json 의 `gateway`/`backend.gojiraApi` 미설정 — 코드 문제 아님, 사용자에게 launcher 설정 확인 요청 |
| `PLATFORM_API` (= `GOJIRA_PLATFORM_URL`) | platform-api 베이스 URL | 위와 동일 |
| `GOJIRA_SESSION_TOKEN` | SSO 세션 토큰(Bearer) | dev 모드 등에서 비어있을 수 있음 — 그 경우 `GOJIRA_USER`(X-User-Id) 폴백이 정상 |
| `GOJIRA_ISSUE_JOURNAL` | 웹발 이슈 변경 저널 경로 | 프로젝트 없는 세션이면 미주입 — 아래 섹션 참조 |

**`CODE`(프로젝트 코드)는 launcher가 주입하는 환경변수가 아니다.** 이걸 env에서 찾으려
하지 말 것 — 사용자 채팅 메시지에서 추출하거나(스킬 SKILL.md 의 "인자 추출" 절 참조),
메시지에 명시가 없으면 `basename "$GOJIRA_REPO_ROOT"`로 유추한 뒤 스스로
`export CODE=...` 한다.

**`PROJECT_ROOT`도 launcher가 주입하는 환경변수가 아니다.** 모든 SKILL.md 스크립트가
`${PROJECT_ROOT}/.claude/skills/_scripts/...` 형태로 이 값을 참조하지만, 어디서도 대신
export해주지 않는다 — 스킬을 실행하기 전에 **반드시 스스로** 아래처럼 먼저 export할 것:

```bash
export PROJECT_ROOT="${GOJIRA_REPO_ROOT:-$(git rev-parse --show-toplevel 2>/dev/null)}"
```

이 클론의 `.claude`는 launcher가 스킬 배포본으로 연결(junction/symlink)해둔 것이라
`PROJECT_ROOT`는 곧 `GOJIRA_REPO_ROOT`와 같다. 예전 개발 환경의 고정 경로
(`/home/parallels/works/GoJIRA` 등)를 기본값으로 쓰지 말 것 — 사내망에는 존재하지 않는다.

## 🚨 이슈 조회 — `issue-query.sh` 사용, curl 직접 호출 금지

이슈(VOS/Feature/Requirement/ADSN 등) 조회가 필요하면 **반드시 아래 스크립트를 실행하라.
`${GOJIRA_API}` 로 curl 을 직접 만들거나 REST 경로를 추측하지 말 것**
(`/projects/{code}/issues`, `/Issue?...` 등 — 전부 존재하지 않는 경로다):

```bash
bash "${PROJECT_ROOT}/.claude/skills/_scripts/issue-query.sh"
```

이유 두 가지:
1. 이슈 쓰기(POST/PUT /Issue)는 Mongo 를 건드리지 않고 로컬 clone 에만 기록되고, Mongo 는
   PR 머지 시점에만 갱신된다 — 세션 중 GoJIRA-API 를 직접 조회하면 방금 만든 이슈가 안 보인다.
2. GoJIRA-API 의 실제 이슈 조회 엔드포인트는 `GET /Issue` + `XBody` 헤더(JSON 필터) 방식이라
   일반적인 REST 관례(`/projects/{code}/issues` 등)와 다르다 — 추측하면 100% 404 다.

문서/프로젝트/PR/커밋이력 조회도 마찬가지로 각 스킬의 `_scripts/*.sh` 헬퍼(`doc-list.sh`,
`doc-read.sh`, `project-status.sh`, `project-query.sh`, `pr-list.sh`, `doc-history.sh` 등)를
통해서만 하고, URL 을 직접 조립하지 말 것.

## 웹발 이슈 변경 저널

세션 시작 시 env `GOJIRA_ISSUE_JOURNAL` 이 가리키는 파일이 **존재하고 비어있지 않으면**,
웹에서 발생한 이슈 변경(생성/수정/삭제/링크)이 설계 일관성 검토를 기다리고 있다.

- `.claude/skills/_shared/issue-journal.md` 의 지침대로 항목별 영향(SATAM 매트릭스,
  유스케이스, 설계 문서, 링크 정합)을 검토·반영하라.
- 처리 완료한 항목은 저널 파일에서 제거하고, 보류 항목은 남긴 채 사용자에게 보고하라.
- 파일이 없거나 비어 있으면 해당 없음 — 사용자 요청 작업을 바로 진행한다.
