#!/usr/bin/env python3
"""validate-satam.py — satam-analyze v3 검증 로직.

v3 (2026-04-24) 구조 변경:
- 원문 분해가 아닌 design × requirement 논리 판정 방식
- 각 risk/sensitivity 는 단일 requirement 를 대상으로 함 (requirement + reqType)
- mitigation 은 cross-ADSN 전용 (targetADSN + targetRisk)
- self-mitigated risk 는 등록 금지 (구조적으로 방지)

stdin JSON:
{
  "adsns": { "<ADSN_KEY>": { summary/objectives/decision/... 등 맥락 } },
  "requirements": {
    "<REQ_KEY>": { key, summary, reqtype: "FR"|"NFR"|"CNST", driverSelected: bool, ... }
  },
  "mapping": {
    "<ADSN_KEY>": {
      "risks": [
        {"id":"R1","requirement":"<REQ_KEY>","reqType":"FR|NFR|CNST","description":"..."}
      ],
      "sensitivities": [{"id":"S1","requirement":"...","reqType":"...","description":"..."}],
      "mitigations": [
        {"id":"M1","targetADSN":"<OTHER_ADSN>","targetRisk":"R5","description":"..."}
      ],
      "concerns": [{"id":"C1","description":"..."}]   # optional
    }
  }
}

stdout JSON:
{
  "pass": bool, "score": 0.0~1.0, "score_effective": number,
  "critical": [...], "suggestions": [...],
  "stats": {...}
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
VALID_REQ_TYPES = {"FR", "NFR", "CNST"}


def _has_text(s) -> bool:
    return bool(s and str(s).strip())


def validate(payload: dict) -> dict:
    adsns = payload.get("adsns") or {}
    requirements = payload.get("requirements") or {}
    mapping = payload.get("mapping") or {}

    critical: list[dict] = []
    suggestions: list[dict] = []
    stats = {
        "adsns": 0, "risks": 0, "sensitivities": 0, "mitigations": 0, "concerns": 0,
        "risksByReqType": {"FR": 0, "NFR": 0, "CNST": 0},
    }

    adsn_keys = set(adsns.keys())

    # 매핑에 있는데 Step 0 ADSN 목록에 없는 키
    for mk in mapping.keys():
        if mk not in adsn_keys:
            critical.append({
                "type": "unknown_adsn", "adsn": mk, "id": "(n/a)",
                "detail": f"mapping 키 {mk} 가 Step 0 ADSN 풀에 없음",
            })

    # 전역 R/S/M id 풀 — ADSN별 risks 모두 모아 전역 R id 목록 만들어 mitigation 검증에 사용
    global_risks: dict[str, str] = {}  # "R5" -> owning ADSN key
    for adsn_key, m in mapping.items():
        for r in (m.get("risks") or []):
            rid = r.get("id", "")
            if ID_RE_RISK.match(rid) and adsn_key in adsn_keys:
                global_risks[rid] = adsn_key

    for adsn_key, m in mapping.items():
        if adsn_key not in adsn_keys:
            continue  # unknown_adsn 는 이미 잡음

        stats["adsns"] += 1
        seen_ids: set = set()

        # ===== risks =====
        for r in (m.get("risks") or []):
            stats["risks"] += 1
            rid = r.get("id", "")
            req_key = r.get("requirement", "")
            req_type = r.get("reqType", "")
            desc = r.get("description", "")

            if not ID_RE_RISK.match(rid):
                critical.append({"type": "bad_id_format", "adsn": adsn_key, "id": rid,
                                 "detail": "Risk id 는 R<숫자> 형식 (예: R1)"})
            if rid in seen_ids:
                critical.append({"type": "duplicate_id", "adsn": adsn_key, "id": rid,
                                 "detail": "ADSN 내 id 중복"})
            seen_ids.add(rid)

            if req_type not in VALID_REQ_TYPES:
                critical.append({"type": "bad_reqType", "adsn": adsn_key, "id": rid,
                                 "detail": f"reqType '{req_type}' — FR/NFR/CNST 중 하나"})

            if not _has_text(req_key):
                critical.append({"type": "missing_requirement", "adsn": adsn_key, "id": rid,
                                 "detail": "risk 에 requirement 필수"})
            elif req_key not in requirements:
                critical.append({"type": "requirement_hallucination", "adsn": adsn_key, "id": rid,
                                 "detail": f"requirement {req_key} 가 프로젝트 풀에 없음"})
            else:
                req = requirements[req_key]
                actual_type = req.get("reqtype", "")
                if req_type and actual_type and req_type != actual_type:
                    critical.append({"type": "reqType_mismatch", "adsn": adsn_key, "id": rid,
                                     "detail": f"reqType={req_type} 인데 {req_key}.reqtype={actual_type}"})
                if req_type == "FR" and not req.get("driverSelected", False):
                    critical.append({"type": "fr_not_asr", "adsn": adsn_key, "id": rid,
                                     "detail": f"FR {req_key} 가 ASR(driverSelected) 아님 — SATAM 대상 아님"})
                if req_type in VALID_REQ_TYPES:
                    stats["risksByReqType"][req_type] = stats["risksByReqType"].get(req_type, 0) + 1

            if not _has_text(desc):
                critical.append({"type": "missing_description", "adsn": adsn_key, "id": rid,
                                 "detail": "risk description 필수"})

        # ===== sensitivities =====
        for s in (m.get("sensitivities") or []):
            stats["sensitivities"] += 1
            sid = s.get("id", "")
            req_key = s.get("requirement", "")
            req_type = s.get("reqType", "")
            desc = s.get("description", "")

            if not ID_RE_SENS.match(sid):
                critical.append({"type": "bad_id_format", "adsn": adsn_key, "id": sid,
                                 "detail": "Sensitivity id 는 S<숫자> 형식"})
            if sid in seen_ids:
                critical.append({"type": "duplicate_id", "adsn": adsn_key, "id": sid,
                                 "detail": "ADSN 내 id 중복"})
            seen_ids.add(sid)

            if req_type not in VALID_REQ_TYPES:
                critical.append({"type": "bad_reqType", "adsn": adsn_key, "id": sid,
                                 "detail": f"reqType '{req_type}'"})

            if not _has_text(req_key):
                critical.append({"type": "missing_requirement", "adsn": adsn_key, "id": sid,
                                 "detail": "sensitivity 에 requirement 필수"})
            elif req_key not in requirements:
                critical.append({"type": "requirement_hallucination", "adsn": adsn_key, "id": sid,
                                 "detail": f"requirement {req_key} 가 프로젝트 풀에 없음"})
            else:
                req = requirements[req_key]
                if req_type == "FR" and not req.get("driverSelected", False):
                    critical.append({"type": "fr_not_asr", "adsn": adsn_key, "id": sid,
                                     "detail": f"FR {req_key} 가 ASR 아님"})

            if not _has_text(desc):
                critical.append({"type": "missing_description", "adsn": adsn_key, "id": sid,
                                 "detail": "sensitivity description 필수"})

        # ===== mitigations (cross-ADSN 전용) =====
        for mi in (m.get("mitigations") or []):
            stats["mitigations"] += 1
            mid = mi.get("id", "")
            target_adsn = mi.get("targetADSN", "")
            target_risk = mi.get("targetRisk", "")
            desc = mi.get("description", "")

            if not ID_RE_MIT.match(mid):
                critical.append({"type": "bad_id_format", "adsn": adsn_key, "id": mid,
                                 "detail": "Mitigation id 는 M<숫자> 형식"})
            if mid in seen_ids:
                critical.append({"type": "duplicate_id", "adsn": adsn_key, "id": mid,
                                 "detail": "ADSN 내 id 중복"})
            seen_ids.add(mid)

            # SATAM 본연 위반: 자기 자신 target 금지
            if target_adsn == adsn_key:
                critical.append({
                    "type": "same_adsn_mitigation", "adsn": adsn_key, "id": mid,
                    "detail": "mitigation.targetADSN 이 자기 자신 — self-mitigated risk 는 애초에 등록 금지. Risk 와 Mitigation 둘 다 삭제 (또는 risk 를 non-risk 로 재판정)",
                })

            # target ADSN 존재 확인
            if not _has_text(target_adsn):
                critical.append({"type": "mitigation_target_unknown", "adsn": adsn_key, "id": mid,
                                 "detail": "mitigation 에 targetADSN 필수"})
            elif target_adsn not in adsn_keys:
                critical.append({"type": "mitigation_target_unknown", "adsn": adsn_key, "id": mid,
                                 "detail": f"targetADSN {target_adsn} 이 프로젝트 ADSN 풀에 없음"})

            # target Risk 존재 확인 (전역 R id 풀)
            if not _has_text(target_risk):
                critical.append({"type": "mitigation_target_unknown", "adsn": adsn_key, "id": mid,
                                 "detail": "mitigation 에 targetRisk 필수"})
            elif not ID_RE_RISK.match(target_risk):
                critical.append({"type": "bad_id_format", "adsn": adsn_key, "id": mid,
                                 "detail": f"targetRisk {target_risk} 형식 오류"})
            elif target_risk not in global_risks:
                critical.append({"type": "mitigation_target_unknown", "adsn": adsn_key, "id": mid,
                                 "detail": f"targetRisk {target_risk} 가 프로젝트 전역 R id 풀에 없음"})
            elif _has_text(target_adsn) and global_risks.get(target_risk) != target_adsn:
                # target_risk 가 가리키는 ADSN 과 명시된 targetADSN 이 다름
                critical.append({
                    "type": "mitigation_target_unknown", "adsn": adsn_key, "id": mid,
                    "detail": f"targetRisk {target_risk} 는 실제로 {global_risks[target_risk]} 에 속함 (targetADSN={target_adsn})",
                })

            if not _has_text(desc):
                critical.append({"type": "missing_description", "adsn": adsn_key, "id": mid,
                                 "detail": "mitigation description 필수"})

        # ===== concerns =====
        for c in (m.get("concerns") or []):
            stats["concerns"] += 1
            cid = c.get("id", "")
            desc = c.get("description", "")
            if not ID_RE_CONCERN.match(cid):
                critical.append({"type": "bad_id_format", "adsn": adsn_key, "id": cid,
                                 "detail": "Concerns id 는 C<숫자> 형식"})
            if cid in seen_ids:
                critical.append({"type": "duplicate_id", "adsn": adsn_key, "id": cid,
                                 "detail": "ADSN 내 id 중복"})
            seen_ids.add(cid)
            if not _has_text(desc):
                critical.append({"type": "missing_description", "adsn": adsn_key, "id": cid,
                                 "detail": "concerns description 필수"})

    total_items = stats["risks"] + stats["sensitivities"] + stats["mitigations"] + max(1, len(adsns))
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
    except json.JSONDecodeError as e:
        print(json.dumps({
            "pass": False,
            "critical": [{"type": "invalid_json", "detail": str(e)}],
            "score": 0.0,
        }, ensure_ascii=False))
        return 1

    result = validate(payload if isinstance(payload, dict) else {})
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result.get("pass") else 0  # 스크립트 exit 0 — 검증 결과는 JSON 으로


if __name__ == "__main__":
    sys.exit(main())
