---
name: archive-project
description: "프로젝트를 백업(Mongo dump + 로컬 레포 폴더) 후 완전 삭제. Gitea 원격·로컬 클론·Mongo 이슈·archSettings·projects 문서 모두 제거. 백업은 $HOME/.gojira-backups/{code}-{timestamp}.zip 으로 저장되어 나중에 restore-project 로 복원 가능. 파괴적 작업이므로 CONFIRM=true 없이는 dry-run 만."
allowed-tools: Bash
---

## 이 스킬을 언제 호출하나

사용자 요청에 "프로젝트 삭제", "아카이브", "백업하고 지워", "완전 삭제" 등이 포함될 때. 단순 컨텐츠 초기화만 원하면 `reset-project` 로 안내.

reset-project 와의 차이:
- **reset-project**: 이슈/archSettings/문서만 초기화. 프로젝트 문서는 유지 (재사용 가능)
- **archive-project (이 스킬)**: 백업 zip 을 만든 뒤 프로젝트 자체를 완전 제거 (Mongo 문서 + Gitea 레포 + 로컬 클론)

## 환경

- `PLATFORM_API` 기본값: `http://localhost:9000/api/v2`
- `GOJIRA_API` 기본값: `http://localhost:9001/api/v2`
- `GOJIRA_BACKUP_DIR` 기본값: `$HOME/.gojira-backups`
- `REPO_BASE_DIR` 기본값: `/tmp/gojira-repos` (Platform-API 의 bare repo root)

## 인자 추출

- `CODE` — projectCode (필수)
- `USER` — user (필수, `X-User-Id`)
- `CONFIRM` — `true` 면 실제 실행, 그 외 dry-run. **절대 기본 true 로 두지 말 것**

예 (dry-run):
```bash
export CODE="GOJIRA3"
export USER="batman"
```

실제 실행 (사용자 명시 동의 후):
```bash
export CONFIRM="true"
```

## 경고

**파괴적 + 비가역 작업**입니다.
- Mongo `projects` 문서, `issues` 컬렉션의 해당 이슈, Gitea 원격 레포, 로컬 bare repo 폴더 **모두** 삭제
- 복구 수단은 **백업 zip 하나뿐** — 백업 생성 실패 시 삭제도 진행하지 않음 (fail-fast)
- dry-run 을 먼저 보이고, 사용자 "진행/확인/yes" 명시 동의 후에만 `CONFIRM=true`

## 1. 사전 체크

```bash
PLATFORM_API="${PLATFORM_API:-http://localhost:9000/api/v2}"
GOJIRA_API="${GOJIRA_API:-http://localhost:9001/api/v2}"
BACKUP_DIR="${GOJIRA_BACKUP_DIR:-$HOME/.gojira-backups}"
REPO_BASE_DIR="${REPO_BASE_DIR:-/tmp/gojira-repos}"
CONFIRM="${CONFIRM:-false}"

if [ -z "$CODE" ] || [ -z "$USER" ]; then
  echo "ERROR: CODE 와 USER 모두 필수"; exit 1
fi

mkdir -p "$BACKUP_DIR"
```

## 2. 영향 범위 산정 — dry-run

```bash
# reset dry-run 으로 이슈 수 + archSettings 상태 확인
IMPACT=$(curl -s -X POST "${GOJIRA_API}/Project/${CODE}/reset" \
  -H "Content-Type: application/json" -H "X-User-Id: ${USER}" \
  -d '{"confirm":false,"resetIssues":true,"resetArchSettings":true,"deleteProject":true}')
echo "$IMPACT" | jq '{jiraCode, wouldDelete}'

# 프로젝트 문서에서 repo 정보 확인
PRJ=$(curl -s "${GOJIRA_API}/Project" -H "XBody: {\"jiraCode\":\"${CODE}\"}" -H "X-User-Id: ${USER}")
REPO_OWNER=$(echo "$PRJ" | jq -r '.[0].documents.owner // empty')
REPO_NAME=$(echo "$PRJ" | jq -r '.[0].documents.repoName // empty')
REPO_PATH="${REPO_BASE_DIR}/${REPO_OWNER}/${REPO_NAME}.git"
echo "repo: $REPO_PATH"
[ -d "$REPO_PATH" ] && echo "  (exists)" || echo "  (MISSING — Gitea 삭제 스킵될 수 있음)"

TIMESTAMP=$(date +%Y%m%d_%H%M%S)
ARCHIVE_PATH="${BACKUP_DIR}/${CODE}-${TIMESTAMP}.zip"
echo "archive: $ARCHIVE_PATH"
```

## 3. Dry-run 최종 보고 (confirm != true)

`CONFIRM` 이 `true` 가 아니면 Step 2 결과 + 백업 예상 경로를 보고하고 **종료**. 실제 실행 금지. 사용자에게 "`CONFIRM=true` 로 재호출 필요" 안내.

