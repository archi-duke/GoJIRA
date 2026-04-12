"""Requirement 정제 스킬 — Feature/Use Case → FR/NFR/CNST + business/requirements.md (mode 분기 지원)."""

import os
import re
import json

from core.database import issues
from core.llm import call_llm
from clients.platform_api import PlatformClient
from skills.helpers import create_issue, link_issues, dispatch_by_mode


_PROMPTS_DIR = os.path.join(os.path.dirname(__file__), "..", "..", "prompts")

_NFR_FIELDS = (
    "qa", "source", "stimulus", "environment", "artifact",
    "response", "measure", "responseScenario",
)


def _load_template() -> str:
    path = os.path.join(_PROMPTS_DIR, "requirements.md")
    try:
        with open(path, "r", encoding="utf-8") as f:
            return f.read()
    except FileNotFoundError:
        return ""


def _is_template_only(content: str) -> bool:
    stripped = re.sub(r'#[^\n]*', '', content or '')
    stripped = re.sub(r'>.*', '', stripped)
    stripped = re.sub(r'\(작성 필요\)', '', stripped).strip()
    return len(stripped) < 30


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


async def _read_doc(client: PlatformClient, code: str, path: str, req_user: str) -> str:
    main = await client.get_file_content(code, path, ref="main")
    if main and not _is_template_only(main):
        return main
    branch = await client.get_file_content(code, path, ref=f"docs/{req_user}")
    return branch or main or ""


