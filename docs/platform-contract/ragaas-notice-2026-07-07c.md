# [통지] Account/Notice 앱 스코핑 — RAGaaS 쪽 조치 불필요 (2026-07-07)

> **대상**: RAGaaS 팀. 참고용 — 특별한 조치를 요청하는 문서가 아닙니다.

## 무엇이 바뀌었나

계약 04(web-ui)/03(공지)에 따라 Account(사용자·권한)와 Notice(공지)는 계속 **Platform-API/셸
단독 소유**입니다. 다만 데이터를 앱별로 분리했습니다 — 같은 사람(knoxId)이 GoJIRA와 RAGaaS에서
서로 다른 권한을 가질 수 있고, 공지도 앱별로 따로 관리됩니다. 상세 규약: 계약 05 §5.5.

## RAGaaS 쪽 조치

**없습니다.** 셸의 Account/Notice 관리 화면과 NoticePopup 이 이미 `app=ragaas` 로 호출하도록
갱신되었고, RAGaaS 코드는 이 API 들을 직접 만들지 않으므로 영향이 없습니다.

## 참고 — RAGaaS 관리자 화면 이용 방법

`/ragaas` 스코프에는 아직 authGroup 이 하나도 없어 시작 상태입니다(의도된 것 — "RAGaaS 는 빈
상태로 시작"). 플랫폼 마스터가 `/ragaas` 에 처음 들어가면 부트스트랩 폴백으로 임시 마스터
권한을 받아 Account 화면에서 RAGaaS 전용 권한그룹(예: MASTER)을 만들고, 실제 RAGaaS 관리자로
지정할 사람을 그 그룹에 배정할 수 있습니다. 그룹이 생기고 나면 그 뒤로는 RAGaaS 스코프 전용
그룹으로만 권한이 계산됩니다.

## 향후 필요해질 수 있는 것 (지금 요청 아님)

RAGaaS 쪽에서 자체 화면 안에 공지 위젯을 넣고 싶다면 계약 03 의 `platformApp/notice`
(`NoticeCenter`) 를 마운트하면 됩니다 — 그때도 `app` 파라미터는 셸이 자동으로 넘기므로 신경 쓸
필요 없습니다.
