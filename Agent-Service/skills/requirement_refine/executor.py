"""Requirement 정제 스킬 — Feature/Use Case → FR/NFR/CNST + business/requirements.md (mode 분기 지원)."""

import os
import re
import json

from core.database import issues
from clients.platform_api import PlatformClient
from skills.helpers import (
    create_issue, link_issues, dispatch_by_mode,
    is_template_only as _is_template_only,
    load_prompt,
)
from skills.critique import iterate_generate_critique, trail_summary


_SKILL_DIR = os.path.dirname(__file__)
_PROMPTS_DIR = os.path.join(_SKILL_DIR, "..", "..", "prompts")

_NFR_FIELDS = (
    "qa", "source", "stimulus", "environment", "artifact",
    "response", "measure", "responseScenario",
)


def _load_template() -> str:
    path = os.path.join(_PROMPTS_DIR, "requirements.md")
    try:
        with open(path, "r", encoding="utf-8") as f:
            return f.read()
    except FileNotFoundError:
        return ""


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


async def _read_doc(client: PlatformClient, code: str, path: str, req_user: str) -> str:
    main = await client.get_file_content(code, path, ref="main")
    if main and not _is_template_only(main):
        return main
    branch = await client.get_file_content(code, path, ref=f"docs/{req_user}")
    return branch or main or ""


