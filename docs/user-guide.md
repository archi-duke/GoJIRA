# GoJIRA 사용자 가이드

> 파트내 개발자 대상 가이드입니다. 과제(프로젝트) 생성부터 이슈 등록, 에이전트를 이용한
> 설계 진행까지의 과정을 다룹니다. 계정 관리·공지 관리 등 관리자 기능은 별도 문서를 참고하세요.

---

## 1. 시작하기

### 1.1 GoJIRA 의 구조 — 웹 + 로컬 런처

GoJIRA 는 **웹(조회·관리) + 로컬 런처(편집·에이전트)** 조합으로 동작합니다.

```
[브라우저]  조회·확인                  [내 PC]  생성·편집
  플랫폼 셸 → GoJIRA 앱                로컬 런처(gojira-launcher)
    Project Map / Issues        ◀──▶     └ VS Code
    Relation Map / SATAM                 └ 에이전트 CLI (opencode | claude)
    Docs                                 └ 로컬 clone (문서·이슈 YAML)
```

핵심은 **이슈와 문서의 실제 생성·편집은 내 PC 의 에이전트가 수행하고, 웹은 그 결과를 보는
화면**이라는 점입니다. 그래서 런처 설치가 필수입니다(§2).

### 1.2 접속과 로그인

1. 브라우저로 안내받은 게이트웨이 주소에 접속합니다 — 예: `http://<서버주소>:44300/`
2. SSO 가 켜진 환경에서는 미인증 시 로그인 페이지로 자동 이동하며, 인증 후 돌아옵니다.
   우상단에 내 계정이 표시됩니다.
3. 접속하면 **플랫폼 셸(NETRIX)** 의 앱 목록이 먼저 나옵니다. **GoJIRA** 카드를 클릭해
   들어갑니다. (같은 셸에 RAGaaS 등 다른 앱이 함께 놓입니다)
4. 로그인 루프가 반복되면 브라우저 하드 리로드(Ctrl/Cmd+Shift+R)로 캐시를 비우세요.

### 1.3 화면 구성 한눈에

화면은 2단 구조입니다.

**플랫폼 셸** — 접속 직후 화면. 앱 카드(GoJIRA / RAGaaS …)와 Account / Notice 탭,
우상단 계정 표시가 있습니다.

**GoJIRA 앱** — 상단 탭 **Project**(기본) / **Account** / **Notice** / **Repository**.
Account·Notice·Repository 는 권한이 있는 계정에만 보입니다.

Project Map 에서 과제명을 클릭해 과제를 열면, **화면 우측 상단에 과제 메뉴 탭**이 나타납니다
— `Docs` / `Issues` / `Relation Map` / `SATAM`. 그 오른쪽에는 화면별 필터 토글과
내보내기 버튼이 놓입니다.

| 화면 | 용도 | 본문 |
|---|---|---|
| Project Map | 과제 타임라인(간트), 과제 생성·편집 | §3, §7.1 |
| Issues | 이슈 목록·상세 조회 | §7.2 |
| Relation Map | 이슈 연결(추적성) 뷰 | §7.3 |
| SATAM | 설계×요구사항 위험 분석 매트릭스 | §7.4 |
| Docs | 설계 문서 열람·편집·공유 | §7.5, §8 |

---

## 2. 로컬 런처 설치 (최초 1회, 필수)

과제 생성, 이슈 등록, 문서 편집, 에이전트 실행 모두 **로컬 런처**가 필요합니다.
런처는 내 PC 의 `127.0.0.1:5599` 에서 도는 경량 데몬으로, 웹의 요청을 받아 VS Code 와
에이전트 CLI 를 띄우고 로컬 clone 을 관리합니다.

**설치 안내 여는 법**: Docs 화면의 **편집** 버튼을 누르거나 신규 과제를 저장할 때 런처가
없으면 "로컬 런처 설치" 안내 모달이 자동으로 뜹니다.

### 2.1 OS별 설치 절차

Windows 기준:

1. 모달에서 **⬇ Windows 런처 다운로드** → `gojira-launcher.exe` 를 적당한 폴더에 둡니다.
2. 명령 프롬프트에서 1회 실행:
   ```
   gojira-launcher.exe install --gateway <접속 주소>
   ```
   모달의 명령에는 현재 접속 주소가 자동으로 채워져 있으니 그대로 복사해 실행하면 됩니다.
   → 스킬 배포 + 설정파일 생성 + 로그인 시 자동시작 등록 + 데몬 기동까지 자동으로 끝납니다.

