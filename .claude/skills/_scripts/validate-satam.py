#!/usr/bin/env python3
"""validate-satam.py — satam-analyze2 의 Type A 검증 로직.

stdin JSON:
{
  "adsns": {
    "<ADSN_KEY>": {
      "risk":"<원문>", "sensitivity":"<원문>", "tradeoffs":"<원문>",
      "mitigation":"<원문>", "linkedQAs": ["<key>", ...]
    },
    ...
  },
  "qaKeys": ["<key>", ...],
  "mapping": {
    "<ADSN_KEY>": {
      "risks": [ {"id":"R1","qa":["<key>"],"description":"..."}, ... ],
      "sensitivities": [ {"id":"S1", ...}, ... ],
      "tradeoffs": [ {"id":"M1","qa":[...],"counterRisk":["R1:<key>"],"description":"..."}, ... ]
    },
    ...
  }
}

stdout JSON:
{
  "pass": bool,
  "score": 0.0~1.0,
  "score_effective": number,
  "critical": [{"type":"...","adsn":"...","id":"...","detail":"..."}],
  "suggestions": [...],
  "stats": {"adsns":N,"risks":R,"sensitivities":S,"tradeoffs":T,"qaCoveredADSNs":Q}
}
"""
from __future__ import annotations

import json
import re
import sys
from collections import Counter


ID_RE_RISK = re.compile(r"^R\d+$")
ID_RE_SENS = re.compile(r"^S\d+$")
ID_RE_MIT = re.compile(r"^M\d+$")
COUNTER_RE = re.compile(r"^R\d+:[A-Za-z0-9_-]+-\d+$")


def _has_text(s: str | None) -> bool:
    return bool(s and s.strip())


