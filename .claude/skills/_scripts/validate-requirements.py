#!/usr/bin/env python3
"""validate-requirements.py — requirement-refine2 의 Type A 검증 로직.

stdin JSON:
{
  "requirements": [
    {"docId":"FR-1","reqType":"FR","summary":"...","description":"...","relatedFeatures":["GOJIRA3-5"],"mode":"create", ...},
    {"docId":"NFR-1","reqType":"NFR","summary":"...","description":"...","relatedFeatures":["GOJIRA3-5"],"mode":"create","qa":"Performance","source":"...","stimulus":"...","environment":"...","artifact":"...","response":"...","measure":"...","responseScenario":"..."},
    {"docId":"CNST-1","reqType":"CNST","summary":"...","description":"...","relatedFeatures":[],"mode":"create"}
  ],
  "featureKeys": ["GOJIRA3-5","GOJIRA3-6", ...],
  "existingRequirementKeys": ["GOJIRA3-100", ...]
}

stdout JSON:
{
  "pass": bool,
  "score": float (0.0~1.0),
  "score_effective": float (score - 0.5 * |critical|),
  "critical": [{"type":"...","docId":"...","detail":"..."}, ...],
  "suggestions": [{"type":"...","docId":"...","detail":"..."}, ...],
  "stats": {"FR":N,"NFR":M,"CNST":K,"features":F,"coveredFeatures":C}
}
"""
from __future__ import annotations

import json
import re
import sys
from collections import Counter, defaultdict

USER_STORY_LOOSE_RE = re.compile(r".+(은|는)\s?.+(을|를|기|하기|화|가|이|여)\s?위해\s?.+\s수\s있다")

FR_REQUIRED = {"summary", "description", "docId", "relatedFeatures", "mode"}
NFR_REQUIRED = FR_REQUIRED | {
    "qa", "source", "stimulus", "environment",
    "artifact", "response", "measure", "responseScenario",
}
CNST_REQUIRED = FR_REQUIRED

VALID_MODES = {"create", "reuse", "modify", "delete_mark"}
VALID_QAS = {"Performance", "Security", "Availability",
             "Usability", "Maintainability", "Scalability",
             "Reliability", "Portability", "Testability",
             "Modifiability", "Interoperability"}


