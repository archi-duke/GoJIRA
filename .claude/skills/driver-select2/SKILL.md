---
name: driver-select2
description: Requirement(FR/NFR/CNST) 중 아키텍처 결정에 영향이 큰 항목을 Architecture Driver 로 선정하고 QAD/QAI/Priority/risk/sensitivity 를 평가합니다. 해당 Requirement 를 업데이트(ASR 라벨 부여)하고 architecture/drivers.md + 정규화 시스템 속성(style_define 입력)을 산출합니다. Self-loop 자가검증(최대 3회). Claude Code 네이티브(curl/jq).
allowed-tools: Bash, Read
---

## 환경
- `PLATFORM_API` 기본값: `http://localhost:9000/api/v2`
- `GOJIRA_API` 기본값: `http://localhost:9001/api/v2`
- 모든 요청에 `X-User-Id: ${USER}` 헤더 필수

## 인자 추출

사용자 메시지에서 다음 값들을 추출해 **먼저** 환경변수로 export 한 뒤 아래 bash 블록들을 실행하라. 누락 시 사용자에게 되물어 확정하라.

- `CODE` — projectCode (예: GOJIRA3)
- `USER` — user (예: batman)
- `W_QAD` — (선택) qad 가중치, 기본 `0.5`
- `W_QAI` — (선택) qai 가중치, 기본 `0.5`

예:
```bash
export CODE="GOJIRA3"
export USER="batman"
# 선택:
# export W_QAD="0.6"
# export W_QAI="0.4"
```

## 역할
노련한 소프트웨어 아키텍트. 요구사항 중 **ASR(Architecturally Significant Requirement)** 를 선별하고, 각 드라이버에 QAD/QAI/Priority/risk/sensitivity/카테고리를 부여. 부산물로 정규화된 시스템 속성(style_define 입력)을 산출.

---

## Step 0 — 데이터 수집

```bash
PLATFORM_API="${PLATFORM_API:-http://localhost:9000/api/v2}"
GOJIRA_API="${GOJIRA_API:-http://localhost:9001/api/v2}"

# project-description.md (main 우선, 없으면 docs/{user})
DESC=$(curl -s "${PLATFORM_API}/DocRepo/${CODE}/file?path=business/project-description.md&ref=main" \
  -H "X-User-Id: ${USER}" | jq -r '.content // empty' | base64 -d 2>/dev/null)
if [ -z "$DESC" ] || [ $(echo -n "$DESC" | wc -c) -lt 50 ]; then
  DESC=$(curl -s "${PLATFORM_API}/DocRepo/${CODE}/file?path=business/project-description.md&ref=docs/${USER}" \
    -H "X-User-Id: ${USER}" | jq -r '.content // empty' | base64 -d 2>/dev/null)
fi
echo "=== project-description.md ==="
echo "$DESC"

# Requirement 목록
echo
echo "=== Requirement 목록 ==="
curl -s "${GOJIRA_API}/Issue" \
  -H "XBody: {\"prjID\":\"${CODE}\",\"infoType\":\"\"}" \
  -H "X-User-Id: ${USER}" \
  | jq '[.[] | select(.issueType=="Requirement") | {key, subkey, summary, description, reqtype, qa, measure}]'

# Feature (참고)
echo
echo "=== Feature (참고) ==="
curl -s "${GOJIRA_API}/Issue" \
  -H "XBody: {\"prjID\":\"${CODE}\",\"infoType\":\"\"}" \
  -H "X-User-Id: ${USER}" \
  | jq '[.[] | select(.issueType=="Feature") | {key, summary}]'

# priorityWeight (projects.archSettings 에서 조회 — 없으면 0.5/0.5)
# W_QAD/W_QAI 가 미리 export 되어 있으면 사용, 아니면 기본 0.5/0.5
W_QAD="${W_QAD:-0.5}"; W_QAI="${W_QAI:-0.5}"
echo "priorityWeight: qad=${W_QAD}, qai=${W_QAI}"
```

Requirement 가 0건이면 **중단** 하고 "requirement_refine 을 먼저 실행하세요" 보고.

---

## Step 1 — 초안 생성 (iter 1)

