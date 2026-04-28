"""설계 결정 스킬 v2 — SKILL.md 통합 self-loop.

LLM 이 generate → self-critique (Driver 커버리지 + ADR 품질) → revise → best-so-far 를
단일 호출로 수행. Python 은 IO, 이슈 생성, 링크, docId 치환, 문서 저장만 담당.
"""

import os
import re
import json
from typing import Any, Dict, Optional

from core.database import issues, projects
from core.llm import call_llm
from clients.platform_api import PlatformClient
from skills.helpers import (
    create_issue, link_issues, dispatch_by_mode,
    build_docid_subkey_map, substitute_doc_ids,
)
from skills.critique import load_step_policy


_SKILL_DIR = os.path.dirname(__file__)
_SKILL_MD_PATH = os.path.join(_SKILL_DIR, "SKILL.md")

_DESIGN_LEVEL_TO_TYPE = {
    "Architectural": "Architectural Design",
    "Detailed": "Detailed Design",
}

# 설계 이슈 저장용 extras 필드
_COMMON_FIELDS = ("designPoint", "approaches", "decision", "decisionRationale",
                  "tradeoffs", "risk", "sensitivity", "pros", "cons")


def _load_skill_md() -> str:
    with open(_SKILL_MD_PATH, encoding="utf-8") as f:
        text = f.read()
    m = re.match(r"^---\n.*?\n---\n(.*)$", text, re.DOTALL)
    return m.group(1) if m else text


def _build_prompt(skill_body: str, code: str, normalized_json: str,
                  chosen: dict, drivers_json: str, driver_count: int,
                  req_json: str, req_count: int,
                  existing_design_json: str, design_count: int,
                  criteria: str, policy_text: str) -> str:
    return (
        f"{skill_body}\n\n"
        f"---\n\n"
        f"## 주입 데이터\n\n"
        f"### projectCode\n{code}\n\n"
        f"### normalized_json\n```json\n{normalized_json}\n```\n\n"
        f"### chosen_styles\n"
        f"- Primary: {', '.join(chosen.get('primary') or []) or '(없음)'}\n"
        f"- Secondary: {', '.join(chosen.get('secondary') or []) or '(없음)'}\n"
        f"- Supporting: {', '.join(chosen.get('supporting') or []) or '(없음)'}\n\n"
        f"### drivers ({driver_count}건, priority 순)\n```json\n{drivers_json}\n```\n\n"
        f"### req_list (Requirement 전체, {req_count}건 — relatedRequirements 유효 키 풀)\n```json\n{req_json}\n```\n\n"
        f"### existing_designs ({design_count}건)\n```json\n{existing_design_json}\n```\n\n"
        f"### criteria\n{criteria}\n\n"
        f"### policy\n{policy_text}\n\n"
        f"이제 위 `실행 흐름` 섹션대로 수행하고 지정된 JSON 하나만 출력하라."
    )


def _extract_json_obj(raw: str) -> Optional[Dict[str, Any]]:
    raw = raw.strip()
    try:
        return json.loads(raw)
    except json.JSONDecodeError:
        pass
    m = re.search(r"\{[\s\S]*\}", raw)
    if not m:
        return None
    try:
        return json.loads(m.group())
    except json.JSONDecodeError:
        return None


def _item_extras(item: dict) -> dict:
    extras: Dict[str, Any] = {}
    for f in _COMMON_FIELDS:
        v = item.get(f)
        if v is None or v == "":
            continue
        extras[f] = v if isinstance(v, (str, int, float, bool)) else json.dumps(v, ensure_ascii=False)
    return extras


