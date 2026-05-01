# 시스템 방향 전환 — Web 조회 + VS Code 편집 아키텍처

> 기록 일자: 2026-04-21
> 상태: 방향 합의 완료, **실행 보류 중** (2026-05-01 갱신)
>
> 보류 사유: 웹 측에서 해결 가능한 사용성 개선 (pumlex 인라인 PlantUML 편집,
> 다이어그램 ✓ 적용 시 자동 저장 등) 을 우선 진행 중. VS Code 로의 편집
> 이전은 웹 측 복잡도가 다시 누적될 때 재검토. 자세한 내용은
> `design-application.md` §3.7.1 참조.

## 배경

현재 GoJIRA 는 웹에 편집(DocEditor) + 실시간 에이전트 채팅(AgentChat) + SSE 스트리밍을 모두 탑재해 운영 중이다. 채팅 스트림 안정화, `docs/{user}` 브랜치 관리, Agent-Service ↔ OpenCode 브리지 등 **웹 + 서버 브리지에서만 해결해야 할 복잡도가 지속적으로 누적**되고 있다.

엔지니어(타겟 편집자) 는 본래 VS Code 환경에 익숙하고, OpenCode 도 공식 VS Code 확장을 제공한다. 편집 작업을 VS Code 로 옮기고 웹은 조회/대시보드로 한정하면 **복잡도를 구조적으로 줄일 수 있다**고 판단했다.

## 최종 아키텍처

```
   ┌──────────────────────────┐         ┌──────────────────────────┐
   │  Web (read-only)         │         │  VS Code + 확장           │
   │  - 이슈/문서 조회         │         │  - OpenCode 확장 (채팅)   │
   │  - 프로젝트 생성          │         │  - Diff / RelationMap /   │
   │  - 대시보드               │         │    SATAM (편집)          │
   │  - Diff / RelationMap /  │         │  - 프로젝트 clone/open   │
   │    SATAM (읽기)          │         │  - 스킬(.claude/skills)  │
   │  - SSO 로그인             │         │    로컬 OpenCode 가 실행 │
   └─────────┬────────────────┘         └──────────┬───────────────┘
             │                                      │
             ▼                                      ▼
   ┌──────────────────────────────────────────────────────────────┐
   │  Platform-API (축소): main read + 프로젝트 init + SSE 브로드  │
   │  JIRA-API: 이슈 CRUD                                          │
   └──────────────────────────────────────────────────────────────┘

   ※ Agent-Service 폐기, docs/{user} 브랜치 및 관련 write API 삭제
```

## 핵심 결정 사항

### 1. 역할 분리
- **Web**: PR 이 머지된 main 내용만 조회. 편집 기능 없음. 프로젝트 생성 / 대시보드 / 코멘트(이슈에 대한) 기능 유지.
- **VS Code 확장**: 편집의 유일한 진입점. 로컬 clone + 로컬 OpenCode 실행.

### 2. Agent-Service 폐기
- OpenCode 가 사용자 PC 에서 로컬 실행되므로 브리지 역할 소멸.
- 워크플로우 stage 는 repo 내용에서 계산 (`project-status.sh` 방식 유지) — 별도 state 저장소 불필요.
- 채팅 이력은 OpenCode 로컬 세션만 사용, 중앙 저장소 불필요.
- LLM 키는 사용자 PC 로컬에서 관리 (사내 프록시 적용 여부는 별도 정책).
- 결과: Agent-Service 프로세스/코드/배포 전부 제거.

### 3. Platform-API 축소
유지:
- main 브랜치 read (tree, file)
- 프로젝트 생성 / 레포 init (웹에서 트리거)
- main push 이벤트 SSE 브로드캐스트 (웹 실시간 갱신용)

삭제:
- `docs/{user}` 브랜치 관련 모든 엔드포인트 (tree, file read/write, commit)
- worktree 관리 / EnsureWorktree
- doc-save 계열 write API
- init-files 등 사용자 브랜치 초기화 API

### 4. 사용자 인증
- 사내 SSO 연동.
- 웹: 기존 SSO 플로우.
- VS Code 확장: SSO 로그인 → `secrets` 저장 → API 호출 시 헤더 주입.

### 5. 공유 컴포넌트 (RelationMap / SATAM)
- 웹에서는 읽기, VS Code 에서는 편집 모드로 동일 컴포넌트 사용.
- 배포 방식: **초기엔 NPM 패키지**로 양쪽 번들. 안정화 후 Module Federation 검토.
- 컴포넌트 계약:
  - props: `{ mode: 'view' | 'edit', host: { save, load, openFile } }`
  - 저장/로드 I/O 는 호스트 주입 callback 으로 분리 (web 은 Platform-API 호출, VS Code 는 로컬 git).
  - 테마: CSS 변수로 추상화해 VS Code dark/light 및 웹 스타일 모두 대응.

