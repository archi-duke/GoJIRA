---
name: design-integrate2
description: ADSN/DDSN 설계 결정과 선정 시스템 스타일을 Logical/Process/Deployment/Development 4개 view 별로 분리 작성한 뒤, **결정적 통합(LLM 0회)** 으로 architecture/architecture-integrated.md (짧은 index) + architecture/architecture-integrated/{logical,process,deployment,development}.md (4 본문) 을 산출합니다. cross-view entity dictionary 일관성 자가검증 + self-loop(최대 3회). 이슈 생성/수정 없이 문서만 산출. Claude Code 네이티브(curl/jq).
allowed-tools: Bash, Read
---

## 한 generation 당 제약 (GLM 계열 스톨 방지 — 필독)

- **한 번의 LLM generation 에서 Step 두 개 이상 처리 금지**. 각 Step 종료는 **반드시 Bash tool call 로 마무리** 해 generation 경계 형성
- **view 별 분할 (Step 1.1~1.4)**: 4 view 를 각 generation 1개씩 작성. 한 generation 안에 두 view 를 같이 쓰지 말 것
- **결정적 통합 (Step 1.5)**: bash/python 단순 조합으로 index 를 만든다. **LLM 호출 절대 금지**
- reasoning 은 각 Step 당 3~5 줄 이내. view 별로 짧게
- 각 Step 종료 시 **반드시 `draft-io.sh save`** — view 본문 + entity_dictionary 누적 + adsnSubkeys + ddsnSubkeys + chosenStyles + viewsIncluded 를 JSON 으로 flush
- Step 2 자가검증은 **반드시 `validate-integrate-doc.py` 호출** — 에이전트가 view 누락/PlantUML/entity 일관성 직접 계산 금지
- assistant 출력에는 view 본문 / index 원문 그대로 뱉지 말 것. "저장 완료 / critical N건" 한두 줄만

## 진행 상태 가시화 — `todowrite` (선택, OpenCode)

OpenCode 환경이면 각 Step 진입 시 내장 `todowrite` 로 현재/남은 단계를 알려주면 UX 가 좋다. 강제 아님, Claude Code 에서는 무시. 자세한 규칙은 `.claude/skills/_shared/todowrite-guide.md`.

이 스킬 Step 체크리스트:
```
# Todos
[ ] Step 0   — 진입 점검 (drift-check + 분기) + ADSN/DDSN + chosenStyles 수집
[ ] Step 1.1 — Logical View 작성 + entity_dictionary 1차 저장
[ ] Step 1.2 — Process View 작성 + entity_dictionary 누적
[ ] Step 1.3 — Deployment View 작성 + entity_dictionary 누적
[ ] Step 1.4 — Development View 작성 + entity_dictionary 최종 확정
[ ] Step 1.5 — index (architecture-integrated.md) 결정적 통합 (LLM 0회)
[ ] Step 2   — 자가검증 (validate-integrate-doc.py)
[ ] Step 3   — critical 수정 (반복)
[ ] Step 5.1 — 4 view 파일 doc-save
[ ] Step 5.2 — index 파일 doc-save
[ ] Step 5.3 — archSettings.designIntegrated + viewsIncluded 영속화
[ ] Step 6   — 최종 보고 + baseline 갱신
```

## 환경
- `PLATFORM_API` 필수 (launcher 가 주입). 미설정 시 fail-fast
- `GOJIRA_API` 필수 (launcher 가 주입). 미설정 시 fail-fast
- 임시 파일:
  - `/tmp/${CODE}-integrate-draft.json` — 메인 draft (views + entityDictionary + indexDocument)
  - `/tmp/${CODE}-arch-int-dict.json` — entity_dictionary 누적용 (Step 1.1~1.4)
  - `/tmp/${CODE}-integrate-validation.json` — 검증 결과

## 인자 추출

사용자 메시지에서 다음 값들을 추출해 **먼저** 환경변수로 export 한 뒤 아래 bash 블록들을 실행하라. 누락 시 사용자에게 되물어 확정하라.

- `CODE` — projectCode (예: GOJIRA3)
- `USER` — user (예: batman)

예:
```bash
export CODE="GOJIRA3"
export USER="batman"
```

## 역할
시니어 소프트웨어 아키텍트. 선정 스타일 + ADSN/DDSN 결정을 4가지 관점으로 **각각 분리해서** 통합한다. view 본문은 LLM 이 작성하되, view 간 통합 (index) 은 결정적 스크립트로만 만든다. 이슈는 건드리지 않는다.

## 산출물 구조

