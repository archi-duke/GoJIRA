"""VOS 수집 스킬 — project-description.md 분석 → VOS 이슈 도출 (mode 분기 지원)."""

import re
import json

from core.database import issues
from clients.platform_api import PlatformClient
from skills.helpers import create_issue, dispatch_by_mode
from skills.critique import iterate_generate_critique, trail_summary


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

    # 3. 생성 + 비판적 평가 루프
    truncated = doc_content[:6000]
    existing_json = json.dumps(existing_vos, ensure_ascii=False, indent=2)

    generate_prompt = f"""# 역할
당신은 노련한 시니어 기획자입니다. 주어진 정보 외에도 다양한 유사 사례와 경험을 바탕으로,
문서에 드러나지 않은 숨겨진 관련자들을 찾아내고 다양한 요구사항을 검토합니다.
- 직접 언급된 이해관계자뿐 아니라, 유사 과제 경험상 관련될 수 있는 간접 이해관계자를 식별
  (예: 운영자, 보안 담당, 감사/규제 기관, 파트너사, 최종 사용자 등)
- 제안자가 놓친 관점에서의 요구사항도 능동적으로 발굴
- 다만 VOS는 원본 과제와 관련성이 명확한 것만 — 무관한 가공 금지

# 작업
다음은 프로젝트 {code}의 과제 설명 문서입니다.

{truncated}

## 기존 VOS ({len(existing_vos)}건)
{existing_json}

## 지시
위 문서를 분석하여 이해관계자(Stakeholder)의 요구사항(VOS: Voice of Stakeholder)을 추출하세요.
사용자 관점 어투("~가 필요해요 / ~해야 합니다 / ~하면 좋겠어요")로 작성합니다.

각 VOS 항목에 **mode**를 의미적 유사도 기반으로 결정하세요:
- `create`: 기존과 의미적으로 다른 새 VOS
- `reuse`: 기존 VOS와 의미가 동일 (existingKey 지정)
- `modify`: 기존과 거의 같지만 보강 필요 (existingKey + modifyReason)
- `delete_mark`: 기존이 더 이상 유효하지 않음 (existingKey + deleteReason)

## 출력 (반드시 JSON 배열로만 응답)
[
  {{
    "summary": "요구사항 제목",
    "description": "요구사항 상세 (배경, 근거)",
    "client": "이해관계자 역할명",
    "mode": "create",
    "existingKey": "",
    "modifyReason": "",
    "deleteReason": ""
  }}
]

추가/수정/마킹할 항목이 없으면 빈 배열 []을 반환하세요."""

    def _critique_prompt(draft: str) -> str:
        return f"""당신은 엄격한 비판적 평가자입니다. 아래 VOS 추출 결과를 평가하세요.

## 원본 과제 설명 (요약)
{truncated[:3000]}

## 기존 VOS
{existing_json}

## 생성 결과
{draft}

## 평가 기준
1. 사용자 관점 어투 사용 ("~가 필요해요", "~해야 합니다", "~하면 좋겠어요")
2. 각 VOS의 client(이해관계자 역할)가 명확히 지정됨
3. 과제 설명의 주요 Stakeholder들이 모두 커버됨 (누락된 관점 없는지)
4. 기존 VOS와 중복 항목이 create 모드로 잘못 생성되지 않음
5. summary가 간결하고 description이 구체적
6. JSON 형식이 유효하고 mode/existingKey가 일관됨

## 출력 (반드시 JSON 객체로만 응답, 다른 텍스트 금지)
{{
  "pass": true 또는 false,
  "issues": ["문제점 1", "문제점 2"],
  "reasoning": "간단한 판단 근거"
}}"""

    def _revise_prompt(draft: str, critique: dict) -> str:
        issues_text = "\n".join(f"- {i}" for i in critique.get("issues", []))
        return f"""이전에 생성한 VOS 추출 결과에 아래 문제가 있습니다. 문제를 수정한 새 결과를 출력하세요.

## 원본 과제 설명 (요약)
{truncated[:3000]}

## 기존 VOS
{existing_json}

## 이전 생성 결과
{draft}

## 지적된 문제
{issues_text}

## 요청
지적된 문제를 반영하여 VOS 추출 결과를 수정하세요.
올바른 부분은 유지하고, 문제 있는 부분만 고쳐야 합니다.
출력 형식은 이전과 동일한 JSON 배열입니다."""

    raw, critique_trail = await iterate_generate_critique(
        generate_prompt, _critique_prompt, _revise_prompt,
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

    tsum = trail_summary(critique_trail)
    msg = _summarize(buckets) + (f" — {tsum}" if tsum else "")
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
