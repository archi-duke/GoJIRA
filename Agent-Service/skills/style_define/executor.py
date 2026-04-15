"""시스템 스타일 정의 스킬 v2 — SKILL.md self-loop 통합.

결정적 로직(카탈로그 로드/검증, 점수 엔진) 은 Python (`catalog.py`, `engine.py`) 이
담당하고, LLM 은 엔진 결과 위에서 조합 해석/근거 서술/자가검증을 단일 호출로 수행.

- Python: 카탈로그 YAML 로드·스키마 검증 → 점수 엔진 실행 → Driver 소급 입력 준비 → 문서 저장
- LLM: 엔진 top 을 기반으로 Primary/Secondary/Supporting 확정, 근거 서술, self-critique 루프
"""

import os
import re
import json
from typing import Any, Dict, Optional

from core.database import projects
from core.llm import call_llm
from clients.platform_api import PlatformClient
from skills.critique import load_step_policy

from skills.style_define.catalog import load_catalog, validate_catalog
from skills.style_define.engine import recommend


_SKILL_DIR = os.path.dirname(__file__)
_SKILL_MD_PATH = os.path.join(_SKILL_DIR, "SKILL.md")


def _load_skill_md() -> str:
    with open(_SKILL_MD_PATH, encoding="utf-8") as f:
        text = f.read()
    m = re.match(r"^---\n.*?\n---\n(.*)$", text, re.DOTALL)
    return m.group(1) if m else text


def _catalog_summary(catalog: dict) -> str:
    lines = []
    for sid, s in sorted(catalog["styles"].items()):
        lines.append(f"- {sid} [{s.get('scope', '-')}]: {s.get('summary', '')}")
    return "\n".join(lines)


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


def _build_prompt(skill_body: str, code: str, normalized: dict,
                  engine_result: dict, top_scored: list, combos: list,
                  catalog_summary: str, criteria: str, policy_text: str) -> str:
    normalized_json = json.dumps(normalized, ensure_ascii=False, indent=2)
    engine_json = json.dumps(engine_result, ensure_ascii=False, indent=2)
    scored_json = json.dumps(top_scored, ensure_ascii=False, indent=2)
    combos_json = json.dumps(combos, ensure_ascii=False, indent=2)
    return (
        f"{skill_body}\n\n"
        f"---\n\n"
        f"## 주입 데이터\n\n"
        f"### projectCode\n{code}\n\n"
        f"### normalized_attributes (Driver 근거)\n```json\n{normalized_json}\n```\n\n"
        f"### engine_result (엔진 제안 — 순위/조합 존중)\n```json\n{engine_json}\n```\n\n"
        f"### top_scored (상위 점수 breakdown)\n```json\n{scored_json}\n```\n\n"
        f"### combinations_applied\n```json\n{combos_json}\n```\n\n"
        f"### catalog_summary\n{catalog_summary}\n\n"
        f"### criteria\n{criteria}\n\n"
        f"### policy\n{policy_text}\n\n"
        f"이제 위 `실행 흐름` 섹션대로 수행하고 지정된 JSON 하나만 출력하라."
    )


