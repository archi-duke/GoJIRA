"""Architecture Driver 선정 스킬 v2 — SKILL.md 통합 self-loop.

LLM 이 단일 호출로 generate → self-critique → revise → best-so-far 수행.
Python 은 IO + 검증 후 Requirement 업데이트 / ASR 라벨 / drivers.md 저장만 담당.
"""

import os
import re
import json
from typing import Any, Dict, Optional

from core.database import issues, projects
from core.llm import call_llm
from clients.platform_api import PlatformClient
from skills.helpers import (
    update_issue_with_history,
    is_template_only as _is_template_only,
)
from skills.critique import load_step_policy


_SKILL_DIR = os.path.dirname(__file__)
_SKILL_MD_PATH = os.path.join(_SKILL_DIR, "SKILL.md")
_ASR_LABEL = "ASR"


def _load_skill_md() -> str:
    with open(_SKILL_MD_PATH, encoding="utf-8") as f:
        text = f.read()
    m = re.match(r"^---\n.*?\n---\n(.*)$", text, re.DOTALL)
    return m.group(1) if m else text


def _build_prompt(skill_body: str, code: str, desc: str, req_list: list,
                  feat_list: list, w_qad: float, w_qai: float,
                  criteria: str, policy_text: str) -> str:
    desc_trunc = (desc or "")[:4000]
    req_json = json.dumps(req_list, ensure_ascii=False, indent=2)
    feat_json = json.dumps(feat_list, ensure_ascii=False, indent=2)
    return (
        f"{skill_body}\n\n"
        f"---\n\n"
        f"## 주입 데이터\n\n"
        f"### projectCode\n{code}\n\n"
        f"### project_description (앞부분 4000자)\n{desc_trunc}\n\n"
        f"### requirements ({len(req_list)}건)\n```json\n{req_json}\n```\n\n"
        f"### features (참고, {len(feat_list)}건)\n```json\n{feat_json}\n```\n\n"
        f"### priorityWeight\nqad={w_qad}, qai={w_qai}\n\n"
        f"### criteria\n{criteria}\n\n"
        f"### policy\n{policy_text}\n\n"
        f"이제 위 `실행 흐름` 섹션대로 수행하고, 지정된 JSON 하나만 출력하라."
    )


def _extract_json_obj(raw: str) -> Optional[Dict[str, Any]]:
    raw = raw.strip()
    try:
        data = json.loads(raw)
        return data if isinstance(data, dict) else None
    except json.JSONDecodeError:
        pass
    m = re.search(r"\{[\s\S]*\}", raw)
    if not m:
        return None
    try:
        data = json.loads(m.group())
        return data if isinstance(data, dict) else None
    except json.JSONDecodeError:
        return None


async def _add_asr_label(issue_key: str, user: str) -> None:
    """이슈에 ASR 라벨 추가 (중복 방지)."""
    from core import issuestore
    doc = issuestore.get_issue(issue_key)
    if not doc:
        return
    if _ASR_LABEL in doc.labels:
        return
    doc.labels = list(doc.labels) + [_ASR_LABEL]
    await issuestore.save_issue(doc, user, commit_message=f"[label] ASR {issue_key}")


