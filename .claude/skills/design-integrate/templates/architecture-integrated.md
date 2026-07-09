# Architecture Integrated

> **이 문서는 자동 생성된 index 입니다.** 4 개 view 의 짧은 머리말과 본문 링크, 그리고 view 간 entity 사전을 담습니다.
>
> 본문은 `architecture-integrated/` 하위 파일을 참조하세요.

## 1. View 구성

| View | 본문 | 핵심 관심사 | 추천 PlantUML |
|---|---|---|---|
| Logical | [logical.md](architecture-integrated/logical.md) | 컴포넌트/모듈 관계와 책임 | component |
| Process | [process.md](architecture-integrated/process.md) | 런타임 프로세스 / 통신 / 동시성 | sequence / activity |
| Deployment | [deployment.md](architecture-integrated/deployment.md) | 노드 / 배포 / 네트워크 토폴로지 | deployment (node, artifact) |
| Development | [development.md](architecture-integrated/development.md) | 코드 구조 / 패키지 / 의존 방향 | package / component |

- **선정 스타일**: {Primary} / {Secondary} / {Supporting}
- **반영 ADSN 수**: {N} / **DDSN 수**: {M}

---

## 2. Cross-View Entity Dictionary

view 간 동일 entity 가 동일 ID 로 등장해야 합니다 (validator critical 항목).

| ID | 이름 | 종류 | 등장 View |
|---|---|---|---|
| {ID} | {표시명} | component / process / node / package | logical, process |
| ... | ... | ... | ... |

> 이 표는 Step 1.5 결정적 통합에서 4 view 의 §2 entity 표를 합쳐 자동 채워집니다. 같은 ID 가 view 마다 다른 이름으로 나오면 critical.

---

## 3. View 별 머리말

각 view 의 §1 개요 첫 단락. 본문 전체는 위 §1 의 링크에서 확인하세요.

### 3.1 Logical View

(logical.md §1 개요 첫 단락 — Step 1.5 에서 자동 인용)

### 3.2 Process View

(process.md §1 개요 첫 단락)

### 3.3 Deployment View

(deployment.md §1 개요 첫 단락)

### 3.4 Development View

(development.md §1 개요 첫 단락)

---

## 4. 통합 시점 / 입력

- 생성: {timestamp}
- 입력: ADSN/DDSN ({N}/{M}건), chosenStyles ({primary/secondary/supporting})
- 4 view 모두 분리 산출 후 결정적 통합 (LLM 호출 0회)

---

## 5. 후속 단계 포인트 (SATAM 집중 점검 후보)

- `ADSN_XXX` — Risk {요약}, 영향 QA: ...
- `ADSN_YYY` — Risk {요약}, 영향 QA: ...

(다음 단계 `satam-analyze` 에서 각 ADSN 의 risk/sensitivity/mitigation 원문이 구조화됨)
