"""SATAM 분석 스킬 — ADSN 의 risk/sensitivity/tradeoffs 를 읽어 YAML 포맷으로 집약."""

import re
import json

from core.database import issues, projects
from core.llm import call_llm


async def execute(params: dict, user: str) -> dict:
    code = params.get("projectCode", "")
    if not code:
        raise ValueError("projectCode 필수")

    prefix = {"key": {"$regex": f"^{re.escape(code)}-"}}
    qa_issues = list(issues().find({**prefix, "issueType": "Requirement"}))
    adsn_issues = list(issues().find({**prefix, "issueType": "Architectural Design"}))

    # ADSN — risk/sensitivity/tradeoffs 가 실제로 기록되는 원천
    adsn_items = []
    risk_count = sens_count = trade_count = 0
    for issue in adsn_issues:
        risk = (issue.get("risk") or "").strip()
        sens = (issue.get("sensitivity") or "").strip()
        trade = (issue.get("tradeoffs") or issue.get("tradeoff") or issue.get("mitigation") or "").strip()
        if risk:
            risk_count += 1
        if sens:
            sens_count += 1
        if trade:
            trade_count += 1
        # contains 링크로 연결된 QA 키 목록
        linked_qas = []
        for ln in issue.get("issueLinks") or []:
            t = (ln.get("type") or {}).get("name", "")
            if "Contains" in t or "contain" in t.lower():
                k = ((ln.get("inwardIssue") or {}).get("key")
                     or (ln.get("outwardIssue") or {}).get("key"))
                if k:
                    linked_qas.append(k)
        adsn_items.append({
            "key": issue.get("key", ""),
            "subkey": issue.get("subkey", ""),
            "summary": issue.get("summary", ""),
            "designPoint": issue.get("designPoint", ""),
            "decision": issue.get("decision", ""),
            "risk": risk,
            "sensitivity": sens,
            "tradeoffs": trade,
            "linkedQAs": linked_qas,
        })

    # QA (Requirement, reqtype=NFR 우선) — 참조용 리스트
    qa_refs = []
    for issue in qa_issues:
        if (issue.get("reqtype") or "").upper() != "NFR":
            continue
        qa_refs.append({
            "key": issue.get("key", ""),
            "subkey": issue.get("subkey", ""),
            "summary": issue.get("summary", ""),
        })

    adsn_json = json.dumps(adsn_items, ensure_ascii=False, indent=2)
    qa_json = json.dumps(qa_refs, ensure_ascii=False, indent=2)

    prompt = f"""프로젝트 {code} 의 SATAM(Software Architecture Trade-off Analysis Method) 문서를 작성한다.

## 입력 데이터

### ADSN (Architectural Design 이슈) — risk / sensitivity / tradeoffs 의 원천
{adsn_json}

### QA (Quality Attribute, Requirement 중 NFR) — Risk/Sensitivity/Trade-off 가 영향을 주는 대상
{qa_json}

## 출력 형식 — 반드시 아래 YAML 양식을 정확히 따를 것 (절대 규칙)

```yaml
Risks:
  - ID: R1
    QA: [PROJECTCODE-XXX, PROJECTCODE-YYY]   # 이 Risk 가 영향을 주는 QA 이슈 key 목록
    Description: <risk 의 상세 내용>

Sensitivities:
  - ID: S1
    QA: [PROJECTCODE-XXX]                     # 이 Sensitivity 가 영향을 주는 QA 이슈 key 목록
    Description: <sensitivity 의 상세 내용>

Trade-offs:
  - ID: M1
    QA: [PROJECTCODE-XXX, PROJECTCODE-YYY]    # 이 Trade-off 가 영향을 주는 QA 이슈 key 목록
    Counter Risk: ["R1:PROJECTCODE-XXX", "R2:PROJECTCODE-YYY"]  # Mitigation 되는 Risk ID + 영향받는 QA key 조합
    Description: <trade-off(Mitigation) 의 상세 내용>
```

## 작성 규칙 (엄수)

1. **ID 규칙**: Risks 는 R1,R2,... / Sensitivities 는 S1,S2,... / Trade-offs 는 M1,M2,... 순차 부여
2. **원천 유지**: ADSN 의 `risk` 필드 내용은 Risks.Description 에, `sensitivity` 는 Sensitivities.Description 에, `tradeoffs` 는 Trade-offs.Description 에 기재한다. 임의 창작 금지
3. **QA 매핑**: 각 항목의 `QA:` 에는 `linkedQAs` 로 연결된 QA 이슈 key 또는 본문 분석으로 실제 영향받는 QA 의 key 를 나열. 반드시 **이슈 key 형식** (예: GOJIRA-123) 사용, subkey(QA_001) 금지
4. **Trade-off 의 Counter Risk**: 그 trade-off 가 완화하는 Risk 의 ID 와, 해당 Risk 가 원래 영향주던 QA 이슈 key 의 조합을 `"R1:GOJIRA-001"` 포맷으로 기재. 다수면 배열로 나열
5. **비어있는 필드 건너뛰기**: ADSN 에 risk/sensitivity/tradeoffs 가 비어있는 항목은 해당 섹션에서 제외
6. **출력은 yaml 코드블록 단일**: 상단에 서두, 하단에 후기 절대 금지. `\`\`\`yaml` 로 시작해 `\`\`\`` 로 닫는다

지금 위 데이터를 바탕으로 YAML 을 작성하라."""

    document = await call_llm(prompt)

    # yaml 코드블록만 추출 (앞뒤 불필요한 설명 제거)
    yaml_block = _extract_yaml(document)

    # 문서 파일 저장
    saved_path = None
    save_error = None
    try:
        from clients.platform_api import PlatformClient
        client = PlatformClient()
        content = f"# SATAM 분석 — {code}\n\n```yaml\n{yaml_block}\n```\n"
        await client.save_file(
            code,
            "architecture/satam.md",
            content,
            f"SATAM 분석 (Risks {risk_count}/Sensitivities {sens_count}/Trade-offs {trade_count})",
            user,
        )
        saved_path = "architecture/satam.md"
        # 완료 플래그 영속화 (user branch 저장 → main 에는 없어도 belief state 인지)
        projects().update_one(
            {"jiraCodes.main.jiraCode": code},
            {"$set": {"archSettings.satamDone": True}},
        )
    except Exception as e:
        save_error = str(e)

    return {
        "projectCode": code,
        "adsnCount": len(adsn_items),
        "qaCount": len(qa_refs),
        "riskCount": risk_count,
        "sensitivityCount": sens_count,
        "tradeoffCount": trade_count,
        "documentSaved": saved_path,
        "saveError": save_error,
        "document": yaml_block,
    }


def _extract_yaml(text: str) -> str:
    """LLM 응답에서 ```yaml ... ``` 블록만 추출. 없으면 원문."""
    m = re.search(r"```ya?ml\s*\n(.*?)```", text, re.DOTALL | re.IGNORECASE)
    if m:
        return m.group(1).strip()
    # 백틱만 있을 때
    m = re.search(r"```\s*\n(.*?)```", text, re.DOTALL)
    if m:
        return m.group(1).strip()
    return text.strip()