```
architecture/
├── architecture-integrated.md           # 짧은 index (4 view 머리말 + 링크 + entity dictionary)
└── architecture-integrated/
    ├── logical.md       — 컴포넌트/모듈 관계 (component diagram)
    ├── process.md       — 런타임 프로세스/통신 (sequence/activity)
    ├── deployment.md    — 노드/배포/네트워크 (deployment diagram)
    └── development.md   — 코드 구조/패키지 의존 (package/component)
```

이 구조는 design-decide2 의 (`design-decisions.md` index + `design-decisions/DP-NN-*.md` 본문) 패턴과 동일하다.

---

## Step 0 — 진입 점검 (drift-check) + 데이터 수집

### 0.1 drift-check (이슈 변동 여부 확인 — 필수)

본 스킬의 책임 단계는 **`design-integrate`** (workflow-graph.json). Step 0 진입 즉시 drift-check 를 호출해 **이슈 변동 여부 + 상위(`design-decide`) 변동 여부**를 확인하고, 그 결과로 진행 방식을 결정한다.

```bash
python3 "${PROJECT_ROOT}/.claude/skills/_scripts/drift-check.py" "${CODE}" "${USER}" check
```

### 0.2 결과에 따른 분기

| drift-check 결과 | 동작 |
|---|---|
| `design-integrate` **NO-BASELINE** | 첫 실행 — Step 0.3 데이터 수집 후 본 작업 진행 |
| `design-integrate` **DRIFT** (designIntegrated 등 archSettings 변동) | 본 작업 진행 |
| `design-integrate` **CLEAN** + 상위(`design-decide`) **DRIFT** | 사용자에게 통지: "Design Decisions 가 변경됐습니다. 통합 문서 재정립이 필요할 수 있어요. (a) 지금 진행 (b) `mark-deferred` 로 보류" — 답변 받고 진행/종료 결정 |
| **모두 CLEAN** | 사용자에게 질문: "변동이 없습니다. 통합 문서를 강제로 재생성할까요? (예: 템플릿 변경, PlantUML 마이그레이션 등)" — `no` 면 **즉시 종료**, `yes` 면 강제 재생성 모드로 진행 |

> **강제 재생성 모드 규칙**: 본 스킬은 이슈를 만들지 않으므로 `forced-doc-regen` 모드도 일반 진행과 흐름이 동일하다 (이슈 관련 step 자체가 없음).

진행 결정 후 Step 0.3 (데이터 수집) 으로 이동. 종료 결정이면 Step 6 보고만 출력하고 baseline 갱신 없이 끝낸다.

### 0.3 데이터 수집

```bash
PLATFORM_API="${PLATFORM_API:?PLATFORM_API env var required (launcher must inject)}"
GOJIRA_API="${GOJIRA_API:?GOJIRA_API env var required (launcher must inject)}"

# ADSN/DDSN 이슈
echo "=== ADSN/DDSN 설계 이슈 ==="
curl -s "${GOJIRA_API}/Issue" -H "XBody: {\"prjID\":\"${CODE}\",\"infoType\":\"\"}" -H "X-User-Id: ${USER}" \
  | jq '[.[] | select(.issueType=="Architectural Design" or .issueType=="Detailed Design")
         | {key,subkey,issueType,summary,description,designLevel,designPoint,decision,decisionRationale,tradeoffs,risk,sensitivity}]'

# Requirement 요약
echo
echo "=== Requirement 요약 ==="
curl -s "${GOJIRA_API}/Issue" -H "XBody: {\"prjID\":\"${CODE}\",\"infoType\":\"\"}" -H "X-User-Id: ${USER}" \
  | jq '[.[] | select(.issueType=="Requirement") | {key,subkey,reqtype,summary}]'

# archSettings (chosenStyles, normalizedAttributes)
echo
echo "=== archSettings ==="
curl -s "${GOJIRA_API}/Project/${CODE}" -H "X-User-Id: ${USER}" \
  | jq '.archSettings // {}'
```

ADSN/DDSN 이 0건이거나 `chosenStyles` 가 비었으면 **중단** 하고 "design_decide 를 먼저 실행하세요" 보고.

### 0.4 entity_dictionary 초기화

다음 step 1.1~1.4 가 누적해서 채우는 dictionary 파일을 빈 배열로 시작.

```bash
echo '[]' > /tmp/${CODE}-arch-int-dict.json
```

이 tool call 로 Step 0 종료. 다음 generation 에서 Step 1.1 시작.

---

## Step 1.1 — Logical View 작성