def validate(payload: dict) -> dict:
    adsns = payload.get("adsns") or {}
    qa_keys = set(payload.get("qaKeys") or [])
    mapping = payload.get("mapping") or {}

    critical: list[dict] = []
    suggestions: list[dict] = []

    stats = {"adsns": 0, "risks": 0, "sensitivities": 0, "tradeoffs": 0, "qaCoveredADSNs": 0}

    adsn_keys = set(adsns.keys())

    # 매핑에 있는데 Step 0 목록에 없는 ADSN
    for mk in mapping.keys():
        if mk not in adsn_keys:
            critical.append({"type": "unknown_adsn", "adsn": mk,
                             "detail": f"Step 0 ADSN 목록에 없는 키"})

    for adsn_key, data in adsns.items():
        m = mapping.get(adsn_key) or {}
        risks = m.get("risks") or []
        sens = m.get("sensitivities") or []
        trades = m.get("tradeoffs") or []

        stats["adsns"] += 1
        stats["risks"] += len(risks)
        stats["sensitivities"] += len(sens)
        stats["tradeoffs"] += len(trades)

        any_qa_covered = False

        # 카테고리별 원문 있는데 리스트 비어있으면 critical
        if _has_text(data.get("risk")) and not risks:
            critical.append({"type": "category_empty", "adsn": adsn_key, "id": "risks",
                             "detail": "risk 원문 있는데 risks 리스트 비어있음"})
        if _has_text(data.get("sensitivity")) and not sens:
            critical.append({"type": "category_empty", "adsn": adsn_key, "id": "sensitivities",
                             "detail": "sensitivity 원문 있는데 sensitivities 리스트 비어있음"})
        if (_has_text(data.get("tradeoffs")) or _has_text(data.get("mitigation"))) and not trades:
            critical.append({"type": "category_empty", "adsn": adsn_key, "id": "tradeoffs",
                             "detail": "tradeoffs/mitigation 원문 있는데 tradeoffs 리스트 비어있음"})

        # id 형식 + 중복 검증 (ADSN 내부 유일)
        def _check_items(items: list, prefix_re, label: str, expect_prefix: str):
            nonlocal any_qa_covered
            ids = []
            for item in items:
                iid = item.get("id", "")
                if not prefix_re.match(iid):
                    critical.append({"type": "bad_id_format", "adsn": adsn_key, "id": iid or "(missing)",
                                     "detail": f"{label} id 는 {expect_prefix}N 형식"})
                ids.append(iid)
                qa = item.get("qa") or []
                if not isinstance(qa, list):
                    critical.append({"type": "bad_qa_type", "adsn": adsn_key, "id": iid,
                                     "detail": f"{label}.qa 는 배열이어야 함"})
                else:
                    for qk in qa:
                        if qa_keys and qk not in qa_keys:
                            critical.append({"type": "qa_hallucination", "adsn": adsn_key, "id": iid,
                                             "detail": f"{label}.qa={qk} 가 QA 참조 목록에 없음"})
                        else:
                            any_qa_covered = True
                if not _has_text(item.get("description")):
                    critical.append({"type": "missing_description", "adsn": adsn_key, "id": iid,
                                     "detail": f"{label}.description 누락"})
            dups = [i for i, c in Counter(ids).items() if c > 1 and i]
            for d in dups:
                critical.append({"type": "duplicate_id", "adsn": adsn_key, "id": d,
                                 "detail": f"{label} id 중복"})

        _check_items(risks, ID_RE_RISK, "risk", "R")
        _check_items(sens, ID_RE_SENS, "sensitivity", "S")
        _check_items(trades, ID_RE_MIT, "tradeoff", "M")

        # tradeoff 의 counterRisk 검증
        risk_ids = {r.get("id") for r in risks}
        for t in trades:
            cr = t.get("counterRisk") or []
            tid = t.get("id", "?")
            if not cr:
                critical.append({"type": "missing_counter_risk", "adsn": adsn_key, "id": tid,
                                 "detail": "trade-off 에 counterRisk 필수"})
                continue
            if not isinstance(cr, list):
                critical.append({"type": "bad_counter_risk_type", "adsn": adsn_key, "id": tid,
                                 "detail": "counterRisk 는 배열이어야 함"})
                continue
            for c in cr:
                if not COUNTER_RE.match(c):
                    critical.append({"type": "bad_counter_risk_format", "adsn": adsn_key, "id": tid,
                                     "detail": f"counterRisk 항목 '{c}' 형식 오류 (예: R1:TPRJ-23)"})
                    continue
                rid = c.split(":", 1)[0]
                if rid not in risk_ids:
                    critical.append({"type": "counter_risk_external_adsn", "adsn": adsn_key, "id": tid,
                                     "detail": f"counterRisk {c} 의 {rid} 가 같은 ADSN 내 Risk 가 아님"})

        # QA 참조가 전부 빈 상태인데 QA 목록이 있으면 critical
        if qa_keys and not any_qa_covered and (risks or sens or trades):
            critical.append({"type": "no_qa_referenced", "adsn": adsn_key, "id": "(global)",
                             "detail": "이 ADSN 의 모든 항목이 qa=[] — QA 참조가 전혀 없음"})
        if any_qa_covered:
            stats["qaCoveredADSNs"] += 1

    total_items = stats["risks"] + stats["sensitivities"] + stats["tradeoffs"] + max(1, len(adsns))
    score = max(0.0, 1.0 - len(critical) / max(1, total_items))
    score_effective = round(score - 0.5 * len(critical), 4)

    return {
        "pass": len(critical) == 0 and score >= 0.8,
        "score": round(score, 4),
        "score_effective": score_effective,
        "critical": critical,
        "suggestions": suggestions,
        "stats": stats,
    }


def main() -> int:
    try:
        payload = json.load(sys.stdin)
    except json.JSONDecodeError as exc:
        print(json.dumps({"pass": False, "score": 0.0, "score_effective": -1.0,
                          "critical": [{"type": "invalid_json", "adsn": "(input)",
                                        "id": "(input)", "detail": str(exc)}],
                          "suggestions": [], "stats": {}},
                         ensure_ascii=False, indent=2))
        return 2
    result = validate(payload)
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["pass"] else 1


if __name__ == "__main__":
    sys.exit(main())
