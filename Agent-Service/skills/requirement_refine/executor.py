"""Requirement 정제 스킬 v2 — SKILL.md 통합 self-loop.

LLM 이 내부에서 generate → self-critique → revise → best-so-far 를
단일 호출로 수행. Python 은 IO 와 mode 분기, 링크 생성, docId 치환, 문서 저장만 담당.
"""

import os
import re
import json
from typing import Any, Dict, Optional

from core.database import issues
from core.llm import call_llm
from clients.platform_api import PlatformClient
from skills.helpers import (
    create_issue, link_issues, dispatch_by_mode,
    is_template_only as _is_template_only,
    build_docid_subkey_map, substitute_doc_ids,
)
from skills.critique import load_step_policy


_SKILL_DIR = os.path.dirname(__file__)
_SKILL_MD_PATH = os.path.join(_SKILL_DIR, "SKILL.md")
_PROMPTS_DIR = os.path.join(_SKILL_DIR, "..", "..", "prompts")

_NFR_FIELDS = (
    "qa", "source", "stimulus", "environment", "artifact",
    "response", "measure", "responseScenario",
)


def _load_skill_md() -> str:
    with open(_SKILL_MD_PATH, encoding="utf-8") as f:
        text = f.read()
    m = re.match(r"^---\n.*?\n---\n(.*)$", text, re.DOTALL)
    return m.group(1) if m else text


def _load_template() -> str:
    path = os.path.join(_PROMPTS_DIR, "requirements.md")
    try:
        with open(path, encoding="utf-8") as f:
            return f.read()
    except FileNotFoundError:
        return ""