async def execute(params: dict, user: str) -> dict:
    code = params.get("projectCode", "")
    if not code:
        raise ValueError("projectCode 필수")
    req_user = params.get("user", user)

    from core.progress import emit_progress

    # 1. 선행 확인
    proj = projects().find_one(
        {"jiraCodes.main.jiraCode": code},
        {"archSettings": 1, "_id": 0},
    ) or {}
    arch = proj.get("archSettings") or {}
    chosen = arch.get("chosenStyles") or {}
    normalized = arch.get("normalizedAttributes") or {}

    drivers = list(issues().find(
        {"key": {"$regex": f"^{re.escape(code)}-"},
         "issueType": "Requirement",
         "driverSelected": True},
        {"key": 1, "subkey": 1, "summary": 1, "reqtype": 1,
         "qad": 1, "qai": 1, "priority": 1, "risk": 1, "sensitivity": 1, "_id": 0},
    ))

    if not drivers or not chosen or not normalized:
        return {
            "projectCode": code,
            "status": "blocked",
            "blockedAt": "style_define",
            "suggestSkill": "style_define",
            "message": "선행 산출물이 부족합니다 (drivers / chosenStyles / normalizedAttributes). style_define 을 먼저 실행하세요.",
        }

    req_list = list(issues().find(
        {"key": {"$regex": f"^{re.escape(code)}-"}, "issueType": "Requirement"},
        {"key": 1, "subkey": 1, "summary": 1, "reqtype": 1, "_id": 0},
    ))
    valid_req_keys = {r["key"] for r in req_list}
    driver_keys = {d["key"] for d in drivers}

    existing_designs = list(issues().find(
        {"key": {"$regex": f"^{re.escape(code)}-"},
         "issueType": {"$in": ["Architectural Design", "Detailed Design"]}},
        {"key": 1, "subkey": 1, "issueType": 1, "summary": 1, "_id": 0},
    ))
    existing_design_keys = {d["key"] for d in existing_designs}
    existing_design_by_key = {d["key"]: d for d in existing_designs}

    await emit_progress("design_decide", "설계안 결정 시작 (v2 self-loop)")

    # 2. LLM 단일 호출 (self-loop 내장)
    policy = load_step_policy(
        "workflow-architecture-design.md",
        "디자인 포인트 선정 및 설계안 결정 (Architectural Approach)",
    )
    skill_body = _load_skill_md()
    prompt = _build_prompt(
        skill_body, code,
        json.dumps(normalized, ensure_ascii=False, indent=2),
        chosen,
        json.dumps(drivers, ensure_ascii=False, indent=2), len(drivers),
        json.dumps(req_list, ensure_ascii=False, indent=2), len(req_list),
        json.dumps(existing_designs, ensure_ascii=False, indent=2), len(existing_designs),
        policy.get("criteria", ""), policy.get("policy_text", ""),
    )
    raw = await call_llm(prompt)

    parsed = _extract_json_obj(raw)
    if not parsed or "finalDesigns" not in parsed:
        return {
            "projectCode": code,
            "status": "failed",
            "message": "LLM 응답 파싱 실패.",
            "raw": raw[:500],
            "created": [], "reused": [], "modified": [], "markedForDeletion": [],
        }

    designs = parsed.get("finalDesigns") or []
    document = parsed.get("finalDocument") or ""
    coverage_matrix = parsed.get("finalCoverage") or {}
    iterations = parsed.get("iterations") or []
    stop_reason = parsed.get("stopReason", "")
    unresolved = parsed.get("unresolvedCritical") or []

    # 3. 이슈 생성 (dispatch_by_mode)
    async def _create_fn(item):
        level = (item.get("designLevel") or "Architectural")
        if level not in _DESIGN_LEVEL_TO_TYPE:
            raise ValueError(f"designLevel 은 Architectural/Detailed 만 허용: {level}")
        issue_type = _DESIGN_LEVEL_TO_TYPE[level]
        create_params = {
            "projectCode": code,
            "summary": (item.get("summary") or "").strip(),
            "description": item.get("description", ""),
        }
        extras = _item_extras(item)
        extras["designLevel"] = level
        return await create_issue(issue_type, create_params, user, extras=extras or None)

    def _update_fields_fn(item):
        updates = {}
        for f in ("summary", "description"):
            v = item.get(f)
            if v is not None and v != "":
                updates[f] = v
        updates.update(_item_extras(item))
        return updates

    buckets = await dispatch_by_mode(designs, existing_design_keys, _create_fn, _update_fields_fn, user)

    # 4. Requirement contains ADSN/DDSN 링크
    linked = []
    covered_drivers: set = set()
    for bucket_name in ("created", "modified", "reused"):
        for entry in buckets.get(bucket_name, []):
            design_key = entry["key"]
            for req_key in (entry["item"].get("relatedRequirements") or []):
                if req_key in valid_req_keys:
                    try:
                        await link_issues(req_key, design_key, "contains", user=user)
                        linked.append({"from": req_key, "to": design_key})
                        if req_key in driver_keys:
                            covered_drivers.add(req_key)
                    except Exception as e:
                        import logging
                        logging.getLogger("agent.design_decide").warning(
                            "link 실패 %s → %s: %s", req_key, design_key, e
                        )

    # 5. docId → subkey 치환
    docid_map = build_docid_subkey_map(buckets, existing_design_by_key)
    document, leftover_doc_ids = substitute_doc_ids(document, docid_map)

    # 6. architecture/design-decisions.md 저장
    client = PlatformClient()
    saved_path = None
    save_error = None
    if document.strip():
        try:
            await client.save_file(
                code,
                "architecture/design-decisions.md",
                document,
                f"설계 결정 (신규 {len(buckets['created'])} / 수정 {len(buckets['modified'])}, v2)",
                req_user,
            )
            saved_path = "architecture/design-decisions.md"
        except Exception as e:
            save_error = str(e)

    # 7. Driver 커버리지 감사
    orphan_drivers = [k for k in driver_keys if k not in covered_drivers]

    # 8. 요약
    parts = [
        f"설계 이슈 — 신규 {len(buckets['created'])}, 재사용 {len(buckets['reused'])}, "
        f"수정 {len(buckets['modified'])}, 삭제마킹 {len(buckets['markedForDeletion'])}",
        f"Requirement-설계 링크 {len(linked)}건",
    ]
    if saved_path:
        parts.append("design-decisions.md 저장 완료")
    elif save_error:
        parts.append(f"문서 저장 실패: {save_error}")
    base_msg = " / ".join(parts)

    iter_info = ""
    if iterations:
        iter_info = f" (iter {len(iterations)}회, best=iter{parsed.get('selectedIter')}, stop={stop_reason})"

    notes = ""
    if leftover_doc_ids:
        notes += f" / ⚠ 미치환 docId {len(leftover_doc_ids)}건"
    if orphan_drivers:
        notes += f" / ⚠ 미커버 Driver {len(orphan_drivers)}건: {', '.join(orphan_drivers[:5])}"

    await emit_progress("design_decide", f"설계 결정 완료 — {base_msg}")

    status = "completed"
    if orphan_drivers or unresolved:
        status = "partial"

    return {
        "projectCode": code,
        "status": status,
        "created": buckets["created"],
        "reused": buckets["reused"],
        "modified": buckets["modified"],
        "markedForDeletion": buckets["markedForDeletion"],
        "linked": linked,
        "linkedCount": len(linked),
        "documentSaved": saved_path,
        "saveError": save_error,
        "iterations": iterations,
        "selectedIter": parsed.get("selectedIter"),
        "stopReason": stop_reason,
        "unresolvedCritical": unresolved,
        "coverageMatrix": coverage_matrix,
        "docIdMap": docid_map,
        "leftoverDocIds": leftover_doc_ids,
        "coverage": {
            "totalDrivers": len(driver_keys),
            "covered": len(covered_drivers),
            "orphan": len(orphan_drivers),
            "orphanKeys": orphan_drivers[:20],
        },
        "message": base_msg + iter_info + notes,
    }