macOS/Linux 는 모달의 OS 탭에서 절차를 확인하세요 (`chmod +x` 후 동일하게 `install --gateway`).

### 2.2 Git 인증 설정 (문서 push 용)

문서를 서버에 올리려면 Git 계정이 필요합니다. 두 가지 방식이 있습니다.

- **사용자별 PAT(권장)**: 관리자가 계정에 등록해 두면 런처가 자동으로 조회해 사용합니다.
  추가 설정이 필요 없습니다.
- **직접 지정**: `~/.gojira/launcher.json` 의 `git.user` / `git.token` 을 채우고 런처를 재시작합니다.

> 런처 기동 로그에 `git auth=per-user (... 미설정 — 폴백 없음)` 경고가 보이면 PAT 조회에
> 의존하는 상태입니다. 조회가 실패하면 익명으로 떨어져 Git 인증 프롬프트가 뜰 수 있습니다.

### 2.3 설치 확인과 자동 업데이트

- 설치 확인: 모달의 **설치 완료 — 다시 시도** 를 누르면 감지 여부를 바로 확인할 수 있습니다.
- 직접 확인: 브라우저로 `http://127.0.0.1:5599/health` → `{"ready":true,"version":"0.1.38"}`
- **자동 업데이트**: 런처는 기동 시 1회 + 60분 주기로 서버 버전을 확인해, 새 버전이 있으면
  내려받아 교체하고 스스로 재시작합니다. 이때 스킬도 함께 최신으로 재추출됩니다.
  즉시 적용하려면 런처를 재시작하세요.

### 2.4 런처가 감지되지 않을 때

| 점검 | 방법 |
|---|---|
| 구버전 프로세스가 떠 있는지 | 작업관리자(Windows)/`ps`(mac) 에서 종료 후 재기동 — **1순위 점검** |
| 데몬이 살아있는지 | `http://127.0.0.1:5599/health` 응답 확인 |
| 게이트웨이 주소가 맞는지 | `~/.gojira/launcher.json` 의 `gateway` 확인 |
| 의존성 | 기동 로그의 `의존성: jq=? python=?` — 미충족이면 `install` 재실행 |

---

## 3. 과제 생성

### 3.1 과제 추가와 입력 항목

1. **Project 탭**(Project Map) 우상단 **`+ Add`** 클릭.
2. "과제 편집" 다이얼로그를 입력합니다:
   - **기본 정보**: 과제명, JIRA Code, 상태, Feature, 개발팀/개발그룹
   - **담당자**: Architect, PL, Members, Auditors (이름 또는 ID 입력 후 Enter)
   - **마일스톤**: 기본 `PA / EA / ER1 / ER2 / CA` — 원을 클릭해 날짜 지정
   - **설계문서 › Doc Repo 이름\*** (필수): 영문/숫자/하이픈 (예: `my-project`)
     입력란을 벗어나면 중복 검사가 돌아갑니다
3. **저장** → 설계문서 저장소(Git repo)가 자동 생성됩니다 (수 초 소요).

> **신규 과제 저장은 런처가 동작 중이어야 진행됩니다.** 저장 직후 내 PC 에 로컬 clone 이
> 백그라운드로 준비됩니다(prewarm).

### 3.2 Doc Repo 와 내 PC 로컬 clone

과제를 만들면 두 곳이 준비됩니다.

- **서버 Doc Repo**: 팀 공식본. `main` 브랜치가 공유 기준입니다.
- **내 PC 로컬 clone**: `~/GoJIRA-projects/<내ID>/<과제코드>/`
  내 작업 브랜치는 `docs/<내ID>` 이며, 에이전트는 이 작업본에서 동작합니다.

clone 안의 구조:

```
~/GoJIRA-projects/<내ID>/<과제코드>/
├── business/        project-description.md, use-cases.md, requirements.md
├── architecture/    drivers.md, architecture-style.md, design-decisions/, architecture-integrated.md
├── data/issues/     이슈 YAML (이슈의 원본)
└── .claude/         스킬 (런처가 연결 — 직접 수정하지 마세요)
```

사용자별로 경로가 분리되어 있어, 같은 PC 에서 계정을 바꿔도 작업본이 섞이지 않습니다.

