"""Architecture Driver 선정 스킬 — Requirement → Driver + 정규화 속성 + architecture/drivers.md."""

import os
import re
import json

from core.database import issues, projects
from clients.platform_api import PlatformClient
from skills.helpers import (
    update_issue_with_history,
    is_template_only as _is_template_only,
    load_prompt,
)
from skills.critique import iterate_generate_critique, trail_summary, load_step_policy


_SKILL_DIR = os.path.dirname(__file__)
_ASR_LABEL = "ASR"


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


async def _add_asr_label(issue_key: str, user: str):
    """이슈에 ASR 라벨 추가 (중복 방지). issuestore 경유."""
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

    # priorityWeight — 우선순위: 요청 파라미터 > 프로젝트 저장값 > 기본값(0.5/0.5)
    # 요청에 주어지면 프로젝트에 영속화해 다음 호출부터 재사용.
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

    client = PlatformClient()

    # project-description 컨텍스트
    desc = await client.get_file_content(code, "business/project-description.md", ref="main")
    if not desc or _is_template_only(desc):
        alt = await client.get_file_content(code, "business/project-description.md", ref=f"docs/{req_user}")
        if alt and not _is_template_only(alt):
            desc = alt
    desc = desc or ""

    # 기존 Requirement 조회
    req_list = list(issues().find(
        {"key": {"$regex": f"^{re.escape(code)}-"}, "issueType": "Requirement"},
        {"key": 1, "subkey": 1, "summary": 1, "description": 1, "reqtype": 1,
         "qa": 1, "measure": 1, "_id": 0},
    ))

    from core.progress import emit_progress
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

    await emit_progress("driver_select", "Architecture Driver 선정 시작")

    req_json = json.dumps(req_list, ensure_ascii=False, indent=2)
    feat_summary = json.dumps(feat_list, ensure_ascii=False, indent=2)
    desc_trunc = desc[:4000]

    generate_prompt = load_prompt(_SKILL_DIR, "generate").format(
        code=code,
        desc_trunc=desc_trunc,
        req_count=len(req_list),
        req_json=req_json,
        feat_count=len(feat_list),
        feat_summary=feat_summary,
        w_qad=w_qad,
        w_qai=w_qai,
    )

    critique_tpl = load_prompt(_SKILL_DIR, "critique")
    revise_tpl = load_prompt(_SKILL_DIR, "revise")

    policy = load_step_policy("workflow-architecture-design.md", "Architecture Driver 선정")

    context_block = f"## Requirement 목록\n{req_json}"

    def _critique_prompt(draft: str) -> str:
        return critique_tpl.format(
            context_block=context_block,
            draft=draft[:8000],
            criteria=policy["criteria"],
            policy=policy["policy_text"],
        )

    def _revise_prompt(draft: str, critique: dict) -> str:
        issues_text = "\n".join(f"- {i}" for i in critique.get("issues", []))
        return revise_tpl.format(req_json=req_json, draft=draft[:10000], issues_text=issues_text)

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
            "drivers": [], "normalizedAttributes": {},
        }

    drivers = parsed.get("drivers") or []
    normalized = parsed.get("normalizedAttributes") or {}
    document = parsed.get("document") or ""

    # Requirement 업데이트 + ASR 라벨 부여
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
            # subkey 보강 (LLM 응답에 없을 수 있음)
            d.setdefault("subkey", req_by_key[key].get("subkey", ""))
            updated.append({"key": key, "qad": qad, "qai": qai, "priority": updates["priority"]})
        except Exception:
            continue

    # 모든 NFR 에 ASR 라벨 자동 부여 (driver 로 선정되지 않았더라도)
    for r in req_list:
        if (r.get("reqtype") or "").upper() == "NFR":
            try:
                await _add_asr_label(r["key"], user)
            except Exception:
                pass

    # normalizedAttributes 를 프로젝트에 영속화 (style_define 재사용)
    if normalized:
        projects().update_one(
            {"jiraCodes.main.jiraCode": code},
            {"$set": {"archSettings.normalizedAttributes": normalized}},
        )

    # architecture/drivers.md 저장
    saved_path = None
    save_error = None
    if document.strip():
        try:
            await client.save_file(
                code,
                "architecture/drivers.md",
                document,
                f"Architecture Driver 선정 ({len(updated)}건)",
                req_user,
            )
            saved_path = "architecture/drivers.md"
        except Exception as e:
            save_error = str(e)

    tsum = trail_summary(critique_trail)
    msg = f"Architecture Driver {len(updated)}건 선정, 정규화 속성 {len(normalized)}개"
    if saved_path:
        msg += " / drivers.md 저장 완료"
    elif save_error:
        msg += f" / 문서 저장 실패: {save_error}"
    if tsum:
        msg += f" / {tsum}"

    await emit_progress("driver_select", f"Architecture Driver 선정 완료 — {msg}")

    return {
        "projectCode": code,
        "status": "completed",
        "drivers": drivers,
        "normalizedAttributes": normalized,
        "updated": updated,
        "documentSaved": saved_path,
        "saveError": save_error,
        "priorityWeight": {"qad": w_qad, "qai": w_qai},
        "critiqueTrail": critique_trail,
        "message": msg,
    }
