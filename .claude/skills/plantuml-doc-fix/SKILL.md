---
name: plantuml-doc-fix
description: 마크다운 안 ```plantuml 블록의 흔한 LLM 실수(sequence destroy 화살표, component 안 state 키워드, sequence 안 cloud/file 키워드, 미정의 참여자)를 결정적 정규식으로 자동 수정합니다. PlantUML 서버 호출 없이 ~30ms 내 처리. 일반적으로는 doc-save.sh 가 자동 호출하므로 명시 호출 불필요. 명시 호출 케이스 — 외부에서 받은 markdown 파일 검증 / 특정 파일/디렉터리 일회성 정리 / 자동 적용 후 잔여 오류 PlantUML 서버로 재 검증.
allowed-tools: Bash, Read
---

## 이 스킬을 언제 호출하나

- 사용자가 "TEST 의 PlantUML 오류 자동 수정", "design-decisions 폴더 PlantUML 정리" 류 요청
- 일반 워크플로우 스킬(usecase/style/design-decide 등) 의 doc-save 흐름은 **`doc-save.sh` 가 자동으로 fix 적용** — 별도 호출 불필요
- 본 스킬은 **명시적 일회성 처리** 또는 **수정 결과 + PlantUML 서버 검증** 까지 함께 받고 싶을 때 사용

## 환경
- `PLATFORM_API` 필수 (launcher 가 주입). 미설정 시 fail-fast
- `PLANTUML_SERVER` 기본값: `http://localhost:8080/svg/` (검증용 — 없어도 fix 자체는 동작)

## 인자

- `CODE` — projectCode (예: TEST)
- `USER` — user (예: batman)
- `PATH_SPEC` — 다음 중 하나
  - 단일 파일: `architecture/design-decisions/DP-01-…md`
  - 디렉터리 prefix: `architecture/design-decisions` (디렉터리 안 모든 .md 처리)
  - 또는 `architecture` 처럼 더 넓은 prefix

```bash
export CODE="TEST"
export USER="batman"
export PATH_SPEC="architecture/design-decisions"
```

## 결정적 수정 패턴 (plantuml-fix.py)

| 패턴 | 다이어그램 | 수정 |
|---|---|---|
| `A x B:` (sequence destroy 화살표 잘못) | sequence | `A -x B:` |
| `state "..." as X` (component/database 안) | component | `[...] as X` |
| `cloud "..." as X` (sequence 안) | sequence | `participant "..." as X` |
| `file "..." as X` (sequence 안) | sequence | `participant "..." as X` |
| 미정의 식별자 (메시지에만 등장) | sequence | `@startuml` 직후 `participant <id>` 추가 |

`detect_kind()` 가 `package` / `[Component] as X` / `node` / `database` 같은 component 신호를 sequence 보다 우선하므로, 혼합 다이어그램에서 잘못된 자동 declare 가 발생하지 않음.

## Step 0 — 데이터 수집 (대상 파일 목록)

```bash
PLATFORM_API="${PLATFORM_API:?PLATFORM_API env var required (launcher must inject)}"

# 단일 파일이면 PATH_SPEC 그대로, 디렉터리 prefix 면 tree 에서 *.md 추출
if [[ "$PATH_SPEC" == *.md ]]; then
  TARGETS=("$PATH_SPEC")
else
  mapfile -t TARGETS < <(curl -s "${PLATFORM_API}/DocRepo/${CODE}/tree?ref=$(printf %s "docs/${USER}" | jq -sRr @uri)" \
    -H "X-User-Id: ${USER}" -H "Authorization: Bearer ${GOJIRA_SESSION_TOKEN}" \
    | jq -r --arg p "${PATH_SPEC}/" '.tree[] | select(.path | startswith($p)) | select(.path | endswith(".md")) | .path')
fi
echo "대상 파일: ${#TARGETS[@]}건"
printf '  %s\n' "${TARGETS[@]}"
```

대상 0건 → **중단** 하고 PATH_SPEC 확인 권고.

## Step 1 — 각 파일 fix 적용