def validate(payload: dict) -> dict:
    reqs = payload.get("requirements", [])
    feature_keys = set(payload.get("featureKeys", []))
    existing_keys = set(payload.get("existingRequirementKeys", []))

    critical: list[dict] = []
    suggestions: list[dict] = []

    doc_ids = [r.get("docId", "") for r in reqs]
    dup_ids = [k for k, v in Counter(doc_ids).items() if v > 1 and k]
    for dup in dup_ids:
        critical.append({"type": "duplicate_docId", "docId": dup,
                         "detail": "docId 중복"})

    type_counts = Counter(r.get("reqType", "?") for r in reqs)
    fr_coverage: dict[str, list[str]] = defaultdict(list)
    any_nfr_qas: set[str] = set()

    for r in reqs:
        doc_id = r.get("docId", "<missing>")
        rtype = r.get("reqType", "")

        if rtype not in {"FR", "NFR", "CNST"}:
            critical.append({"type": "bad_reqType", "docId": doc_id,
                             "detail": f"reqType={rtype!r}"})
            continue

        required = FR_REQUIRED if rtype == "FR" else (NFR_REQUIRED if rtype == "NFR" else CNST_REQUIRED)
        for field in required:
            val = r.get(field)
            if val is None:
                critical.append({"type": "missing_field", "docId": doc_id,
                                 "detail": f"{field} 누락"})
                continue
            if field == "relatedFeatures":
                if not isinstance(val, list):
                    critical.append({"type": "bad_relatedFeatures_type",
                                     "docId": doc_id,
                                     "detail": "relatedFeatures 는 배열이어야 함"})
                continue
            if field in {"summary", "description"} and not str(val).strip():
                critical.append({"type": "empty_field", "docId": doc_id,
                                 "detail": f"{field} 빈 값"})

        prefix_map = {"FR": "FR-", "NFR": "NFR-", "CNST": "CNST-"}
        if doc_id and not doc_id.startswith(prefix_map[rtype]):
            critical.append({"type": "docId_prefix_mismatch", "docId": doc_id,
                             "detail": f"{rtype} 는 {prefix_map[rtype]} 접두 필요"})

        mode = r.get("mode", "")
        if mode not in VALID_MODES:
            critical.append({"type": "bad_mode", "docId": doc_id,
                             "detail": f"mode={mode!r} (허용: {sorted(VALID_MODES)})"})
        elif mode in {"reuse", "modify", "delete_mark"}:
            ek = r.get("existingKey", "")
            if not ek:
                critical.append({"type": "missing_existingKey", "docId": doc_id,
                                 "detail": f"mode={mode} 인데 existingKey 없음"})
            elif existing_keys and ek not in existing_keys:
                critical.append({"type": "bad_existingKey", "docId": doc_id,
                                 "detail": f"existingKey={ek} 가 기존 Requirement 에 없음"})

        rel = r.get("relatedFeatures")
        if isinstance(rel, list):
            if feature_keys:
                unknown = [k for k in rel if k not in feature_keys]
                if unknown:
                    critical.append({"type": "relatedFeatures_hallucination",
                                     "docId": doc_id,
                                     "detail": f"Feature 목록에 없는 키: {unknown}"})
            if rtype == "FR":
                for fk in rel:
                    fr_coverage[fk].append(doc_id)

        if rtype == "FR":
            desc = r.get("description", "") or ""
            if desc and not USER_STORY_LOOSE_RE.search(desc):
                critical.append({"type": "user_story_format", "docId": doc_id,
                                 "detail": "FR description 이 User Story 형식 아님 ('~은(는) ~을 위해 ~할 수 있다')"})

        if rtype == "NFR":
            qa = r.get("qa", "")
            if qa:
                any_nfr_qas.add(qa)
                if qa not in VALID_QAS:
                    suggestions.append({"type": "unusual_qa", "docId": doc_id,
                                        "detail": f"흔치 않은 QA 속성: {qa}"})

    if feature_keys:
        covered_by_any = set()
        for r in reqs:
            for fk in (r.get("relatedFeatures") or []):
                if fk in feature_keys:
                    covered_by_any.add(fk)
        uncovered = feature_keys - covered_by_any
        for fk in sorted(uncovered):
            has_global = any(isinstance(r.get("relatedFeatures"), list)
                             and len(r.get("relatedFeatures")) == 0
                             and r.get("reqType") in {"NFR", "CNST"}
                             for r in reqs)
            if has_global:
                suggestions.append({"type": "feature_only_global_covered",
                                    "docId": fk,
                                    "detail": "직접 매핑 Requirement 없음 (전역 NFR/CNST 로 간접 커버)"})
            else:
                critical.append({"type": "feature_uncovered", "docId": fk,
                                 "detail": "매핑된 Requirement (FR/NFR/CNST) 없음"})

        feature_fr_counts = [len(v) for v in fr_coverage.values()]
        fr_covered_features = len(fr_coverage)
        if fr_covered_features:
            avg_fr = sum(feature_fr_counts) / fr_covered_features
            if avg_fr < 1.5:
                suggestions.append({"type": "fr_subdivision_low",
                                    "docId": "(global)",
                                    "detail": f"FR-커버된 Feature 당 평균 FR={avg_fr:.2f} (권장 >=1.5)"})

    if type_counts.get("NFR", 0) > 0:
        core_qas = {"Performance", "Security", "Availability"}
        missing_core = core_qas - any_nfr_qas
        if len(missing_core) >= 2:
            suggestions.append({"type": "qa_diversity_low",
                                "docId": "(global)",
                                "detail": f"핵심 QA 속성 중 {missing_core} 누락"})

    total_checks = max(1, len(reqs) * 4 + (len(feature_keys) if feature_keys else 0))
    score = max(0.0, 1.0 - (len(critical) / total_checks))
    score_effective = round(score - 0.5 * len(critical), 4)
    passes = len(critical) == 0 and score >= 0.8

    return {
        "pass": passes,
        "score": round(score, 4),
        "score_effective": score_effective,
        "critical": critical,
        "suggestions": suggestions,
        "stats": {
            "FR": type_counts.get("FR", 0),
            "NFR": type_counts.get("NFR", 0),
            "CNST": type_counts.get("CNST", 0),
            "features": len(feature_keys),
            "coveredFeatures": len({fk for r in reqs
                                    for fk in (r.get("relatedFeatures") or [])
                                    if fk in feature_keys}),
        },
    }


def main() -> int:
    try:
        payload = json.load(sys.stdin)
    except json.JSONDecodeError as exc:
        print(json.dumps({
            "pass": False, "score": 0.0, "score_effective": -1.0,
            "critical": [{"type": "invalid_json",
                          "docId": "(input)",
                          "detail": str(exc)}],
            "suggestions": [], "stats": {},
        }, ensure_ascii=False, indent=2))
        return 2

    result = validate(payload)
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["pass"] else 1


if __name__ == "__main__":
    sys.exit(main())
