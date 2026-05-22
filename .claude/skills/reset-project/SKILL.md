---
name: reset-project
description: "[개발/테스트 전용] 프로젝트의 이슈(Mongo+Gitea YAML) / archSettings / business·architecture 문서를 초기 상태로 리셋합니다. 파괴적 작업이므로 CONFIRM=true 없이는 dry-run(영향 범위)만 반환합니다. Skill tool 로도 호출 가능하지만 CONFIRM=true 는 반드시 사용자 명시 동의 후에만 설정."
allowed-tools: Bash
---

## 환경

- `GOJIRA_API` 기본값: `http://10.211.55.3:44360/api/v2` — 이슈/archSettings 리셋용
- `GOJIRA_REPO_ROOT` — 로컬 clone 루트 (런처 주입). 미설정 시 `git rev-parse --show-toplevel` fallback.
- 문서 리셋은 **로컬 clone 작업본에 직접 fs write/delete** (서버 `PUT/DELETE /file` 미사용 — local-agent 모델)

## 인자 추출

사용자 메시지에서 다음 값들을 추출해 **먼저** 환경변수로 export 한 뒤 아래 bash 블록들을 실행하라. 누락 시 사용자에게 되물어 확정하라.

- `CODE` — projectCode (필수)
- `USER` — user (필수, `X-User-Id` 및 `docs/{user}` 브랜치)
- `CONFIRM` — (선택) `true` 이면 실제 실행, 그 외 dry-run. **절대 기본 true 로 두지 말 것.** 미지정/미확인 시 dry-run 유지
- `RESET_ISSUES` — (선택) `true`/`false`, 기본 `true`
- `RESET_ARCH_SETTINGS` — (선택) `true`/`false`, 기본 `true`
- `RESET_DOCS` — (선택) `true`/`false`, 기본 `true`

예 (dry-run):
```bash
export CODE="GOJIRA3"
export USER="batman"
# CONFIRM 은 사용자 명시 동의 후에만 설정
```

실제 실행 (사용자 동의 후):
```bash
export CONFIRM="true"
```

## 경고

이 스킬은 **파괴적(destructive)** 입니다.
- 프로젝트의 모든 이슈를 삭제하고(Gitea YAML + Mongo 양쪽), archSettings 를 초기화하고, 문서를 초기 템플릿으로 되돌립니다
- **반드시** 사용자에게 명시적 동의를 얻은 뒤에만 `CONFIRM=true` 로 실행
- 기본 동작은 dry-run. dry-run 결과를 보여주고, 사용자가 "확인/진행/yes" 등 명시 동의 후에만 두 번째 호출에서 `CONFIRM=true`
- 서버측 일괄 삭제 엔드포인트(`POST /Project/{code}/reset`)를 사용하므로 클라이언트 bash 루프 없음 — sandbox 의 bulk-destructive 감지에 덜 민감

### Soft delete 정책 (PR #11, 2026-04-29)

이슈 삭제는 **휴지통 (issues_trash 컬렉션) 으로 soft delete** 됩니다:
- Mongo 원본 이슈는 `issues_trash` 로 이동 (`_trashedAt` / `_trashedBy` 추가)
- 3일 후 sweeper goroutine 이 자동 영구 삭제 (`purgeOldTrashWorker`)
- Gitea YAML 은 즉시 git rm + commit (git history 가 자체 휴지통 역할)
- **3일 내 복구 가능** — `issues_trash` mongo doc → yaml.Marshal + Issues insert (수동, 향후 endpoint)

→ 사고 발생 시 즉시 sweeper 멈추고 trash 에서 회복 시도 가능.

## 1. 사전 체크

