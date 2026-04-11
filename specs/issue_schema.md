# 타입별 Issue 필드 정의

### 공통

| 필드명 | 타입 | JIRA 필드 | 설명 |
|--------|------|-----------|------|
| key | string | issue key | 이슈 키 |
| id | string | issue id | 이슈 ID |
| duedate | string | issue duedate | 이슈 마감일 |
| issueType | string | issue type | 이슈 타입 |
| subkey | string | customfield_15209 | 서브 키 |
| components | array | issue components | Issue components의 name을 list로 |
| summary | string | issue summary | 이슈 요약 |
| description | string | issue description | 이슈 설명 |
| labels | array | issue labels | 이슈 라벨 |
| status | object | issue status | 이슈 상태 (`{"id":3, "name":""}`) |
| resolution | object | issue resolution | 이슈 해결 (`{"id":"", "name":""}`) |
| reporter | object | issue reporter | 이슈 보고자 (`{"id":"", "name":""}`) |
| issueLinks | array | issue Links | 이슈 링크 (`[{"id":"", "type":{"name":""}, "inwardIssue":{"key":""}, "outwardIssue":{"key":""}}]`) |
| attachment | array | attachment | 첨부파일 |
| priority | object | priority | 우선순위 (`{"id":"", "name":""}`) |
| environment | string | issue environment | 환경 정보 (Requirement에서는 품질 속성 시나리오의 환경/측정 조건으로 사용) |
| Features | array | customfield_15248 | 이슈와 연관된 Feature 목록(key 값 사용) |
| Requirements | array | customfield_15249 | 이슈와 연관된 Requirements 목록(key 값 사용) |
| VOS | array | customfield_15247 | 이슈와 연관된 VOS 목록(key 값 사용) |
| Test | array | customfield_18201 | 이슈와 연관된 Test 목록(key 값 사용) |
| A-Design | array | customfield_18202 | 이슈와 연관된 Architectural Design(ADSN) 목록(key 값 사용) |
| D-Design | array | customfield_18203 | 이슈와 연관된 Detailed Design(DDSN) 목록(key 값 사용) |

### VOS

| 필드명 | 타입 | JIRA 필드 | 설명 |
|--------|------|-----------|------|
| client | string | customfield_15235 | VOS를 요청한 사람 (knoxID) |

### Feature

| 필드명 | 타입 | JIRA 필드 | 설명 |
|--------|------|-----------|------|
| actor | string | customfield_15215 | Use Case의 Story에서 행위 주체 |
| precondition | string | customfield_15216 | Use Case의 Story에서 진입시점 상태 |
| basicflow | string | customfield_15217 | Use Case의 Story에서 기본 작업 흐름 |
| alternativeflow | string | customfield_15218 | Use Case의 Story에서 발생가능한 대체 또는 경우에 따른 작업 흐름 |
| exceptionflow | string | customfield_15219 | Use Case의 Story에서 예외 발생시 처리 과정 |
| postcondition | string | customfield_15220 | Use Case의 Story에서 Use Case가 종료됐을때의 상태 |

### Requirement

| 필드명 | 타입 | JIRA 필드 | 설명 |
|--------|------|-----------|------|
| reqtype | string | customfield_12555 | 요구사항의 종류 (FR:기능, NFR 또는 QA:비기능 품질요구사항, CNST:제약사항) |
| qa | array | customfield_18217 | NFR,QA일 경우 품질속성 이름 |
| source | string | customfield_15210 | NFR,QA일 경우 품질 속성 시나리오에서 Source(자극, Stimulus를 발생시키는 주체) |
| stimulus | string | customfield_15211 | NFR,QA일 경우 품질 속성 시나리오에서 Stimulus(Source로부터 생성됨) |
| environment | string | issue environment (standard) | NFR,QA일 경우 품질 속성 시나리오에서 환경 (측정의 조건이 되기도 함) |
| artifact | string | customfield_15208 | NFR,QA일 경우 품질 속성 시나리오에서 자극을 받아 반응(Response)를 생성시키는 요소 |
| response | string | customfield_15212 | NFR,QA일 경우 품질 속성 시나리오에서 반응(자극에 대한 Artifact의 반응) |
| measure | string | customfield_15213 | NFR,QA일 경우 품질 속성 시나리오에서 반응의 측정값과 범위(반드시 측정가능한 수치) |
| responseScenario | string | customfield_15207 | NFR,QA일 경우 품질 속성 시나리오에서 요약 |
| qai | string | customfield_15204 | NFR,QA일 경우 품질 속성의 평가 점수중 중요도 |
| qad | string | customfield_15205 | NFR,QA일 경우 품질 속성의 평가 점수중 난이도 |
| qas | string | customfield_15206 | NFR,QA일 경우 품질 속성의 평가 종합 점수 (일반적으로 QAI,QAD의 평균 또는 가중 평균) |

### Architectural Design (ADSN) / Detailed Design (DDSN)

