---
name: project-query
description: GoJIRA 프로젝트 목록을 조회합니다. 프로젝트 수, 이름, 상태를 확인할 때 사용합니다.
allowed-tools: Bash
---

프로젝트 목록을 조회합니다.

## 전체 조회
!`gojira-cli project list`

## 특정 프로젝트 조회
특정 프로젝트만 조회하려면:
```
gojira-cli project list --code $ARGUMENTS
```

결과를 한국어로 정리하여 사용자에게 보여주세요.
프로젝트 수, 이름, 상태를 마크다운 표로 정리합니다.