### 1.1 Driver 후보 검토
- 모든 NFR 은 기본 Driver 후보
- FR 중 **구조적 복잡도가 큰 것**(분산/이벤트/대량 처리) 포함
- CNST 중 **아키텍처 선택을 제한하는 제약**(기술/규정/플랫폼) 포함

### 1.2 Driver 카테고리 태깅
각 driver 에 `category` 지정:
- `Functional` — FR 기반 구조 드라이버
- `QualityAttribute` — NFR (성능/가용성/보안 등)
- `Constraint` — CNST
- `Business` — 사업 목표/비용/시장 제약

### 1.3 QAD / QAI (1~5 정수) + rationale
- QAD: 비즈니스/품질 영향도
- QAI: 기술/구조적 난이도
- 근거 없이 모두 동일 값 금지

### 1.4 Priority
`priority = qad * W_QAD + qai * W_QAI`

### 1.5 risk / sensitivity
- risk: 드라이버가 유발할 수 있는 위험
- sensitivity: 설계 결정 민감도

### 1.6 Driver 선정
Priority 상위 5~10건. 단일 속성/카테고리 편향 금지.

### 1.7 정규화 속성 (normalizedAttributes)

허용 어휘만 사용 (관련 없으면 생략):

- **품질속성**: scalability, modifiability, availability, performance, security, consistency, latency, auditability, debuggability, testability, simplicity, portability, decoupling
- **요구 패턴/워크로드**: event_volume, async_processing_need, synchronous_low_latency, real_time_processing, read_write_ratio_skewed, sporadic_workload, simple_crud, data_volume, domain_complexity, long_running_job, stateful_heavy, short_lifecycle, long_term_maintenance
- **프론트엔드**: large_ui_codebase, polyglot_frontend_tech, strict_ui_consistency, ui_team_independence
- **인프라/규모**: service_count, multi_cluster, inter_service_traffic_control, observability_requirement, strong_vendor_independence
- **조직/운영**: team_size(small/medium/large), ops_maturity(low/medium/high), release_frequency(low/medium/high), team_independent_deploy(low/medium/high), strong_consistency(low/medium/high)

값 체계: `low`/`medium`/`high` 또는 `required`. 불명은 **생략**.

### 1.8 document (drivers.md 본문)

```
# Architecture Drivers

## 1. 선정 Driver 목록
| # | Requirement | 타입 | 카테고리 | 이슈 키 | QAD | QAI | Priority | risk | sensitivity |

## 2. Driver 별 상세 근거
### QA_001 — (이슈 요약)
- QAD 근거 / QAI 근거 / risk / sensitivity

## 3. 정규화된 시스템 속성 (style_define 입력)
| 속성 | 값 | 근거 Requirement |

## 4. 선정 편향 점검
```

이슈 키 칸에는 `subkey`(FR_001/QA_001/CNST_001) 를 사용. Mermaid 는 선택 (`graph LR` 만 허용, `classDiagram` 금지).

---

## Step 2 — 자가검증

### critical 기준
- 필수 필드(key, qad, qai, priority, category, rationale) 누락
- `key` 가 입력 Requirement 목록에 없음 (hallucination)
- `qad`/`qai` 가 1~5 범위 밖, 혹은 근거 없이 모두 동일 값
- 카테고리 편향 (QualityAttribute 0건, 단일 카테고리만 존재)
- NFR 인데 Driver 에 포함되지 않은 항목 존재
- `normalizedAttributes` 가 허용 어휘 밖의 키/값 사용
- document 에서 `subkey` 대신 이슈 key / placeholder 노출
- Mermaid 를 `classDiagram` 으로 작성, 또는 외부 이미지 참조

### 점수
- `score`: 0.0~1.0
- `score_effective = score - 0.5 × |critical|`
- `pass`: `score_effective >= 0.8`

---

## Step 3 — 수정 (pass=false)
critical + suggestions 항목만 국소 수정. 나머지 원문 유지. 수정 후 Step 2.

---

## Step 4 — 종료 조건
1. pass=true
2. iter 3 완료
3. 같은 critical 2회 연속 재발
4. 새 수정이 이전 best 보다 낮은 score_effective → 포기