### 3.3 과제 편집 · 상세 · 삭제

Project Map 에서 과제를 **우클릭** → `상세정보 / 편집 / 삭제 / Relation Map / SATAM`.

삭제는 과제명을 정확히 입력해야 실행되며, 서버에 백업 zip 이 만들어진 뒤 이슈·문서·repo 와
내 PC 로컬 clone 이 모두 정리됩니다.

---

## 4. 에이전트 시작하기

### 4.1 에이전트 터미널 열기

화면 좌상단 **GoJIRA 로고를 클릭**하면 현재 과제의 로컬 clone 에서 에이전트 터미널이
새 창으로 열립니다. 같은 과제의 에이전트가 이미 떠 있으면 그 창을 재사용하라는 안내가
표시됩니다(안내 문구의 창 제목을 확인하세요).

### 4.2 요청 방법 — 자연어 vs 스킬명

두 가지 방식이 있고, 스킬 종류에 따라 다릅니다.

- **설계 프로세스·조회 스킬(§6, §5.4)**: 자연어로 요청하면 에이전트가 알아서 해당 스킬을
  고릅니다. "요구사항 도출해줘" 처럼 말해도 되고, 스킬명을 직접 말해도 됩니다.
- **이슈 등록·수정 스킬(§5.2, §5.3)**: 자동 선택되지 않습니다(`disable-model-invocation`).
  **스킬명을 명시해서 호출**하세요 — 예: "`issue-create-vos` 로 VOS 하나 만들어줘".
  쓰기 작업이라 실수를 막기 위해 의도적으로 막아 둔 것입니다.

### 4.3 산출물이 저장되는 위치

에이전트가 만든 결과는 **문서든 이슈든 모두 내 PC 로컬 clone 에 먼저 기록**됩니다.
서버 공식본에 반영되려면 등록 → 발행 → 머지를 거쳐야 합니다.

| 산출물 | 저장 위치 | 공식본 반영 |
|---|---|---|
| 설계 문서 (`.md`) | 로컬 clone 의 `docs/<내ID>` 작업본 | **등록 → 발행 → 머지** 후 main 에 반영 (§8) |
| 이슈 | 로컬 clone 의 `data/issues/*.yaml` | 같음 — 머지되면 서버 캐시(Mongo)가 갱신됨 |

그래서 **에이전트로 만든 이슈는 `내작업` 뷰에서 먼저 보이고, `공식` 뷰에는 머지된 뒤에
나타납니다**(§7.2 의 뷰 전환 참고). 작업 중 "분명 만들었는데 화면에 없다" 싶으면
`공식` 뷰를 보고 있는 것은 아닌지 먼저 확인하세요.

> 로컬 clone 의 `data/issues/*.yaml` 이 이슈의 **원본**이고 서버 Mongo 는 캐시입니다.
> main 에 머지되면 그 YAML 기준으로 캐시가 재동기화됩니다.

---

## 5. 이슈 등록과 관리

### 5.1 이슈 타입 체계

| 이슈 타입 | subkey | 색 (Relation Map) | 주로 만들어지는 단계 |
|---|---|---|---|
| VOS (이해관계자 요구) | `VOS_NNN` | 초록 | ② VOS 도출 |
| Feature / Use Case | `UC_NNN` | 주황 | ③ Use Case 분석 |
| Requirement — 기능 | `FR_NNN` | 빨강 | ④ 요구사항 정제 |
| Requirement — 품질(NFR) | `QA_NNN` | 빨강 | ④ 요구사항 정제 |
| Requirement — 제약 | `CNST_NNN` | 빨강 | ④ 요구사항 정제 |
| Architectural Design | `ADSN_NNN` | 파랑 | ⑥ 스타일 선정 / ⑦ 설계 결정 |
| Detailed Design | `DDSN_NNN` | 보라 | 상세 설계 |

- 이슈 키는 `<과제코드>-NNN`, subkey 는 `<접두사>_NNN` 형식입니다.
- 상위→하위 연결은 **Contains** 링크로 잇습니다 (VOS → Feature → Requirement → Design).
  Relation Map 에 그려지는 선이 이 링크입니다.
- **ASR**(Architecturally Significant Requirement) = ⑤ 드라이버 선정에서 뽑힌 요구사항.
  Relation Map/SATAM 의 `A` 토글로 걸러 볼 수 있습니다.

