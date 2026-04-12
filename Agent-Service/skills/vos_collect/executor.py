"""VOS 수집 스킬 — project-description.md 분석 → VOS 이슈 도출 (mode 분기 지원)."""

import re
import json

from core.database import issues
from core.llm import call_llm
from clients.platform_api import PlatformClient
from skills.helpers import create_issue, dispatch_by_mode


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

    # 2. 기존 VOS 이슈 조회 (LLM 컨텍스트)
    existing_vos = list(issues().find(
        {"key": {"$regex": f"^{re.escape(code)}-"}, "issueType": "VOS"},
        {"key": 1, "summary": 1, "description": 1, "client": 1, "_id": 0},
    ))
    existing_keys = {v["key"] for v in existing_vos}

    # 3. LLM에 기존 VOS 컨텍스트 + mode 결정 요청
    truncated = doc_content[:6000]
    prompt = f"""다음은 프로젝트 {code}의 과제 설명 문서입니다.

{truncated}

## 기존 VOS ({len(existing_vos)}건)
{json.dumps(existing_vos, ensure_ascii=False, indent=2)}

## 작업
위 문서를 분석하여 이해관계자(Stakeholder)의 요구사항(VOS: Voice of Stakeholder)을 추출하세요.
사용자 관점 어투("~가 필요해요 / ~해야 합니다 / ~하면 좋겠어요")로 작성합니다.

각 VOS 항목에 **mode**를 의미적 유사도 기반으로 결정하세요:
- `create`: 기존과 의미적으로 다른 새 VOS
- `reuse`: 기존 VOS와 의미가 동일 (existingKey 지정, 추가 작업 없음)
- `modify`: 기존 VOS와 거의 같지만 description/client 등 보강 필요 (existingKey + modifyReason 지정)
- `delete_mark`: 기존 VOS가 더 이상 유효하지 않아 폐기되어야 함 (existingKey + deleteReason 지정, 실제 삭제는 안 함)

## 출력 (반드시 JSON 배열로만 응답, 다른 텍스트 금지)
[
  {{
    "summary": "요구사항 제목 (간결하게)",
    "description": "요구사항 상세 (배경, 근거 포함)",
    "client": "이해관계자 역할명 (예: 사용자, 관리자, 운영자)",
    "mode": "create",
    "existingKey": "",
    "modifyReason": "",
    "deleteReason": ""
  }}
]

추가/수정/마킹할 항목이 없으면 빈 배열 []을 반환하세요."""

    raw = await call_llm(prompt)
    items = _extract_json_list(raw)
    if items is None:
        return {
            "projectCode": code,
            "message": "LLM 응답을 파싱할 수 없습니다.",
            "raw": raw[:500],
            "created": [], "reused": [], "modified": [], "markedForDeletion": [],
        }

    # 4. mode 분기 처리
    def _create_fn(item):
        client_val = item.get("client", "")
        return create_issue(
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

    buckets = dispatch_by_mode(items, existing_keys, _create_fn, _update_fields_fn, user)

    return {
        "projectCode": code,
        "existingCount": len(existing_vos),
        "created": buckets["created"],
        "reused": buckets["reused"],
        "modified": buckets["modified"],
        "markedForDeletion": buckets["markedForDeletion"],
        "message": _summarize(buckets),
    }


def _is_template_only(content: str) -> bool:
    stripped = re.sub(r'#[^\n]*', '', content or '')
    stripped = re.sub(r'>.*', '', stripped)
    stripped = re.sub(r'\(작성 필요\)', '', stripped)
    for phrase in ('기록하는 문서입니다', '기술하는 문서입니다', '반드시 작성해', '참조합니다'):
        stripped = stripped.replace(phrase, '')
    return len(stripped.strip()) < 30


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
