"""이슈 관계 분석 스킬."""

import re

from core.database import issues


def execute(params: dict, user: str) -> dict:
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
