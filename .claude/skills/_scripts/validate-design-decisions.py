#!/usr/bin/env python3
"""validate-design-decisions.py — design-decide2 Type A 검증.

stdin JSON:
{
  "driverKeys": ["<key>", ...],
  "requirementKeys": ["<key>", ...],
  "designs": [
    {"docId":"ADSN-1","summary":"...","designPoint":"...",
     "approaches":[{"name":"A","pros":"","cons":""}, {"name":"B","pros":"","cons":""}],
     "decision":"A","decisionRationale":"...","tradeoffs":"...",
     "risk":"...","sensitivity":"...","designLevel":"Architectural",
     "relatedRequirements":["<key>"],"mode":"create","existingKey":""},
    ...
  ],
  "document": "<markdown>"
}

stdout JSON:
{
  "pass": bool, "score":..., "score_effective":...,
  "critical":[...], "suggestions":[...],
  "stats": {"designs":N,"adsn":A,"ddsn":D,"driverCoverage":"C/T",
            "approachAvg":F,"requirementCovered":R}
}
"""
from __future__ import annotations

import json
import re
import sys
from collections import Counter


DESIGN_REQUIRED = {
    "summary", "designPoint", "approaches", "decision",
    "decisionRationale", "designLevel", "docId", "relatedRequirements",
}
VALID_MODES = {"create", "reuse", "modify", "delete_mark"}
VALID_LEVELS = {"Architectural", "Detailed"}

DOCID_RE = re.compile(r"^(ADSN|DDSN)-\d+$")