### 5.2 이슈 직접 등록

설계 프로세스(§6)를 돌리면 이슈가 자동으로 만들어지지만, 개별 이슈를 직접 등록할 수도
있습니다. 타입별로 전용 스킬이 있습니다. **스킬명을 명시해 호출하세요**(§4.2).

| 스킬 | 만드는 이슈 | 핵심 입력 |
|---|---|---|
| `issue-create-vos` | VOS | 제목·상세(**사용자 관점 어투**), 요구를 제기한 이해관계자 ID |
| `issue-create-feature` | Feature | 제목·상세 + Use Case Story 6필드 |
| `issue-create-requirement` | Requirement | **reqType** `FR`/`NFR`/`CNST` (NFR 은 QA 시나리오) |
| `issue-create-design` | 설계 | **designLevel** `Architectural`(ADSN) / `Detailed`(DDSN) |

타입별로 채우는 항목:

- **VOS** — 가공하지 않은 이해관계자의 목소리를 그대로 담습니다. 요청 주체(이해관계자 ID)와
  사용자 관점 어투가 핵심입니다. 정제는 이후 Requirement 단계에서 합니다.
- **Feature** — Use Case Story: `actor` / `precondition` / `basicflow` / `alternativeflow` /
  `exceptionflow` / `postcondition`. 보통 여러 Requirement 를 Contains 로 묶습니다.
- **Requirement** — `FR`(기능) / `NFR`(품질속성) / `CNST`(제약)을 `reqType` 으로 구분합니다.
  NFR 은 품질속성 시나리오(`qa` / `source` / `stimulus` / `environment` / `artifact` /
  `response` / `measure` / `responseScenario`)와 평가 점수(`qai` / `qad` / `qas`)를 함께 기록합니다.
- **설계** — `Architectural` 이면 ADSN(아키텍처 설계 + 평가 + SATAM 필드), `Detailed` 면
  DDSN(상세 설계)으로 만들어집니다. 다이어그램·설계 모델 필드를 함께 채웁니다.

예시 요청:

```
issue-create-vos 로 VOS 등록해줘.
  제목: 로그인 상태를 오래 유지하면 좋겠어요
  상세: 자주 쓰는 기기에서 반복 로그인하지 않게 30일 유지가 필요해요
  요청자: alice.kim
```

값이 빠지면 에이전트가 되묻습니다. 또 **생성 전에 기존 이슈를 조회해 중복·유사 이슈를
확인**하도록 되어 있어, 비슷한 이슈가 있으면 새로 만들지 않고 재사용·수정을 권합니다.

### 5.3 수정 · 삭제 · 연결

| 스킬 | 용도 |
|---|---|
| `issue-update` | 기존 이슈 필드 수정 (status / summary / priority 등) |
| `issue-delete` | 이슈 삭제 — 로컬 YAML 삭제가 반영됩니다 (git 이력이 휴지통 역할) |
| `issue-link` | 이슈 간 관계 설정 — `Contains` / `Relates` / `Blocks` |

연결이 전반적으로 어긋났다면 하나씩 고치는 대신 **재정립(relink)** 을 쓰는 편이 빠릅니다(§6.3).

### 5.4 이슈 조회와 관계 분석

| 요청 예시 | 스킬 | 내용 |
|---|---|---|
| "이슈 목록 보여줘 (FR만)" | `issue-query` | 타입/서브타입 필터 조회 |
| "이슈 관계 분석해줘" | `issue-analyze` | Contains 체인, 고아 이슈, 타입 통계 |
| "일정 진행률 분석해줘" | `schedule-analyze` | 이슈 상태 기반 진행률·지연 리스크 |
| "과제 목록" | `project-query` | 과제 목록 조회 |

---

## 6. 설계 프로세스 — 에이전트 9단계

### 6.1 전체 흐름

```
① 과제 설명 ─▶ ② VOS ─▶ ③ Use Case/Feature ─▶ ④ 요구사항 ─▶ ⑤ 드라이버
                                                                   │
        ⑨ 통합 문서 ◀─ ⑧ SATAM ◀─ ⑦ 설계 결정 ◀─ ⑥ 스타일 선정 ◀─┘
```

앞 단계의 산출물이 다음 단계의 입력이 되므로 순서대로 진행하는 것을 권합니다.

### 6.2 단계별 요청과 산출물