async def execute(params: dict, user: str) -> dict:
    code = params.get("projectCode", "")
    if not code:
        raise ValueError("projectCode 필수")
    req_user = params.get("user", user)

    client = PlatformClient()

    desc = await _read_doc(client, code, "business/project-description.md", req_user)
    use_cases = await _read_doc(client, code, "business/use-cases.md", req_user)

    if _is_template_only(desc):
        return {
            "projectCode": code,
            "message": "프로젝트 관련 정보가 불충분합니다. 과제 설명 문서 작성을 먼저 진행해야 합니다.",
            "suggestSkill": "projdesc_interview",
            "created": [], "reused": [], "modified": [], "markedForDeletion": [],
        }

    feat_list = list(issues().find(
        {"key": {"$regex": f"^{re.escape(code)}-"}, "issueType": "Feature"},
        {"key": 1, "summary": 1, "description": 1,
         "actor": 1, "precondition": 1, "basicflow": 1,
         "alternativeflow": 1, "exceptionflow": 1, "postcondition": 1, "_id": 0},
    ))
    from core.progress import emit_progress
    prereqs_ran = []
    if not feat_list:
        # 선행 위임: Feature 없으면 usecase_analyze를 먼저 자동 실행 (이는 다시 vos_collect로 위임 가능)
        await emit_progress("requirement_refine", "Feature가 없어 usecase_analyze 선행 실행")
        from skills.usecase_analyze.executor import execute as usecase_analyze_execute
        uc_result = await usecase_analyze_execute(params, user)
        prereqs_ran.append({"skill": "usecase_analyze", "result": uc_result})
        # 더 상위 선행이 막혔으면 그대로 위로 전파 (예: projdesc_interview 필요)
        if uc_result.get("suggestSkill"):
            return {
                **uc_result,
                "prerequisitesRan": (uc_result.get("prerequisitesRan") or []) + prereqs_ran,
                "blockedAt": uc_result.get("blockedAt") or "usecase_analyze",
            }
        # 위임 결과 → Feature 목록 재조회
        feat_list = list(issues().find(
            {"key": {"$regex": f"^{re.escape(code)}-"}, "issueType": "Feature"},
            {"key": 1, "summary": 1, "description": 1,
             "actor": 1, "precondition": 1, "basicflow": 1,
             "alternativeflow": 1, "exceptionflow": 1, "postcondition": 1, "_id": 0},
        ))
        if not feat_list:
            return {
                "projectCode": code,
                "message": "Use Case 분석 후에도 Feature가 도출되지 않았습니다.",
                "prerequisitesRan": prereqs_ran,
                "created": [], "reused": [], "modified": [], "markedForDeletion": [],
            }
        # 하위 위임에서 prereqs_ran을 흡수
        prereqs_ran = (uc_result.get("prerequisitesRan") or []) + prereqs_ran

    vos_list = list(issues().find(
        {"key": {"$regex": f"^{re.escape(code)}-"}, "issueType": "VOS"},
        {"key": 1, "summary": 1, "_id": 0},
    ))
    existing_reqs = list(issues().find(
        {"key": {"$regex": f"^{re.escape(code)}-"}, "issueType": "Requirement"},
        {"key": 1, "summary": 1, "description": 1, "reqtype": 1,
         "qa": 1, "measure": 1, "_id": 0},
    ))
    existing_req_keys = {r["key"] for r in existing_reqs}
    valid_feat_keys = {f["key"] for f in feat_list}

    template = _load_template()
    desc_trunc = desc[:4000]
    uc_trunc = use_cases[:4000]
    feat_json = json.dumps(feat_list, ensure_ascii=False, indent=2)
    vos_json = json.dumps(vos_list, ensure_ascii=False, indent=2)
    existing_req_json = json.dumps(existing_reqs, ensure_ascii=False, indent=2)

    await emit_progress("requirement_refine", "Requirement 정제 시작")
    generate_prompt = load_prompt(_SKILL_DIR, "generate").format(
        code=code,
        desc_trunc=desc_trunc,
        uc_trunc=uc_trunc,
        feat_count=len(feat_list),
        feat_json=feat_json,
        vos_count=len(vos_list),
        vos_json=vos_json,
        existing_req_count=len(existing_reqs),
        existing_req_json=existing_req_json,
        template=template,
    )

    critique_tpl = load_prompt(_SKILL_DIR, "critique")
    revise_tpl = load_prompt(_SKILL_DIR, "revise")

    def _critique_prompt(draft: str) -> str:
        return critique_tpl.format(feat_json=feat_json, draft=draft[:8000])

    def _revise_prompt(draft: str, critique: dict) -> str:
        issues_text = "\n".join(f"- {i}" for i in critique.get("issues", []))
        return revise_tpl.format(feat_json=feat_json, draft=draft[:10000], issues_text=issues_text)

    raw, critique_trail = await iterate_generate_critique(
        generate_prompt, _critique_prompt, _revise_prompt,
    )
    parsed = _extract_json_obj(raw)
    if not parsed:
        return {
            "projectCode": code,
            "message": "LLM 응답을 파싱할 수 없습니다.",
            "raw": raw[:500],
            "created": [], "reused": [], "modified": [], "markedForDeletion": [],
        }

    req_items = parsed.get("requirements") or []
    document = parsed.get("document") or ""

    # mode 분기 처리
    def _create_fn(item):
        req_type = (item.get("reqType") or "").upper()
        if req_type not in ("FR", "NFR", "CNST"):
            raise ValueError(f"잘못된 reqType: {req_type}")
        params_for_create = {
            "projectCode": code,
            "reqType": req_type,
            "summary": (item.get("summary") or "").strip(),
            "description": item.get("description", ""),
        }
        extras = {"reqtype": req_type}
        if req_type == "NFR":
            for f in _NFR_FIELDS:
                if item.get(f):
                    params_for_create[f] = item[f]
                    extras[f] = item[f]
            if item.get("measureAuto"):
                extras["measureAuto"] = True
        return create_issue("Requirement", params_for_create, user, extras=extras)

    def _update_fields_fn(item):
        updates = {}
        for f in ("summary", "description"):
            v = item.get(f)
            if v is not None and v != "":
                updates[f] = v
        req_type = (item.get("reqType") or "").upper()
        if req_type in ("FR", "NFR", "CNST"):
            updates["reqtype"] = req_type
        if req_type == "NFR":
            for f in _NFR_FIELDS:
                v = item.get(f)
                if v is not None and v != "":
                    updates[f] = v
            if item.get("measureAuto") is not None:
                updates["measureAuto"] = bool(item["measureAuto"])
        return updates

    buckets = dispatch_by_mode(req_items, existing_req_keys, _create_fn, _update_fields_fn, user)

    # Feature contains Requirement 연결 (created/modified만)
    linked = []
    auto_measure_items = []
    for bucket_name in ("created", "modified"):
        for entry in buckets[bucket_name]:
            req_key = entry["key"]
            item = entry["item"]
            for fk in (item.get("relatedFeatures") or []):
                if fk in valid_feat_keys:
                    try:
                        link_issues(fk, req_key, "contains")
                        linked.append({"from": fk, "to": req_key})
                    except Exception:
                        pass
            if item.get("reqType", "").upper() == "NFR" and item.get("measureAuto"):
                auto_measure_items.append({
                    "key": req_key,
                    "qa": item.get("qa", ""),
                    "measure": item.get("measure", ""),
                })

    # business/requirements.md 저장
    saved_path = None
    save_error = None
    if document.strip():
        try:
            await client.save_file(
                code,
                "business/requirements.md",
                document,
                f"Requirement 정제 (신규 {len(buckets['created'])} / 수정 {len(buckets['modified'])})",
                req_user,
            )
            saved_path = "business/requirements.md"
        except Exception as e:
            save_error = str(e)

    msg_parts = [
        f"Requirement — 신규 {len(buckets['created'])}, 재사용 {len(buckets['reused'])}, "
        f"수정 {len(buckets['modified'])}, 삭제마킹 {len(buckets['markedForDeletion'])}",
        f"Feature-Requirement 연결 {len(linked)}건",
    ]
    await emit_progress("requirement_refine", f"Requirement 정제 완료 — {msg_parts[0]}")
    if saved_path:
        msg_parts.append("requirements.md 저장 완료")
    elif save_error:
        msg_parts.append(f"문서 저장 실패: {save_error}")
    if auto_measure_items:
        msg_parts.append(
            f"⚠ 자동 결정된 NFR Measure {len(auto_measure_items)}건 — 설계자 검토/조정 필요"
        )
    if buckets["markedForDeletion"]:
        msg_parts.append("⚠ 삭제 마킹된 항목은 검토 후 실제 삭제 또는 마킹 해제가 필요합니다")

    tsum = trail_summary(critique_trail)
    if tsum:
        msg_parts.append(tsum)
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
        "autoMeasureItems": auto_measure_items,
        "needsReview": bool(auto_measure_items) or bool(buckets["markedForDeletion"]),
        "critiqueTrail": critique_trail,
        "prerequisitesRan": prereqs_ran,
        "message": " / ".join(msg_parts),
    }
