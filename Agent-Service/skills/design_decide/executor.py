"""설계 결정 스킬 — Driver + Style → ADSN/DDSN + contains 링크 + architecture/design-decisions.md."""

import os
import re
import json
from typing import Dict, Any, List

from core.database import issues, projects
from clients.platform_api import PlatformClient
from skills.helpers import (
    create_issue, link_issues, dispatch_by_mode,
    load_prompt,
    build_docid_subkey_map, substitute_doc_ids,
)
from skills.critique import iterate_generate_critique, trail_summary, load_step_policy


_SKILL_DIR = os.path.dirname(__file__)

_DESIGN_LEVEL_TO_TYPE = {
    "Architectural": "Architectural Design",
    "Detailed": "Detailed Design",
}

# 설계 이슈 저장용 extras
_COMMON_FIELDS = ("designPoint", "approaches", "decision", "decisionRationale",
                  "tradeoffs", "risk", "sensitivity", "pros", "cons")


def _extract_json_obj(raw: str):
    try:
        data = json.loads(raw)
        return data if isinstance(data, dict) else None
    except json.JSONDecodeError:
        m = re.search(r'\{.*\}', raw, re.DOTALL)
        if m:
            try:
                data = json.loads(m.group())
                return data if isinstance(data, dict) else None
            except json.JSONDecodeError:
                return None
    return None


def _item_extras(item: dict) -> dict:
    extras: Dict[str, Any] = {}
    for f in _COMMON_FIELDS:
        v = item.get(f)
        if v is None or v == "":
            continue
        # approaches 는 리스트/객체일 수 있어 JSON 직렬화
        extras[f] = v if isinstance(v, (str, int, float, bool)) else json.dumps(v, ensure_ascii=False)
    return extras