| 단계 | 요청 예시 (스킬) | 산출물 |
|---|---|---|
| ① 과제 설명 | "과제 설명 인터뷰 하자" (`projdesc-interview`) | `business/project-description.md` — 대화식 인터뷰로 작성 |
| ② VOS 도출 | "VOS 도출해줘" (`vos-collect`) | 이해관계자 요구(VOS) 이슈 |
| ③ Use Case/Feature | "Use Case 분석해줘" (`usecase-analyze`) | Feature/UC 이슈 + VOS↔Feature 링크 + `business/use-cases.md` |
| ④ 요구사항 정제 | "요구사항 도출해줘" (`requirement-refine`) | FR/NFR/CNST 이슈 + Feature↔Requirement 링크 + `business/requirements.md` |
| ⑤ 아키텍처 드라이버 | "드라이버 선정해줘" (`driver-select`) | ASR 라벨 부여 + `architecture/drivers.md` |
| ⑥ 스타일 선정 | "아키텍처 스타일 정하자" (`style-define`) | 스타일 조합 후보 비교·선정(ADSN) + `architecture/architecture-style.md` |
| ⑦ 설계 결정 | "설계 결정 진행해줘" (`design-decide`) | Design Point 별 후보안 비교·선정 + `architecture/design-decisions/` |
| ⑧ SATAM 분석 | "SATAM 분석해줘" (`satam-analyze`) | 설계×요구사항 Risk/Sensitivity/Mitigation 판정 (ADSN 필드에 기록) |
| ⑨ 통합 문서 | "설계 통합 문서 만들어줘" (`design-integrate`) | 4-view(논리/프로세스/배포/개발) `architecture/architecture-integrated.md` |

각 단계가 끝나면 웹에서 결과를 확인하세요 — 이슈는 **Issues**, 연결 구조는 **Relation Map**,
위험 분석은 **SATAM**, 문서는 **Docs**(Local 탭).

① 은 인터뷰 방식입니다. 이미 내용이 정리돼 있다면 `projdesc-generate` 로 비대화식 작성도
가능합니다.

### 6.3 재정립(relink) — 이슈는 보존하고 연결만 다시

Feature↔VOS, Requirement↔Feature 연결이 어긋났을 때 이슈를 다시 만들 필요가 없습니다.

- "Feature-VOS 연결 재정리해줘" → `usecase-analyze` 가 relink 모드로 동작
- "요구사항 링크만 다시 정리해줘" → `requirement-refine` relink 모드

relink 는 **기존 이슈를 보존하고 링크와 문서만 재작성**합니다.

### 6.4 진행 중 알아둘 점

- **단계마다 시간이 걸립니다.** 설계 후반 단계(⑥~⑨)는 한 단계에 수 분~수십 분이 걸릴 수
  있습니다. 에이전트가 자가검증(self-loop)을 최대 3회 돌리기 때문입니다.
- **자가검증**: 각 스킬은 산출물이 기준을 만족하는지 스스로 점검하고 보완합니다
  (예: 모든 VOS 가 최소 1개 Feature 에 연결됐는지).
- **중간에 끊겼다면** 같은 단계를 다시 요청하면 됩니다. 이미 만들어진 이슈는 중복 생성되지
  않고 재사용·보완됩니다.
- **다이어그램은 PlantUML** 로 작성됩니다 (Use Case/배포/패키지 등). Docs 미리보기에서
  렌더되고, 그림 위에서 직접 배치를 조정할 수도 있습니다(§7.5).

---

## 7. 결과 확인 화면

### 7.1 Project Map

과제 타임라인(간트) 뷰입니다.

- **조회기간 슬라이더**: 좌우 손잡이를 끌어 기간 조절 (Shift = 한쪽만 이동)
- **정렬**: `시작일 / 종료일 / 과제명 / 부서명` — 다시 누르면 오름/내림 토글
- 행의 원 = 마일스톤(원 안 숫자 = 이전 마일스톤과의 일수), 빨간 점선 = 오늘
- **과제명 클릭** → Issues 진입 / **우클릭** → 메뉴(상세·편집·삭제·Relation Map·SATAM)
- **알림 아이콘**: 🟠 앰버 = 내 문서 브랜치가 main 과 다름 / 🟢 녹색 = 열린 PR 있음

### 7.2 Issues