```bash
mkdir -p /tmp/${CODE}-puf
TOTAL_CHANGED=0
TOTAL_PATTERNS=()

for p in "${TARGETS[@]}"; do
  bash "${PROJECT_ROOT}/.claude/skills/_scripts/doc-read.sh" "${CODE}" "${USER}" "$p" > "/tmp/${CODE}-puf/$(basename "$p")"
done

# 일괄 적용 (디렉터리 단위)
python3 "${PROJECT_ROOT}/.claude/skills/_scripts/plantuml-fix.py" --dir "/tmp/${CODE}-puf"
```

출력에서 변경 파일 수 + 패턴별 수정 횟수를 보고.

## Step 2 — PlantUML 서버 재 렌더 검증 (선택)

```bash
PLANTUML_SERVER="${PLANTUML_SERVER:-http://localhost:8080/svg/}"

python3 - <<'PY'
import os, re, pathlib, urllib.request, urllib.error, sys
server = os.environ.get("PLANTUML_SERVER", "http://localhost:8080/svg/")
files = sorted(pathlib.Path(f"/tmp/{os.environ['CODE']}-puf").glob("*.md"))
pat = re.compile(r"```plantuml\n(.*?)\n```", re.DOTALL)
ok = err = 0; bad = []
for f in files:
    text = f.read_text(encoding="utf-8")
    for i, body in enumerate(pat.findall(text), 1):
        try:
            req = urllib.request.Request(server, data=body.encode("utf-8"),
                headers={"Content-Type":"text/plain; charset=utf-8"}, method="POST")
            urllib.request.urlopen(req, timeout=10).read()
            ok += 1
        except urllib.error.HTTPError:
            err += 1; bad.append(f"{f.name}#{i}")
print(f"렌더 검증 OK={ok} ERR={err}")
if bad: print("잔여 오류:", bad)
PY
```

PlantUML 서버 미가동 시 이 step 은 스킵 (환경변수 비우거나 무시).

## Step 3 — 변경 있는 파일만 doc-save 로 다시 push

```bash
for p in "${TARGETS[@]}"; do
  LOCAL="/tmp/${CODE}-puf/$(basename "$p")"
  ORIG=$(bash "${PROJECT_ROOT}/.claude/skills/_scripts/doc-read.sh" "${CODE}" "${USER}" "$p")
  NEW=$(cat "$LOCAL")
  if [[ "$ORIG" != "$NEW" ]]; then
    # doc-save.sh 자체가 plantuml-fix 다시 적용 (idempotent — no-op)
    DOC_FIX=0 bash "${PROJECT_ROOT}/.claude/skills/_scripts/doc-save.sh" \
      "${CODE}" "${USER}" "$p" --file "$LOCAL"
    echo "saved: $p"
  else
    echo "no change: $p"
  fi
done
```

> `DOC_FIX=0` — 이미 fix 적용한 결과를 push 할 뿐이므로 doc-save 의 자동 fix 비활성화 (이중 호출 회피, idempotent 라 안 해도 무해).

## Step 4 — 최종 보고

```
## PlantUML 자동 수정 완료

- 대상: TEST architecture/design-decisions (5 파일)
- 패턴별 수정:
  · seq_destroy_arrow: 2
  · component_state_keyword: 5
  · seq_cloud_keyword: 4
  · seq_file_keyword: 1
  · seq_undefined_participant: 1
- 변경 파일: 5/5 → docs/batman 에 push
- 렌더 검증: OK 20 / ERR 0 (PlantUML 서버 가동 중)
```

## 주의

- 도구는 결정적 — PlantUML 서버 호출 없음. 안전.
- doc-save.sh 가 이미 fix 자동 적용. 본 스킬은 일회성 또는 외부 입력용.
- 잔여 ERR 가 있으면 원인 분석 + 패턴 추가 또는 LLM 으로 블록 단위 수정 검토. 본 스킬은 그 결정을 사용자에게 위임.
- `DOC_FIX=0` 환경변수로 doc-save.sh 의 자동 fix 비활성화 가능 (디버그 / 명시 제어 필요 시).
