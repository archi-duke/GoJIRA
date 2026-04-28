"""Use Case 분석 스킬 v2 — SKILL.md 통합 self-loop.

LLM 이 generate → self-critique (coverage 검증 포함) → revise → best-so-far 를
단일 호출로 수행. Python 은 IO, Mermaid 재검증, docId 치환, 링크 생성만 담당.
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
from skills.mermaid_utils import (
    get_mermaid_rules, validate_document_mermaid, build_mermaid_fix_prompt,
)


_SKILL_DIR = os.path.dirname(__file__)
_SKILL_MD_PATH = os.path.join(_SKILL_DIR, "SKILL.md")
_PROMPTS_DIR = os.path.join(_SKILL_DIR, "..", "..", "prompts")
_USECASE_FIELDS = (
    "actor", "precondition", "basicflow",
    "alternativeflow", "exceptionflow", "postcondition",
)


def _load_skill_md() -> str:
    with open(_SKILL_MD_PATH, encoding="utf-8") as f:
        text = f.read()
    m = re.match(r"^---\n.*?\n---\n(.*)$", text, re.DOTALL)
    return m.group(1) if m else text


def _load_template() -> str:
    path = os.path.join(_PROMPTS_DIR, "use-cases.md")
    try:
        with open(path, encoding="utf-8") as f:
            return f.read()
    except FileNotFoundError:
        return ""


def _build_prompt(skill_body: str, code: str, desc: str, vos_list: list,
                  existing_features: list, template: str,
                  criteria: str, policy_text: str, mermaid_rules: str) -> str:
    desc_trunc = (desc or "")[:5000]
    vos_json = json.dumps(vos_list, ensure_ascii=False, indent=2)
    feat_json = json.dumps(existing_features, ensure_ascii=False, indent=2)
    return (
        f"{skill_body}\n\n"
        f"---\n\n"
        f"## 주입 데이터\n\n"
        f"### projectCode\n{code}\n\n"
        f"### project_description (5000자)\n{desc_trunc}\n\n"
        f"### vos_list ({len(vos_list)}건)\n```json\n{vos_json}\n```\n\n"
        f"### existing_features ({len(existing_features)}건)\n```json\n{feat_json}\n```\n\n"
        f"### template (use-cases.md 양식)\n{template}\n\n"
        f"### criteria\n{criteria}\n\n"
        f"### policy\n{policy_text}\n\n"
        f"### mermaid_rules\n{mermaid_rules}\n\n"
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


async def execute(params: dict, user: str) -> dict:
    code = params.get("projectCode", "")
    if not code:
        raise ValueError("projectCode 필수")
    req_user = params.get("user", user)

    client = PlatformClient()
    from core.progress import emit_progress

    # 1. project-description.md (docs/{user} → main)
    desc = await client.get_file_content(code, "business/project-description.md", ref=f"docs/{req_user}")
    if not desc or _is_template_only(desc):
        desc = await client.get_file_content(code, "business/project-description.md", ref="main")
    if not desc or _is_template_only(desc):
        return {
            "projectCode": code,
            "status": "blocked",
            "blockedAt": "projdesc",
            "message": "프로젝트 관련 정보가 불충분합니다. 과제 설명 문서 작성을 먼저 진행해야 합니다.",
            "suggestSkill": "projdesc_interview",
            "created": [], "reused": [], "modified": [], "markedForDeletion": [],
        }

    # 2. 기존 VOS / Feature
    vos_list = list(issues().find(
        {"key": {"$regex": f"^{re.escape(code)}-"}, "issueType": "VOS"},
        {"key": 1, "summary": 1, "description": 1, "client": 1, "_id": 0},
    ))
    if not vos_list:
        return {
            "projectCode": code,
            "status": "blocked",
            "blockedAt": "vos_collect",
            "suggestSkill": "vos_collect",
            "message": "VOS가 수집되어 있지 않습니다. vos_collect 를 먼저 실행하세요.",
            "created": [], "reused": [], "modified": [], "markedForDeletion": [],
        }

    existing_features = list(issues().find(
        {"key": {"$regex": f"^{re.escape(code)}-"}, "issueType": "Feature"},
        {"key": 1, "subkey": 1, "summary": 1, "description": 1, "actor": 1,
         "precondition": 1, "basicflow": 1, "alternativeflow": 1,
         "exceptionflow": 1, "postcondition": 1, "_id": 0},
    ))
    existing_feature_keys = {f["key"] for f in existing_features}
    existing_feature_by_key = {f["key"]: f for f in existing_features}
    valid_vos_keys = {v["key"] for v in vos_list}

    await emit_progress("usecase_analyze", "Use Case/Feature 분석 시작 (v2 self-loop)")

    # 3. LLM 단일 호출 (self-loop 내장)
    policy = load_step_policy("workflow-requirement-analysis.md", "Use Case 분석")
    template = _load_template()
    skill_body = _load_skill_md()
    prompt = _build_prompt(
        skill_body, code, desc, vos_list, existing_features, template,
        policy.get("criteria", ""), policy.get("policy_text", ""),
        get_mermaid_rules(),
    )
    raw = await call_llm(prompt)

    parsed = _extract_json_obj(raw)
    if not parsed or "finalFeatures" not in parsed:
        return {
            "projectCode": code,
            "status": "failed",
            "message": "LLM 응답 파싱 실패.",
            "raw": raw[:500],
            "created": [], "reused": [], "modified": [], "markedForDeletion": [],
        }

    features = parsed.get("finalFeatures") or []
    document = parsed.get("finalDocument") or ""
    coverage_matrix = parsed.get("finalCoverage") or {}
    iterations = parsed.get("iterations") or []
    stop_reason = parsed.get("stopReason", "")
    unresolved = parsed.get("unresolvedCritical") or []

    # 3.5 Mermaid 검증 (LLM 자가수정과 별개로 1회 자동 수정 시도)
    mermaid_validation = validate_document_mermaid(document)
    if not mermaid_validation["valid"] and document:
        try:
            fixed = await call_llm(build_mermaid_fix_prompt(document, mermaid_validation["errors"]))
            recheck = validate_document_mermaid(fixed)
            if len(recheck["errors"]) < len(mermaid_validation["errors"]):
                document = fixed
                mermaid_validation = recheck
        except Exception:
            pass

    # 4. mode 분기 처리
    async def _create_fn(item):
        fp = {
            "projectCode": code,
            "summary": (item.get("summary") or "").strip(),
            "description": item.get("description", ""),
        }
        for f in _USECASE_FIELDS:
            if item.get(f):
                fp[f] = item[f]
        extras = {f: fp[f] for f in _USECASE_FIELDS if fp.get(f)}
        return await create_issue("Feature", fp, user, extras=extras or None)

    def _update_fields_fn(item):
        updates = {}
        for f in ("summary", "description") + _USECASE_FIELDS:
            v = item.get(f)
            if v is not None and v != "":
                updates[f] = v
        return updates

    buckets = await dispatch_by_mode(features, existing_feature_keys, _create_fn, _update_fields_fn, user)

    # 5. VOS-Feature 링크 (created/modified/reused 전부)
    linked = []
    covered_vos: set = set()
    for bucket_name in ("created", "modified", "reused"):
        for entry in buckets.get(bucket_name, []):
            feat_key = entry["key"]
            for vos_key in (entry["item"].get("relatedVOS") or []):
                if vos_key in valid_vos_keys and vos_key not in covered_vos:
                    try:
                        await link_issues(vos_key, feat_key, "contains", user=user)
                        linked.append({"from": vos_key, "to": feat_key})
                        covered_vos.add(vos_key)
                    except Exception as e:
                        # silent drop 금지 — 로그에 남김
                        import logging
                        logging.getLogger("agent.usecase").warning(
                            "link 실패 %s → %s: %s", vos_key, feat_key, e
                        )

    # 5.5 docId → subkey 치환
    docid_map = build_docid_subkey_map(buckets, existing_feature_by_key)
    document, leftover_doc_ids = substitute_doc_ids(document, docid_map)

    # 6. business/use-cases.md 저장
    saved_path = None
    save_error = None
    if document.strip():
        try:
            await client.save_file(
                code,
                "business/use-cases.md",
                document,
                f"Use Case 분석 (Feature 신규 {len(buckets['created'])} / 수정 {len(buckets['modified'])}, v2)",
                req_user,
            )
            saved_path = "business/use-cases.md"
        except Exception as e:
            save_error = str(e)

    # 7. orphan 감사 — LLM 이 자가검증했지만 실제 링크 생성 후 누락된 것 확인
    nfr_deferred = {
        k for k, v in coverage_matrix.items()
        if isinstance(v, list) and any("NFR" in str(x) for x in v)
    }
    still_orphan = [v for v in valid_vos_keys if v not in covered_vos and v not in nfr_deferred]

    # 8. 요약
    parts = [
        f"Feature — 신규 {len(buckets['created'])}, 재사용 {len(buckets['reused'])}, "
        f"수정 {len(buckets['modified'])}, 삭제마킹 {len(buckets['markedForDeletion'])}",
        f"VOS-Feature 연결 {len(linked)}건",
    ]
    if saved_path:
        parts.append("use-cases.md 저장 완료")
    elif save_error:
        parts.append(f"문서 저장 실패: {save_error}")
    base_msg = " / ".join(parts)

    iter_info = ""
    if iterations:
        iter_info = f" (iter {len(iterations)}회, best=iter{parsed.get('selectedIter')}, stop={stop_reason})"

    notes = ""
    if mermaid_validation.get("errors"):
        notes += f" / ⚠ Mermaid 경고 {len(mermaid_validation['errors'])}건"
    if leftover_doc_ids:
        notes += f" / ⚠ 미치환 docId {len(leftover_doc_ids)}건"
    if still_orphan:
        notes += f" / ⚠ orphan VOS {len(still_orphan)}건: {', '.join(still_orphan[:5])}"

    await emit_progress("usecase_analyze", f"Use Case 분석 완료 — {base_msg}")

    status = "completed"
    if still_orphan or unresolved:
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
        "mermaidValidation": mermaid_validation,
        "docIdMap": docid_map,
        "leftoverDocIds": leftover_doc_ids,
        "coverage": {
            "totalVos": len(valid_vos_keys),
            "covered": len(covered_vos),
            "nfrDeferred": sorted(nfr_deferred),
            "orphan": len(still_orphan),
            "orphanKeys": still_orphan[:20],
        },
        "message": base_msg + iter_info + notes,
    }
