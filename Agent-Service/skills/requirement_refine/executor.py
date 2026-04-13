"""Requirement 정제 스킬 — Feature/Use Case → FR/NFR/CNST + business/requirements.md (mode 분기 지원)."""

import os
import re
import json

from core.database import issues
from clients.platform_api import PlatformClient
from skills.helpers import create_issue, link_issues, dispatch_by_mode
from skills.critique import iterate_generate_critique, trail_summary


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
    desc_trunc = desc[:4000]
    uc_trunc = use_cases[:4000]
    feat_json = json.dumps(feat_list, ensure_ascii=False, indent=2)
    vos_json = json.dumps(vos_list, ensure_ascii=False, indent=2)
    existing_req_json = json.dumps(existing_reqs, ensure_ascii=False, indent=2)

    generate_prompt = f"""당신은 소프트웨어 아키텍트입니다. 프로젝트 {code}의 Requirement를 정제하세요.

## project-description.md
{desc_trunc}

## use-cases.md
{uc_trunc}

## Feature ({len(feat_list)}건)
{feat_json}

## VOS ({len(vos_list)}건)
{vos_json}

## 기존 Requirement ({len(existing_reqs)}건)
{existing_req_json}

## 작업
각 Feature와 Use Case 시나리오에서 FR/NFR/CNST를 도출하세요.

### FR
- **User Story 형식 필수**: "{{역할}}은(는) {{목적}}을 위해 {{기능}}을 할 수 있다"

### NFR
- **QAS 필수** — qa, source, stimulus, environment, artifact, response, measure, responseScenario
- Measure가 원본에 없으면 합리적 수준으로 결정 후 `measureAuto: true`

### CNST
- 반드시 지켜야 하는 사양/규제/표준

## 각 항목의 mode 결정
- `create` / `reuse` / `modify` / `delete_mark` (existingKey/사유 포함)

## 출력 (반드시 단일 JSON 객체)
{{
  "requirements": [
    {{"reqType": "FR", "summary": "...", "description": "User Story 본문",
      "relatedFeatures": ["..."], "mode": "create", "existingKey": "", "modifyReason": "", "deleteReason": ""}},
    {{"reqType": "NFR", "summary": "...", "description": "...",
      "qa": "...", "source": "...", "stimulus": "...", "environment": "...",
      "artifact": "...", "response": "...", "measure": "...", "responseScenario": "...",
      "measureAuto": true, "relatedFeatures": ["..."], "mode": "create",
      "existingKey": "", "modifyReason": "", "deleteReason": ""}},
    {{"reqType": "CNST", "summary": "...", "description": "...", "relatedFeatures": [],
      "mode": "create", "existingKey": "", "modifyReason": "", "deleteReason": ""}}
  ],
  "document": "전체 requirements.md 본문 (Markdown). 자동 결정 NFR은 비고에 [자동결정] 표시. 양식 준수."
}}

## requirements.md 양식
{template}
"""

    def _critique_prompt(draft: str) -> str:
        return f"""당신은 엄격한 비판적 평가자입니다. Requirement 정제 결과를 평가하세요.

## Feature 목록
{feat_json}

## 생성 결과 (requirements + document)
{draft[:8000]}

## 평가 기준
1. **FR은 User Story 형식**: "{{역할}}은(는) {{목적}}을 위해 {{기능}}을 할 수 있다" — 격식 준수 여부
2. **NFR은 QAS 모든 필드 채움**: qa / source / stimulus / environment / artifact / response / measure / responseScenario
3. **NFR Measure는 측정 가능한 수치**: 모호한 표현("빠르게", "안전하게") 금지, 수치+단위 필수
4. **CNST는 진짜 제약**: 단순 선호/권장사항이 아닌 반드시 지켜야 할 규제/표준
5. **각 Feature에 FR 최소 1개 연결** (relatedFeatures로 매핑됨)
6. **relatedFeatures가 유효한 Feature 키** 사용
7. **중복 없음**: 의미적으로 같은 Requirement가 여러 개 생성되지 않음
8. **document에 FR/NFR/CNST/QAW 섹션이 모두 있고 양식 준수**
9. **자동 결정된 Measure는 `[자동결정]` 표시**가 document에 반영됨
10. **JSON 형식 유효, mode/reqType 일관**

## 출력 (반드시 JSON 객체)
{{
  "pass": true 또는 false,
  "issues": ["문제점 1", "문제점 2"],
  "reasoning": "판단 근거"
}}"""

    def _revise_prompt(draft: str, critique: dict) -> str:
        issues_text = "\n".join(f"- {i}" for i in critique.get("issues", []))
        return f"""이전에 생성한 Requirement 정제 결과에 아래 문제가 있습니다. 수정하세요.

## Feature 목록
{feat_json}

## 이전 생성 결과
{draft[:10000]}

## 지적된 문제
{issues_text}

## 요청
지적된 문제만 고치고, 올바른 부분은 유지하세요.
출력 형식은 이전과 동일한 JSON 객체 ({{requirements, document}})입니다.
document는 전체 Markdown 본문을 다시 완전하게 포함해야 합니다."""

    raw, critique_trail = await iterate_generate_critique(
        generate_prompt, _critique_prompt, _revise_prompt,
    )
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

    tsum = trail_summary(critique_trail)
    if tsum:
        msg_parts.append(tsum)
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
        "critiqueTrail": critique_trail,
        "message": " / ".join(msg_parts),
    }