async def execute(params: dict, user: str) -> dict:
    code = params.get("projectCode", "")
    if not code:
        raise ValueError("projectCode 필수")
    req_user = params.get("user", user)

    client = PlatformClient()

    desc = await _read_doc(client, code, "business/project-description.md", req_user)
    use_cases = await _read_doc(client, code, "business/use-cases.md", req_user)

    if _is_template_only(desc):
        return {
            "projectCode": code,
            "message": "프로젝트 관련 정보가 불충분합니다. 과제 설명 문서 작성을 먼저 진행해야 합니다.",
            "suggestSkill": "projdesc_interview",
            "created": [], "reused": [], "modified": [], "markedForDeletion": [],
        }

    feat_list = list(issues().find(
        {"key": {"$regex": f"^{re.escape(code)}-"}, "issueType": "Feature"},
        {"key": 1, "summary": 1, "description": 1,
         "actor": 1, "precondition": 1, "basicflow": 1,
         "alternativeflow": 1, "exceptionflow": 1, "postcondition": 1, "_id": 0},
    ))
    if not feat_list:
        return {
            "projectCode": code,
            "message": "등록된 Feature가 없습니다. Use Case/Feature 분석을 먼저 진행해야 합니다.",
            "suggestSkill": "usecase_analyze",
            "created": [], "reused": [], "modified": [], "markedForDeletion": [],
        }

    vos_list = list(issues().find(
        {"key": {"$regex": f"^{re.escape(code)}-"}, "issueType": "VOS"},
        {"key": 1, "summary": 1, "_id": 0},
    ))
    existing_reqs = list(issues().find(
        {"key": {"$regex": f"^{re.escape(code)}-"}, "issueType": "Requirement"},
        {"key": 1, "summary": 1, "description": 1, "reqtype": 1,
         "qa": 1, "measure": 1, "_id": 0},
    ))
    existing_req_keys = {r["key"] for r in existing_reqs}
    valid_feat_keys = {f["key"] for f in feat_list}

    template = _load_template()
    prompt = f"""당신은 소프트웨어 아키텍트입니다. 프로젝트 {code}의 Requirement를 정제하세요.

## project-description.md
{desc[:4000]}

## use-cases.md
{use_cases[:4000]}

## Feature ({len(feat_list)}건)
{json.dumps(feat_list, ensure_ascii=False, indent=2)}

## VOS ({len(vos_list)}건)
{json.dumps(vos_list, ensure_ascii=False, indent=2)}

## 기존 Requirement ({len(existing_reqs)}건)
{json.dumps(existing_reqs, ensure_ascii=False, indent=2)}

## 작업
각 Feature와 Use Case 시나리오에서 FR/NFR/CNST를 도출하세요.

### FR (Functional Requirement)
- **User Story 형식 필수**: "{{역할}}은(는) {{목적}}을 위해 {{기능}}을 할 수 있다"
- description에 User Story 본문

### NFR (Quality Attribute / Non-Functional Requirement)
- **QAS 필수** — 필드: qa, source, stimulus, environment, artifact, response, measure, responseScenario
- Measure가 원본에 없으면 일반적 수준으로 합리적 결정 (예: 웹 응답 2초, API 500ms, 가용성 99.5% 등)
- 자동 결정한 경우 `measureAuto: true` 표시

### CNST (Constraint)
- 반드시 지켜야 하는 사양/규제/표준

## 각 Requirement 항목의 mode 결정 (의미적 유사도 기반)
- `create`: 기존과 다른 새 Requirement
- `reuse`: 기존과 의미 동일 (existingKey)
- `modify`: 기존 + 보강/수정 필요 (existingKey + modifyReason)
- `delete_mark`: 기존이 더 이상 유효하지 않음 (existingKey + deleteReason)

## 출력 (반드시 단일 JSON 객체로만 응답, 다른 텍스트 금지)
{{
  "requirements": [
    {{
      "reqType": "FR",
      "summary": "...",
      "description": "User Story 본문",
      "relatedFeatures": ["GOJIRA-12"],
      "mode": "create",
      "existingKey": "",
      "modifyReason": "",
      "deleteReason": ""
    }},
    {{
      "reqType": "NFR",
      "summary": "...",
      "description": "...",
      "qa": "Performance",
      "source": "...", "stimulus": "...", "environment": "...",
      "artifact": "...", "response": "...", "measure": "...",
      "responseScenario": "...",
      "measureAuto": true,
      "relatedFeatures": ["GOJIRA-13"],
      "mode": "create",
      "existingKey": "", "modifyReason": "", "deleteReason": ""
    }},
    {{
      "reqType": "CNST",
      "summary": "...",
      "description": "...",
      "relatedFeatures": [],
      "mode": "create",
      "existingKey": "", "modifyReason": "", "deleteReason": ""
    }}
  ],
  "document": "전체 requirements.md 본문 (Markdown). 자동 결정된 NFR Measure에는 비고에 [자동결정] 표시. 양식은 아래를 따를 것."
}}

## requirements.md 양식
{template}
"""

    raw = await call_llm(prompt)
    parsed = _extract_json_obj(raw)
    if not parsed:
        return {
            "projectCode": code,
            "message": "LLM 응답을 파싱할 수 없습니다.",
            "raw": raw[:500],
            "created": [], "reused": [], "modified": [], "markedForDeletion": [],
        }

    req_items = parsed.get("requirements") or []
    document = parsed.get("document") or ""

    # mode 분기 처리
    def _create_fn(item):
        req_type = (item.get("reqType") or "").upper()
        if req_type not in ("FR", "NFR", "CNST"):
            raise ValueError(f"잘못된 reqType: {req_type}")
        params_for_create = {
            "projectCode": code,
            "reqType": req_type,
            "summary": (item.get("summary") or "").strip(),
            "description": item.get("description", ""),
        }
        extras = {"reqtype": req_type}
        if req_type == "NFR":
            for f in _NFR_FIELDS:
                if item.get(f):
                    params_for_create[f] = item[f]
                    extras[f] = item[f]
            if item.get("measureAuto"):
                extras["measureAuto"] = True
        return create_issue("Requirement", params_for_create, user, extras=extras)

    def _update_fields_fn(item):
        updates = {}
        for f in ("summary", "description"):
            v = item.get(f)
            if v is not None and v != "":
                updates[f] = v
        req_type = (item.get("reqType") or "").upper()
        if req_type in ("FR", "NFR", "CNST"):
            updates["reqtype"] = req_type
        if req_type == "NFR":
            for f in _NFR_FIELDS:
                v = item.get(f)
                if v is not None and v != "":
                    updates[f] = v
            if item.get("measureAuto") is not None:
                updates["measureAuto"] = bool(item["measureAuto"])
        return updates

    buckets = dispatch_by_mode(req_items, existing_req_keys, _create_fn, _update_fields_fn, user)

    # Feature contains Requirement 연결 (created/modified만)
    linked = []
    auto_measure_items = []
    for bucket_name in ("created", "modified"):
        for entry in buckets[bucket_name]:
            req_key = entry["key"]
            item = entry["item"]
            for fk in (item.get("relatedFeatures") or []):
                if fk in valid_feat_keys:
                    try:
                        link_issues(fk, req_key, "contains")
                        linked.append({"from": fk, "to": req_key})
                    except Exception:
                        pass
            if item.get("reqType", "").upper() == "NFR" and item.get("measureAuto"):
                auto_measure_items.append({
                    "key": req_key,
                    "qa": item.get("qa", ""),
                    "measure": item.get("measure", ""),
                })

    # business/requirements.md 저장
    saved_path = None
    save_error = None
    if document.strip():
        try:
            await client.save_file(
                code,
                "business/requirements.md",
                document,
                f"Requirement 정제 (신규 {len(buckets['created'])} / 수정 {len(buckets['modified'])})",
                req_user,
            )
            saved_path = "business/requirements.md"
        except Exception as e:
            save_error = str(e)

    msg_parts = [
        f"Requirement — 신규 {len(buckets['created'])}, 재사용 {len(buckets['reused'])}, "
        f"수정 {len(buckets['modified'])}, 삭제마킹 {len(buckets['markedForDeletion'])}",
        f"Feature-Requirement 연결 {len(linked)}건",
    ]
    if saved_path:
        msg_parts.append("requirements.md 저장 완료")
    elif save_error:
        msg_parts.append(f"문서 저장 실패: {save_error}")
    if auto_measure_items:
        msg_parts.append(
            f"⚠ 자동 결정된 NFR Measure {len(auto_measure_items)}건 — 설계자 검토/조정 필요"
        )
    if buckets["markedForDeletion"]:
        msg_parts.append("⚠ 삭제 마킹된 항목은 검토 후 실제 삭제 또는 마킹 해제가 필요합니다")

    return {
        "projectCode": code,
        "created": buckets["created"],
        "reused": buckets["reused"],
        "modified": buckets["modified"],
        "markedForDeletion": buckets["markedForDeletion"],
        "linked": linked,
        "linkedCount": len(linked),
        "documentSaved": saved_path,
        "saveError": save_error,
        "autoMeasureItems": auto_measure_items,
        "needsReview": bool(auto_measure_items) or bool(buckets["markedForDeletion"]),
        "message": " / ".join(msg_parts),
    }
