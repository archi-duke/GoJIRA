"""분석 스킬 실행기 — Go executor.go SATAM/issue_analyze/schedule_analyze 대응."""

import re
import json
import logging
from datetime import datetime

from core.database import issues, projects
from core.llm import call_llm

log = logging.getLogger("agent.skills.analysis")


async def exec_satam_analyze(params: dict, user: str) -> dict:
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

    # LLM 분석
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


def exec_issue_analyze(params: dict, user: str) -> dict:
    code = params.get("projectCode", "")
    if not code:
        raise ValueError("projectCode 필수")

    all_issues = list(issues().find({"key": {"$regex": f"^{re.escape(code)}-"}}))

    type_counts = {}
    key_to_type = {}
    contained_by = {}
    link_count = {}

    for issue in all_issues:
        key = issue.get("key", "")
        i_type = issue.get("issueType", "")
        type_counts[i_type] = type_counts.get(i_type, 0) + 1
        key_to_type[key] = i_type

        links = issue.get("issueLinks", [])
        if not isinstance(links, list):
            links = []
        for link in links:
            if not isinstance(link, dict):
                continue
            lt = link.get("type", "")
            if not isinstance(lt, str):
                continue
            link_count[lt] = link_count.get(lt, 0) + 1
            if lt == "contain":
                inward = link.get("inward", "")
                if isinstance(inward, str) and inward:
                    contained_by[key] = inward

    # 고아 이슈 (contain 관계 없는 이슈, VOS 제외)
    orphans = []
    for issue in all_issues:
        key = issue.get("key", "")
        i_type = issue.get("issueType", "")
        if i_type == "VOS":
            continue
        if key not in contained_by:
            orphans.append({
                "key": key,
                "issueType": i_type,
                "summary": issue.get("summary", ""),
            })

    # contain 체인 통계
    chain_stats = {
        "VOS→Feature": 0,
        "Feature→Requirement": 0,
        "Requirement→ADSN": 0,
    }
    for child, parent in contained_by.items():
        p_type = key_to_type.get(parent, "")
        c_type = key_to_type.get(child, "")
        chain = f"{p_type}→{c_type}"
        if chain in chain_stats:
            chain_stats[chain] += 1

    return {
        "projectCode": code,
        "typeCounts": type_counts,
        "linkCounts": link_count,
        "chainStats": chain_stats,
        "orphanCount": len(orphans),
        "orphans": orphans,
        "totalIssues": len(all_issues),
    }


async def exec_schedule_analyze(params: dict, user: str) -> dict:
    code = params.get("projectCode", "")
    if not code:
        raise ValueError("projectCode 필수")

    prj = projects().find_one(
        {"jiraCodes.main.jiraCode": code}
    )
    if not prj:
        raise ValueError(f"프로젝트 없음: {code}")

    name = prj.get("name", "")
    status = prj.get("status", "")

    all_issues = list(issues().find({"key": {"$regex": f"^{re.escape(code)}-"}}))
    total = len(all_issues)
    done = sum(1 for i in all_issues if i.get("status") == "Done")
    prog = sum(1 for i in all_issues if i.get("status") == "In Progress")

    info = (
        f"프로젝트: {name} ({code}), 상태: {status}, "
        f"이슈: 총{total} 완료{done} 진행{prog} 미착수{total - done - prog}, "
        f"오늘: {datetime.now().strftime('%Y-%m-%d')}"
    )

    analysis = await call_llm(f"프로젝트 일정 분석:\n{info}")

    return {
        "projectCode": code,
        "projectName": name,
        "issueStats": {"total": total, "done": done, "inProgress": prog},
        "analysis": analysis,
    }