async def execute(params: dict, user: str) -> dict:
    code = params.get("projectCode", "")
    if not code:
        raise ValueError("projectCode 필수")
    req_user = params.get("user", user)

    from core.progress import emit_progress

    # 1. 선행 확인
    prereqs_ran: List[Dict[str, Any]] = []
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

    need_delegate = (not drivers) or (not chosen) or (not normalized)
    if need_delegate:
        await emit_progress("design_decide", "선행(Driver / Style) 부족 — style_define 선행 실행")
        from skills.style_define.executor import execute as style_define_execute
        sd = await style_define_execute(params, user)
        prereqs_ran.append({"skill": "style_define", "result": sd})
        if sd.get("suggestSkill"):
            return {
                **sd,
                "prerequisitesRan": (sd.get("prerequisitesRan") or []) + prereqs_ran,
                "blockedAt": sd.get("blockedAt") or "style_define",
            }
        prereqs_ran = (sd.get("prerequisitesRan") or []) + prereqs_ran
        # 재조회
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
            "message": "선행 산출물이 여전히 부족합니다 (drivers / chosenStyles / normalizedAttributes).",
            "prerequisitesRan": prereqs_ran,
        }

    # 전체 Requirement (relatedRequirements 유효성 검증용)
    req_list = list(issues().find(
        {"key": {"$regex": f"^{re.escape(code)}-"}, "issueType": "Requirement"},
        {"key": 1, "subkey": 1, "summary": 1, "reqtype": 1, "_id": 0},
    ))
    valid_req_keys = {r["key"] for r in req_list}

    existing_designs = list(issues().find(
        {"key": {"$regex": f"^{re.escape(code)}-"},
         "issueType": {"$in": ["Architectural Design", "Detailed Design"]}},
        {"key": 1, "subkey": 1, "issueType": 1, "summary": 1, "_id": 0},
    ))
    existing_design_keys = {d["key"] for d in existing_designs}
    existing_design_by_key = {d["key"]: d for d in existing_designs}

    await emit_progress("design_decide", "설계안 결정 시작")

    drivers_json = json.dumps(drivers, ensure_ascii=False, indent=2)
    req_json = json.dumps(req_list, ensure_ascii=False, indent=2)
    existing_design_json = json.dumps(existing_designs, ensure_ascii=False, indent=2)
    normalized_json = json.dumps(normalized, ensure_ascii=False, indent=2)

    generate_prompt = load_prompt(_SKILL_DIR, "generate").format(
        code=code,
        normalized_json=normalized_json,
        primary=", ".join(chosen.get("primary") or []),
        secondary=", ".join(chosen.get("secondary") or []),
        supporting=", ".join(chosen.get("supporting") or []),
        drivers_json=drivers_json,
        driver_count=len(drivers),
        req_count=len(req_list),
        req_json=req_json,
        design_count=len(existing_designs),
        existing_design_json=existing_design_json,
    )

    critique_tpl = load_prompt(_SKILL_DIR, "critique")
    revise_tpl = load_prompt(_SKILL_DIR, "revise")

    policy = load_step_policy(
        "workflow-architecture-design.md",
        "디자인 포인트 선정 및 설계안 결정 (Architectural Approach)",
    )

    context_block = (
        f"## Driver 목록\n{drivers_json}\n\n"
        f"## 선정 스타일\nPrimary: {chosen.get('primary')}\nSecondary: {chosen.get('secondary')}\n"
        f"Supporting: {chosen.get('supporting')}"
    )

    def _critique_prompt(draft: str) -> str:
        return critique_tpl.format(
            context_block=context_block,
            draft=draft[:8000],
            criteria=policy["criteria"],
            policy=policy["policy_text"],
        )

    def _revise_prompt(draft: str, critique: dict) -> str:
        issues_text = "\n".join(f"- {i}" for i in critique.get("issues", []))
        return revise_tpl.format(
            drivers_json=drivers_json,
            draft=draft[:10000],
            issues_text=issues_text,
        )

    raw, critique_trail = await iterate_generate_critique(
        generate_prompt, _critique_prompt, _revise_prompt,
        max_iter=policy["max_iter"],
        threshold=policy["threshold"],
        on_fail=policy["on_fail"],
    )

    parsed = _extract_json_obj(raw)
    if not parsed:
        return {
            "projectCode": code,
            "message": "LLM 응답을 파싱할 수 없습니다.",
            "raw": raw[:500],
            "prerequisitesRan": prereqs_ran,
        }

    designs = parsed.get("designs") or []
    document = parsed.get("document") or ""

    # 이슈 생성 (dispatch_by_mode)
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

    # Requirement contains ADSN/DDSN 링크
    linked = []
    for bucket_name in ("created", "modified"):
        for entry in buckets[bucket_name]:
            design_key = entry["key"]
            for req_key in (entry["item"].get("relatedRequirements") or []):
                if req_key in valid_req_keys:
                    try:
                        await link_issues(req_key, design_key, "contains")
                        linked.append({"from": req_key, "to": design_key})
                    except Exception:
                        pass

    # docId → subkey 치환
    docid_map = build_docid_subkey_map(buckets, existing_design_by_key)
    document, leftover_doc_ids = substitute_doc_ids(document, docid_map)

    # architecture/design-decisions.md 저장
    client = PlatformClient()
    saved_path = None
    save_error = None
    if document.strip():
        try:
            await client.save_file(
                code,
                "architecture/design-decisions.md",
                document,
                f"설계 결정 (신규 {len(buckets['created'])} / 수정 {len(buckets['modified'])})",
                req_user,
            )
            saved_path = "architecture/design-decisions.md"
        except Exception as e:
            save_error = str(e)

    tsum = trail_summary(critique_trail)
    parts = [
        f"설계 이슈 — 신규 {len(buckets['created'])}, 재사용 {len(buckets['reused'])}, "
        f"수정 {len(buckets['modified'])}, 삭제마킹 {len(buckets['markedForDeletion'])}",
        f"Requirement-설계 링크 {len(linked)}건",
    ]
    if saved_path:
        parts.append("design-decisions.md 저장 완료")
    elif save_error:
        parts.append(f"문서 저장 실패: {save_error}")
    if leftover_doc_ids:
        parts.append(f"⚠ 미치환 docId {len(leftover_doc_ids)}건: {', '.join(leftover_doc_ids[:5])}")
    if tsum:
        parts.append(tsum)
    msg = " / ".join(parts)

    await emit_progress("design_decide", f"설계 결정 완료 — {msg}")

    return {
        "projectCode": code,
        "created": buckets["created"],
        "reused": buckets["reused"],
        "modified": buckets["modified"],
        "markedForDeletion": buckets["markedForDeletion"],
        "linked": linked,
        "linkedCount": len(linked),
        "documentSaved": saved_path,
        "saveError": save_error,
        "critiqueTrail": critique_trail,
        "docIdMap": docid_map,
        "leftoverDocIds": leftover_doc_ids,
        "prerequisitesRan": prereqs_ran,
        "message": msg,
    }