과제의 이슈 목록입니다(생성·수정은 에이전트로, §5).

**`공식` / `내작업` 뷰 전환** — 상단 토글로 보는 대상을 바꿉니다. 이 전환은
**Issues · Relation Map · SATAM** 세 화면에 공통으로 있습니다.

| 뷰 | 보는 대상 | 특징 |
|---|---|---|
| **공식** | 서버 `main` 기준 공식본 (Mongo) | 팀이 공유하는 확정 내용. 편집 불가 |
| **내작업** | 내 PC 로컬 clone 의 작업본 (런처가 읽음) | 에이전트가 방금 만든 것이 여기 먼저 보임. 편집 가능 |

에이전트 작업 결과는 **`내작업`에 먼저** 나타나고, 등록 → 발행 → 머지를 거쳐야 `공식`에
반영됩니다(§4.3, §8). `내작업` 뷰는 런처가 동작 중이어야 보입니다.
`Pull` 버튼은 공식본을 서버 기준으로 다시 받아옵니다.

- 목록 컬럼: **Key / SubKey / Type / Summary / Status / Priority**.
  컬럼 헤더를 클릭해 정렬할 수 있습니다(`Key ▲`)
- 상단 `V / F / R / A / D` 토글 = VOS / Feature / Requirement / Architectural Design /
  Detailed Design 필터. 각각 켜고 끌 수 있으며, 목록 하단에 **총 N건**이 표시됩니다
- 오른쪽 끝 **⬇ 버튼** 으로 목록을 내려받을 수 있습니다
- **Key 또는 SubKey 링크를 클릭하면 이슈 상세**가 팝업으로 열립니다
  (행의 빈 곳을 눌러서는 열리지 않습니다)

상세 팝업은 타입에 따라 구성이 다릅니다:

| 타입 | 구성 |
|---|---|
| VOS | 탭 없이 한 화면 — Summary / Description / Status / Priority / Due Date / Reporter / **Client** |
| Feature | **Basic** + **Use Scenario** 탭 (Actor·Pre Condition·Basic/Alternative Flow·Post Condition) |
| NFR(QA) | Basic + **Quality Attribute Scenario** (Source·Stimulus·Environment·Artifact·Response·Measure) + **Priority Score** (QAD/QAI/QAS) |
| Architectural Design | Basic + **Architecture Design** (다이어그램·Pros/Cons·Score·Decision) + **Trade Off** (Risks·TradeOffs·Sensitivities) |
| Detailed Design | Basic + **Detailed Design** (다이어그램·Design Model) |

이슈 간 연결은 상세가 아니라 **Relation Map** 에서 봅니다.

### 7.3 Relation Map

설계 추적성 뷰 — 이슈를 `VOS → Feature → Requirement → Design` 컬럼으로 놓고 연결 관계를
선으로 표시합니다.

- **카드 클릭** = 연결 체인 하이라이트(상·하위 전파, 나머지는 흐리게)
- **Shift+클릭** = 하이라이트를 컬럼 상단으로 정렬
- 배지 클릭 = 이슈 상세, 빈 곳 클릭 = 해제
- 상단 토글: **`A`** = ASR 만 / **`R`** = Reserved 표시 / **`T`** = Design 대신 Test 컬럼
- 상태 색: ASR 미지정 FR = 연빨강, 결정 전 ADSN = 회색 — 프로세스 진행도가 색으로 드러납니다
- 토글 오른쪽의 **⬇ / ⤴ 버튼** 으로 내려받기·내보내기
- 상단에 **`공식` / `내작업`** 뷰 전환이 있습니다 (§7.2 와 동일)

### 7.4 SATAM

설계 위험 분석 매트릭스 — **행 = Requirement(FR/NFR/CNST), 열 = Design(ADSN/DDSN)**.

- 셀 마크: **R**(빨강)=Risk, **S**(주황)=Sensitivity, **M**(파랑)=Mitigation.
  Mitigation 으로 해소된 Risk 는 취소선으로 표시됩니다
- 마크 클릭 = 상세 팝업, 행/열 배지 클릭 = 이슈 상세
- 상단 토글 `A / F / S / D / R`: ASR 만 / FR 표시 / 결정된 설계만 / DDSN 표시 / Reserved 표시
- 상단 **Effects List** 가 범례입니다 — ● Contains / **S** Sensitivity / **R** Risk /
  **M** Mitigation
