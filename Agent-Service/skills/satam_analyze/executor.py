"""SATAM 분석 스킬 — ADSN 의 risk/sensitivity/tradeoffs 원문을 구조화 YAML 로 변환해
각 ADSN 이슈의 risk/sensitivity/mitigation 필드에 직접 반영한다.

문서 파일은 생성하지 않는다. 프론트엔드 SATAM 컴포넌트가 이 필드를 직접 파싱해 렌더링한다.
"""

import re
import json
from typing import List, Dict

from core.database import issues, projects
from core.llm import call_llm


async def execute(params: dict, user: str) -> dict:
    code = params.get("projectCode", "")
    if not code:
        raise ValueError("projectCode 필수")

    prefix = {"key": {"$regex": f"^{re.escape(code)}-"}}
    qa_issues = list(issues().find(
        {**prefix, "issueType": "Requirement", "reqtype": "NFR"},
        {"key": 1, "subkey": 1, "summary": 1, "_id": 0},
    ))
    adsn_issues = list(issues().find({**prefix, "issueType": "Architectural Design"}))

    if not adsn_issues:
        return {
            "projectCode": code,
            "message": "ADSN(Architectural Design) 이슈가 없습니다. design_decide 를 먼저 실행하세요.",
            "adsnCount": 0,
        }

    # 1) LLM 에 넘길 입력 데이터 준비
    adsn_inputs = []
    for ad in adsn_issues:
        # contains 링크로 연결된 QA 키
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

    # 2) LLM 프롬프트 — 각 ADSN 의 원문 텍스트를 구조화 YAML 로 변환. QA 연결 추론.
    prompt = _build_prompt(code, adsn_inputs, qa_issues)
    raw = await call_llm(prompt)
    mapping = _extract_json(raw) or {}

    # 3) 각 ADSN 이슈의 risk/sensitivity/mitigation 필드를 업데이트
    updated_keys = []
    failed = []
    risk_total = sens_total = miti_total = 0

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
            from core import issuestore
            doc = issuestore.get_issue(key)
            if not doc:
                raise ValueError(f"이슈 없음: {key}")
            doc.risk = risk_yaml
            doc.sensitivity = sens_yaml
            doc.mitigation = miti_yaml
            await issuestore.save_issue(doc, user, commit_message=f"[satam] {key} R/S/M 업데이트")
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

    return {
        "projectCode": code,
        "adsnCount": len(adsn_inputs),
        "updatedCount": len(updated_keys),
        "riskCount": risk_total,
        "sensitivityCount": sens_total,
        "mitigationCount": miti_total,
        "failed": failed,
        "nextAction": {"skill": "ui_navigate", "params": {"projectCode": code, "page": "satam"}},
        "message": (
            f"SATAM 분석 완료 — ADSN {len(updated_keys)}건의 risk/sensitivity/mitigation 필드 갱신. "
            f"Risks {risk_total} / Sensitivities {sens_total} / Trade-offs {miti_total}. "
            "SATAM 화면으로 이동하려면 ui_navigate(page=satam) 를 호출하세요."
        ),
    }


def _build_prompt(code: str, adsn_inputs: List[Dict], qa_refs: List[Dict]) -> str:
    adsn_json = json.dumps(adsn_inputs, ensure_ascii=False, indent=2)
    qa_json = json.dumps(qa_refs, ensure_ascii=False, indent=2)
    return f"""프로젝트 {code} 의 SATAM 분석을 수행한다.
각 ADSN 이슈의 riskText / sensitivityText / tradeoffText 원문을 읽어 구조화된 항목 리스트로 변환하고,
영향받는 QA(Quality Attribute) 이슈 key 를 매핑한다. 이 결과는 각 ADSN 이슈의 risk/sensitivity/mitigation
필드에 YAML 로 저장되어 프론트엔드 SATAM 컴포넌트가 직접 파싱한다.

## 입력

### ADSN 목록 (각 항목의 원문을 구조화 대상)
{adsn_json}

### QA 참조 (Requirement 중 NFR) — Risk/Sensitivity/Trade-off 가 영향주는 대상
{qa_json}

## 출력 (반드시 단일 JSON, 코드블록 없이)

{{
  "<ADSN_ISSUE_KEY>": {{
    "risks": [
      {{ "id": "R1", "qa": ["QA_ISSUE_KEY", ...], "description": "..." }}
    ],
    "sensitivities": [
      {{ "id": "S1", "qa": ["QA_ISSUE_KEY", ...], "description": "..." }}
    ],
    "tradeoffs": [
      {{ "id": "M1", "qa": ["QA_ISSUE_KEY", ...], "counterRisk": ["R1:QA_ISSUE_KEY", ...], "description": "..." }}
    ]
  }},
  ...
}}

## 규칙 (엄수)

1. **키는 ADSN 이슈 key** (예: "{code}-23"). subkey 사용 금지
2. **ID 순차**: ADSN 별로 R1,R2... / S1,S2... / M1,M2... 부여. 다른 ADSN 과 번호 공유 안 함 (ADSN 내부에서 유일)
3. **QA 매핑**: `qa` 배열은 **이슈 key** 형식 (subkey 금지). `linkedQAs` 우선 사용, 없거나 부족하면 QA 참조 목록에서 summary 로 매칭해 선택
4. **Counter Risk (Trade-off 전용)**: 해당 Mitigation 이 완화하는 Risk 의 `"R1:QA_ISSUE_KEY"` 조합 배열. 같은 ADSN 내 Risk ID 참조
5. **원문 유지**: 원문(riskText/sensitivityText/tradeoffText) 이 의미상 분리 가능한 복수 항목이면 여러 항목으로 분해, 단일이면 1개 항목. 임의 창작 금지
6. **빈 원문은 빈 배열**: 원문이 비어있으면 해당 리스트는 `[]`
7. **출력은 JSON 객체 단일** — 앞뒤 설명, 코드블록 모두 금지. 바로 `{{` 로 시작"""


def _extract_json(text: str) -> Dict:
    """LLM 응답에서 JSON 객체 추출."""
    t = text.strip()
    # ```json ... ``` 블록 제거
    m = re.search(r"```(?:json)?\s*\n(.*?)```", t, re.DOTALL | re.IGNORECASE)
    if m:
        t = m.group(1).strip()
    # 첫 { 부터 매칭되는 } 까지
    start = t.find("{")
    if start < 0:
        return {}
    depth = 0
    for i in range(start, len(t)):
        ch = t[i]
        if ch == "{":
            depth += 1
        elif ch == "}":
            depth -= 1
            if depth == 0:
                try:
                    return json.loads(t[start:i + 1])
                except json.JSONDecodeError:
                    return {}
    return {}


def _serialize_items(items: List[Dict], counter: bool) -> str:
    """항목 리스트를 SATAM 프론트엔드 parseRST 포맷의 YAML 로 직렬화.

    형식:
      - ID : R1
        QA : [KEY1, KEY2]
        Counter Risk : ["R1:KEY1", ...]   (mitigation 전용)
        Description : ...
    """
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