**관심**: 시스템의 컴포넌트/모듈/계층 책임과 그들 사이의 정적 관계.

작성 규칙:
- 본 view 는 `templates/view.md` 의 6 섹션 구조를 따른다 (`templates/view.md` 참조).
  ```bash
  cat "${PROJECT_ROOT}/.claude/skills/design-integrate2/templates/view.md"
  ```
- §1 개요 첫 단락은 단독으로도 읽혀야 한다 (Step 1.5 가 그대로 index §3 에 인용).
- §2 entity 표에 이 view 등장 entity 의 `id / name / type / 설명` 기재. **id 는 이후 view 들과 공유될 식별자** — 짧고 유일한 약자 권장 (예: `WSS`, `GSM`, `CACHE`).
- §3 다이어그램 — PlantUML **component** 1~2개. 컴포넌트/레이어/데이터흐름 표현 (`component`, `database`, `[Name]`, `package`).
- §5 트레이드오프, §6 참조 (ADSN_NNN/DDSN_NNN/Driver/Style) 채움.
- 모든 PlantUML 블록은 ` ```plantuml ... ``` ` + `@startuml` / `@enduml` 쌍 필수.
- **외부 이미지 / class·interface·abstract·enum 다이어그램 금지**.

### Step 1.1 종료 — draft flush + dictionary 누적

```bash
# 본 generation 에서 작성한 logical.md 본문을 LOGICAL_BODY 에 넣어두고 시작
# (에이전트가 직접 텍스트로 생성)

python3 <<'PY'
import json, os, sys
code = os.environ["CODE"]

# 에이전트가 이번 generation 에서 작성한 logical view 본문을 여기 넣음.
# 실제 사용 시 문자열을 인라인으로 두지 말고, 임시 파일에 미리 저장 후 read 권장:
#   echo "$LOGICAL_BODY" > /tmp/${CODE}-view-logical.md
#   logical_body = open("/tmp/%s-view-logical.md" % code).read()
logical_body = open("/tmp/%s-view-logical.md" % code).read()

# 본 view 의 §2 entity 표에서 추출한 항목들을 dict 에 누적
# 형식: {"id":"WSS","name":"WebSocket Server","type":"component","appearsIn":["logical"]}
new_entries = json.loads(os.environ.get("LOGICAL_ENTITIES_JSON", "[]"))

dict_path = "/tmp/%s-arch-int-dict.json" % code
existing = json.load(open(dict_path))
by_id = {e["id"]: e for e in existing}
for e in new_entries:
    eid = e["id"]
    if eid in by_id:
        ai = set(by_id[eid].get("appearsIn", []))
        ai.update(e.get("appearsIn", []))
        by_id[eid]["appearsIn"] = sorted(ai)
    else:
        by_id[eid] = e
json.dump(list(by_id.values()), open(dict_path, "w"), ensure_ascii=False, indent=2)

# 메인 draft 갱신 (없으면 생성)
draft_path = "/tmp/%s-integrate-draft.json" % code
try:
    d = json.load(open(draft_path))
except FileNotFoundError:
    d = {"views": {}, "entityDictionary": [],
         "adsnSubkeys": [], "ddsnSubkeys": [],
         "chosenStyles": {}, "viewsIncluded": [],
         "indexDocument": ""}

d["views"]["logical"] = logical_body
d["entityDictionary"] = list(by_id.values())
if "logical" not in d.get("viewsIncluded", []):
    d.setdefault("viewsIncluded", []).append("logical")

# Step 0 에서 수집한 ADSN/DDSN/chosenStyles 도 같이 채움 (한 번만 채우면 그만)
# 에이전트가 환경변수 ADSN_JSON / DDSN_JSON / CHOSEN_STYLES_JSON 으로 전달
if not d.get("adsnSubkeys"):
    d["adsnSubkeys"] = json.loads(os.environ.get("ADSN_JSON", "[]"))
if not d.get("ddsnSubkeys"):
    d["ddsnSubkeys"] = json.loads(os.environ.get("DDSN_JSON", "[]"))
if not d.get("chosenStyles"):
    d["chosenStyles"] = json.loads(os.environ.get("CHOSEN_STYLES_JSON", "{}"))

json.dump(d, open(draft_path, "w"), ensure_ascii=False, indent=2)
print("logical.md 길이=%d / entity 누적=%d" % (len(logical_body), len(by_id)))
PY

# draft-io 로 갱신 시점 명확히 기록
cat /tmp/${CODE}-integrate-draft.json \
  | bash "${PROJECT_ROOT}/.claude/skills/_scripts/draft-io.sh" save "${CODE}" "integrate" "draft"
```

