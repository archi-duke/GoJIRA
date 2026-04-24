# 시스템 스타일 결정 (Integrated Architecture Style)

## 개요

프로젝트의 시스템 스타일은 **2~3개의 통합 스타일 후보** (Primary + Secondary + Supporting 조합) 를 QA(NFR) 가중 평가로 비교하여 최종 선정합니다. 개별 스타일은 ADSN 이슈로 만들지 않고, **각 조합 후보를 ADSN 이슈로** 만듭니다.

각 후보 조합의 상세 비교·다이어그램·점수는 `architecture-style/ADSN_NNN-*.md` 파일에 수록됩니다 (파일명은 이슈 생성 후 실제 subkey 로 치환). Step 1 초안 단계에서는 docId 로 `ADSN-1`, `ADSN-2` 를 쓰고 Step 5.3 저장 시 `ADSN_001`, `ADSN_002` 등으로 치환합니다.

---

## 1. 후보 조합 스타일

| 후보 | Primary | Secondary | Supporting | 총점 | 선정 | 상세 |
|---|---|---|---|---|---|---|
| ADSN_001 | {스타일} | {스타일} | {스타일(들)} | X.XX |  | [ADSN_001](./architecture-style/ADSN_001-{slug}.md) |
| ADSN_002 | {스타일} | {스타일} | {스타일(들)} | X.XX | ✓ | [ADSN_002](./architecture-style/ADSN_002-{slug}.md) |
| ADSN_003 | {스타일} | {스타일} | {스타일(들)} | X.XX |  | [ADSN_003](./architecture-style/ADSN_003-{slug}.md) |

- **총점**: QA 가중 평균 (0.0 ~ 5.0). 가중치는 2 섹션 참조
- **선정**: 최종 선정된 후보. 혼합 채택 시 복수 ✓

---

## 2. QA 가중치

이 스타일 결정 전반에서 후보 비교에 사용된 가중치.

| QA | Weight | 가중치 근거 |
|---|---|---|
| {QA} | 0.XX | {왜 이 QA 가 이 비중인지} |
| **합계** | **1.00** | |

---

## 3. 최종 선정

**선정안**: ADSN_NNN — {조합 이름}
**채택 ADSN 키**: {이슈 키}
**선정 근거**:
- 총점 기준 최고 (X.XX)
- Driver {KEY} ({QA 이름}) 에 대해 {어떻게 만족시키는지}
- 다른 후보 대비 {결정적 차별 포인트}

**(혼합 채택 시)**:
- Candidate X 의 {X 측면}
- Candidate Y 의 {Y 측면}
- 통합 방식: ...

---

## 4. Driver 추적성 매트릭스

선정된 스타일 조합이 각 Architecture Driver 를 어떻게 충족시키는지.

| Driver | 담당 스타일(조합 내) | 역할 |
|---|---|---|
| {Driver-KEY} ({QA/CNST 이름}) | {Primary 의 스타일명} | {어떻게 이 Driver 를 만족시키는지} |
| ... | ... | ... |

---

## 5. 미채택 후보의 트레이드오프

| 후보 | 기각 이유 | 어떤 조건에서 재검토 가능한지 |
|---|---|---|
| ADSN_NNN | ... | ... |
