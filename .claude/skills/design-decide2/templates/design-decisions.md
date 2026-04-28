# 설계 결정 Index

## 개요

이 문서는 프로젝트의 Architectural Design Decision 전체 목차이며, 각 Design Point 의 상세는 `design-decisions/DP-NN-*.md` 에 수록됩니다.

Design Point (DP) 는 아키텍처 수준에서 결정이 필요한 관심사 단위이며, 각 DP 마다 2개 이상의 후보 대안을 QA(NFR) 기준으로 평가하여 최종 선정안을 도출합니다.

---

## 1. Design Point 목록

| DP | 주제 | Design Objective | designLevel | 선정안 | 상세 |
|---|---|---|---|---|---|
| DP-01 | {주제} | {한 줄 요약} | Architectural | {Candidate / Mix 설명} | [DP-01](./design-decisions/DP-01-{slug}.md) |
| DP-02 | ... | ... | Architectural | ... | [DP-02](./design-decisions/DP-02-{slug}.md) |
| DP-03 | ... | ... | Detailed | ... | [DP-03](./design-decisions/DP-03-{slug}.md) |

- **designLevel**: `Architectural` (시스템 전체 영향) / `Detailed` (컴포넌트·모듈 수준)
- **선정안**: 해당 DP 의 최종 채택 대안. 혼합 채택 시 "Mix of {A}+{C}" 형식

---

## 2. Driver 추적성 매트릭스

각 Architecture Driver 가 어느 Design Point 에서 다뤄지는지의 매트릭스. 모든 Driver 는 최소 1개 DP 에 연결되어야 합니다.

| Driver | 담당 DP | 선정안에서의 반영 |
|---|---|---|
| {Driver-KEY} ({QA/CNST 이름}) | DP-01, DP-03 | {어떻게 해당 Driver 를 만족시키는지 한 줄} |
| ... | ... | ... |

⚠️ 미커버 Driver 는 validator critical.

---

## 3. QA 가중치 요약

DP별 QA 가중치는 각 DP 파일 내부에 기록. 여기는 프로젝트 전반 경향만 요약.

- **핵심 QA**: {Performance / Security / Availability / ...} — 전체 DP 중 {N}개에서 최고 가중 속성
- **가중치 차이 관찰**: DP 별로 강조점이 다름 — 상세는 개별 파일 참조

---

## 4. 리스크 요약

각 DP 의 채택된 후보가 가진 주요 리스크의 프로젝트 수준 요약. SATAM 단계에서 구조화됩니다.

| DP | 주요 리스크 | 영향 QA | 대응 방향 |
|---|---|---|---|
| DP-01 | ... | Performance, Availability | ... |
| ... | ... | ... | ... |
