# TC2: 문서 이력관리 및 PR 알림 흐름

## 목적
문서 관리의 이력관리 기능과 실시간 알림이 올바로 동작하는지 검증한다.
- 문서 편집/저장 → 상태 배지 변화
- 발행신청(PR) → 실시간 알림 수신
- PR 도착 확인

## 사전 조건
- TC1(tc1-project-create.md) 성공 — E2E-TEST-PRJ 프로젝트가 존재하는 상태
- Platform-App(:3000), GoJIRA-App(:3001), Platform-API(:9000), GoJIRA-API(:9001) 가동
- Gitea 정상 동작

## 테스트 환경
- 브라우저 A: 사용자 duke.kimm (리뷰어 역할)
- 브라우저 B: 사용자 batman (작성자 역할)
- 테스트 대상 프로젝트: E2E-TEST-PRJ (E2ETEST)

---

## 시나리오

### Step 1: 초기 상태 — 두 브라우저에서 ProjectMap 진입
| 구분 | 동작 | 예상 결과 |
|------|------|----------|
| A | http://localhost:3000 접속 | ProjectMap 표시 |
| A | 사용자 셀렉터에서 duke.kimm 선택 | 사용자 전환, ProjectMap 새로고침 |
| B | http://localhost:3000 접속 (별도 브라우저/시크릿) | ProjectMap 표시 |
| B | 사용자 셀렉터에서 batman 선택 | 사용자 전환, ProjectMap 새로고침 |

**확인**: 양쪽 모두 프로젝트 목록이 정상 표시되고, 알림 아이콘 없는 상태

### Step 2: 작업공간 동기화 (Pull)
| 구분 | 동작 | 예상 결과 |
|------|------|----------|
| A | E2E-TEST-PRJ 클릭 → Docs 탭 | DocEditor 진입, 작업공간 탭에 파일 목록 표시 |
| A | Pull 버튼(git-fetch 아이콘) 클릭 → 확인 | "main 동기화 완료" 알림, 파일 목록 갱신 |
| A | Project 탭 클릭 → ProjectMap 복귀 | ProjectMap에서 E2E-TEST-PRJ의 알림 아이콘이 **제거**됨 (branchDiff 해소) |
| B | E2E-TEST-PRJ 클릭 → Docs 탭 | DocEditor 진입 |
| B | Pull 버튼 클릭 → 확인 | "main 동기화 완료" 알림 |
| B | Project 탭 클릭 → ProjectMap 복귀 | ProjectMap에서 E2E-TEST-PRJ의 알림 아이콘이 **제거**됨 |

**확인**: 양쪽 모두 main과 동일한 최신 상태, ProjectMap에 알림 아이콘 없음

### Step 3: 문서 편집 및 저장 (B)
| 구분 | 동작 | 예상 결과 |
|------|------|----------|
| B | 편집모드 ON 클릭 | 편집모드 활성화 (+ 버튼, 발행신청 버튼 표시) |
| B | project-description.md 클릭 → 편집 버튼 클릭 | 텍스트 편집기(textarea) 표시 |
| B | 내용 임의 수정 | 파일명 옆에 ● 표시 (변경됨), 저장 버튼 활성화 |
| B | 저장 버튼 클릭 → "저장(커밋)하시겠습니까?" 확인 | "저장 완료" 알림 |

**확인 (B)**:
- 파일 목록에서 project-description.md 옆에 **M 배지** (modified, 주황색) 표시
- 발행신청 버튼이 활성화 상태 (진한 배경색)
- 발행신청 버튼 좌측에 **git-pull.svg 아이콘** 표시

### Step 4: 발행신청 — PR 생성 (B)
| 구분 | 동작 | 예상 결과 |
|------|------|----------|
| B | 발행신청 버튼 클릭 | "발행 신청(PR)하시겠습니까?" 확인 다이얼로그 |
| B | 확인 클릭 | "발행 신청(PR)이 생성되었습니다." 알림 |

**확인 (B)**: PR 생성 성공 메시지

### Step 5: PR 수신 알림 확인 (A)
| 구분 | 동작 | 예상 결과 |
|------|------|----------|
| A | (자동) ProjectMap에서 E2E-TEST-PRJ 행 확인 | 프로젝트명 색상이 **#27ae60** (녹색)으로 변경 |
| A | (자동) E2E-TEST-PRJ 옆 아이콘 확인 | **alert-pr.svg** 아이콘 표시 (PR 수신) |

**확인 (A)**: WebSocket을 통해 hasOpenPR 상태가 실시간 반영됨

### Step 6: PR 확인 (A)
| 구분 | 동작 | 예상 결과 |
|------|------|----------|
| A | Repository 탭 클릭 | DocManager 진입 |
| A | 프로젝트 선택 → PRs 탭 | PR 목록 표시 |
| A | 방금 생성된 PR 확인 | title: "[E2ETEST] 설계문서 배포 요청", state: open, user: batman |

**확인 (A)**: PR이 정상 도착, 리뷰 상태 "대기중"

---

## 검증 항목 요약

| # | 검증 항목 | Step | 기대 동작 |
|---|----------|------|----------|
| V1 | Pull 동기화 + 알림 해소 | 2 | main 동기화 후 ProjectMap 알림 아이콘 제거 |
| V2 | 편집/저장 후 상태 배지 | 3 | M 배지 표시, 발행신청 활성화 |
| V3 | PR 생성 | 4 | 정상 생성, 성공 알림 |
| V4 | hasOpenPR 실시간 알림 | 5 | A의 ProjectMap에 녹색 + alert-pr.svg |
| V5 | PR 목록 확인 | 6 | Repository에서 PR 도착 확인 |

## 비고
- WebSocket 연결이 끊어진 상태에서는 실시간 알림이 동작하지 않음 (3초 자동 재연결)
- 사용자 전환 시 sessionStorage가 초기화되므로 이전 세션 상태는 유지되지 않음
- PR 색상(녹색)이 branchDiff 색상(빨간색)보다 우선하여 표시됨
