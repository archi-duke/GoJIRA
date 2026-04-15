"""Use Case 분석 스킬 — VOS → Use Case → Feature 이슈 + business/use-cases.md (mode 분기 지원)."""

import os
import re
import json
from typing import Optional

from core.database import issues
from clients.platform_api import PlatformClient
from skills.helpers import (
    create_issue, link_issues, dispatch_by_mode,
    is_template_only as _is_template_only,
    load_prompt,
    build_docid_subkey_map, substitute_doc_ids,
)
from skills.critique import iterate_generate_critique, trail_summary, load_step_policy
from skills.mermaid_utils import (
    get_mermaid_rules, validate_document_mermaid, build_mermaid_fix_prompt,
)
from core.llm import call_llm


_SKILL_DIR = os.path.dirname(__file__)
_PROMPTS_DIR = os.path.join(_SKILL_DIR, "..", "..", "prompts")
_USECASE_FIELDS = (
    "actor", "precondition", "basicflow",
    "alternativeflow", "exceptionflow", "postcondition",
)


def _load_template() -> str:
    path = os.path.join(_PROMPTS_DIR, "use-cases.md")
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


async def execute(params: dict, user: str) -> dict:
    code = params.get("projectCode", "")
    if not code:
        raise ValueError("projectCode 필수")
    req_user = params.get("user", user)

    client = PlatformClient()

    # 1. project-description.md
    desc = await client.get_file_content(code, "business/project-description.md", ref="main")
    if not desc or _is_template_only(desc):
        alt = await client.get_file_content(code, "business/project-description.md", ref=f"docs/{req_user}")
        if alt and not _is_template_only(alt):
            desc = alt
    if not desc or _is_template_only(desc):
        return {
            "projectCode": code,
            "status": "blocked",
            "blockedAt": "projdesc",
            "message": "프로젝트 관련 정보가 불충분합니다. 과제 설명 문서 작성을 먼저 진행해야 합니다.",
            "suggestSkill": "projdesc_interview",
            "created": [], "reused": [], "modified": [], "markedForDeletion": [],
        }

    # 2. 기존 이슈 조회
    vos_list = list(issues().find(
        {"key": {"$regex": f"^{re.escape(code)}-"}, "issueType": "VOS"},
        {"key": 1, "summary": 1, "description": 1, "client": 1, "_id": 0},
    ))
    from core.progress import emit_progress
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

    await emit_progress("usecase_analyze", "Use Case/Feature 분석 시작")
    # 3. 생성 + 비판적 평가 루프 (프롬프트는 prompts/*.txt로 분리)
    template = _load_template()
    desc_trunc = desc[:5000]
    vos_json = json.dumps(vos_list, ensure_ascii=False, indent=2)
    feat_json = json.dumps(existing_features, ensure_ascii=False, indent=2)

    generate_prompt = load_prompt(_SKILL_DIR, "generate").format(
        code=code,
        desc_trunc=desc_trunc,
        vos_count=len(vos_list),
        vos_json=vos_json,
        existing_feature_count=len(existing_features),
        feat_json=feat_json,
        template=template,
        mermaid_rules=get_mermaid_rules(),
    )

    critique_tpl = load_prompt(_SKILL_DIR, "critique")
    revise_tpl = load_prompt(_SKILL_DIR, "revise")

    policy = load_step_policy("workflow-requirement-analysis.md", "Use Case 분석")

    context_block = (
        f"## 원본 과제 설명 (요약)\n{desc_trunc[:3000]}\n\n"
        f"## VOS 목록\n{vos_json}"
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
            vos_json=vos_json,
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
            "status": "failed",
            "message": "LLM 응답을 파싱할 수 없습니다.",
            "raw": raw[:500],
            "created": [], "reused": [], "modified": [], "markedForDeletion": [],
        }

    features = parsed.get("features") or []
    document = parsed.get("document") or ""

    # 3.5 Mermaid 정적 검증 — 오류 있으면 한번만 자동 수정 시도
    mermaid_validation = validate_document_mermaid(document)
    if not mermaid_validation["valid"] and document:
        try:
            fixed = await call_llm(build_mermaid_fix_prompt(document, mermaid_validation["errors"]))
            # 수정 후 재검증
            recheck = validate_document_mermaid(fixed)
            # 수정본이 덜 틀리면 채택
            if len(recheck["errors"]) < len(mermaid_validation["errors"]):
                document = fixed
                mermaid_validation = recheck
        except Exception:
            pass  # 실패해도 원본 유지

    # 4. mode 분기 처리
    async def _create_fn(item):
        feature_params = {
            "projectCode": code,
            "summary": (item.get("summary") or "").strip(),
            "description": item.get("description", ""),
        }
        for f in _USECASE_FIELDS:
            if item.get(f):
                feature_params[f] = item[f]
        extras = {f: feature_params[f] for f in _USECASE_FIELDS if feature_params.get(f)}
        return await create_issue("Feature", feature_params, user, extras=extras or None)

    def _update_fields_fn(item):
        updates = {}
        for f in ("summary", "description") + _USECASE_FIELDS:
            v = item.get(f)
            if v is not None and v != "":
                updates[f] = v
        return updates

    buckets = await dispatch_by_mode(features, existing_feature_keys, _create_fn, _update_fields_fn, user)

    # 5. VOS contains Feature 연결 (created/modified만)
    linked = []
    for bucket_name in ("created", "modified"):
        for entry in buckets[bucket_name]:
            feat_key = entry["key"]
            for vos_key in (entry["item"].get("relatedVOS") or []):
                if vos_key in valid_vos_keys:
                    try:
                        await link_issues(vos_key, feat_key, "contains", user=user)
                        linked.append({"from": vos_key, "to": feat_key})
                    except Exception:
                        pass

    # 5.5 docId → subkey 치환 (문서 ID와 실제 이슈 subkey 동기화)
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
                f"Use Case 분석 결과 업데이트 (Feature 신규 {len(buckets['created'])} / 수정 {len(buckets['modified'])})",
                req_user,
            )
            saved_path = "business/use-cases.md"
        except Exception as e:
            save_error = str(e)

    tsum = trail_summary(critique_trail)
    base_msg = _summarize(buckets, len(linked), saved_path, save_error)
    await emit_progress("usecase_analyze", f"Use Case 분석 완료 — {base_msg}")
    mermaid_note = ""
    if mermaid_validation.get("errors"):
        mermaid_note = f" / ⚠ Mermaid 검증 경고 {len(mermaid_validation['errors'])}건"
    docid_note = ""
    if leftover_doc_ids:
        docid_note = f" / ⚠ 미치환 docId {len(leftover_doc_ids)}건: {', '.join(leftover_doc_ids[:5])}"
    return {
        "projectCode": code,
        "status": "completed",
        "created": buckets["created"],
        "reused": buckets["reused"],
        "modified": buckets["modified"],
        "markedForDeletion": buckets["markedForDeletion"],
        "linked": linked,
        "linkedCount": len(linked),
        "documentSaved": saved_path,
        "saveError": save_error,
        "critiqueTrail": critique_trail,
        "mermaidValidation": mermaid_validation,
        "docIdMap": docid_map,
        "leftoverDocIds": leftover_doc_ids,
        "message": base_msg + (f" / {tsum}" if tsum else "") + mermaid_note + docid_note,
    }


def _summarize(buckets: dict, link_cnt: int, saved: Optional[str], err: Optional[str]) -> str:
    parts = [f"Feature — 신규 {len(buckets['created'])}, 재사용 {len(buckets['reused'])}, "
             f"수정 {len(buckets['modified'])}, 삭제마킹 {len(buckets['markedForDeletion'])}"]
    parts.append(f"VOS-Feature 연결 {link_cnt}건")
    if saved:
        parts.append("use-cases.md 저장 완료")
    elif err:
        parts.append(f"문서 저장 실패: {err}")
    if buckets["markedForDeletion"]:
        parts.append("⚠ 삭제 마킹된 항목은 검토 후 실제 삭제 또는 마킹 해제가 필요합니다")
    return " / ".join(parts)