- 맨 아래에 `Design N건 × Requirement N건` 집계와 Counter Strategy 테이블·Concerns 목록
- 설계·요구사항이 아직 없으면 빈 매트릭스로 보입니다 (`Design 0건 × Requirement 0건`)
- 상단에 **`공식` / `내작업`** 뷰 전환이 있습니다 — SATAM 분석을 막 끝냈다면 결과는
  `내작업`에 먼저 보이고, 머지 후 `공식`에 반영됩니다 (§4.3)

### 7.5 Docs (설계 문서)

좌측 사이드바 = 파일 목록, 우측 = 문서 뷰어/편집기.

**Main 탭 vs Local 탭**

사이드바 맨 위에 `Main` / `Local` 탭과 **브랜치 그래프 아이콘**(Commit Graph)이 있고,
바로 아래 줄에 현재 브랜치와 동작 버튼이 놓입니다.

- **Main**: 서버의 `main` 브랜치 (팀 공식본, 조회 전용). 줄에 `main` + **`Pull`** 버튼.
- **Local**: 내 PC 로컬 clone 의 작업본. 줄에 `local · docs/<내ID>` + **`등록`**/**`발행`**
  버튼(상태에 따라 바뀝니다 — §8). 파일별 상태 배지:
  - `-M / -A / -D` = 수정/추가/삭제했지만 **아직 커밋 전**
  - `+M / +A / +D` = 커밋됨(등록됨 — PR 에 포함될 내용)
  - 회색 이탤릭 + `M` = main 에만 있는 파일 → Main 탭 `Pull` 로 동기화
  - 파일에 마우스를 올리면 **✎**(이름변경/이동) / **🗑**(삭제)

파일 트리에는 문서(`business/`, `architecture/`)뿐 아니라 **`data/issues/*.yaml`**
(이슈 원본)도 함께 보입니다. 이슈를 추가·수정하면 여기에 변경으로 잡히며, 문서와 같은
방식으로 등록·발행합니다.

**보기 모드** — 문서를 열면 상단에 파일 경로와 함께 **미리보기 / Raw 보기** 가 있고,
오른쪽 끝에 **편집** 버튼이 있습니다.

- 미리보기는 마크다운 + 다이어그램을 렌더합니다. ` ```plantuml `(또는 ` ```puml `) 블록이
  그림으로 표시되고, YAML 파일은 색상 뷰로 보입니다.
- **Diff 분석** 탭은 Local 에서 main 과 내용이 다른 파일일 때 추가로 나타납니다:
  *Semantic Diff*(섹션 단위 추가/수정/삭제 + 문단 하이라이트) / *Diff*(라인 diff) /
  *Summary*(자연어 요약) 3개 탭

**문서 편집**

- **편집** 버튼 → 인라인 편집 → **저장**(로컬 디스크에만 저장) / **VS Code**(저장 후 IDE 로
  열기) / **취소**
- **PlantUML 다이어그램은 그림 위에서 직접 편집**할 수 있습니다. 렌더된 다이어그램
  **우측 상단의 `✎ 편집`** 클릭 → 요소·라벨을 드래그로 배치 조정 → **✓ 적용**(문서의
  plantuml 블록이 자동 갱신되어 로컬 저장) / **✗ 취소**
  - `✎ 편집` 은 **Local 탭 + 런처 동작 중**일 때만 나타납니다. Main 탭에서는 보이지 않습니다.

---

## 8. 설계 문서 공유 — 등록 → 발행 → 리뷰 → 머지

로컬에서 작성한 문서를 팀 공식본(main)에 반영하는 흐름입니다.

```
[내 로컬 clone] ──등록(커밋+push)──▶ [서버 docs/<내ID>] ──발행──▶ [PR] ──승인/머지──▶ [main]
```

1. **등록** (Docs › Local 탭, 사이드바 상단 `local · docs/<내ID>` 줄의 버튼):
   미커밋(`-` 배지) 변경이 있으면 버튼이 `등록` — 클릭하면 커밋 + 서버 push
2. **발행**: push 된 내용이 main 과 다르면 같은 자리의 버튼이 `발행` 으로 바뀝니다 —
   클릭하면 PR 생성
3. **리뷰** (Auditor): 상단 **Repository 탭 › PRs** 에서
   - PR 행의 **승인**(의견 입력, 기본 "LGTM") 또는 **반려**(사유 입력)
   - 리뷰 상태: ✓ 승인자 / ✗ 반려자 / ◦ 대기중, `승인수/총수` 표시.
     행을 펼치면 본문·코멘트를 확인할 수 있습니다
   - **상세보기** 링크로 Git 서버 화면에서 리뷰해도 됩니다
4. **머지**: 모든 승인이 완료(반려·대기 0)되어야 머지 버튼이 활성화됩니다 → main 반영
5. **반려되면**: 작성자에게 팝업 알림이 옵니다. Main 탭 **`Pull`** 로 main 최신을 받아
   수정한 뒤 다시 등록·발행하세요.

> ⚠️ **`Pull` 은 내 로컬 docs 브랜치를 main 으로 완전히 덮어씁니다.** 보존할 변경이 있으면
> 반드시 먼저 `등록` 하세요.

---

## 9. 부록

### 9.1 스킬 목록

**설계 프로세스 (10종)** — 자연어 요청 가능

`projdesc-interview` · `projdesc-generate` · `vos-collect` · `usecase-analyze` ·
`requirement-refine` · `driver-select` · `style-define` · `design-decide` ·
`satam-analyze` · `design-integrate`

**이슈 쓰기 (7종)** — 스킬명 명시 호출 필요

`issue-create-vos` · `issue-create-feature` · `issue-create-requirement` ·
`issue-create-design` · `issue-update` · `issue-delete` · `issue-link`

**조회·분석 (10종)** — 자연어 요청 가능

`issue-query` · `issue-analyze` · `schedule-analyze` · `project-query` · `doc-list` ·
`doc-summarize` · `doc-diff` · `doc-history` · `pr-list` · `pr-create`

**문서 도구**

`plantuml-doc-fix` — 마크다운 PlantUML 블록의 흔한 오류를 자동 교정 (문서 저장 시 자동 실행)

### 9.2 자주 묻는 문제

| 증상 | 조치 |
|---|---|
| "로컬 런처가 감지되지 않았습니다" | §2 설치. 설치했는데도 안 되면 구버전 exe 프로세스 종료 후 재기동 |
| 로고 클릭했는데 에이전트 창이 안 뜸 | 런처 감지 확인 → 이미 열린 같은 과제 터미널 창이 있는지 확인(안내 문구의 창 제목 참조) |
| 과제 생성 시 저장이 실패 | 런처 동작 확인(§2.3). 신규 과제는 런처가 떠 있어야 저장됩니다 |
| 이슈 목록에서 행을 눌러도 상세가 안 열림 | 행의 빈 곳이 아니라 **Key 또는 SubKey 링크**를 클릭하세요(§7.2) |
| 이슈 등록을 요청했는데 에이전트가 안 함 | 이슈 쓰기 스킬은 자동 선택되지 않습니다 — `issue-create-vos` 처럼 스킬명을 명시하세요(§4.2) |
| 에이전트로 만든 이슈가 화면에 없음 | `공식` 뷰를 보고 있는지 확인 — 에이전트 결과는 `내작업`에 먼저 보이고, 공식본은 등록→발행→머지 후 반영됩니다(§4.3) |
| 머지까지 했는데 `공식`에 안 보임 | 머지 시 공식본 캐시 갱신이 누락된 경우입니다 — 관리자에게 해당 과제의 이슈 재동기화를 요청하세요 |
| 에이전트로 만든 문서가 Docs 에 안 보임 | Local 탭인지 확인(Main 은 발행·머지 후에만 반영). Local 탭이 비어 있으면 런처 감지 확인 |
| 문서 push 실패 | Git 인증 확인(§2.2) 후 런처 재시작 |
| 과제에 🟠 아이콘이 계속 표시 | 내 docs 브랜치와 main 의 차이 — `등록`→`발행` 으로 PR 하거나, 필요 없으면 Main 탭 `Pull` 로 동기화 |
| PR 이 반려됨 | 알림 안내대로 Main 탭 `Pull` → 수정 → 다시 `등록`/`발행` |
| 다이어그램 ✎ 편집 버튼이 안 보임 | Main 탭이거나 런처 미감지 상태 — Local 탭 + 런처 동작 중일 때만 인라인 편집 가능 |
| 설계 단계가 너무 오래 걸림 | 정상입니다(§6.4). 자가검증을 최대 3회 반복합니다 |
