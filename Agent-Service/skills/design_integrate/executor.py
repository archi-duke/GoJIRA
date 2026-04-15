"""디자인 통합 스킬 — ADSN/DDSN + Style → architecture/architecture-integrated.md (문서만)."""

import os
import re
import json
from core.database import issues, projects
from clients.platform_api import PlatformClient
from skills.helpers import load_prompt
from skills.critique import iterate_generate_critique, trail_summary, load_step_policy
from skills.mermaid_utils import (
    get_mermaid_rules, validate_document_mermaid, build_mermaid_fix_prompt,
)
from core.llm import call_llm


_SKILL_DIR = os.path.dirname(__file__)


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

    from core.progress import emit_progress

    # 1. 선행 확인
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

    await emit_progress("design_integrate", "디자인 통합 문서 작성 시작")

    designs_json = json.dumps(designs, ensure_ascii=False, indent=2)
    req_summary = json.dumps(
        [{"key": r["key"], "subkey": r.get("subkey", ""),
          "reqtype": r.get("reqtype", ""), "summary": r.get("summary", "")}
         for r in req_list],
        ensure_ascii=False, indent=2,
    )
    normalized_json = json.dumps(normalized, ensure_ascii=False, indent=2)

    generate_prompt = load_prompt(_SKILL_DIR, "generate").format(
        code=code,
        normalized_json=normalized_json,
        primary=", ".join(chosen.get("primary") or []),
        secondary=", ".join(chosen.get("secondary") or []),
        supporting=", ".join(chosen.get("supporting") or []),
        design_count=len(designs),
        designs_json=designs_json,
        req_count=len(req_list),
        req_summary=req_summary,
        mermaid_rules=get_mermaid_rules(),
    )

    critique_tpl = load_prompt(_SKILL_DIR, "critique")
    revise_tpl = load_prompt(_SKILL_DIR, "revise")

    policy = load_step_policy(
        "workflow-architecture-design.md",
        "디자인 통합 (Integrated Design Views)",
    )

    context_block = (
        f"## 선정 스타일\n{json.dumps(chosen, ensure_ascii=False)}\n\n"
        f"## 설계 결정 요약\n"
        + "\n".join(
            f"- {d.get('subkey','')}: {d.get('summary','')}"
            for d in designs
        )
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
            designs_json=designs_json,
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
        }

    views = parsed.get("viewsIncluded") or []
    document = parsed.get("document") or ""

    # Mermaid 정적 검증 + 1회 자동 수정
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

    client = PlatformClient()
    saved_path = None
    save_error = None
    if document.strip():
        try:
            await client.save_file(
                code,
                "architecture/architecture-integrated.md",
                document,
                f"디자인 통합 ({len(views)} views)",
                req_user,
            )
            saved_path = "architecture/architecture-integrated.md"
            # 완료 플래그 저장 — workflow_state.next_recommended_step() 에서 중복 호출 방지용
            projects().update_one(
                {"jiraCodes.main.jiraCode": code},
                {"$set": {"archSettings.designIntegrated": True,
                          "archSettings.viewsIncluded": views}},
            )
        except Exception as e:
            save_error = str(e)

    tsum = trail_summary(critique_trail)
    parts = [f"관점 {len(views)}개 ({', '.join(views)})"]
    if saved_path:
        parts.append("architecture-integrated.md 저장 완료")
    elif save_error:
        parts.append(f"문서 저장 실패: {save_error}")
    if mermaid_validation.get("errors"):
        parts.append(f"⚠ Mermaid 검증 경고 {len(mermaid_validation['errors'])}건")
    if tsum:
        parts.append(tsum)
    msg = " / ".join(parts)

    await emit_progress("design_integrate", f"디자인 통합 완료 — {msg}")

    return {
        "projectCode": code,
        "status": "completed",
        "viewsIncluded": views,
        "documentSaved": saved_path,
        "saveError": save_error,
        "mermaidValidation": mermaid_validation,
        "critiqueTrail": critique_trail,
        "message": msg,
    }