```bash
GOJIRA_API="${GOJIRA_API:-http://10.211.55.3:44360/api/v2}"
CONFIRM="${CONFIRM:-false}"
RESET_ISSUES="${RESET_ISSUES:-true}"
RESET_ARCH_SETTINGS="${RESET_ARCH_SETTINGS:-true}"
RESET_DOCS="${RESET_DOCS:-true}"

if [ -z "$CODE" ] || [ -z "$USER" ]; then
  echo "ERROR: projectCode 와 user 모두 필수"
  exit 1
fi

# 로컬 clone 루트 결정 (런처가 GOJIRA_REPO_ROOT 주입, 없으면 git toplevel)
ROOT="${GOJIRA_REPO_ROOT:-}"
if [ -z "$ROOT" ]; then
  ROOT=$(git rev-parse --show-toplevel 2>/dev/null || true)
fi
if [ -z "$ROOT" ] || [ ! -d "$ROOT" ]; then
  echo "ERROR: repo 루트를 찾을 수 없음 — GOJIRA_REPO_ROOT 를 설정하거나 clone 안에서 실행"
  exit 1
fi
```

## 2. 영향 범위 산정 — `POST /Project/{code}/reset` dry-run

서버측 엔드포인트가 영향 범위(이슈 키 목록, archSettings 존재 여부) 를 반환한다.

```bash
curl -s -X POST "${GOJIRA_API}/Project/${CODE}/reset" \
  -H "Content-Type: application/json" -H "X-User-Id: ${USER}" \
  -d "$(jq -n --argjson ri "$RESET_ISSUES" --argjson ra "$RESET_ARCH_SETTINGS" \
    '{confirm:false, resetIssues:$ri, resetArchSettings:$ra}')" \
  | jq
```

응답 예:
```json
{
  "jiraCode": "TPRJ",
  "mode": "dry-run",
  "wouldDelete": {
    "issueCount": 67,
    "issueKeys": ["TPRJ-1", "TPRJ-2", ...],
    "resetArchSettings": true,
    "archSettingsPresent": true
  },
  "message": "DRY-RUN: 67이슈 삭제 예정. 실제 실행은 confirm=true 로 재호출."
}
```

`RESET_DOCS=true` 라면 아래 8 파일이 로컬 clone(`${ROOT}`)에서 덮어써짐(보고에만 포함, 서버측 처리 아님 — Step 4-2 에서 스킬이 직접 로컬 fs write):
- `business/project-description.md`
- `business/use-cases.md`
- `business/requirements.md`
- `architecture/architecture.md`
- `architecture/drivers.md`
- `architecture/architecture-style.md`
- `architecture/design-decisions.md`
- `architecture/architecture-integrated.md`

추가로 **하위 디렉토리의 후보별/뷰별 파일도 정리** (Step 4-3 에서 처리):
- `architecture/architecture-style/<subkey>-*.md` — style-define2 후보별 ADSN 파일
- `architecture/design-decisions/DP-NN-*.md` — design-decide2 DP별 파일
- `architecture/architecture-integrated/{logical,process,deployment,development}.md` — design-integrate2 view별
- `data/issues/*.yaml` — 이슈 yaml (서버측 reset 이 자기 과제 이슈만 삭제하므로 잔존 가능)

## 3. Dry-run 최종 보고 (confirm != true)

`CONFIRM` 이 `true` 가 아니면 여기서 종료. 사용자에게 Step 2 응답 + 문서 목록을 정리해 보고하고 "`CONFIRM=true` 로 재호출 필요" 안내.

## 4. 실제 실행 (confirm=true)

### 4-1. 이슈 + archSettings 일괄 리셋 (서버측 1회 호출)

```bash
RESET_RESULT=$(curl -s -X POST "${GOJIRA_API}/Project/${CODE}/reset" \
  -H "Content-Type: application/json" -H "X-User-Id: ${USER}" \
  -d "$(jq -n --argjson ri "$RESET_ISSUES" --argjson ra "$RESET_ARCH_SETTINGS" \
    '{confirm:true, resetIssues:$ri, resetArchSettings:$ra}')")
echo "$RESET_RESULT" | jq '{mode, deletedCount, failedCount, archSettingsCleared}'
```

응답에 `failedCount > 0` 이면 최종 보고에 `failed[]` 내용을 포함. 그래도 Step 4-2 진행(부분 성공이어도 문서 초기화는 시도).

### 4-2. 문서 초기화 (로컬 clone)

