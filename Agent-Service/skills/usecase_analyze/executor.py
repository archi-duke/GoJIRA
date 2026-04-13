"""Use Case 분석 스킬 — VOS → Use Case → Feature 이슈 + business/use-cases.md (mode 분기 지원)."""

import os
import re
import json
from typing import Optional

from core.database import issues
from clients.platform_api import PlatformClient
from skills.helpers import create_issue, link_issues, dispatch_by_mode
from skills.critique import iterate_generate_critique, trail_summary


_PROMPTS_DIR = os.path.join(os.path.dirname(__file__), "..", "..", "prompts")
_USECASE_FIELDS = (
    "actor", "precondition", "basicflow",
    "alternativeflow", "exceptionflow", "postcondition",
)


def _load_template() -> str:
    path = os.path.join(_PROMPTS_DIR, "use-cases.md")
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


async def execute(params: dict, user: str) -> dict:
    code = params.get("projectCode", "")
    if not code:
        raise ValueError("projectCode 필수")
    req_user = params.get("user", user)

    client = PlatformClient()

    # 1. project-description.md
    desc = await client.get_file_content(code, "business/project-description.md", ref="main")
    if not desc or _is_template_only(desc):
        alt = await client.get_file_content(code, "business/project-description.md", ref=f"docs/{req_user}")
        if alt and not _is_template_only(alt):
            desc = alt
    if not desc or _is_template_only(desc):
        return {
            "projectCode": code,
            "message": "프로젝트 관련 정보가 불충분합니다. 과제 설명 문서 작성을 먼저 진행해야 합니다.",
            "suggestSkill": "projdesc_interview",
            "created": [], "reused": [], "modified": [], "markedForDeletion": [],
        }

    # 2. 기존 이슈 조회
    vos_list = list(issues().find(
        {"key": {"$regex": f"^{re.escape(code)}-"}, "issueType": "VOS"},
        {"key": 1, "summary": 1, "description": 1, "client": 1, "_id": 0},
    ))
    if not vos_list:
        return {
            "projectCode": code,
            "message": "등록된 이해관계자 요구사항이 없습니다. VOS 수집을 먼저 진행해야 합니다.",
            "suggestSkill": "vos_collect",
            "created": [], "reused": [], "modified": [], "markedForDeletion": [],
        }

    existing_features = list(issues().find(
        {"key": {"$regex": f"^{re.escape(code)}-"}, "issueType": "Feature"},
        {"key": 1, "summary": 1, "description": 1, "actor": 1,
         "precondition": 1, "basicflow": 1, "alternativeflow": 1,
         "exceptionflow": 1, "postcondition": 1, "_id": 0},
    ))
    existing_feature_keys = {f["key"] for f in existing_features}
    valid_vos_keys = {v["key"] for v in vos_list}

    # 3. 생성 + 비판적 평가 루프
    template = _load_template()
    desc_trunc = desc[:5000]
    vos_json = json.dumps(vos_list, ensure_ascii=False, indent=2)
    feat_json = json.dumps(existing_features, ensure_ascii=False, indent=2)

    generate_prompt = f"""당신은 소프트웨어 아키텍트입니다. 프로젝트 {code}의 Use Case를 분석하세요.

## project-description.md
{desc_trunc}

## 기존 VOS ({len(vos_list)}건)
{vos_json}

## 기존 Feature ({len(existing_features)}건)
{feat_json}

## 작업
1. Business Domain 컨텍스트에서 Actor와 외부 시스템을 식별
2. 각 VOS를 충족시키는 Use Case 도출 (Actor와 매핑)
3. **Actor와 직접 관계를 가진 Use Case만 Feature로 선정** (보조/하위/include/extend 제외)
4. 각 Feature 후보 Use Case 시나리오 정의 (Basic/Alternative/Exception/Pre/Post)
5. Mermaid 다이어그램: Context (graph LR), Use Case (graph TB), Feature별 Sequence (sequenceDiagram)

## 각 Feature 항목의 mode 결정
- `create` / `reuse` / `modify` / `delete_mark` (existingKey/modifyReason/deleteReason 포함)

## 출력 (반드시 단일 JSON 객체로만 응답)
{{
  "features": [
    {{
      "summary": "Use Case 이름",
      "description": "Use Case 개요",
      "actor": "주요 Actor",
      "precondition": "...",
      "basicflow": "...",
      "alternativeflow": "...",
      "exceptionflow": "...",
      "postcondition": "...",
      "relatedVOS": ["VOS-키1"],
      "mode": "create",
      "existingKey": "", "modifyReason": "", "deleteReason": ""
    }}
  ],
  "document": "전체 use-cases.md 본문 (Markdown, Mermaid 코드블록 포함). 양식 준수."
}}

## use-cases.md 양식
{template}
"""

    def _critique_prompt(draft: str) -> str:
        return f"""당신은 엄격한 비판적 평가자입니다. Use Case 분석 결과를 평가하세요.

## 원본 과제 설명 (요약)
{desc_trunc[:3000]}

## VOS 목록
{vos_json}

## 생성 결과 (features + document)
{draft[:8000]}

## 평가 기준
1. Actor와 직접 관계된 Use Case만 Feature로 선정됨 (보조/하위/include/extend는 Feature 아님)
2. 각 Feature에 actor, precondition, basicflow, alternativeflow, exceptionflow, postcondition 모든 필드가 채워짐
3. relatedVOS가 실제 존재하는 VOS 키를 사용
4. 모든 VOS가 최소 1개 Feature와 매핑되어 누락이 없음
5. document에 Context Diagram, Use Case Diagram, 각 Feature별 Sequence Diagram(Mermaid)이 포함됨
6. Mermaid 다이어그램 구문 오류가 없음
7. mode 값이 유효하고 existingKey가 일관됨
8. JSON 형식이 유효함

## 출력 (반드시 JSON 객체로만 응답)
{{
  "pass": true 또는 false,
  "issues": ["문제점 1", "문제점 2"],
  "reasoning": "판단 근거"
}}"""

    def _revise_prompt(draft: str, critique: dict) -> str:
        issues_text = "\n".join(f"- {i}" for i in critique.get("issues", []))
        return f"""이전에 생성한 Use Case 분석 결과에 아래 문제가 있습니다. 문제를 수정한 새 결과를 출력하세요.

## VOS 목록
{vos_json}

## 이전 생성 결과
{draft[:10000]}

## 지적된 문제
{issues_text}

## 요청
지적된 문제만 고치고, 올바른 부분은 유지하세요.
출력 형식은 이전과 동일한 JSON 객체 ({{features, document}})입니다.
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

    features = parsed.get("features") or []
    document = parsed.get("document") or ""

    # 4. mode 분기 처리
    def _create_fn(item):
        feature_params = {
            "projectCode": code,
            "summary": (item.get("summary") or "").strip(),
            "description": item.get("description", ""),
        }
        for f in _USECASE_FIELDS:
            if item.get(f):
                feature_params[f] = item[f]
        extras = {f: feature_params[f] for f in _USECASE_FIELDS if feature_params.get(f)}
        return create_issue("Feature", feature_params, user, extras=extras or None)

    def _update_fields_fn(item):
        updates = {}
        for f in ("summary", "description") + _USECASE_FIELDS:
            v = item.get(f)
            if v is not None and v != "":
                updates[f] = v
        return updates

    buckets = dispatch_by_mode(features, existing_feature_keys, _create_fn, _update_fields_fn, user)

    # 5. VOS contains Feature 연결 (created/modified만)
    linked = []
    for bucket_name in ("created", "modified"):
        for entry in buckets[bucket_name]:
            feat_key = entry["key"]
            for vos_key in (entry["item"].get("relatedVOS") or []):
                if vos_key in valid_vos_keys:
                    try:
                        link_issues(vos_key, feat_key, "contains")
                        linked.append({"from": vos_key, "to": feat_key})
                    except Exception:
                        pass

    # 6. business/use-cases.md 저장
    saved_path = None
    save_error = None
    if document.strip():
        try:
            await client.save_file(
                code,
                "business/use-cases.md",
                document,
                f"Use Case 분석 결과 업데이트 (Feature 신규 {len(buckets['created'])} / 수정 {len(buckets['modified'])})",
                req_user,
            )
            saved_path = "business/use-cases.md"
        except Exception as e:
            save_error = str(e)

    tsum = trail_summary(critique_trail)
    base_msg = _summarize(buckets, len(linked), saved_path, save_error)
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
        "critiqueTrail": critique_trail,
        "message": base_msg + (f" / {tsum}" if tsum else ""),
    }


def _summarize(buckets: dict, link_cnt: int, saved: Optional[str], err: Optional[str]) -> str:
    parts = [f"Feature — 신규 {len(buckets['created'])}, 재사용 {len(buckets['reused'])}, "
             f"수정 {len(buckets['modified'])}, 삭제마킹 {len(buckets['markedForDeletion'])}"]
    parts.append(f"VOS-Feature 연결 {link_cnt}건")
    if saved:
        parts.append("use-cases.md 저장 완료")
    elif err:
        parts.append(f"문서 저장 실패: {err}")
    if buckets["markedForDeletion"]:
        parts.append("⚠ 삭제 마킹된 항목은 검토 후 실제 삭제 또는 마킹 해제가 필요합니다")
    return " / ".join(parts)