### 6. 비개발자 Stakeholder 지원
가장 큰 약점으로 인식. 보완 전략:
- 웹에 이슈 코멘트 기능 유지 (JIRA 스타일 — read + comment 까지 허용).
- 필요 시 주요 산출물 PDF/HTML 스냅샷 export 버튼 추가.
- 편집 권한 없이 피드백은 Slack/Email → 엔지니어가 반영.

### 7. 감사 로그
- 스킬의 쓰기 호출은 모두 JIRA-API / Platform-API 를 경유하므로, 이들의 기존 access log 가 감사 추적으로 충분하다고 판단.
- 별도 텔레메트리 수집 불필요.

### 8. 마이그레이션
- 기존 `docs/{user}` 브랜치 전부 삭제 (데이터 캐리 없음).
- 기존 프로젝트는 main 브랜치 내용만 유지.

### 9. VS Code 확장 배포
- 사내 private extension registry 또는 수동 `.vsix` 배포.
- 자동 업데이트 채널은 registry 채택 후 결정.

### 10. "프로젝트 열기" 흐름
- 웹 프로젝트 카드에 "VS Code 에서 열기" 버튼 → `vscode://gojira.ext/open?code=...` 딥링크 → 확장이:
  - 로컬 clone 이 있으면 workspace 오픈
  - 없으면 지정 경로(예: `~/GoJIRA-projects/{code}`)에 clone 후 오픈

## 구현 전 남은 세부 설계 항목

1. **VS Code 확장 기능 스코프 확정**
   - OpenCode 확장 채팅 UI 그대로 사용 + GoJIRA 전용 panel 3개(Diff / RelationMap / SATAM)
   - 이슈 CRUD UI 를 VS Code 에 만들지, 계속 웹에서만 수행할지

2. **SSO 토큰 주입 경로**
   - VS Code 확장이 획득한 토큰을 OpenCode 가 실행하는 스킬의 curl 호출에 어떻게 전달할지
   - 환경변수 / 로컬 프록시 / 헤더 주입 레이어 중 선택

3. **Shared 모듈의 호스트 주입 인터페이스**
   - save/load/openFile 시그니처 확정
   - 테마 변수 네이밍 합의

4. **제거 스펙 (파일/엔드포인트 레벨)**
   - Agent-Service 디렉토리 통째 삭제
   - Platform-API 에서 삭제할 핸들러 목록
   - GoJIRA-App 에서 삭제할 컴포넌트 목록(DocEditor, AgentChat, 관련 라우팅)
   - 워크플로우 스킬의 `${AGENT_API}` 참조 제거 지점

## 권장 MVP 단계

### Phase 1 — 웹 축소 + Agent-Service 폐기
- DocEditor / AgentChat 제거
- `docs/{user}` 브랜치 관련 API 삭제
- Agent-Service 폐기
- 웹은 조회/대시보드/프로젝트 생성만 남김

### Phase 2 — VS Code 확장 MVP
- OpenCode 공식 확장 설치 가이드
- GoJIRA 확장: 프로젝트 clone/open 명령, 간단한 Diff 뷰
- SSO 토큰 주입 파이프라인

### Phase 3 — 편집 가능한 RelationMap / SATAM
- 공유 컴포넌트 NPM 패키지화
- VS Code webview 에 마운트
- 웹은 동일 컴포넌트를 view 모드로 재사용

## 상대적 강점 정리

- 구조적 복잡도 대폭 감소: SSE 스트리밍, 워크플로우 state, `docs/{user}` 브랜치 관리, OpenCode 브리지 전부 제거.
- 편집자 UX 개선: IDE 의 Git / LSP / 터미널을 그대로 활용.
- LLM 정책 대응: 로컬 실행으로 키 관리 / 프록시 정책 유연화.
- 공유 컴포넌트 재사용: 읽기/편집 모드만 스위치해서 양쪽 호스트 지원.

## 상대적 약점 / 리스크

- 비엔지니어 stakeholder 는 편집 참여 불가 → 코멘트/export 로 보완해야 함.
- VS Code 로 IDE 고정 → JetBrains 등 다른 IDE 사용자 배제.
- VS Code 확장 배포/버전관리 인프라 구축 필요.
- 공유 컴포넌트 테마/CSP 대응 비용.
- OpenCode 공식 확장의 성숙도에 의존.

## 기록 메모

- 당일 논의: 2026-04-21, `skill_v2` 브랜치 진행 중 발생한 SSE 스트림 안정화 / OpenCode delta 이벤트 처리 작업을 거치며 구조적 한계를 재검토.
- Agent-Service 의 `opencode_bridge` 코드는 `76dcbbb` 기준 기능적으로 작동하지만, 이 전환 이후 Phase 1 단계에서 폐기 예정.