async def execute(params: dict, user: str) -> dict:
    code = params.get("projectCode", "")
    if not code:
        raise ValueError("projectCode 필수")
    req_user = params.get("user", user)

    from core.progress import emit_progress

    # 1. normalizedAttributes 확보
    normalized = params.get("normalizedAttributes") or {}
    if not normalized:
        proj = projects().find_one(
            {"jiraCodes.main.jiraCode": code},
            {"archSettings": 1, "_id": 0},
        ) or {}
        normalized = (proj.get("archSettings") or {}).get("normalizedAttributes") or {}

    if not normalized:
        return {
            "projectCode": code,
            "status": "blocked",
            "blockedAt": "driver_select",
            "suggestSkill": "driver_select",
            "message": "정규화 속성이 없습니다. driver_select 를 먼저 실행하세요.",
        }

    # 2. 카탈로그 로드 + 검증 (Python 결정적 로직)
    catalog = load_catalog()
    problems = validate_catalog(catalog)
    if problems:
        return {
            "projectCode": code,
            "status": "failed",
            "message": "카탈로그 검증 실패",
            "catalogProblems": problems,
        }

    await emit_progress("style_define", "점수 엔진 실행 중 (v2 self-loop)")

    # 3. 점수 엔진 (Python 결정적)
    rec = recommend(catalog, normalized)
    top_n = int((catalog["index"].get("engine") or {}).get("top_n", 5))
    engine_top = rec["scored"][:top_n]
    top_scored = [
        {"id": s["id"], "score": s["score"], "breakdown": s["breakdown"][:8]}
        for s in engine_top
    ]
    engine_result = {
        "primary": rec["primary"],
        "secondary": rec["secondary"],
        "supporting": rec["supporting"],
        "top": [{"id": s["id"], "score": s["score"]} for s in engine_top],
    }

    # 4. LLM 단일 호출 (self-loop 내장)
    policy = load_step_policy(
        "workflow-architecture-design.md",
        "시스템 스타일 정의 (Architecture Style)",
    )
    skill_body = _load_skill_md()
    prompt = _build_prompt(
        skill_body, code, normalized,
        engine_result, top_scored, rec["combinations_applied"],
        _catalog_summary(catalog),
        policy.get("criteria", ""), policy.get("policy_text", ""),
    )
    raw = await call_llm(prompt)
    parsed = _extract_json_obj(raw)
    if not parsed or "finalChosen" not in parsed:
        return {
            "projectCode": code,
            "status": "failed",
            "message": "LLM 응답 파싱 실패.",
            "raw": raw[:500],
            "recommendation": rec,
        }

    chosen = parsed.get("finalChosen") or {}
    document = parsed.get("finalDocument") or ""
    iterations = parsed.get("iterations") or []
    stop_reason = parsed.get("stopReason", "")
    unresolved = parsed.get("unresolvedCritical") or []

    # 4.5 카탈로그 id 검증 (hallucination 방어)
    valid_ids = set(catalog["styles"].keys())
    invalid_chosen = []
    for bucket in ("primary", "secondary", "supporting"):
        for sid in (chosen.get(bucket) or []):
            if sid not in valid_ids:
                invalid_chosen.append(f"{bucket}:{sid}")

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
                f"시스템 스타일 정의 (primary {len(chosen.get('primary') or [])}건, v2)",
                req_user,
            )
            saved_path = "architecture/architecture-style.md"
        except Exception as e:
            save_error = str(e)

    # 6. chosen 을 프로젝트에 영속화
    if chosen and not invalid_chosen:
        projects().update_one(
            {"jiraCodes.main.jiraCode": code},
            {"$set": {"archSettings.chosenStyles": chosen}},
        )

    # 7. 요약
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

    iter_info = ""
    if iterations:
        iter_info = f" (iter {len(iterations)}회, best=iter{parsed.get('selectedIter')}, stop={stop_reason})"
    notes = ""
    if invalid_chosen:
        notes += f" / ⚠ 카탈로그 외 id {len(invalid_chosen)}건: {', '.join(invalid_chosen)}"
    if unresolved:
        notes += f" / ⚠ unresolvedCritical {len(unresolved)}건"

    await emit_progress("style_define", f"시스템 스타일 정의 완료 — {msg}")

    status = "completed"
    if invalid_chosen or unresolved:
        status = "partial"

    return {
        "projectCode": code,
        "status": status,
        "chosen": chosen,
        "recommendation": {
            "primary": rec["primary"],
            "secondary": rec["secondary"],
            "supporting": rec["supporting"],
            "topScored": [{"id": s["id"], "score": s["score"]} for s in engine_top],
        },
        "normalizedAttributes": normalized,
        "documentSaved": saved_path,
        "saveError": save_error,
        "iterations": iterations,
        "selectedIter": parsed.get("selectedIter"),
        "stopReason": stop_reason,
        "unresolvedCritical": unresolved,
        "invalidChosen": invalid_chosen,
        "message": msg + iter_info + notes,
    }
