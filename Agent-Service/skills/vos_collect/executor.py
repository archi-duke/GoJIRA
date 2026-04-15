"""VOS 수집 스킬 — project-description.md 분석 → VOS 이슈 도출 (mode 분기 + critique 루프)."""

import os
import re
import json

from core.database import issues
from clients.platform_api import PlatformClient
from skills.helpers import (
    create_issue, dispatch_by_mode,
    is_template_only as _is_template_only,
    load_prompt,
)
from skills.critique import iterate_generate_critique, trail_summary, load_step_policy


_SKILL_DIR = os.path.dirname(__file__)


async def execute(params: dict, user: str) -> dict:
    code = params.get("projectCode", "")
    if not code:
        raise ValueError("projectCode 필수")

    req_user = params.get("user", user)

    # 1. project-description.md 조회
    client = PlatformClient()
    doc_content = await client.get_file_content(code, "business/project-description.md", ref="main")
    if not doc_content or _is_template_only(doc_content):
        user_content = await client.get_file_content(code, "business/project-description.md", ref=f"docs/{req_user}")
        if user_content and not _is_template_only(user_content):
            doc_content = user_content
    if not doc_content or _is_template_only(doc_content):
        return {
            "projectCode": code,
            "message": "프로젝트 관련 정보가 불충분합니다. 과제 설명 문서 작성을 먼저 진행해야 합니다.",
            "suggestSkill": "projdesc_interview",
            "created": [], "reused": [], "modified": [], "markedForDeletion": [],
        }

    from core.progress import emit_progress
    await emit_progress("vos_collect", "VOS 수집 시작 — 과제 설명 분석 중")

    # 2. 기존 VOS 이슈 조회 (LLM 컨텍스트)
    existing_vos = list(issues().find(
        {"key": {"$regex": f"^{re.escape(code)}-"}, "issueType": "VOS"},
        {"key": 1, "summary": 1, "description": 1, "client": 1, "_id": 0},
    ))
    existing_keys = {v["key"] for v in existing_vos}

    # 3. 생성 + 비판적 평가 루프
    truncated = doc_content[:6000]
    existing_json = json.dumps(existing_vos, ensure_ascii=False, indent=2)

    generate_prompt = load_prompt(_SKILL_DIR, "generate").format(
        code=code,
        truncated=truncated,
        existing_count=len(existing_vos),
        existing_json=existing_json,
    )

    critique_tpl = load_prompt(_SKILL_DIR, "critique")
    revise_tpl = load_prompt(_SKILL_DIR, "revise")

    # 워크플로우 문서에서 검증 기준/정책 로드 (단일 진실 출처)
    policy = load_step_policy("workflow-requirement-analysis.md", "VOS 도출")

    context_block = (
        f"## 원본 과제 설명 (요약)\n{truncated[:3000]}\n\n"
        f"## 기존 VOS\n{existing_json}"
    )

    def _critique_prompt(draft: str) -> str:
        return critique_tpl.format(
            context_block=context_block,
            draft=draft,
            criteria=policy["criteria"],
            policy=policy["policy_text"],
        )

    def _revise_prompt(draft: str, critique: dict) -> str:
        issues_text = "\n".join(f"- {i}" for i in critique.get("issues", []))
        return revise_tpl.format(
            desc_trunc=truncated[:3000],
            existing_json=existing_json,
            draft=draft,
            issues_text=issues_text,
        )

    raw, critique_trail = await iterate_generate_critique(
        generate_prompt, _critique_prompt, _revise_prompt,
        max_iter=policy["max_iter"],
        threshold=policy["threshold"],
        on_fail=policy["on_fail"],
    )

    items = _extract_json_list(raw)
    if items is None:
        return {
            "projectCode": code,
            "message": "LLM 응답을 파싱할 수 없습니다.",
            "raw": raw[:500],
            "created": [], "reused": [], "modified": [], "markedForDeletion": [],
        }

    # 4. mode 분기 처리
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

    buckets = await dispatch_by_mode(items, existing_keys, _create_fn, _update_fields_fn, user)

    tsum = trail_summary(critique_trail)
    msg = _summarize(buckets) + (f" — {tsum}" if tsum else "")
    await emit_progress("vos_collect", f"VOS 수집 완료 — {msg}", **{
        "createdCount": len(buckets["created"]),
        "reusedCount": len(buckets["reused"]),
    })
    return {
        "projectCode": code,
        "existingCount": len(existing_vos),
        "created": buckets["created"],
        "reused": buckets["reused"],
        "modified": buckets["modified"],
        "markedForDeletion": buckets["markedForDeletion"],
        "critiqueTrail": critique_trail,
        "message": msg,
    }


def _extract_json_list(raw: str):
    try:
        data = json.loads(raw)
        return data if isinstance(data, list) else None
    except json.JSONDecodeError:
        m = re.search(r'\[.*\]', raw, re.DOTALL)
        if m:
            try:
                data = json.loads(m.group())
                return data if isinstance(data, list) else None
            except json.JSONDecodeError:
                return None
    return None


def _summarize(buckets: dict) -> str:
    parts = []
    if buckets["created"]:
        parts.append(f"신규 {len(buckets['created'])}건")
    if buckets["reused"]:
        parts.append(f"재사용 {len(buckets['reused'])}건")
    if buckets["modified"]:
        parts.append(f"수정 {len(buckets['modified'])}건")
    if buckets["markedForDeletion"]:
        parts.append(f"⚠ 삭제마킹 {len(buckets['markedForDeletion'])}건")
    return "VOS — " + (", ".join(parts) if parts else "변경 없음")
