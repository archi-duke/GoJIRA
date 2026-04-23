#!/usr/bin/env python3
"""validate-satam.py — satam-analyze2 의 Type A 검증 로직.

v2 기준: risk/sensitivity 는 "현 설계에서 명확한 문제 / 조건 변화 시 risk 될 사항"
으로 엄격 분류. 나머지 관찰·검토 사항은 concerns (C-prefix) 로 분리.

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
      "tradeoffs": [ {"id":"M1","qa":[...],"counterRisk":["R1:<key>"],"description":"..."}, ... ],
      "concerns":  [ {"id":"C1","qa":[...],"description":"..."}, ... ]   # optional
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
ID_RE_CONCERN = re.compile(r"^C\d+$")
COUNTER_RE = re.compile(r"^R\d+:[A-Za-z0-9_-]+-\d+$")


def _has_text(s: str | None) -> bool:
    return bool(s and s.strip())


def _desc_similar(a: str, b: str) -> bool:
    """두 description 이 의미상 같은지 개략 비교.

    완벽한 의미 비교는 불가능하므로 매우 관대한 기준:
    - 정규화 후 동일 → 같음
    - 정규화 후 Jaccard token 유사도 ≥ 0.6 → 같음
    """
    if a is None or b is None:
        return False
    a_norm = re.sub(r"\s+", " ", (a or "").strip().lower())
    b_norm = re.sub(r"\s+", " ", (b or "").strip().lower())
    if a_norm == b_norm:
        return True
    if not a_norm or not b_norm:
        return False
    ta = set(a_norm.split())
    tb = set(b_norm.split())
    if not ta or not tb:
        return False
    inter = len(ta & tb)
    union = len(ta | tb)
    return (inter / union) >= 0.6


def validate(payload: dict) -> dict:
    adsns = payload.get("adsns") or {}
    qa_keys = set(payload.get("qaKeys") or [])
    mapping = payload.get("mapping") or {}

    critical: list[dict] = []
    suggestions: list[dict] = []

    stats = {"adsns": 0, "risks": 0, "sensitivities": 0, "tradeoffs": 0,
             "concerns": 0, "qaCoveredADSNs": 0}

    # 프로젝트 전역 ID 일관성 추적용: id -> {"desc": str, "adsn": str}
    # R/S/M 은 전역 유일, 같은 ID 공유 시 description 이 의미상 같아야 함 (개략 비교)
    global_id_desc: dict[str, dict] = {}

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
        concerns = m.get("concerns") or []

        stats["adsns"] += 1
        stats["risks"] += len(risks)
        stats["sensitivities"] += len(sens)
        stats["tradeoffs"] += len(trades)
        stats["concerns"] += len(concerns)

        any_qa_covered = False

        # v2: 엄격 분류 기준상 risks/sensitivities 가 비어있는 게 오히려 정상일 수 있음.
        # 원문이 있어도 실제 "명확한 문제" 가 아니면 concerns 로 분리 가능.
        # 단, 원문이 있는데 risks + sensitivities + concerns 가 **모두** 비어있으면 분석 누락 의심.
        original_has_rst = (_has_text(data.get("risk"))
                             or _has_text(data.get("sensitivity"))
                             or _has_text(data.get("tradeoffs"))
                             or _has_text(data.get("mitigation")))
        if original_has_rst and not (risks or sens or trades or concerns):
            critical.append({"type": "analysis_skipped", "adsn": adsn_key, "id": "(all)",
                             "detail": "원문 risk/sensitivity/tradeoffs/mitigation 있는데 분석 결과 전체 비어있음"})

        # tradeoffs/mitigation 원문이 있는데 mitigation(M) 항목이 없으면 여전히 critical
        # (mitigation 은 원문 존재 시 분석 필수)
        if (_has_text(data.get("tradeoffs")) or _has_text(data.get("mitigation"))) and not trades:
            critical.append({"type": "mitigation_missing", "adsn": adsn_key, "id": "tradeoffs",
                             "detail": "tradeoffs/mitigation 원문 있는데 대응 mitigation 목록 비어있음"})

        # id 형식 + 중복 검증 (ADSN 내부 유일)
        # 전역 유일 정책: R/S/M 은 프로젝트 전체에서 같은 ID 공유 시 의미(description) 일관성 체크.
        # C(Concerns) 는 ADSN 국소 식별자라 전역 체크 제외.
        def _check_items(items: list, prefix_re, label: str, expect_prefix: str,
                         global_unique: bool = True):
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
                desc = item.get("description", "")
                if not _has_text(desc):
                    critical.append({"type": "missing_description", "adsn": adsn_key, "id": iid,
                                     "detail": f"{label}.description 누락"})

                # 전역 ID 일관성 — 같은 ID 가 여러 ADSN 에 있을 때 description 이 차이 크면 suggestion
                if global_unique and iid and prefix_re.match(iid):
                    prev = global_id_desc.get(iid)
                    if prev is None:
                        global_id_desc[iid] = {"desc": desc, "adsn": adsn_key}
                    else:
                        if _desc_similar(prev["desc"], desc):
                            # 정상 재사용 — 별도 보고 없음
                            pass
                        else:
                            suggestions.append({
                                "type": "inconsistent_global_id",
                                "adsn": adsn_key, "id": iid,
                                "detail": (f"{label} {iid} 가 {prev['adsn']} 에서 이미 사용된 ID 인데 "
                                           f"description 이 의미상 다름 — 같은 내용이면 동일 문구로, "
                                           f"다른 내용이면 새 번호로 할당 권장"),
                            })
            dups = [i for i, c in Counter(ids).items() if c > 1 and i]
            for d in dups:
                critical.append({"type": "duplicate_id", "adsn": adsn_key, "id": d,
                                 "detail": f"{label} id ADSN 내 중복"})

        _check_items(risks, ID_RE_RISK, "risk", "R")
        _check_items(sens, ID_RE_SENS, "sensitivity", "S")
        _check_items(trades, ID_RE_MIT, "tradeoff", "M")
        _check_items(concerns, ID_RE_CONCERN, "concern", "C", global_unique=False)

        # tradeoff 의 counterRisk 검증
        # v2: 전역 ID 체계이므로 Risk 가 이 ADSN 에 없어도 다른 ADSN 의 Risk 참조 허용.
        # 단 프로젝트 전역 Risk 풀에는 있어야 함.
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
                # Risk 존재성은 2-pass 처리 후 별도로 체크 (아래 Pass 2 섹션)

        # QA 참조가 전부 빈 상태인데 QA 목록이 있으면 critical (concerns 는 QA 없어도 OK 라 제외)
        if qa_keys and not any_qa_covered and (risks or sens or trades):
            critical.append({"type": "no_qa_referenced", "adsn": adsn_key, "id": "(global)",
                             "detail": "이 ADSN 의 risk/sensitivity/tradeoff 모두 qa=[] — QA 참조 없음"})
        if any_qa_covered:
            stats["qaCoveredADSNs"] += 1

    # Pass 2 — counterRisk 가 참조하는 R-id 가 프로젝트 전역 Risk 풀에 존재하는지 확인
    global_risk_ids = {iid for iid, meta in global_id_desc.items() if iid.startswith("R")}
    for adsn_key, m in mapping.items():
        for t in (m.get("tradeoffs") or []):
            tid = t.get("id", "?")
            for c in (t.get("counterRisk") or []):
                if not COUNTER_RE.match(c):
                    continue  # 형식 오류는 Pass 1 에서 처리됨
                rid = c.split(":", 1)[0]
                if rid not in global_risk_ids:
                    critical.append({"type": "counter_risk_unknown", "adsn": adsn_key, "id": tid,
                                     "detail": f"counterRisk {c} 가 참조하는 {rid} 가 프로젝트 전역 Risk 풀에 없음"})

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
