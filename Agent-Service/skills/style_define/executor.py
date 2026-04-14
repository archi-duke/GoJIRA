"""시스템 스타일 정의 스킬 — 정규화 속성 → 점수 엔진 → LLM 설명 → architecture-style.md."""

import os
import re
import json
from typing import Dict, Any, List

from core.database import projects
from clients.platform_api import PlatformClient
from skills.helpers import load_prompt
from skills.critique import iterate_generate_critique, trail_summary, load_step_policy

from skills.style_define.catalog import load_catalog, validate_catalog
from skills.style_define.engine import recommend

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


def _catalog_summary(catalog: dict) -> str:
    """LLM 에 전달할 카탈로그 요약(스타일 id / scope / summary 만)."""
    lines = []
    for sid, s in sorted(catalog["styles"].items()):
        lines.append(f"- {sid} [{s.get('scope', '-')}]: {s.get('summary', '')}")
    return "\n".join(lines)


async def execute(params: dict, user: str) -> dict:
    code = params.get("projectCode", "")
    if not code:
        raise ValueError("projectCode 필수")
    req_user = params.get("user", user)

    from core.progress import emit_progress

    # 1. normalizedAttributes 확보
    prereqs_ran: List[Dict[str, Any]] = []
    normalized = params.get("normalizedAttributes") or {}
    if not normalized:
        proj = projects().find_one(
            {"jiraCodes.main.jiraCode": code},
            {"archSettings": 1, "_id": 0},
        ) or {}
        normalized = (proj.get("archSettings") or {}).get("normalizedAttributes") or {}

    if not normalized:
        await emit_progress("style_define", "정규화 속성 없음 — driver_select 선행 실행")
        from skills.driver_select.executor import execute as driver_select_execute
        ds = await driver_select_execute(params, user)
        prereqs_ran.append({"skill": "driver_select", "result": ds})
        if ds.get("suggestSkill"):
            return {
                **ds,
                "prerequisitesRan": (ds.get("prerequisitesRan") or []) + prereqs_ran,
                "blockedAt": ds.get("blockedAt") or "driver_select",
            }
        normalized = ds.get("normalizedAttributes") or {}
        prereqs_ran = (ds.get("prerequisitesRan") or []) + prereqs_ran
        if not normalized:
            return {
                "projectCode": code,
                "message": "driver_select 후에도 정규화 속성이 비어있습니다.",
                "prerequisitesRan": prereqs_ran,
            }

    # 2. 카탈로그 로드 + 검증
    catalog = load_catalog()
    problems = validate_catalog(catalog)
    if problems:
        return {
            "projectCode": code,
            "message": "카탈로그 검증 실패",
            "catalogProblems": problems,
            "prerequisitesRan": prereqs_ran,
        }

    await emit_progress("style_define", "점수 엔진 실행 중")
    # 3. 점수 엔진 실행
    rec = recommend(catalog, normalized)
    engine_top = rec["scored"][:int((catalog["index"].get("engine") or {}).get("top_n", 5))]

    # 4. LLM 설명 계층
    normalized_json = json.dumps(normalized, ensure_ascii=False, indent=2)
    primary_json = json.dumps(rec["primary"], ensure_ascii=False)
    secondary_json = json.dumps(rec["secondary"], ensure_ascii=False)
    supporting_json = json.dumps(rec["supporting"], ensure_ascii=False)
    scored_json = json.dumps([
        {"id": s["id"], "score": s["score"], "breakdown": s["breakdown"][:8]}
        for s in engine_top
    ], ensure_ascii=False, indent=2)
    combos_json = json.dumps(rec["combinations_applied"], ensure_ascii=False, indent=2)
    top_n = len(engine_top)

    generate_prompt = load_prompt(_SKILL_DIR, "explain").format(
        code=code,
        normalized_json=normalized_json,
        primary_json=primary_json,
        secondary_json=secondary_json,
        supporting_json=supporting_json,
        scored_json=scored_json,
        combos_json=combos_json,
        top_n=top_n,
        catalog_summary=_catalog_summary(catalog),
    )

    critique_tpl = load_prompt(_SKILL_DIR, "critique")
    revise_tpl = load_prompt(_SKILL_DIR, "revise")

    policy = load_step_policy("workflow-architecture-design.md", "시스템 스타일 정의 (Architecture Style)")

    engine_result_json = json.dumps({
        "primary": rec["primary"],
        "secondary": rec["secondary"],
        "supporting": rec["supporting"],
        "top": [{"id": s["id"], "score": s["score"]} for s in engine_top],
    }, ensure_ascii=False, indent=2)

    context_block = (
        f"## 정규화된 시스템 속성\n{normalized_json}\n\n"
        f"## 점수 엔진 결과 (top)\n{engine_result_json}"
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
            engine_result_json=engine_result_json,
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
            "recommendation": rec,
            "prerequisitesRan": prereqs_ran,
        }

    chosen = parsed.get("chosen") or {}
    document = parsed.get("document") or ""

    # 5. 문서 저장
    client = PlatformClient()
    saved_path = None
    save_error = None
    if document.strip():
        try:
            await client.save_file(
                code,
                "architecture/architecture-style.md",
                document,
                f"시스템 스타일 정의 (primary {len(chosen.get('primary') or [])}건)",
                req_user,
            )
            saved_path = "architecture/architecture-style.md"
        except Exception as e:
            save_error = str(e)

    # 6. 결과 저장 (chosen 도 프로젝트에 영속화)
    if chosen:
        projects().update_one(
            {"jiraCodes.main.jiraCode": code},
            {"$set": {"archSettings.chosenStyles": chosen}},
        )

    tsum = trail_summary(critique_trail)
    parts = []
    if chosen.get("primary"):
        parts.append(f"Primary: {', '.join(chosen['primary'])}")
    if chosen.get("secondary"):
        parts.append(f"Secondary: {', '.join(chosen['secondary'])}")
    if chosen.get("supporting"):
        parts.append(f"Supporting: {', '.join(chosen['supporting'])}")
    if saved_path:
        parts.append("architecture-style.md 저장 완료")
    elif save_error:
        parts.append(f"문서 저장 실패: {save_error}")
    msg = " / ".join(parts) or "스타일 추천 완료"
    if tsum:
        msg += f" / {tsum}"

    await emit_progress("style_define", f"시스템 스타일 정의 완료 — {msg}")

    return {
        "projectCode": code,
        "chosen": chosen,
        "recommendation": {
            "primary": rec["primary"],
            "secondary": rec["secondary"],
            "supporting": rec["supporting"],
            "topScored": [
                {"id": s["id"], "score": s["score"]} for s in engine_top
            ],
        },
        "normalizedAttributes": normalized,
        "documentSaved": saved_path,
        "saveError": save_error,
        "critiqueTrail": critique_trail,
        "prerequisitesRan": prereqs_ran,
        "message": msg,
    }