**best-so-far**: `(drivers, normalizedAttributes, document, score_effective)` 매 iter 기록. 종료 시 최댓값 iter 의 결과를 selected.

---

## Step 5 — Requirement 업데이트 + 라벨 + 문서 저장

### 5.1 각 driver 에 대해 Requirement 업데이트
```bash
curl -s -X PUT "${GOJIRA_API}/Issue" \
  -H "Content-Type: application/json" -H "X-User-Id: ${USER}" \
  -d "$(jq -n --arg p "$CODE" --arg k "$KEY" --arg u "$USER" \
    --argjson qad $QAD --argjson qai $QAI --argjson pri $PRIORITY \
    --arg risk "$RISK" --arg sens "$SENS" \
    '{prjID:$p, key:$k, user:$u, data:{qad:$qad, qai:$qai, priority:$pri, risk:$risk, sensitivity:$sens, driverSelected:true}}')"
```

### 5.2 ASR 라벨 부여 (선정된 Driver + 모든 NFR)
```bash
# 현재 labels 조회 → ASR 누락 시 append
LABELS=$(curl -s "${GOJIRA_API}/Issue/${KEY}" -H "X-User-Id: ${USER}" | jq -c '.labels // []')
NEW_LABELS=$(echo "$LABELS" | jq -c 'if index("ASR") then . else . + ["ASR"] end')
curl -s -X PUT "${GOJIRA_API}/Issue" \
  -H "Content-Type: application/json" -H "X-User-Id: ${USER}" \
  -d "$(jq -n --arg p "$CODE" --arg k "$KEY" --arg u "$USER" --argjson l "$NEW_LABELS" \
    '{prjID:$p, key:$k, user:$u, data:{labels:$l}}')"
```

선정되지 않았더라도 **모든 NFR Requirement** 에 ASR 라벨 부여.

### 5.3 normalizedAttributes 영속화
```bash
# projects.archSettings.normalizedAttributes 에 저장 — Platform-API 엔드포인트
# (현재 직접 HTTP 저장 API 없음 → Agent-Service 경유 또는 수동 확인)
echo "normalizedAttributes = $(echo $NORM_JSON | jq .)"
```
(이 Claude Code 네이티브 경로는 참고용 — Agent-Service executor 는 MongoDB 직접 업데이트.)

### 5.4 architecture/drivers.md 저장
```bash
curl -s -X PUT "${PLATFORM_API}/DocRepo/${CODE}/file" \
  -H "Content-Type: application/json" -H "X-User-Id: ${USER}" \
  -d "$(jq -n --arg c "$DOCUMENT" '{path:"architecture/drivers.md", content:$c, message:"Architecture Driver 선정 (driver-select2)"}')"
```

---

## Step 6 — 최종 보고

```
## Architecture Driver 선정 완료 (v2 Claude Code)

- Driver 선정: N건 (키 + 카테고리 분포)
- Requirement 업데이트: qad/qai/priority/risk/sensitivity
- ASR 라벨: Driver + 모든 NFR
- 정규화 속성: {scalability: high, ...}
- 문서 저장: architecture/drivers.md

### 카테고리 분포
| 카테고리 | 건수 |
|---------|-----|
| QualityAttribute | X |
| Functional | Y |
| Constraint | Z |
| Business | W |

### Self-loop
- iter N회 / selected=iter X (score_eff Y) / stop=pass|max_iter|repeat_critical|regression
- unresolvedCritical: [...]
```

---

## 주의
- 모든 curl 에 `X-User-Id: ${USER}` 헤더 필수 (403 방지)
- Requirement 의 `key` 는 입력 목록 실존 key 만 사용 (hallucination 금지)
- document 의 "이슈 키" 칸은 **subkey** (FR_001/QA_001/CNST_001)
- Platform-API `/file` 응답 content 는 base64 → `base64 -d`, 저장 시 plain UTF-8 전달
- normalizedAttributes 는 허용 어휘 외 사용 금지
- 한 세션에서 Steps 1~4 는 추론, Step 0 과 Step 5 는 실제 Bash 호출