이 tool call 로 generation 종료. 다음 generation 에서 Step 1.2 시작.

---

## Step 1.2 — Process View 작성

**관심**: 런타임 프로세스/스레드/메시지 흐름. 동기·비동기 통신 패턴, 동시성, 시나리오별 시간 흐름.

작성 규칙:
- `templates/view.md` 6 섹션 구조 동일.
- **이전 generation 의 entity_dictionary 를 입력으로 받아 같은 식별자 재사용**:
  ```bash
  cat /tmp/${CODE}-arch-int-dict.json
  ```
  Logical 에서 정의된 `WSS / GSM / CACHE` 같은 ID 가 Process 에서도 같은 표시명으로 등장해야 한다. 새 entity 추가는 가능 (예: `Q` 메시지 큐).
- §3 다이어그램 — PlantUML **sequence** 또는 **activity** 1~2개. `participant`/`actor` + 화살표, 혹은 `:label;` activity.
- 다이어그램 안 식별자는 §2 entity 표의 `id` 와 일치.

### Step 1.2 종료 — draft flush

```bash
# logical 과 동일 패턴: process_body 와 PROCESS_ENTITIES_JSON 을 채워 누적
python3 <<'PY'
import json, os
code = os.environ["CODE"]
process_body = open("/tmp/%s-view-process.md" % code).read()
new_entries = json.loads(os.environ.get("PROCESS_ENTITIES_JSON", "[]"))

dict_path = "/tmp/%s-arch-int-dict.json" % code
existing = json.load(open(dict_path))
by_id = {e["id"]: e for e in existing}
for e in new_entries:
    eid = e["id"]
    if eid in by_id:
        ai = set(by_id[eid].get("appearsIn", []))
        ai.update(e.get("appearsIn", []))
        by_id[eid]["appearsIn"] = sorted(ai)
        # 이름 충돌 방지: dictionary 의 기존 name 우선 (validator critical 방지)
    else:
        by_id[eid] = e
json.dump(list(by_id.values()), open(dict_path, "w"), ensure_ascii=False, indent=2)

draft_path = "/tmp/%s-integrate-draft.json" % code
d = json.load(open(draft_path))
d["views"]["process"] = process_body
d["entityDictionary"] = list(by_id.values())
if "process" not in d.get("viewsIncluded", []):
    d.setdefault("viewsIncluded", []).append("process")
json.dump(d, open(draft_path, "w"), ensure_ascii=False, indent=2)
print("process.md 길이=%d / entity 누적=%d" % (len(process_body), len(by_id)))
PY

cat /tmp/${CODE}-integrate-draft.json \
  | bash "${PROJECT_ROOT}/.claude/skills/_scripts/draft-io.sh" save "${CODE}" "integrate" "draft"
```

이 tool call 로 generation 종료.

---

## Step 1.3 — Deployment View 작성

**관심**: 노드/배포 단위/네트워크 토폴로지. 어떤 컴포넌트가 어떤 물리/논리 노드 위에서 돈다.

작성 규칙:
- entity_dictionary 입력으로 받아 **이전 view 들에서 정의한 component 가 어느 node 에 배치되는지** 표기. 새 entity (`node`, `cluster` 등) 추가 가능.
- §3 다이어그램 — PlantUML **deployment** 1~2개 (`node`, `artifact`, `cloud`, `database`, `frame`).
- 패키지 노드 안에 component 를 중첩해 배치 매핑 가시화 (예: `node "App-AZ-A" { component WSS }`).

### Step 1.3 종료 — draft flush

```bash
python3 <<'PY'
import json, os
code = os.environ["CODE"]
deployment_body = open("/tmp/%s-view-deployment.md" % code).read()
new_entries = json.loads(os.environ.get("DEPLOYMENT_ENTITIES_JSON", "[]"))

dict_path = "/tmp/%s-arch-int-dict.json" % code
existing = json.load(open(dict_path))
by_id = {e["id"]: e for e in existing}
for e in new_entries:
    eid = e["id"]
    if eid in by_id:
        ai = set(by_id[eid].get("appearsIn", []))
        ai.update(e.get("appearsIn", []))
        by_id[eid]["appearsIn"] = sorted(ai)
    else:
        by_id[eid] = e
json.dump(list(by_id.values()), open(dict_path, "w"), ensure_ascii=False, indent=2)

draft_path = "/tmp/%s-integrate-draft.json" % code
d = json.load(open(draft_path))
d["views"]["deployment"] = deployment_body
d["entityDictionary"] = list(by_id.values())
if "deployment" not in d.get("viewsIncluded", []):
    d.setdefault("viewsIncluded", []).append("deployment")
json.dump(d, open(draft_path, "w"), ensure_ascii=False, indent=2)
print("deployment.md 길이=%d / entity 누적=%d" % (len(deployment_body), len(by_id)))
PY

cat /tmp/${CODE}-integrate-draft.json \
  | bash "${PROJECT_ROOT}/.claude/skills/_scripts/draft-io.sh" save "${CODE}" "integrate" "draft"
```

