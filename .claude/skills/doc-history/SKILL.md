---
name: doc-history
description: 프로젝트 문서 저장소(DocRepo)의 커밋 이력을 조회합니다. 브랜치별 커밋 로그와 변경 이력을 확인합니다.
allowed-tools: Bash
---

## 환경

- `PLATFORM_API` 기본값: `http://localhost:9000/api/v2`

## 인자 추출

사용자 메시지에서 다음 값들을 추출해 **먼저** 환경변수로 export 한 뒤 아래 bash 블록들을 실행하라. 누락 시 사용자에게 되물어 확정하라.

- `CODE` — projectCode (필수, 예: GOJIRA)
- `BRANCH` — (선택) 기본 `main`. 사용자 브랜치는 `docs/{user}`
- `LIMIT` — (선택) 기본 `20`

예:
```bash
export CODE="GOJIRA"
# 선택:
# export BRANCH="docs/batman"
# export LIMIT="20"
```

## 규칙

- projectCode는 필수. 없으면 중단.
- branch 미지정 시 `main` 브랜치의 커밋 조회.
- 사용자 브랜치를 보려면 `docs/사용자ID` 형태로 지정.
- 결과는 SHA(7자), 메시지(첫 줄), 작성자, 날짜를 마크다운 표로 정리해 한국어로 보고.

## 실행

```bash
PLATFORM_API="${PLATFORM_API:-http://localhost:9000/api/v2}"
BRANCH="${BRANCH:-main}"
LIMIT="${LIMIT:-20}"

if [ -z "$CODE" ]; then
  echo "ERROR: projectCode 필수"
  exit 1
fi

curl -s "${PLATFORM_API}/DocRepo/${CODE}/commits?branch=$(printf %s "$BRANCH" | jq -sRr @uri)&perPage=${LIMIT}" \
  | jq -r '
      if type == "array" then
        "| SHA | 메시지 | 작성자 | 날짜 |",
        "|-----|--------|--------|------|",
        (.[] | "| \(.sha[0:7]) | \((.commit.message // "") | split("\n")[0]) | \(.commit.author.name // "") | \(.commit.author.date // "") |")
      else
        "조회 실패 또는 결과 없음: \(.)"
      end
    '
```

## 보고 형식

1. 상단에 `프로젝트 {CODE} / 브랜치 {BRANCH} — 커밋 {N}건` 요약.
2. 이어서 마크다운 표 출력.