## 4. 실제 실행 (confirm=true)

### 4-1. 작업 디렉토리 준비

```bash
WORK_DIR=$(mktemp -d -t gojira-archive-XXXXXX)
mkdir -p "${WORK_DIR}/mongo" "${WORK_DIR}/repo"
echo "작업 디렉토리: $WORK_DIR"
```

### 4-2. Mongo dump → mongo/*.json

```bash
DUMP=$(curl -s "${GOJIRA_API}/Project/${CODE}/dump" -H "X-User-Id: ${USER}")
if [ "$(echo "$DUMP" | jq -r '.error // empty')" != "" ]; then
  echo "ERROR: dump 실패 — $(echo $DUMP | jq -r .error)"; rm -rf "$WORK_DIR"; exit 1
fi

echo "$DUMP" | jq '.project' > "${WORK_DIR}/mongo/project.json"
echo "$DUMP" | jq '.issues'  > "${WORK_DIR}/mongo/issues.json"
echo "mongo dump: $(jq 'length' < ${WORK_DIR}/mongo/issues.json) 이슈"
```

### 4-3. 로컬 bare repo 복사 → repo/

```bash
if [ -n "$REPO_OWNER" ] && [ -n "$REPO_NAME" ] && [ -d "$REPO_PATH" ]; then
  # bare repo 전체를 repo/ 아래로 복사 (.git 내용이 그대로 들어감)
  cp -R "$REPO_PATH" "${WORK_DIR}/repo/${REPO_NAME}.git"
  echo "repo copied: $(du -sh ${WORK_DIR}/repo | awk '{print $1}')"
else
  echo "WARNING: bare repo 경로 없음 — repo/ 비워둠"
fi
```

### 4-4. manifest.json 작성

```bash
cat > "${WORK_DIR}/manifest.json" <<MANIFEST
{
  "schemaVersion": "1",
  "jiraCode": "${CODE}",
  "createdAt": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "createdBy": "${USER}",
  "mongo": {
    "project": "mongo/project.json",
    "issues": "mongo/issues.json"
  },
  "repo": {
    "owner": "${REPO_OWNER}",
    "repoName": "${REPO_NAME}",
    "path": "repo/${REPO_NAME}.git"
  }
}
MANIFEST
cat "${WORK_DIR}/manifest.json"
```

### 4-5. zip 으로 묶기

```bash
(cd "$WORK_DIR" && zip -qr "$ARCHIVE_PATH" manifest.json mongo repo)
ls -lh "$ARCHIVE_PATH"
rm -rf "$WORK_DIR"

# zip 생성 실패 시 fail-fast — 삭제 진행하지 않음
if [ ! -f "$ARCHIVE_PATH" ]; then
  echo "ERROR: 백업 zip 생성 실패 — 삭제 중단"; exit 2
fi
```

### 4-6. Gitea 원격 + 로컬 클론 삭제

```bash
curl -s -X DELETE "${PLATFORM_API}/DocRepo/${CODE}/github" \
  -H "X-User-Id: ${USER}" \
  -w "\nGITEA DELETE -> %{http_code}\n"
```

### 4-7. Mongo hard delete (이슈 + archSettings + projects 문서)

```bash
curl -s -X POST "${GOJIRA_API}/Project/${CODE}/reset" \
  -H "Content-Type: application/json" -H "X-User-Id: ${USER}" \
  -d '{"confirm":true,"resetIssues":true,"resetArchSettings":true,"deleteProject":true}' \
  | jq '{mode, deletedCount, failedCount, archSettingsCleared, projectDeleted}'
```

## 5. 최종 보고

```
## 프로젝트 아카이브 완료

- projectCode: ${CODE}
- 백업 zip: ${ARCHIVE_PATH} (크기 N)
- 백업 내용:
  - manifest.json
  - mongo/project.json, mongo/issues.json (N개 이슈)
  - repo/${REPO_NAME}.git/ (bare repo 전체)
- Gitea 삭제: HTTP {code}
- Mongo hard delete: {이슈 N건 / archSettings / projects 문서}
- 복원: 차후 `restore-project` 스킬 (schemaVersion=1 호환)
```

dry-run 은 영향 범위 + 백업 예상 경로 + "CONFIRM=true 재호출 필요" 안내.

## 주의

- `CONFIRM=true` 는 반드시 사용자 명시 동의 후에만
- 백업 zip 생성 실패 시 삭제 단계 (4-6/4-7) 진행 금지 — 데이터 손실 방지
- bare repo 경로가 다르면 `REPO_BASE_DIR` 환경변수로 override
- 백업 zip 의 schemaVersion 은 차후 restore-project 의 호환성 체크에 사용
- zip 도구: macOS/Linux 기본 `zip` 명령 사용 (미설치 환경 대비 필요 시 Python `zipfile` 로 대체)