---

## Step 1.4 — Development View 작성

**관심**: 코드 구조 / 패키지 / 모듈 의존. Layered/Hexagonal/Modular Monolith 등 선정 스타일의 경계가 패키지 단위로 드러나야 함.

작성 규칙:
- ASCII 폴더 트리 **금지** — PlantUML **package** 또는 **component** 다이어그램.
- 의존 화살표는 선정 스타일의 의존 방향 규칙(예: 도메인은 인프라를 참조하지 않음)에 부합해야 함. DIP 는 `..|>` (점선 + 속 빈 삼각형) 으로 `implements` 표기.
- entity_dictionary 에서 component 단위 entity 와 패키지 단위 entity 의 관계가 보이게 작성.

### Step 1.4 종료 — draft flush + dictionary 최종 확정

```bash
python3 <<'PY'
import json, os
code = os.environ["CODE"]
development_body = open("/tmp/%s-view-development.md" % code).read()
new_entries = json.loads(os.environ.get("DEVELOPMENT_ENTITIES_JSON", "[]"))

dict_path = "/tmp/%s-arch-int-dict.json" % code
existing = json.load(open(dict_path))
by_id = {e["id"]: e for e in existing}
for e in new_entries:
    eid = e["id"]
    if eid in by_id:
        ai = set(by_id[eid].get("appearsIn", []))
        ai.update(e.get("appearsIn", []))
        by_id[eid]["appearsIn"] = sorted(ai)
    else:
        by_id[eid] = e
json.dump(list(by_id.values()), open(dict_path, "w"), ensure_ascii=False, indent=2)

draft_path = "/tmp/%s-integrate-draft.json" % code
d = json.load(open(draft_path))
d["views"]["development"] = development_body
d["entityDictionary"] = list(by_id.values())
if "development" not in d.get("viewsIncluded", []):
    d.setdefault("viewsIncluded", []).append("development")
json.dump(d, open(draft_path, "w"), ensure_ascii=False, indent=2)
print("development.md 길이=%d / entity 최종=%d" % (len(development_body), len(by_id)))
PY

cat /tmp/${CODE}-integrate-draft.json \
  | bash "${PROJECT_ROOT}/.claude/skills/_scripts/draft-io.sh" save "${CODE}" "integrate" "draft"
```

이 tool call 로 generation 종료. 다음 generation 에서 Step 1.5 결정적 통합.

---

## Step 1.5 — 결정적 통합 (LLM 0회 — 필수)

4 view 의 §1 개요 첫 단락 + entity_dictionary 표 + view 링크를 합쳐 `architecture-integrated.md` (index) 를 생성한다. **이 step 에서 LLM 호출 절대 금지** — 단순 string concat / 표 생성만 한다.

