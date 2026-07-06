# GoJIRA 사용자 가이드 (알파)

> 파트내 개발자 대상 알파 오픈용 간략 가이드입니다.
> GoJIRA 는 **웹(조회·관리) + 로컬 런처(편집·에이전트)** 조합으로 동작합니다 —
> 이슈/문서의 실제 생성·편집은 내 PC 의 에이전트가 수행하고, 웹은 그 결과를 보는 화면입니다.

---

## 1. 초기 접속

1. 브라우저로 안내받은 주소(게이트웨이)에 접속합니다 — 예: `http://<서버주소>:44300/`
2. 알파 환경은 SSO 없이 접속 즉시 지정된 계정으로 로그인됩니다.
   - 우상단에 내 계정이 표시됩니다. 관리자 권한이면 드롭다운으로 **사용자 전환**(개발용)이 가능합니다.
3. 상단 탭: **Project**(기본) / **Account** / **Notice** / **Repository** — Account·Notice·Repository 는 권한이 있어야 보입니다.

---

## 2. 로컬 런처 설치 (필수 — 최초 1회)

과제 생성, 문서 편집, 에이전트 실행 모두 **로컬 런처(gojira-launcher)** 가 필요합니다.
런처는 내 PC 의 127.0.0.1 경량 데몬으로, 웹의 요청을 받아 VS Code 와 에이전트 CLI 를 띄워줍니다.

**설치 안내 여는 법**: 문서 화면(Docs)의 **편집** 버튼이나 신규 과제 저장 시 런처가 없으면
"로컬 런처 설치" 안내 모달이 자동으로 뜹니다. 절차(Windows 기준):

1. 모달에서 **⬇ Windows 런처 다운로드** 클릭 → `gojira-launcher.exe` 를 적당한 폴더에 둡니다.
2. 명령 프롬프트에서 1회 실행:
   ```
   gojira-launcher.exe install --gateway <접속 주소>
   ```
   (모달의 명령에는 현재 접속 주소가 자동으로 채워져 있습니다 — 그대로 복사 실행)
   → 스킬 배포 + 설정파일 생성 + 로그인 시 자동시작 등록 + 데몬 기동까지 자동.
3. `~/.gojira/launcher.json` 의 `git.user` / `git.token` 을 채우고 런처를 재시작합니다.
   (문서 push 에 필요한 Git 계정 — 양식은 관리자에게 요청)
4. 이후 런처는 기동 시마다 자동 업데이트됩니다. 모달의 **설치 완료 — 다시 시도** 로 감지를 확인하세요.

> macOS/Linux 는 모달의 OS 탭에서 절차 확인 (`chmod +x` 후 동일한 `install --gateway`).
> **런처가 감지 안 될 때 1순위 점검**: 예전에 받아둔 구버전 exe 가 떠 있지 않은지 (작업관리자에서 종료 후 재기동).

---

## 3. 과제 생성

1. **Project 탭**(Project Map) 우상단 **`+ Add`** 클릭.
2. "과제 편집" 다이얼로그 입력:
   - **기본 정보**: 과제명, JIRA Code, 상태, Feature, 개발팀/개발그룹
   - **담당자**: Architect, PL, Members, Auditors (이름 또는 ID 입력 후 Enter)
   - **마일스톤**: 기본 `PA / EA / ER1 / ER2 / CA` — 원을 클릭해 날짜 지정
   - **설계문서 › Doc Repo 이름*** (필수): 영문/숫자/하이픈 (예: `my-project`) — 입력을 벗어나면 중복 검사
3. **저장** → 설계문서 저장소(Git repo)가 자동 생성됩니다 (수 초 소요).
   - 신규 과제 저장은 **런처가 동작 중이어야** 진행됩니다. 저장 직후 내 PC 에 로컬 clone 이 백그라운드로 준비됩니다.

과제 **편집/삭제/상세**는 Project Map 에서 과제를 **우클릭** → `상세정보 / 편집 / 삭제 / Relation Map / SATAM`.
(삭제는 과제명을 정확히 입력해야 하며, 백업 zip 생성 후 이슈/문서/repo 가 모두 정리됩니다.)

---

## 4. 설계 진행 방법 (설계 프로세스)

설계는 **에이전트**와 함께 진행합니다. 화면 좌상단 **GoJIRA 로고를 클릭**하면 현재 과제의
로컬 clone 에서 에이전트 터미널이 새 창으로 열립니다. (같은 과제 에이전트가 이미 있으면 그 창을 재사용)

에이전트에게 아래 순서대로 요청하면, 각 단계가 이슈와 설계 문서를 자동 생성/갱신합니다.
자연어로 요청해도 되고(예: "요구사항 도출해줘"), 단계별 스킬명을 직접 말해도 됩니다.

| 단계 | 요청 예시 (스킬) | 산출물 |
|---|---|---|
| ① 과제 설명 | "과제 설명 인터뷰 하자" (`projdesc-interview`) | `business/project-description.md` — 대화식 인터뷰로 작성 |
| ② VOS 도출 | "VOS 도출해줘" (`vos-collect2`) | 이해관계자 요구(VOS) 이슈 |
| ③ Use Case/Feature | "Use Case 분석해줘" (`usecase-analyze2`) | US/UC/Feature 이슈 + `business/use-cases.md` |
| ④ 요구사항 정제 | "요구사항 도출해줘" (`requirement-refine2`) | FR/NFR/CNST 이슈 + `business/requirements.md` |
| ⑤ 아키텍처 드라이버 | "드라이버 선정해줘" (`driver-select2`) | ASR 선정 + `architecture/drivers.md` |
| ⑥ 스타일 선정 | "아키텍처 스타일 정하자" (`style-define2`) | 스타일 조합 후보 비교·선정(ADSN) + `architecture/architecture-style.md` |
| ⑦ 설계 결정 | "설계 결정 진행해줘" (`design-decide2`) | Design Point 별 후보안 비교·선정 + `architecture/design-decisions/` |
| ⑧ SATAM 분석 | "SATAM 분석해줘" (`satam-analyze2`) | 설계×요구사항 Risk/Sensitivity/Mitigation 판정 |
| ⑨ 통합 문서 | "설계 통합 문서 만들어줘" (`design-integrate2`) | 4-view(논리/프로세스/배포/개발) `architecture/architecture-integrated.md` |

