# GoJIRA 프로젝트 클론 — 에이전트 세션 메모리

<!--
이 파일은 gojira-launcher 가 각 프로젝트 clone 에 배포하는 프로젝트 메모리다.
원본: gojira-launcher/assets-extra/CLAUDE.md
배포 체인: build-launcher.sh 가 assets/.claude/CLAUDE.md 로 복사 → exe 임베드 →
런처가 ~/.gojira/skills/.claude/CLAUDE.md 로 추출 → ensureAgentAssets 가 clone 의
.claude 로 연결(junction/symlink). GoJIRA 개발 레포 자체에는 적용되지 않는다.
-->

## 웹발 이슈 변경 저널

세션 시작 시 env `GOJIRA_ISSUE_JOURNAL` 이 가리키는 파일이 **존재하고 비어있지 않으면**,
웹에서 발생한 이슈 변경(생성/수정/삭제/링크)이 설계 일관성 검토를 기다리고 있다.

- `.claude/skills/_shared/issue-journal.md` 의 지침대로 항목별 영향(SATAM 매트릭스,
  유스케이스, 설계 문서, 링크 정합)을 검토·반영하라.
- 처리 완료한 항목은 저널 파일에서 제거하고, 보류 항목은 남긴 채 사용자에게 보고하라.
- 파일이 없거나 비어 있으면 해당 없음 — 사용자 요청 작업을 바로 진행한다.
