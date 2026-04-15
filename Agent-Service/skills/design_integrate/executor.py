"""디자인 통합 스킬 v2 — SKILL.md 통합 self-loop.

LLM 이 generate → self-critique → revise → best-so-far 를 단일 호출로 수행.
Python 은 IO, Mermaid 재검증, 문서 저장만 담당. 이슈는 건드리지 않는다.
"""

import os
import re
import json
from typing import Any, Dict, Optional

from core.database import issues, projects
from core.llm import call_llm
from clients.platform_api import PlatformClient
from skills.critique import load_step_policy
from skills.mermaid_utils import (
    get_mermaid_rules, validate_document_mermaid, build_mermaid_fix_prompt,
)


_SKILL_DIR = os.path.dirname(__file__)
_SKILL_MD_PATH = os.path.join(_SKILL_DIR, "SKILL.md")


def _load_skill_md() -> str:
    with open(_SKILL_MD_PATH, encoding="utf-8") as f:
        text = f.read()
    m = re.match(r"^---\n.*?\n---\n(.*)$", text, re.DOTALL)
    return m.group(1) if m else text


def _build_prompt(skill_body: str, code: str, chosen: dict, normalized: dict,
                  designs: list, req_list: list,
                  criteria: str, policy_text: str, mermaid_rules: str) -> str:
    chosen_json = json.dumps(chosen, ensure_ascii=False, indent=2)
    normalized_json = json.dumps(normalized, ensure_ascii=False, indent=2)
    designs_json = json.dumps(designs, ensure_ascii=False, indent=2)
    req_summary = json.dumps(
        [{"key": r.get("key"), "subkey": r.get("subkey", ""),
          "reqtype": r.get("reqtype", ""), "summary": r.get("summary", "")}
         for r in req_list],
        ensure_ascii=False, indent=2,
    )
    return (
        f"{skill_body}\n\n"
        f"---\n\n"
        f"## 주입 데이터\n\n"
        f"### projectCode\n{code}\n\n"
        f"### chosen_styles\n```json\n{chosen_json}\n```\n\n"
        f"### normalized_attributes\n```json\n{normalized_json}\n```\n\n"
        f"### designs ({len(designs)}건, ADSN/DDSN)\n```json\n{designs_json}\n```\n\n"
        f"### req_summary ({len(req_list)}건)\n```json\n{req_summary}\n```\n\n"
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

    from core.progress import emit_progress

    # 1. 선행 산출물 수집
    designs = list(issues().find(
        {"key": {"$regex": f"^{re.escape(code)}-"},
         "issueType": {"$in": ["Architectural Design", "Detailed Design"]}},
        {"key": 1, "subkey": 1, "issueType": 1, "summary": 1, "description": 1,
         "designLevel": 1, "designPoint": 1, "decision": 1, "decisionRationale": 1,
         "tradeoffs": 1, "risk": 1, "sensitivity": 1, "_id": 0},
    ))

    proj = projects().find_one(
        {"jiraCodes.main.jiraCode": code},
        {"archSettings": 1, "_id": 0},
    ) or {}
    arch = proj.get("archSettings") or {}
    chosen = arch.get("chosenStyles") or {}
    normalized = arch.get("normalizedAttributes") or {}

    if not designs or not chosen:
        return {
            "projectCode": code,
            "status": "blocked",
            "blockedAt": "design_decide",
            "suggestSkill": "design_decide",
            "message": "설계 이슈 / 스타일 선행 산출물이 부족합니다. design_decide 를 먼저 실행하세요.",
        }

    req_list = list(issues().find(
        {"key": {"$regex": f"^{re.escape(code)}-"}, "issueType": "Requirement"},
        {"key": 1, "subkey": 1, "summary": 1, "reqtype": 1, "_id": 0},
    ))

    await emit_progress("design_integrate", "디자인 통합 시작 (v2 self-loop)")

    # 2. 정책 로드 + SKILL.md + 주입 데이터로 단일 LLM 호출
    policy = load_step_policy(
        "workflow-architecture-design.md",
        "디자인 통합 (Integrated Design Views)",
    )
    skill_body = _load_skill_md()
    prompt = _build_prompt(
        skill_body, code, chosen, normalized, designs, req_list,
        policy.get("criteria", ""), policy.get("policy_text", ""),
        get_mermaid_rules(),
    )
    raw = await call_llm(prompt)

    parsed = _extract_json_obj(raw)
    if not parsed or "finalDocument" not in parsed:
        return {
            "projectCode": code,
            "status": "failed",
            "message": "LLM 응답 파싱 실패.",
            "raw": raw[:500],
        }

    document = parsed.get("finalDocument") or ""
    views = parsed.get("finalViews") or []
    iterations = parsed.get("iterations") or []
    stop_reason = parsed.get("stopReason", "")
    unresolved = parsed.get("unresolvedCritical") or []

    # 3. Mermaid 정적 검증 + 1회 자동 수정
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

    # 4. 문서 저장 + 완료 플래그
    client = PlatformClient()
    saved_path = None
    save_error = None
    if document.strip():
        try:
            await client.save_file(
                code,
                "architecture/architecture-integrated.md",
                document,
                f"디자인 통합 ({len(views)} views, v2)",
                req_user,
            )
            saved_path = "architecture/architecture-integrated.md"
            projects().update_one(
                {"jiraCodes.main.jiraCode": code},
                {"$set": {"archSettings.designIntegrated": True,
                          "archSettings.viewsIncluded": views}},
            )
        except Exception as e:
            save_error = str(e)

    # 5. 요약
    parts = [f"관점 {len(views)}개 ({', '.join(views)})"]
    if saved_path:
        parts.append("architecture-integrated.md 저장 완료")
    elif save_error:
        parts.append(f"문서 저장 실패: {save_error}")
    if mermaid_validation.get("errors"):
        parts.append(f"⚠ Mermaid 경고 {len(mermaid_validation['errors'])}건")
    base_msg = " / ".join(parts)

    iter_info = ""
    if iterations:
        iter_info = f" (iter {len(iterations)}회, best=iter{parsed.get('selectedIter')}, stop={stop_reason})"

    await emit_progress("design_integrate", f"디자인 통합 완료 — {base_msg}")

    status = "completed"
    if unresolved:
        status = "partial"

    return {
        "projectCode": code,
        "status": status,
        "viewsIncluded": views,
        "documentSaved": saved_path,
        "saveError": save_error,
        "iterations": iterations,
        "selectedIter": parsed.get("selectedIter"),
        "stopReason": stop_reason,
        "unresolvedCritical": unresolved,
        "mermaidValidation": mermaid_validation,
        "message": base_msg + iter_info,
    }