`.claude/skills/reset-project/templates/` 하위 8개 파일을 로컬 clone(`${ROOT}`)의 대응 경로에 직접 복사해 덮어쓴다. 서버 왕복(`PUT /file`/`init-files`) 없음 — 결과는 이후 런처 push → PR 로 반영.

```bash
TEMPLATE_DIR="${ROOT}/.claude/skills/reset-project/templates"

put_template() {
  local REL_PATH="$1"
  local DST="${ROOT}/${REL_PATH}"
  if [ ! -f "${TEMPLATE_DIR}/${REL_PATH}" ]; then
    echo "WRITE ${REL_PATH} -> SKIP (template 없음)"
    return
  fi
  mkdir -p "$(dirname "$DST")"
  cp "${TEMPLATE_DIR}/${REL_PATH}" "$DST"
  echo "WRITE ${REL_PATH} -> ok"
}

if [ "$CONFIRM" = "true" ] && [ "$RESET_DOCS" = "true" ]; then
  for F in \
    business/project-description.md \
    business/use-cases.md \
    business/requirements.md \
    architecture/architecture.md \
    architecture/drivers.md \
    architecture/architecture-style.md \
    architecture/design-decisions.md \
    architecture/architecture-integrated.md; do
    put_template "$F"
  done
fi
```

### 4-3. 하위 디렉토리 후보별/뷰별 파일 정리 (PR #11 보강)

style-define2 / design-decide2 / design-integrate2 가 만드는 sub-directory 의 동적 파일들 정리. 로컬 clone(`${ROOT}`)의 해당 디렉토리를 직접 비운다(서버 tree/DELETE 미사용).

```bash
if [ "$CONFIRM" = "true" ] && [ "$RESET_DOCS" = "true" ]; then
  # 정리 대상 디렉토리 (로컬 clone 기준)
  DIRS=(
    "architecture/architecture-style"
    "architecture/design-decisions"
    "architecture/architecture-integrated"
  )

  for d in "${DIRS[@]}"; do
    TARGET="${ROOT}/${d}"
    [ -d "$TARGET" ] || { echo "CLEAN ${d}/ -> SKIP (없음)"; continue; }
    # 디렉토리 내부 .md 파일만 제거 (서브디렉토리 구조는 유지)
    find "$TARGET" -maxdepth 1 -type f -name '*.md' -print -delete \
      | sed "s|^${ROOT}/|DELETE |"
  done
fi
```

서버측 `POST /Project/{code}/reset` 의 issue 정리는 자기 과제 이슈 (key prefix=CODE) 만 처리한다. 문서 정리는 이제 로컬 clone 단일 작업본에서만 일어나므로 cross-user worktree 잔존 문제는 사라졌다(서버측 user 별 worktree 자체가 제거됨 — 트랙 A/C).

## 5. 최종 보고

- **dry-run**: 이슈 수 / archSettings 존재 여부 / 덮어쓸 파일 목록 + `CONFIRM=true` 재호출 안내
- **executed**: 삭제된 이슈 수/실패 수 + archSettingsCleared + 로컬 문서 write/delete 결과 + 런처 push→PR 안내(로컬 변경은 push 전까지 서버/웹 미반영)

모든 보고는 한국어 마크다운. 실패(4xx/5xx) 는 반드시 보고에 명시.

## 주의

- `CONFIRM=true` 는 반드시 사용자 명시 동의 후에만 설정
- 서버측 `POST /Project/{code}/reset` 이 없는 옛 버전 GoJIRA-API 에서는 404 반환 → 최신 빌드로 업그레이드 필요
- 템플릿 8개는 `.claude/skills/reset-project/templates/` 하위에 있으며 로컬 clone 안에 동봉됨
- 문서 리셋은 **로컬 clone fs 직접 write/delete** — 결과는 런처 push → PR 로만 서버/웹 반영
- **Soft delete 인지** (PR #11): 이슈는 `issues_trash` 컬렉션으로 이동, 3일 후 자동 영구 삭제. 사고 시 3일 내 복구 시도 가능
