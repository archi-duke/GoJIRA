"""SATAM 분석 스킬 v2 — SKILL.md 통합 self-loop.

LLM 이 내부에서 generate → self-critique(QA 매핑/카테고리 검증) → revise → best-so-far 를
단일 호출로 수행. Python 은 IO(ADSN/QA 조회), 결과 파싱, 이슈 필드 저장만 담당.

문서 파일은 생성하지 않는다. 프론트엔드 SATAM 컴포넌트가 각 ADSN 이슈의
risk/sensitivity/mitigation YAML 필드를 직접 파싱해 렌더링한다.
"""

import os
import re
import json
from typing import Any, Dict, List, Optional

from core.database import issues, projects
from core.llm import call_llm


_SKILL_DIR = os.path.dirname(__file__)
_SKILL_MD_PATH = os.path.join(_SKILL_DIR, "SKILL.md")


def _load_skill_md() -> str:
    with open(_SKILL_MD_PATH, encoding="utf-8") as f:
        text = f.read()
    m = re.match(r"^---\n.*?\n---\n(.*)$", text, re.DOTALL)
    return m.group(1) if m else text


def _build_prompt(skill_body: str, code: str,
                  adsn_inputs: List[Dict], qa_refs: List[Dict]) -> str:
    adsn_json = json.dumps(adsn_inputs, ensure_ascii=False, indent=2)
    qa_json = json.dumps(qa_refs, ensure_ascii=False, indent=2)
    return (
        f"{skill_body}\n\n"
        f"---\n\n"
        f"## 주입 데이터\n\n"
        f"### projectCode\n{code}\n\n"
        f"### adsn_inputs ({len(adsn_inputs)}건)\n```json\n{adsn_json}\n```\n\n"
        f"### qa_refs ({len(qa_refs)}건)\n```json\n{qa_json}\n```\n\n"
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


def _serialize_items(items: List[Dict], counter: bool) -> str:
    """항목 리스트를 프론트엔드 parseRST 포맷의 YAML 로 직렬화."""
    if not items:
        return ""
    lines = []
    for it in items:
        iid = (it.get("id") or "").strip()
        if not iid:
            continue
        qa = it.get("qa") or []
        cr = it.get("counterRisk") or it.get("counter_risk") or []
        desc = (it.get("description") or "").strip().replace("\n", " ")
        lines.append(f"- ID : {iid}")
        if qa:
            lines.append(f"  QA : [{', '.join(qa)}]")
        if counter and cr:
            quoted = ", ".join('"' + c + '"' for c in cr)
            lines.append(f"  Counter Risk : [{quoted}]")
        if desc:
            lines.append(f"  Description : {desc}")
    return "\n".join(lines)


async def execute(params: dict, user: str) -> dict:
    code = params.get("projectCode", "")
    if not code:
        raise ValueError("projectCode 필수")

    from core.progress import emit_progress

    prefix = {"key": {"$regex": f"^{re.escape(code)}-"}}
    qa_issues = list(issues().find(
        {**prefix, "issueType": "Requirement", "reqtype": "NFR"},
        {"key": 1, "subkey": 1, "summary": 1, "_id": 0},
    ))
    adsn_issues = list(issues().find({**prefix, "issueType": "Architectural Design"}))

    if not adsn_issues:
        return {
            "projectCode": code,
            "status": "blocked",
            "blockedAt": "design_decide",
            "suggestSkill": "design_decide",
            "message": "ADSN(Architectural Design) 이슈가 없습니다. design_decide 를 먼저 실행하세요.",
            "adsnCount": 0,
        }

    # 1) LLM 입력 데이터 준비
    adsn_inputs = []
    for ad in adsn_issues:
        linked_qas = []
        for ln in ad.get("issueLinks") or []:
            t = (ln.get("type") or {}).get("name", "")
            if "Contains" in t or "contain" in t.lower():
                k = ((ln.get("inwardIssue") or {}).get("key")
                     or (ln.get("outwardIssue") or {}).get("key"))
                if k:
                    linked_qas.append(k)
        adsn_inputs.append({
            "key": ad.get("key", ""),
            "subkey": ad.get("subkey", ""),
            "summary": ad.get("summary", ""),
            "designPoint": ad.get("designPoint", ""),
            "decision": ad.get("decision", ""),
            "riskText": (ad.get("risk") or "").strip(),
            "sensitivityText": (ad.get("sensitivity") or "").strip(),
            "tradeoffText": (ad.get("tradeoffs") or ad.get("tradeoff") or ad.get("mitigation") or "").strip(),
            "linkedQAs": linked_qas,
        })

    await emit_progress("satam_analyze", f"SATAM 분석 시작 — ADSN {len(adsn_inputs)}건 (v2 self-loop)")

    # 2) SKILL.md + 주입 데이터로 단일 LLM 호출 (self-loop 내장)
    skill_body = _load_skill_md()
    prompt = _build_prompt(skill_body, code, adsn_inputs, qa_issues)
    raw = await call_llm(prompt)

    parsed = _extract_json_obj(raw)
    if not parsed or "finalMapping" not in parsed:
        return {
            "projectCode": code,
            "status": "failed",
            "message": "LLM 응답 파싱 실패.",
            "raw": raw[:500],
            "adsnCount": len(adsn_inputs),
            "updatedCount": 0,
        }

    mapping: Dict[str, Dict] = parsed.get("finalMapping") or {}
    iterations = parsed.get("iterations") or []
    stop_reason = parsed.get("stopReason", "")
    unresolved = parsed.get("unresolvedCritical") or []

    # 3) 각 ADSN 이슈의 risk/sensitivity/mitigation 필드 업데이트
    updated_keys: List[str] = []
    failed: List[Dict] = []
    risk_total = sens_total = miti_total = 0

    from core import issuestore

    for ad in adsn_inputs:
        key = ad["key"]
        entry = mapping.get(key) or {}
        risk_items = entry.get("risks") or []
        sens_items = entry.get("sensitivities") or []
        miti_items = entry.get("tradeoffs") or entry.get("mitigations") or []

        risk_yaml = _serialize_items(risk_items, counter=False)
        sens_yaml = _serialize_items(sens_items, counter=False)
        miti_yaml = _serialize_items(miti_items, counter=True)

        try:
            doc = issuestore.get_issue(key)
            if not doc:
                raise ValueError(f"이슈 없음: {key}")
            doc.risk = risk_yaml
            doc.sensitivity = sens_yaml
            doc.mitigation = miti_yaml
            await issuestore.save_issue(doc, user, commit_message=f"[satam] {key} R/S/M 업데이트 (v2)")
            updated_keys.append(key)
            risk_total += len(risk_items)
            sens_total += len(sens_items)
            miti_total += len(miti_items)
        except Exception as e:
            failed.append({"key": key, "error": str(e)})

    # 4) 완료 플래그
    projects().update_one(
        {"jiraCodes.main.jiraCode": code},
        {"$set": {"archSettings.satamDone": True}},
    )

    iter_info = ""
    if iterations:
        iter_info = f" (iter {len(iterations)}회, best=iter{parsed.get('selectedIter')}, stop={stop_reason})"

    base_msg = (
        f"SATAM 분석 완료 — ADSN {len(updated_keys)}건의 risk/sensitivity/mitigation 갱신. "
        f"Risks {risk_total} / Sensitivities {sens_total} / Trade-offs {miti_total}."
    )

    await emit_progress("satam_analyze", base_msg + iter_info)

    status = "completed"
    if failed or unresolved:
        status = "partial"

    return {
        "projectCode": code,
        "status": status,
        "satamDone": True,
        "adsnCount": len(adsn_inputs),
        "updatedCount": len(updated_keys),
        "riskCount": risk_total,
        "sensitivityCount": sens_total,
        "mitigationCount": miti_total,
        "failed": failed,
        "iterations": iterations,
        "selectedIter": parsed.get("selectedIter"),
        "stopReason": stop_reason,
        "unresolvedCritical": unresolved,
        "nextAction": {"skill": "ui_navigate", "params": {"projectCode": code, "page": "satam"}},
        "message": (
            base_msg + iter_info +
            " SATAM 화면으로 이동하려면 ui_navigate(page=satam) 를 호출하세요."
        ),
    }