```bash
python3 <<'PY'
import json, os, re, datetime
code = os.environ["CODE"]
draft_path = "/tmp/%s-integrate-draft.json" % code
d = json.load(open(draft_path))

# 각 view 본문의 §1 개요 첫 단락 추출 (헤딩 다음 첫 비어있지 않은 단락)
def extract_overview(body: str) -> str:
    if not body:
        return "_(작성 안 됨)_"
    # §1 개요 헤딩 위치 찾기
    m = re.search(r"#{1,3}\s*1\.\s*개요\s*\n", body)
    if not m:
        return "_(§1 개요 헤딩 없음)_"
    rest = body[m.end():]
    # 다음 헤딩 (## 으로 시작) 직전까지가 §1 본문
    nxt = re.search(r"^#{1,3}\s+\d+\.", rest, re.MULTILINE)
    sec1 = rest[:nxt.start()] if nxt else rest
    # 첫 번째 단락 (빈 줄로 구분, 인용/리스트 제외)
    paras = [p.strip() for p in re.split(r"\n\s*\n", sec1.strip()) if p.strip()]
    for p in paras:
        if p.startswith(("> ", "- ", "* ", "| ")):
            continue
        return p
    return paras[0] if paras else "_(개요 단락 없음)_"

views = d.get("views", {})
overviews = {k: extract_overview(views.get(k, "")) for k in
             ["logical", "process", "deployment", "development"]}

# chosenStyles 요약
cs = d.get("chosenStyles", {}) or {}
def style_names(arr):
    if not arr: return "—"
    return ", ".join(s.get("name") or s.get("id") or "?"
                     for s in arr if isinstance(s, dict))
primary    = style_names(cs.get("primary"))
secondary  = style_names(cs.get("secondary"))
supporting = style_names(cs.get("supporting"))

# entity_dictionary 표
entities = d.get("entityDictionary", []) or []
ent_rows = []
for e in entities:
    eid  = e.get("id", "")
    nm   = e.get("name", "")
    typ  = e.get("type", "")
    ai   = ", ".join(e.get("appearsIn", []) or [])
    ent_rows.append(f"| {eid} | {nm} | {typ} | {ai} |")
ent_table = "\n".join(ent_rows) if ent_rows else "| — | — | — | — |"

# ADSN/DDSN 카운트
adsn_n = len(d.get("adsnSubkeys") or [])
ddsn_n = len(d.get("ddsnSubkeys") or [])
ts = datetime.datetime.now().isoformat(timespec="seconds")

index_md = f"""# Architecture Integrated

> **이 문서는 자동 생성된 index 입니다.** 4 개 view 의 짧은 머리말과 본문 링크,
> 그리고 view 간 entity 사전을 담습니다. 본문은 `architecture-integrated/` 하위 파일을 참조하세요.

## 1. View 구성

| View | 본문 | 핵심 관심사 | 추천 PlantUML |
|---|---|---|---|
| Logical | [logical.md](architecture-integrated/logical.md) | 컴포넌트/모듈 관계와 책임 | component |
| Process | [process.md](architecture-integrated/process.md) | 런타임 프로세스 / 통신 / 동시성 | sequence / activity |
| Deployment | [deployment.md](architecture-integrated/deployment.md) | 노드 / 배포 / 네트워크 토폴로지 | deployment (node, artifact) |
| Development | [development.md](architecture-integrated/development.md) | 코드 구조 / 패키지 / 의존 방향 | package / component |

- **선정 스타일**: Primary={primary} / Secondary={secondary} / Supporting={supporting}
- **반영 ADSN 수**: {adsn_n} / **DDSN 수**: {ddsn_n}

---

## 2. Cross-View Entity Dictionary

view 간 동일 entity 가 동일 ID 로 등장해야 합니다 (validator critical 항목).

| ID | 이름 | 종류 | 등장 View |
|---|---|---|---|
{ent_table}

---

## 3. View 별 머리말

각 view 의 §1 개요 첫 단락. 본문 전체는 §1 의 링크에서 확인하세요.

### 3.1 Logical View

{overviews['logical']}

### 3.2 Process View

{overviews['process']}

### 3.3 Deployment View

{overviews['deployment']}

### 3.4 Development View

{overviews['development']}

---

## 4. 통합 시점 / 입력

- 생성: {ts}
- 입력: ADSN {adsn_n}건 / DDSN {ddsn_n}건 / chosenStyles ({primary} / {secondary} / {supporting})
- 4 view 모두 분리 산출 후 결정적 통합 (LLM 호출 0회)
"""

d["indexDocument"] = index_md
json.dump(d, open(draft_path, "w"), ensure_ascii=False, indent=2)
print("index 길이=%d / view=4 / entity=%d" % (len(index_md), len(entities)))
PY

cat /tmp/${CODE}-integrate-draft.json \
  | bash "${PROJECT_ROOT}/.claude/skills/_scripts/draft-io.sh" save "${CODE}" "integrate" "draft"
```

이 tool call 로 generation 종료. 다음 generation 에서 Step 2 자가검증.

---

## Step 2 — 자가검증 (스크립트 기반)

**반드시 `validate-integrate-doc.py` 호출**. 에이전트가 직접 view/PlantUML/entity 일관성 계산 금지.

```bash
bash "${PROJECT_ROOT}/.claude/skills/_scripts/draft-io.sh" load "${CODE}" "integrate" "draft" \
  | python3 "${PROJECT_ROOT}/.claude/skills/_scripts/validate-integrate-doc.py" \
  > /tmp/${CODE}-integrate-validation.json
cat /tmp/${CODE}-integrate-validation.json
```