각 단계 후 웹에서 결과를 확인하세요 — 이슈는 **Issues**, 연결 구조는 **Relation Map**, 분석은 **SATAM**, 문서는 **Docs**.

### 문서 등록과 발행 (Docs 화면)

에이전트/편집으로 만든 문서는 내 로컬(`docs/<내ID>` 브랜치)에 있습니다. 팀에 공유하려면:

1. **Docs** 탭 → 좌측 **Local** 탭에서 변경 파일 확인 (`-M/-A/-D` = 아직 커밋 전, `+M/+A/+D` = 커밋됨)
2. **`등록`** 클릭 = 커밋 + 서버 push
3. push 가 끝나면 버튼이 **`발행`** 으로 바뀜 = PR 생성 → Auditor 리뷰 후 main 반영
4. main 이 앞서 있으면(파란 `M` 배지 / PR 반려 시) **Main 탭의 `Pull`** 로 최신을 받아 수정 후 재등록
   - ⚠️ Pull 은 내 로컬 docs 브랜치를 main 으로 **완전히 덮어씁니다** — 미등록 변경이 있으면 먼저 `등록` 하세요.

웹에서 직접 고치고 싶으면 파일 열고 **편집** → **저장**(로컬 저장) 또는 **VS Code** 버튼으로 IDE 에서 계속.

---

## 5. 화면 사용법

### 5.1 Project Map (기본 화면)

과제 타임라인(간트) 뷰입니다.

- **조회기간 슬라이더**: 좌우 손잡이를 끌어 기간 조절 (Shift = 한쪽만 이동)
- **정렬**: `시작일 / 종료일 / 과제명 / 부서명` — 다시 누르면 오름/내림 토글
- 행의 원 = 마일스톤(원 안 숫자 = 이전 마일스톤과의 일수), 빨간 점선 = 오늘
- **과제명 클릭** → 이슈 목록(Issues) 진입 / **우클릭** → 메뉴(상세·편집·삭제·Relation Map·SATAM)
- **알림 아이콘**: 🟠 앰버 = 내 문서 브랜치가 main 과 다름(확인 필요) / 🟢 녹색 = 열린 PR 있음

### 5.2 Issues

과제의 이슈 목록(조회 전용). 상단 `V / F / R / A / D` 토글 = VOS / Feature / Requirement /
Architectural Design / Detailed Design 타입 필터. 행 클릭 → 이슈 상세. 이슈의 생성/수정은 에이전트로.

### 5.3 Relation Map

설계 추적성 뷰 — 이슈를 `VOS → Feature → Requirement → Design` 컬럼으로 놓고 연결 관계를 선으로 표시합니다.

- **카드 클릭** = 연결 체인 하이라이트(상·하위 전파, 나머지는 흐리게) / **Shift+클릭** = 하이라이트를 컬럼 상단으로 정렬
- 배지 클릭 = 이슈 상세, 빈 곳 클릭 = 해제
- 상단 토글: **`A`** = ASR 만 / **`R`** = Reserved 표시 / **`T`** = Design 대신 Test 컬럼
- 상태 색: ASR 미지정 FR = 연빨강, 결정 전 ADSN = 회색 — 프로세스 진행도가 색으로 드러납니다

### 5.4 SATAM

설계 위험 분석 매트릭스 — **행 = Requirement(FR/NFR/CNST), 열 = Design(ADSN/DDSN)**.

- 셀 마크: **R**(빨강)=Risk, **S**(주황)=Sensitivity, **M**(파랑)=Mitigation. Mitigation 으로 해소된 Risk 는 취소선
- 마크 클릭 = 상세 팝업(설명·Counter-Risk), 행/열 배지 클릭 = 이슈 상세
- 상단 토글 `A / F / S / D / R`: ASR 만 / FR 표시 / 결정된 설계만 / DDSN 표시 / Reserved 표시
- 하단에 범례(Effects List)·Counter Strategy 테이블·Concerns 목록

---

## 6. 자주 묻는 문제

| 증상 | 조치 |
|---|---|
| "로컬 런처가 감지되지 않았습니다" | §2 설치. 설치했는데도 안 되면 구버전 exe 프로세스 종료 후 재기동 |
| 로고 클릭했는데 에이전트 창이 안 뜸 | 런처 감지 확인 → 이미 열린 같은 과제 터미널 창이 있는지 확인(안내 문구의 창 제목 참조) |
| 문서 push 실패 | `~/.gojira/launcher.json` 의 `git.user`/`git.token` 확인 후 런처 재시작 |
| 과제에 🟠 아이콘이 계속 표시 | 내 docs 브랜치와 main 의 차이 — `등록`→`발행`으로 PR 하거나, 필요 없으면 Main 탭 `Pull` 로 동기화 |
| PR 이 반려됨 | 알림 안내대로 Main 탭 `Pull` → 수정 → 다시 `등록`/`발행` |