| 필드명 | 타입 | JIRA 필드 | 적용 | 설명 |
|--------|------|-----------|------|------|
| dmodel | string | customfield_18218 | ADSN | 디자인이 적용된 도메인 모델(또는 시스템)의 영역 |
| archdiagram | string | customfield_18215 | ADSN | 디자인의 통합 구조 |
| objectives | string | customfield_18206 | ADSN | 디자인의 주요 목표 |
| pros | string | customfield_18207 | ADSN | 디자인의 장점 |
| cons | string | customfield_18208 | ADSN | 디자인의 단점 |
| score | string | customfield_18209 | ADSN | 디자인의 평가 점수 (주로 5점 또는 10점 기준) |
| eval | string | customfield_18210 | ADSN | 디자인의 평가 내용 |
| sensitivity | string | customfield_18212 | ADSN | 디자인이 악영향을 주는 Requirement과 내용. 현재 기준으로 Risk는 아니지만 조건이 변하면 Risk로 발전할 수 있는 내용 (Sensitive 양식 참조) |
| nonrisk | string | customfield_18214 | ADSN | 디자인이 좋은 영향을 주는 Requirement과 내용 (사용안함, reserved) |
| mitigation | string | customfield_18213 | ADSN | 디자인이 악영향을 감소시키는 대상 Risk 정보 (Mitigation 양식 참조) |
| risk | string | customfield_18216 | ADSN | 디자인이 악영향을 주는 Requirement과 내용. 현재 기준으로 명확히 해당 요구사항을 달성하는데 장애가 확정적인 사항 (Risk 양식 참조) |
| decision | string | customfield_18211 | ADSN | 디자인 채택여부. 이 필드가 비어있지 않으면 최종 선택된 설계안 (내용은 무관) |
| adiagram | string | customfield_18204 | ADSN+DDSN | 디자인의 구조 다이어그램(정적) |
| bdiagram | string | customfield_18205 | ADSN+DDSN | 디자인의 동적 동작 다이어그램 |

---

## Sensitivity / Risk / Mitigation 기록 양식

`sensitivity`, `risk`, `mitigation` 필드는 YAML-like 형식의 문자열로 저장되며, 여러 항목을 개행으로 구분하여 기록한다.
파싱 코드: `GoJIRA-App/src/components/SATAM.js` — `parseRST()` / `serializeRST()`

### 공통 구조

```yaml
- ID : {접두어}{번호}
  QA : [{Requirement Issue Key}, ...]
  Description : {설명}
```

| 항목 | 설명 |
|------|------|
| ID | 항목 식별자. 접두어 + 순번 (예: R1, S1, M1) |
| QA | 영향을 받는 Requirement 이슈 키 목록 (대괄호로 감싸고 콤마 구분) |
| Description | 항목에 대한 설명 텍스트 |

### ID 접두어 규칙

| 타입 | 접두어 | 예시 |
|------|--------|------|
| Risk | R | R1, R2, R3 |
| Sensitivity | S | S1, S2, S3 |
| Mitigation | M | M1, M2, M3 |

### Risk 양식

설계가 요구사항 달성에 확정적인 장애가 되는 사항을 기록한다.

```yaml
- ID : R1
  QA : [GOJIRA-20]
  Description : rrr
```

### Sensitivity 양식

현재는 Risk가 아니지만, 조건이 변하면 Risk로 발전할 수 있는 사항을 기록한다.

```yaml
- ID : S1
  QA : [GOJIRA-20]
  Description : ssss
```

### Mitigation 양식

Risk의 악영향을 감소시키는 대응 조치를 기록한다. **Counter Risk** 필드가 추가된다.

```yaml
- ID : M1
  QA : [GOJIRA-20]
  Counter Risk : ["R1:GOJIRA-20"]
  Description : mmm1111
```

| 항목 | 설명 |
|------|------|
| Counter Risk | 이 Mitigation이 대응하는 Risk 정보. `"{RiskID}:{Requirement Issue Key}"` 형식으로 기록하며, 여러 개일 경우 콤마로 구분한다 |

### 복수 항목 기록 예시

하나의 필드에 여러 항목을 기록할 때는 연속으로 나열한다.

```yaml
- ID : R1
  QA : [GOJIRA-20, GOJIRA-24]
  Description : DB 스키마 마이그레이션 필요
- ID : R2
  QA : [GOJIRA-25]
  Description : 외부 API 응답 지연 가능성
```

### 파싱 결과 (JSON)

위 양식은 프론트엔드에서 다음과 같은 JSON 배열로 파싱된다.

```json
[
  {
    "id": "R1",
    "qa": ["GOJIRA-20", "GOJIRA-24"],
    "counterRisk": [],
    "description": "DB 스키마 마이그레이션 필요"
  },
  {
    "id": "R2",
    "qa": ["GOJIRA-25"],
    "counterRisk": [],
    "description": "외부 API 응답 지연 가능성"
  }
]
```