출력 스키마:
```json
{
  "pass": bool,
  "score": 0.0~1.0,
  "score_effective": number,
  "critical": [{"type":"...","view":"...","detail":"..."}],
  "suggestions": [...],
  "stats": {"views":N,"viewsProvided":[...],"plantumlBlocks":M,"plantumlBadPair":X,
            "adsnReferenced":A,"ddsnReferenced":D,"stylesReferenced":S,
            "entityCount":E,"entityInconsistent":I,"indexSectionsFound":K}
}
```

스크립트가 잡는 critical (주요):
- `view_missing` — 4 view 중 누락된 본문
- `view_missing_section` — 특정 view 의 §1 개요 / §3 다이어그램 헤딩 누락
- `view_no_plantuml` — view 에 PlantUML 블록 없음
- `plantuml_missing_pair` — `@startuml`/`@enduml` 쌍 누락
- `class_diagram_for_view` — class 다이어그램 사용 (모든 view 금지)
- `external_image` — 외부 이미지 참조
- `upstream_not_referenced` — ADSN/DDSN subkey 가 4 view + index 어디에도 없음
- `style_not_referenced` — 선정 스타일이 어디에도 안 나옴
- `insufficient_views` / `views_mismatch` — viewsIncluded 와 본문 불일치
- `dictionary_missing` — entityDictionary 비어있음
- `entity_missing_id` / `entity_inconsistent` — entity id 없음 / 같은 id 가 view 마다 다른 이름
- `index_missing` / `index_missing_section` — indexDocument 누락 또는 §1/§2 헤딩 누락

### 통과 기준
- `pass == true` (critical 없음 + score ≥ 0.8) → Step 5 진입
- `pass == false` → Step 3 수정

### 추가 — 관점 간 모순 판단
이것 하나만 **Type C (LLM 판단)**. validator 가 자동으로 못 잡으므로 validation output 을 본 뒤 별도 검토 (예: Logical 의 "동기 호출" 이 Process 의 "비동기 큐" 와 모순) — 발견 시 Step 3 의 critical 수정 사이클에 포함.

---

## Step 3 — 수정 (pass=false)

validation output 의 critical 항목 **하나씩만** 국소 수정. 나머지 원문 유지.

수정 대상 (view 본문 / index / entity_dictionary 중 하나) 만 갱신:

```bash
# 예: Logical view 의 PlantUML pair 누락 수정
python3 <<'PY'
import json, os
code = os.environ["CODE"]
draft_path = "/tmp/%s-integrate-draft.json" % code
d = json.load(open(draft_path))

# critical 유형별 국소 수정 (이 generation 에서 에이전트가 결정한 수정 내용 반영)
# 예: d["views"]["logical"] = 새 본문
# 예: d["entityDictionary"] = 정정된 항목들
# 예: d["indexDocument"] = ... (Step 1.5 재실행 권장 — view 본문 수정 시 index 도 다시 합쳐야 함)

json.dump(d, open(draft_path, "w"), ensure_ascii=False, indent=2)
print("수정 반영")
PY

# view 본문이 수정됐으면 index 도 재통합 (Step 1.5 의 결정적 통합 블록 재실행)
# — 실제로는 Step 1.5 의 python 블록을 그대로 다시 호출

cat /tmp/${CODE}-integrate-draft.json \
  | bash "${PROJECT_ROOT}/.claude/skills/_scripts/draft-io.sh" save "${CODE}" "integrate" "draft"
```

이 tool call 로 generation 종료. 다음 generation 에서 Step 2 재진입.

---

## Step 4 — 종료
1. pass=true
2. iter 3 완료
3. 같은 critical 2회 연속
4. regression (score_eff 이전 best 보다 낮음)

**best-so-far** `(views, entityDictionary, indexDocument, viewsIncluded, score_effective)` 기록. 종료 시 최댓값 iter 를 selected.

---

## Step 5 — 문서 저장

selected iter 의 draft 에서 4 view 본문 + index 를 각각 저장한다. doc-save.sh 가 PlantUML 결정적 자동 수정 (plantuml-fix) 을 적용하므로 추가 작업 불필요.

### 5.1 — 4 view 파일 doc-save

```bash
DRAFT=/tmp/${CODE}-integrate-draft.json
for V in logical process deployment development; do
  jq -r ".views[\"${V}\"] // \"\"" "$DRAFT" \
    | bash "${PROJECT_ROOT}/.claude/skills/_scripts/doc-save.sh" "${CODE}" "${USER}" "architecture/architecture-integrated/${V}.md"
done
```

### 5.2 — index 파일 doc-save