async def execute(params: dict, user: str) -> dict:
    code = params.get("projectCode", "")
    if not code:
        raise ValueError("projectCode 필수")
    req_user = params.get("user", user)

    from core.progress import emit_progress

    # 1. priorityWeight 해석 + 영속화
    proj = projects().find_one(
        {"jiraCodes.main.jiraCode": code},
        {"archSettings": 1, "_id": 0},
    ) or {}
    stored = ((proj.get("archSettings") or {}).get("priorityWeight") or {})
    requested = params.get("priorityWeight") or {}
    if requested:
        w_qad = float(requested.get("qad", stored.get("qad", 0.5)))
        w_qai = float(requested.get("qai", stored.get("qai", 0.5)))
        projects().update_one(
            {"jiraCodes.main.jiraCode": code},
            {"$set": {"archSettings.priorityWeight": {"qad": w_qad, "qai": w_qai}}},
        )
    else:
        w_qad = float(stored.get("qad", 0.5))
        w_qai = float(stored.get("qai", 0.5))

    # 2. project-description.md (main → docs/{user} fallback — 기존 동작 보존)
    client = PlatformClient()
    desc = await client.get_file_content(code, "business/project-description.md", ref="main")
    if not desc or _is_template_only(desc):
        alt = await client.get_file_content(code, "business/project-description.md", ref=f"docs/{req_user}")
        if alt and not _is_template_only(alt):
            desc = alt
    desc = desc or ""

    # 3. Requirement 조회
    req_list = list(issues().find(
        {"key": {"$regex": f"^{re.escape(code)}-"}, "issueType": "Requirement"},
        {"key": 1, "subkey": 1, "summary": 1, "description": 1, "reqtype": 1,
         "qa": 1, "measure": 1, "_id": 0},
    ))
    if not req_list:
        return {
            "projectCode": code,
            "status": "blocked",
            "blockedAt": "requirement_refine",
            "suggestSkill": "requirement_refine",
            "message": "Requirement 가 없습니다. requirement_refine 을 먼저 실행하세요.",
            "drivers": [], "normalizedAttributes": {},
        }

    feat_list = list(issues().find(
        {"key": {"$regex": f"^{re.escape(code)}-"}, "issueType": "Feature"},
        {"key": 1, "summary": 1, "_id": 0},
    ))

    await emit_progress("driver_select", "Architecture Driver 선정 시작 (v2 self-loop)")

    # 4. SKILL.md + 주입 데이터로 단일 LLM 호출
    policy = load_step_policy("workflow-architecture-design.md", "Architecture Driver 선정")
    skill_body = _load_skill_md()
    prompt = _build_prompt(
        skill_body, code, desc, req_list, feat_list, w_qad, w_qai,
        policy.get("criteria", ""), policy.get("policy_text", ""),
    )
    raw = await call_llm(prompt)

    parsed = _extract_json_obj(raw)
    if not parsed or "finalItems" not in parsed:
        return {
            "projectCode": code,
            "status": "failed",
            "message": "LLM 응답 파싱 실패.",
            "raw": raw[:500],
            "drivers": [], "normalizedAttributes": {},
        }

    drivers = parsed.get("finalItems") or []
    normalized = parsed.get("finalNormalizedAttributes") or {}
    document = parsed.get("finalDocument") or ""
    iterations = parsed.get("iterations") or []
    stop_reason = parsed.get("stopReason", "")
    unresolved = parsed.get("unresolvedCritical") or []

    # 5. Requirement 업데이트 + ASR 라벨
    valid_req_keys = {r["key"] for r in req_list}
    req_by_key = {r["key"]: r for r in req_list}
    updated = []
    for d in drivers:
        key = d.get("key", "")
        if key not in valid_req_keys:
            continue
        qad = int(d.get("qad", 0) or 0)
        qai = int(d.get("qai", 0) or 0)
        priority = d.get("priority")
        if priority is None:
            priority = qad * w_qad + qai * w_qai
        updates = {
            "qad": qad,
            "qai": qai,
            "priority": round(float(priority), 2),
            "driverSelected": True,
        }
        if d.get("risk"):
            updates["risk"] = d["risk"]
        if d.get("sensitivity"):
            updates["sensitivity"] = d["sensitivity"]
        try:
            await update_issue_with_history(key, updates, "Architecture Driver 선정", user)
            await _add_asr_label(key, user)
            d.setdefault("subkey", req_by_key[key].get("subkey", ""))
            updated.append({"key": key, "qad": qad, "qai": qai, "priority": updates["priority"]})
        except Exception:
            continue

    # 6. 모든 NFR 에 ASR 라벨 자동 부여
    for r in req_list:
        if (r.get("reqtype") or "").upper() == "NFR":
            try:
                await _add_asr_label(r["key"], user)
            except Exception:
                pass

    # 7. normalizedAttributes 영속화
    if normalized:
        projects().update_one(
            {"jiraCodes.main.jiraCode": code},
            {"$set": {"archSettings.normalizedAttributes": normalized}},
        )

    # 8. architecture/drivers.md 저장
    saved_path = None
    save_error = None
    if document.strip():
        try:
            await client.save_file(
                code,
                "architecture/drivers.md",
                document,
                f"Architecture Driver 선정 ({len(updated)}건, v2)",
                req_user,
            )
            saved_path = "architecture/drivers.md"
        except Exception as e:
            save_error = str(e)

    # 9. 요약
    parts = [f"Driver {len(updated)}건 선정", f"정규화 속성 {len(normalized)}개"]
    if saved_path:
        parts.append("drivers.md 저장 완료")
    elif save_error:
        parts.append(f"문서 저장 실패: {save_error}")
    base_msg = " / ".join(parts)

    iter_info = ""
    if iterations:
        iter_info = f" (iter {len(iterations)}회, best=iter{parsed.get('selectedIter')}, stop={stop_reason})"

    await emit_progress("driver_select", f"Architecture Driver 선정 완료 — {base_msg}")

    status = "completed"
    if unresolved:
        status = "partial"

    return {
        "projectCode": code,
        "status": status,
        "drivers": drivers,
        "normalizedAttributes": normalized,
        "updated": updated,
        "documentSaved": saved_path,
        "saveError": save_error,
        "priorityWeight": {"qad": w_qad, "qai": w_qai},
        "iterations": iterations,
        "selectedIter": parsed.get("selectedIter"),
        "stopReason": stop_reason,
        "unresolvedCritical": unresolved,
        "message": base_msg + iter_info,
    }
