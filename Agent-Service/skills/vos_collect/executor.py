"""VOS 수집 스킬 v2 — SKILL.md 통합 방식.

LLM 이 내부에서 generate → self-critique → revise → best-so-far 를
단일 호출로 수행. Python 은 IO 와 결과 파싱만 담당.
"""

import os
import re
import json
from typing import Any, Dict, List, Optional

from core.database import issues
from core.llm import call_llm
from clients.platform_api import PlatformClient
from skills.helpers import (
    create_issue, dispatch_by_mode,
    is_template_only as _is_template_only,
)
from skills.critique import load_step_policy


_SKILL_DIR = os.path.dirname(__file__)
_SKILL_MD_PATH = os.path.join(_SKILL_DIR, "SKILL.md")


def _load_skill_md() -> str:
    with open(_SKILL_MD_PATH, encoding="utf-8") as f:
        text = f.read()
    # YAML frontmatter 제거 (LLM 에는 본문만 전달)
    m = re.match(r"^---\n.*?\n---\n(.*)$", text, re.DOTALL)
    return m.group(1) if m else text


def _build_prompt(skill_body: str, code: str, desc: str, existing_vos: list,
                  criteria: str, policy_text: str) -> str:
    desc_trunc = (desc or "")[:6000]
    existing_json = json.dumps(existing_vos, ensure_ascii=False, indent=2)
    return (
        f"{skill_body}\n\n"
        f"---\n\n"
        f"## 주입 데이터\n\n"
        f"### projectCode\n{code}\n\n"
        f"### project_description (앞부분 6000자)\n{desc_trunc}\n\n"
        f"### existing_vos ({len(existing_vos)}건)\n```json\n{existing_json}\n```\n\n"
        f"### criteria\n{criteria}\n\n"
        f"### policy\n{policy_text}\n\n"
        f"이제 위 `실행 흐름` 섹션대로 수행하고, 지정된 JSON 형식 한 개만 출력하라."
    )


def _extract_json_object(raw: str) -> Optional[Dict[str, Any]]:
    """LLM 응답에서 JSON 오브젝트 추출. 바깥 설명문/마크다운 허용."""
    raw = raw.strip()
    try:
        return json.loads(raw)
    except json.JSONDecodeError:
        pass
    # 가장 바깥 {...} 블록 추출
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

    # 1. project-description.md 조회 (docs/{user} → main 순서)
    client = PlatformClient()
    doc_content = await client.get_file_content(code, "business/project-description.md", ref=f"docs/{req_user}")
    if not doc_content or _is_template_only(doc_content):
        doc_content = await client.get_file_content(code, "business/project-description.md", ref="main")
    if not doc_content or _is_template_only(doc_content):
        return {
            "projectCode": code,
            "status": "blocked",
            "blockedAt": "projdesc",
            "message": "프로젝트 관련 정보가 불충분합니다. 과제 설명 문서 작성을 먼저 진행해야 합니다.",
            "suggestSkill": "projdesc_interview",
            "created": [], "reused": [], "modified": [], "markedForDeletion": [],
        }

    await emit_progress("vos_collect", "VOS 수집 시작 — 과제 설명 분석 중 (v2 self-loop)")

    # 2. 기존 VOS 조회
    existing_vos = list(issues().find(
        {"key": {"$regex": f"^{re.escape(code)}-"}, "issueType": "VOS"},
        {"key": 1, "summary": 1, "description": 1, "client": 1, "_id": 0},
    ))
    existing_keys = {v["key"] for v in existing_vos}

    # 3. 워크플로우 검증 정책 로드
    policy = load_step_policy("workflow-requirement-analysis.md", "VOS 도출")

    # 4. SKILL.md + 주입 데이터로 단일 LLM 호출 (self-loop 내장)
    skill_body = _load_skill_md()
    prompt = _build_prompt(
        skill_body, code, doc_content, existing_vos,
        policy.get("criteria", ""), policy.get("policy_text", ""),
    )
    raw = await call_llm(prompt)

    result = _extract_json_object(raw)
    if not result or "finalItems" not in result:
        return {
            "projectCode": code,
            "status": "failed",
            "message": "LLM 응답 파싱 실패.",
            "raw": raw[:500],
            "created": [], "reused": [], "modified": [], "markedForDeletion": [],
        }

    final_items: List[dict] = result.get("finalItems") or []
    iterations = result.get("iterations") or []
    stop_reason = result.get("stopReason", "")
    unresolved = result.get("unresolvedCritical") or []

    # 5. mode 분기 처리
    async def _create_fn(item):
        client_val = item.get("client", "")
        return await create_issue(
            "VOS",
            {
                "projectCode": code,
                "summary": (item.get("summary") or "").strip(),
                "description": item.get("description", ""),
            },
            user,
            extras={"client": client_val} if client_val else None,
        )

    def _update_fields_fn(item):
        updates = {}
        for f in ("summary", "description", "client"):
            v = item.get(f)
            if v is not None and v != "":
                updates[f] = v
        return updates

    buckets = await dispatch_by_mode(final_items, existing_keys, _create_fn, _update_fields_fn, user)

    # 6. 요약
    parts = []
    if buckets["created"]:
        parts.append(f"신규 {len(buckets['created'])}건")
    if buckets["reused"]:
        parts.append(f"재사용 {len(buckets['reused'])}건")
    if buckets["modified"]:
        parts.append(f"수정 {len(buckets['modified'])}건")
    if buckets["markedForDeletion"]:
        parts.append(f"⚠ 삭제마킹 {len(buckets['markedForDeletion'])}건")
    msg = "VOS — " + (", ".join(parts) if parts else "변경 없음")

    iter_summary = ""
    if iterations:
        best_iter = result.get("selectedIter")
        iter_summary = f" (iter {len(iterations)}회, best=iter{best_iter}, stop={stop_reason})"

    await emit_progress("vos_collect", f"VOS 수집 완료 — {msg}{iter_summary}", **{
        "createdCount": len(buckets["created"]),
        "reusedCount": len(buckets["reused"]),
    })

    status = "completed"
    if unresolved:
        status = "partial"

    return {
        "projectCode": code,
        "status": status,
        "existingCount": len(existing_vos),
        "created": buckets["created"],
        "reused": buckets["reused"],
        "modified": buckets["modified"],
        "markedForDeletion": buckets["markedForDeletion"],
        "iterations": iterations,
        "selectedIter": result.get("selectedIter"),
        "stopReason": stop_reason,
        "unresolvedCritical": unresolved,
        "message": msg + iter_summary,
    }