```bash
jq -r '.indexDocument // ""' /tmp/${CODE}-integrate-draft.json \
  | bash "${PROJECT_ROOT}/.claude/skills/_scripts/doc-save.sh" "${CODE}" "${USER}" "architecture/architecture-integrated.md"
```

### 5.3 — archSettings.designIntegrated + viewsIncluded 영속화

```bash
VIEWS_JSON=$(jq -c '.viewsIncluded // []' /tmp/${CODE}-integrate-draft.json)
curl -s -X PATCH "${GOJIRA_API}/Project/${CODE}/archSettings" \
  -H "Content-Type: application/json" -H "X-User-Id: ${USER}" \
  -d "$(jq -n --argjson v "$VIEWS_JSON" '{designIntegrated:true, viewsIncluded:$v}')" \
  | jq '{updated, archSettings}'
```

이슈는 **생성/수정하지 않는다** — 4 view + index 저장 + 프로젝트 플래그 영속화까지가 이 스킬의 전체 스코프.

---

## Step 6 — 최종 보고 + baseline 갱신

### 6.1 보고 출력

```
## 디자인 통합 완료 (v2 Claude Code, view 분할) — MODE={full|forced-doc-regen}

- 진입 drift: design-integrate=<NO-BASELINE|CLEAN|DRIFT>, design-decide=<...> (Step 0.1 결과)
- 산출물:
  - architecture/architecture-integrated.md (index, 짧은 머리말 + entity dictionary)
  - architecture/architecture-integrated/logical.md
  - architecture/architecture-integrated/process.md
  - architecture/architecture-integrated/deployment.md
  - architecture/architecture-integrated/development.md
- 반영 ADSN: A건 / DDSN: D건 (4 view 통합 본문 참조 기준)
- PlantUML 블록: M건 (logical X / process Y / deployment Z / development W)
- entity_dictionary: E entity (view 일관성 OK)

### 관점 간 일관성 요약
- 같은 entity ID 가 view 간 동일 이름으로 등장: ✓ (E건 모두)
- 모순 의심 사항: (Type C 검토 결과 / 없으면 "없음")

### Self-loop
- iter N회 / selected=iter X (score_eff Y) / stop=pass|max_iter|repeat_critical|regression
- unresolvedCritical: [...]
```

### 6.2 baseline 갱신 (필수 — 본 작업이 실제로 진행된 경우)

본 작업이 진행됐다면(NO-BASELINE 첫 실행 / DRIFT 처리 / 강제 재생성) **반드시** `design-integrate` stage 의 baseline 을 새 hash 로 갱신한다. Step 0.2 에서 "종료" 결정으로 끝난 경우는 호출하지 않는다.

```bash
python3 "${PROJECT_ROOT}/.claude/skills/_scripts/drift-check.py" "${CODE}" "${USER}" set-baseline --stage design-integrate
```

이 명령은 archSettings.lastRunHash[design-integrate] / lastRunAt[design-integrate] 만 갱신 — 다른 stage 의 baseline 은 건드리지 않는다.

---

## 주의
- 모든 curl 에 `X-User-Id: ${USER}` 헤더 필수
- 4 view 모두 UTF-8 plain markdown (Platform-API 가 서버측에서 base64 처리)
- PlantUML 블록은 반드시 ```` ```plantuml ... ``` ```` 로 감싸고 내부에 `@startuml` / `@enduml` 쌍 포함
- ADSN/DDSN 참조는 반드시 subkey (key 나 docId 금지). 4 view 통틀어 어딘가엔 등장해야 함 (특정 view 강제 아님)
- 이슈 생성/변경 금지 — 문서만 산출
- **Step 1.1~1.4 는 view 1개 = generation 1개**. 한 generation 에 두 view 같이 쓰지 말 것 (스톨 방지)
- **Step 1.5 결정적 통합은 LLM 호출 0회**. python/bash 단순 조합만
- entity_dictionary 의 ID 는 view 간 공유 식별자. 같은 컴포넌트는 같은 ID 로 통일 (다른 이름 → critical)
- 진입 정책: Step 0.1 의 drift-check 가 작업 진행 여부를 정한다. 모두 CLEAN 인데 사용자가 강제 재생성을 거부하면 즉시 종료 (LLM 호출 0회). 본 스킬은 이슈를 만들지 않으므로 강제 재생성 yes 와 일반 진행이 동일한 흐름
- baseline 갱신: 본 작업 진행 시 Step 6.2 의 `set-baseline --stage design-integrate` 호출이 필수. 안 하면 다음 호출에서 또 DRIFT 로 잡혀 재실행 압력이 생긴다