def _build_prompt(skill_body: str, code: str, desc: str, use_cases: str,
                  feat_list: list, vos_list: list, existing_reqs: list,
                  template: str, criteria: str, policy_text: str) -> str:
    desc_trunc = (desc or "")[:4000]
    uc_trunc = (use_cases or "")[:4000]
    feat_json = json.dumps(feat_list, ensure_ascii=False, indent=2)
    vos_json = json.dumps(vos_list, ensure_ascii=False, indent=2)
    existing_req_json = json.dumps(existing_reqs, ensure_ascii=False, indent=2)
    return (
        f"{skill_body}\n\n"
        f"---\n\n"
        f"## 주입 데이터\n\n"
        f"### projectCode\n{code}\n\n"
        f"### project_description (4000자)\n{desc_trunc}\n\n"
        f"### use_cases (4000자)\n{uc_trunc}\n\n"
        f"### feat_list ({len(feat_list)}건)\n```json\n{feat_json}\n```\n\n"
        f"### vos_list ({len(vos_list)}건)\n```json\n{vos_json}\n```\n\n"
        f"### existing_reqs ({len(existing_reqs)}건)\n```json\n{existing_req_json}\n```\n\n"
        f"### template (requirements.md 양식)\n{template}\n\n"
        f"### criteria\n{criteria}\n\n"
        f"### policy\n{policy_text}\n\n"
        f"이제 위 `실행 흐름` 섹션대로 수행하고, 지정된 JSON 한 개만 출력하라."
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
    from core.progress import emit_progress

    # 1. 문서 로드
    desc = await _read_doc(client, code, "business/project-description.md", req_user)
    use_cases = await _read_doc(client, code, "business/use-cases.md", req_user)

    if _is_template_only(desc):
        return {
            "projectCode": code,
            "status": "blocked",
            "blockedAt": "projdesc",
            "message": "프로젝트 관련 정보가 불충분합니다. 과제 설명 문서 작성을 먼저 진행해야 합니다.",
            "suggestSkill": "projdesc_interview",
            "created": [], "reused": [], "modified": [], "markedForDeletion": [],
        }

    # 2. 기존 이슈 조회
    feat_list = list(issues().find(
        {"key": {"$regex": f"^{re.escape(code)}-"}, "issueType": "Feature"},
        {"key": 1, "summary": 1, "description": 1,
         "actor": 1, "precondition": 1, "basicflow": 1,
         "alternativeflow": 1, "exceptionflow": 1, "postcondition": 1, "_id": 0},
    ))
    if not feat_list:
        return {
            "projectCode": code,
            "status": "blocked",
            "blockedAt": "usecase_analyze",
            "suggestSkill": "usecase_analyze",
            "message": "Feature 가 없습니다. usecase_analyze 를 먼저 실행하세요.",
            "created": [], "reused": [], "modified": [], "markedForDeletion": [],
        }

    vos_list = list(issues().find(
        {"key": {"$regex": f"^{re.escape(code)}-"}, "issueType": "VOS"},
        {"key": 1, "summary": 1, "_id": 0},
    ))
    existing_reqs = list(issues().find(
        {"key": {"$regex": f"^{re.escape(code)}-"}, "issueType": "Requirement"},
        {"key": 1, "subkey": 1, "summary": 1, "description": 1, "reqtype": 1,
         "qa": 1, "measure": 1, "_id": 0},
    ))
    existing_req_keys = {r["key"] for r in existing_reqs}
    existing_req_by_key = {r["key"]: r for r in existing_reqs}
    valid_feat_keys = {f["key"] for f in feat_list}

    await emit_progress("requirement_refine", "Requirement 정제 시작 (v2 self-loop)")

    # 3. LLM 단일 호출 (self-loop 내장)
    policy = load_step_policy("workflow-requirement-analysis.md", "Requirement 정제")
    template = _load_template()
    skill_body = _load_skill_md()
    prompt = _build_prompt(
        skill_body, code, desc, use_cases, feat_list, vos_list, existing_reqs,
        template, policy.get("criteria", ""), policy.get("policy_text", ""),
    )
    raw = await call_llm(prompt)

    parsed = _extract_json_obj(raw)
    if not parsed or "finalRequirements" not in parsed:
        return {
            "projectCode": code,
            "status": "failed",
            "message": "LLM 응답 파싱 실패.",
            "raw": raw[:500],
            "created": [], "reused": [], "modified": [], "markedForDeletion": [],
        }

    req_items = parsed.get("finalRequirements") or []
    document = parsed.get("finalDocument") or ""
    coverage_matrix = parsed.get("finalCoverage") or {}
    iterations = parsed.get("iterations") or []
    stop_reason = parsed.get("stopReason", "")
    unresolved = parsed.get("unresolvedCritical") or []

    # 4. mode 분기
    async def _create_fn(item):
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
        return await create_issue("Requirement", params_for_create, user, extras=extras)

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

    buckets = await dispatch_by_mode(req_items, existing_req_keys, _create_fn, _update_fields_fn, user)

    # 5. Feature contains Requirement 연결 (created/modified)
    linked = []
    auto_measure_items = []
    covered_features: set = set()
    for bucket_name in ("created", "modified"):
        for entry in buckets[bucket_name]:
            req_key = entry["key"]
            item = entry["item"]
            for fk in (item.get("relatedFeatures") or []):
                if fk in valid_feat_keys:
                    try:
                        await link_issues(fk, req_key, "contains", user=user)
                        linked.append({"from": fk, "to": req_key})
                        covered_features.add(fk)
                    except Exception as e:
                        import logging
                        logging.getLogger("agent.requirement").warning(
                            "link 실패 %s → %s: %s", fk, req_key, e
                        )
            if item.get("reqType", "").upper() == "NFR" and item.get("measureAuto"):
                auto_measure_items.append({
                    "key": req_key,
                    "qa": item.get("qa", ""),
                    "measure": item.get("measure", ""),
                })

    # 6. docId → subkey 치환
    docid_map = build_docid_subkey_map(buckets, existing_req_by_key)
    document, leftover_doc_ids = substitute_doc_ids(document, docid_map)

    # 7. 문서 저장
    saved_path = None
    save_error = None
    if document.strip():
        try:
            await client.save_file(
                code,
                "business/requirements.md",
                document,
                f"Requirement 정제 (신규 {len(buckets['created'])} / 수정 {len(buckets['modified'])}, v2)",
                req_user,
            )
            saved_path = "business/requirements.md"
        except Exception as e:
            save_error = str(e)

    # 8. orphan Feature 감사 (LLM 자가검증 후 실제 링크 기준)
    global_covered = {
        k for k, v in coverage_matrix.items()
        if isinstance(v, list) and any(("CNST" in str(x) or "NFR" in str(x) or "전역" in str(x)) for x in v)
    }
    still_orphan = [f for f in valid_feat_keys if f not in covered_features and f not in global_covered]

    # 9. 요약
    msg_parts = [
        f"Requirement — 신규 {len(buckets['created'])}, 재사용 {len(buckets['reused'])}, "
        f"수정 {len(buckets['modified'])}, 삭제마킹 {len(buckets['markedForDeletion'])}",
        f"Feature-Requirement 연결 {len(linked)}건",
    ]
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
    if leftover_doc_ids:
        msg_parts.append(f"⚠ 미치환 docId {len(leftover_doc_ids)}건: {', '.join(leftover_doc_ids[:5])}")
    if still_orphan:
        msg_parts.append(f"⚠ orphan Feature {len(still_orphan)}건: {', '.join(still_orphan[:5])}")

    iter_info = ""
    if iterations:
        iter_info = f" (iter {len(iterations)}회, best=iter{parsed.get('selectedIter')}, stop={stop_reason})"

    base_msg = " / ".join(msg_parts)
    await emit_progress("requirement_refine", f"Requirement 정제 완료 — {msg_parts[0]}")

    needs_review = bool(auto_measure_items) or bool(buckets["markedForDeletion"]) or bool(still_orphan) or bool(unresolved)
    status = "partial" if needs_review else "completed"

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
        "autoMeasureItems": auto_measure_items,
        "needsReview": needs_review,
        "iterations": iterations,
        "selectedIter": parsed.get("selectedIter"),
        "stopReason": stop_reason,
        "unresolvedCritical": unresolved,
        "coverageMatrix": coverage_matrix,
        "docIdMap": docid_map,
        "leftoverDocIds": leftover_doc_ids,
        "coverage": {
            "totalFeatures": len(valid_feat_keys),
            "covered": len(covered_features),
            "globalDeferred": sorted(global_covered),
            "orphan": len(still_orphan),
            "orphanKeys": still_orphan[:20],
        },
        "message": base_msg + iter_info,
    }
