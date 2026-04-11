"""SATAM 분석 스킬."""

import re
import json

from core.database import issues
from core.llm import call_llm


async def execute(params: dict, user: str) -> dict:
    code = params.get("projectCode", "")
    if not code:
        raise ValueError("projectCode 필수")

    prefix = {"key": {"$regex": f"^{re.escape(code)}-"}}
    qa_issues = list(issues().find({**prefix, "issueType": "Requirement"}))
    adsn_issues = list(issues().find({**prefix, "issueType": "Architectural Design"}))

    items = []
    risk_count = 0
    sens_count = 0
    mitig_missing = 0

    for issue in qa_issues + adsn_issues:
        risk = issue.get("risk", "")
        sens = issue.get("sensitivity", "")
        mitig = issue.get("mitigation", "")
        if risk:
            risk_count += 1
        if sens:
            sens_count += 1
        if (risk or sens) and not mitig:
            mitig_missing += 1
        items.append({
            "key": issue.get("key", ""),
            "summary": issue.get("summary", ""),
            "reqType": issue.get("reqtype", ""),
            "risk": risk,
            "sensitivity": sens,
            "mitigation": mitig,
            "issueType": issue.get("issueType", ""),
        })

    items_json = json.dumps(items, ensure_ascii=False)
    if len(items_json) > 6000:
        items_json = items_json[:6000] + "\n...(truncated)"
    prompt = f"""다음 프로젝트 {code}의 SATAM 분석 데이터를 한국어로 분석해주세요.

데이터:
{items_json}

분석 항목:
1. 위험도(risk) 있는 항목 목록과 심각도
2. 민감도(sensitivity) 있는 항목 목록
3. 경감조치(mitigation) 미완료 항목 (위험/민감인데 경감조치 없음)
4. 전체 커버리지 요약
5. 개선 권고사항"""

    analysis = await call_llm(prompt)

    return {
        "projectCode": code,
        "totalItems": len(items),
        "riskCount": risk_count,
        "sensitiveCount": sens_count,
        "mitigMissing": mitig_missing,
        "items": items,
        "analysis": analysis,
    }
