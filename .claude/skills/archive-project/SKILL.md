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

- `PLATFORM_API` 기본값: `http://10.211.55.3:44350/api/v2`
- 백업 위치는 Platform-API 가 결정 (`$HOME/.gojira-backups/{code}-{timestamp}.zip`)
- 모든 단계 (dump/zip/repo 정리/Mongo hard delete) 는 `POST /Project/{code}/archive` 한 번 호출로 처리. UI 의 삭제 버튼과 동일 경로.

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
- 복구 수단은 **백업 zip 하나뿐** — endpoint 가 zip 생성 실패하면 삭제 진행 안 함 (fail-fast)
- dry-run 을 먼저 보이고, 사용자 "진행/확인/yes" 명시 동의 후에만 `CONFIRM=true`

## 1. 사전 체크

```bash
PLATFORM_API="${PLATFORM_API:-http://10.211.55.3:44350/api/v2}"
CONFIRM="${CONFIRM:-false}"

if [ -z "$CODE" ] || [ -z "$USER" ]; then
  echo "ERROR: CODE 와 USER 모두 필수"; exit 1
fi
```

## 2. Dry-run — 영향 범위 확인

```bash
DRY=$(curl -s -X POST "${PLATFORM_API}/Project/${CODE}/archive" \
  -H "Content-Type: application/json" -H "X-User-Id: ${USER}" \
  -d '{"confirm":false}')
echo "$DRY" | jq .
```

응답 예시:
```json
{
  "code": "GOJIRA3",
  "mode": "dry-run",
  "repoOwner": "gojira",
  "repoName": "gojira3-prj",
  "dumpBytes": 12345,
  "message": "DRY-RUN. confirm=true 로 재호출 시 zip 생성 + 자원 정리 실행"
}
```

## 3. Dry-run 최종 보고 (confirm != true)

`CONFIRM` 이 `true` 가 아니면 Step 2 결과를 보고하고 **종료**. 실제 실행 금지. 사용자에게 "`CONFIRM=true` 로 재호출 필요" 안내.

## 4. 실제 실행 (confirm=true)

```bash
RESULT=$(curl -s -X POST "${PLATFORM_API}/Project/${CODE}/archive" \
  -H "Content-Type: application/json" -H "X-User-Id: ${USER}" \
  -d '{"confirm":true}')
echo "$RESULT" | jq .
```

응답:
- `mode`: "executed"
- `backupPath`: `$HOME/.gojira-backups/{code}-{timestamp}.zip`
- `repoArchived`: bare repo 가 zip 에 포함됐는지
- `githubDeleted`: GitHub repo 삭제 여부
- `reset`: GoJIRA-API 의 hard-delete 결과 (`{deletedCount, archSettingsCleared, projectDeleted, ...}`)

zip 안 구조 (참고): `mongo/dump.json` + `manifest.json` + `repo/{owner}/{repoName}.git/...`

## 5. 최종 보고

```
## 프로젝트 아카이브 완료

- projectCode: ${CODE}
- 백업 zip: {backupPath}
- 백업 내용 (zip): mongo/dump.json + manifest.json + repo/{owner}/{repoName}.git/
- GitHub 삭제: {githubDeleted}
- Mongo hard delete: {reset.deletedCount} 이슈 / archSettings / projects 문서
- 복원: 차후 `restore-project` 스킬 (schemaVersion=1 호환)
```

dry-run 은 영향 범위 + 백업 위치 안내 + "CONFIRM=true 재호출 필요" 안내.

## 주의

- `CONFIRM=true` 는 반드시 사용자 명시 동의 후에만
- 단일 endpoint 가 zip 생성 → 자원 정리 → hard delete 를 atomic 흐름으로 처리. zip 생성 실패 시 endpoint 가 정리 단계로 진행하지 않으므로 데이터 안전
- 백업 zip 의 schemaVersion 은 차후 restore-project 의 호환성 체크에 사용
- 이전(2026-04-27 이전) 버전의 SKILL.md 는 dump/copy/zip/delete repo/reset 을 단계별 bash 로 직접 수행했음. 이 endpoint 가 같은 흐름을 Go 로 옮긴 것이라 결과는 동일