def validate(payload: dict) -> dict:
    driver_keys = set(payload.get("driverKeys") or [])
    req_keys = set(payload.get("requirementKeys") or [])
    designs = payload.get("designs") or []
    doc = payload.get("document", "") or ""

    critical: list[dict] = []
    suggestions: list[dict] = []

    doc_ids = [d.get("docId", "") for d in designs]
    dup_ids = [k for k, v in Counter(doc_ids).items() if v > 1 and k]
    for d in dup_ids:
        critical.append({"type": "duplicate_docId", "docId": d,
                         "detail": "docId 중복"})

    driver_hit = set()
    req_hit = set()
    approach_counts = []
    adsn = ddsn = 0

    for d in designs:
        doc_id = d.get("docId", "<missing>")

        if doc_id and not DOCID_RE.match(doc_id):
            critical.append({"type": "bad_docId_format", "docId": doc_id,
                             "detail": "docId 는 ADSN-N 또는 DDSN-N 형식"})

        for field in DESIGN_REQUIRED:
            val = d.get(field)
            if val is None:
                critical.append({"type": "missing_field", "docId": doc_id,
                                 "detail": f"{field} 누락"})
                continue
            if field == "approaches":
                if not isinstance(val, list):
                    critical.append({"type": "bad_approaches_type", "docId": doc_id,
                                     "detail": "approaches 배열 아님"})
                    approach_counts.append(0)
                else:
                    approach_counts.append(len(val))
                    if len(val) < 2:
                        critical.append({"type": "insufficient_approaches", "docId": doc_id,
                                         "detail": f"approaches={len(val)} — ADR 은 2개 이상 필수"})
                    for i, a in enumerate(val):
                        if not isinstance(a, dict):
                            critical.append({"type": "bad_approach_item", "docId": doc_id,
                                             "detail": f"approaches[{i}] 가 dict 아님"})
                            continue
                        for k in ("name", "pros", "cons"):
                            if not (a.get(k) or "").strip():
                                suggestions.append({"type": "approach_field_empty",
                                                    "docId": doc_id,
                                                    "detail": f"approaches[{i}].{k} 비어있음"})
            elif field == "relatedRequirements":
                if not isinstance(val, list):
                    critical.append({"type": "bad_relatedRequirements_type",
                                     "docId": doc_id,
                                     "detail": "배열 아님"})
                elif not val:
                    critical.append({"type": "no_requirement_link", "docId": doc_id,
                                     "detail": "어느 Driver/Requirement 에도 연결되지 않음 — rationale 추적성 결여"})
                else:
                    unknown = [k for k in val if req_keys and k not in req_keys]
                    if unknown:
                        critical.append({"type": "relatedRequirements_hallucination",
                                         "docId": doc_id,
                                         "detail": f"Requirement 목록에 없는 키: {unknown}"})
                    for k in val:
                        req_hit.add(k)
                        if k in driver_keys:
                            driver_hit.add(k)
            elif field == "decisionRationale":
                if not str(val).strip():
                    critical.append({"type": "empty_rationale", "docId": doc_id,
                                     "detail": "decisionRationale 비어있음 — Driver 추적 불가"})

        lvl = d.get("designLevel")
        if lvl not in VALID_LEVELS:
            critical.append({"type": "bad_designLevel", "docId": doc_id,
                             "detail": f"designLevel={lvl!r} (허용: {sorted(VALID_LEVELS)})"})
        elif lvl == "Architectural":
            adsn += 1
            if doc_id and not doc_id.startswith("ADSN-"):
                critical.append({"type": "docId_level_mismatch", "docId": doc_id,
                                 "detail": "Architectural 은 ADSN-N 접두 필요"})
        elif lvl == "Detailed":
            ddsn += 1
            if doc_id and not doc_id.startswith("DDSN-"):
                critical.append({"type": "docId_level_mismatch", "docId": doc_id,
                                 "detail": "Detailed 은 DDSN-N 접두 필요"})

        mode = d.get("mode", "create")
        if mode not in VALID_MODES:
            critical.append({"type": "bad_mode", "docId": doc_id,
                             "detail": f"mode={mode!r}"})

    # Driver 커버리지
    if driver_keys:
        uncovered = driver_keys - driver_hit
        for k in sorted(uncovered):
            critical.append({"type": "driver_uncovered", "docId": k,
                             "detail": "어느 설계 결정에도 매핑 안 됨"})

    # 문서 체크 — 기본 섹션 / 각 ADSN 등장
    for sid in doc_ids:
        if sid and sid not in doc:
            suggestions.append({"type": "design_not_in_doc", "docId": sid,
                                "detail": "문서 본문에 해당 docId 가 등장하지 않음"})

    section_patterns = [
        re.compile(r"#{1,3}.*1\..*Design Point", re.IGNORECASE),
        re.compile(r"#{1,3}.*2\..*설계 결정 상세|#{1,3}.*2\..*Design Decision", re.IGNORECASE),
        re.compile(r"#{1,3}.*3\..*Driver.*매트릭스|#{1,3}.*3\..*Driver.*Matrix", re.IGNORECASE),
        re.compile(r"#{1,3}.*4\..*리스크|#{1,3}.*4\..*Risk", re.IGNORECASE),
    ]
    for i, sec in enumerate(section_patterns, start=1):
        if not sec.search(doc):
            critical.append({"type": "missing_section", "docId": "(doc)",
                             "detail": f"섹션 {i} 헤딩 누락"})

    approach_avg = (sum(approach_counts) / len(approach_counts)) if approach_counts else 0
    total = max(1, 3 + len(designs) + len(driver_keys))
    score = max(0.0, 1.0 - len(critical) / total)
    score_effective = round(score - 0.5 * len(critical), 4)

    return {
        "pass": len(critical) == 0 and score >= 0.8,
        "score": round(score, 4),
        "score_effective": score_effective,
        "critical": critical,
        "suggestions": suggestions,
        "stats": {
            "designs": len(designs),
            "adsn": adsn,
            "ddsn": ddsn,
            "driverCoverage": f"{len(driver_hit)}/{len(driver_keys)}",
            "approachAvg": round(approach_avg, 2),
            "requirementCovered": len(req_hit),
        },
    }


def main() -> int:
    try:
        payload = json.load(sys.stdin)
    except json.JSONDecodeError as exc:
        print(json.dumps({"pass": False, "score": 0.0, "score_effective": -1.0,
                          "critical": [{"type": "invalid_json",
                                        "docId": "(input)", "detail": str(exc)}],
                          "suggestions": [], "stats": {}},
                         ensure_ascii=False, indent=2))
        return 2
    result = validate(payload)
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["pass"] else 1


if __name__ == "__main__":
    sys.exit(main())
