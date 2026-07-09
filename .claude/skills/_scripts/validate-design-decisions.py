#!/usr/bin/env python3
"""validate-design-decisions.py — design-decide Type A 검증 (v2 스키마).

v2 변경점: 1 Design Point = N+1 ADSN (후보 N + 선정 ADSN 가능). 후보별 QA 점수 /
구조·동적 diagram / 가중치 기반 선정 근거. document 는 {index, dpFiles} 로 분리.

stdin JSON:
{
  "driverKeys": ["<key>", ...],
  "requirementKeys": ["<key>", ...],
  "designs": [
    {
      "docId": "DP-1",
      "designPoint": "데이터 일관성 전략",
      "designLevel": "Architectural",
      "objectives": "...",                       // Design Objective (ADSN.objectives 매핑)
      "dmodel": "",                              // optional
      "qaWeights": {
        "Performance": {"weight": 0.5, "rationale": "..."},
        "Security":    {"weight": 0.3, "rationale": "..."}
      },
      "approaches": [
        {
          "candidateId": "A",
          "name": "...",
          "pros": "...",
          "cons": "...",
          "qaScores": {"Performance": 4, "Security": 3},
          "weightedScore": 3.7,
          "eval": "...",
          "structuralDiagram": "```plantuml\\n@startuml\\n...\\n@enduml\\n```",
          "dynamicDiagram": "```plantuml\\n@startuml\\n...\\n@enduml\\n```",
          "chosen": false,
          "decisionNote": ""                     // chosen=true 면 선정 근거 문구
        },
        ...
      ],
      "decisionSummary": "Candidate B 단독 / Mix of A+C 등",
      "decisionRationale": "Driver 역추적 + 가중합 근거",
      "tradeoffs": "",                           // optional, 기각 대안 트레이드오프
      "relatedRequirements": ["<key>", ...],
      "mode": "create",
      "existingKey": ""
    }
  ],
  "document": {
    "index": "<design-decisions.md 마크다운>",
    "dpFiles": {
      "DP-1": "<DP-1-slug.md 마크다운>",
      ...
    }
  }
}

stdout JSON:
{
  "pass": bool, "score":..., "score_effective":...,
  "critical":[{"type":"...","docId":"...","detail":"..."}],
  "suggestions":[...],
  "stats": {...}
}
"""
from __future__ import annotations

import json
import math
import re
import sys
from collections import Counter


# ── 필수 필드 정의

DP_REQUIRED = {
    "docId", "designPoint", "designLevel", "objectives",
    "qaWeights", "approaches", "decisionSummary",
    "decisionRationale", "relatedRequirements", "mode",
}
APPROACH_REQUIRED = {
    "candidateId", "name", "pros", "cons", "qaScores",
    "weightedScore", "eval", "chosen",
}
APPROACH_DIAGRAM_FIELDS = {"structuralDiagram", "dynamicDiagram"}

VALID_MODES = {"create", "reuse", "modify", "delete_mark"}
VALID_LEVELS = {"Architectural", "Detailed"}

import plantuml_util as pu


DP_ID_RE = re.compile(r"^DP-\d+$")


def _has_text(v) -> bool:
    return isinstance(v, str) and bool(v.strip())


def _weights_sum(weights_map: dict) -> float:
    total = 0.0
    for v in weights_map.values():
        if isinstance(v, dict):
            w = v.get("weight")
        else:
            w = v
        try:
            total += float(w)
        except (TypeError, ValueError):
            pass
    return total


def validate(payload: dict) -> dict:
    driver_keys = set(payload.get("driverKeys") or [])
    req_keys = set(payload.get("requirementKeys") or [])
    designs = payload.get("designs") or []
    doc = payload.get("document") or {}

    # document 구조가 dict 가 아니면 {index: <string>} 으로 수용 (하위호환)
    if isinstance(doc, str):
        doc = {"index": doc, "dpFiles": {}}

    index_doc = doc.get("index", "") if isinstance(doc, dict) else ""
    dp_files = doc.get("dpFiles", {}) if isinstance(doc, dict) else {}

    critical: list[dict] = []
    suggestions: list[dict] = []

    doc_ids = [d.get("docId", "") for d in designs]
    dup_ids = [k for k, v in Counter(doc_ids).items() if v > 1 and k]
    for d in dup_ids:
        critical.append({"type": "duplicate_docId", "docId": d,
                         "detail": "DP docId 중복"})

    driver_hit: set[str] = set()
    req_hit: set[str] = set()
    approach_counts: list[int] = []
    candidate_chosen_counts: list[int] = []
    adsn_level = ddsn_level = 0

    for d in designs:
        doc_id = d.get("docId", "<missing>")

        # docId 포맷: DP-N
        if doc_id and not DP_ID_RE.match(doc_id):
            critical.append({"type": "bad_docId_format", "docId": doc_id,
                             "detail": "DP docId 는 DP-N 형식"})

        for field in DP_REQUIRED:
            val = d.get(field)
            if val is None:
                critical.append({"type": "missing_field", "docId": doc_id,
                                 "detail": f"{field} 누락"})
                continue
            if field == "objectives":
                if not _has_text(val):
                    critical.append({"type": "empty_objectives", "docId": doc_id,
                                     "detail": "Design Objective 비어있음"})
            elif field == "decisionRationale":
                if not _has_text(val):
                    critical.append({"type": "empty_rationale", "docId": doc_id,
                                     "detail": "decisionRationale 비어있음 — Driver 추적 불가"})
            elif field == "decisionSummary":
                if not _has_text(val):
                    critical.append({"type": "empty_decisionSummary", "docId": doc_id,
                                     "detail": "최종 선정 요약이 비어있음"})
            elif field == "qaWeights":
                if not isinstance(val, dict) or not val:
                    critical.append({"type": "bad_qaWeights", "docId": doc_id,
                                     "detail": "qaWeights 는 {QA:{weight,rationale}} 형식 dict 필수"})
                else:
                    total = _weights_sum(val)
                    if not math.isclose(total, 1.0, abs_tol=0.02):
                        critical.append({"type": "qaWeights_sum_not_one",
                                         "docId": doc_id,
                                         "detail": f"QA 가중치 합={total:.3f} (1.0 ± 0.02 필수)"})
                    for qa, meta in val.items():
                        if not isinstance(meta, dict):
                            critical.append({"type": "bad_qaWeight_item",
                                             "docId": doc_id,
                                             "detail": f"{qa} 항목이 dict 아님"})
                            continue
                        if not _has_text(meta.get("rationale")):
                            suggestions.append({"type": "qaWeight_rationale_empty",
                                                "docId": doc_id,
                                                "detail": f"{qa} 가중치 근거 비어있음"})
            elif field == "approaches":
                if not isinstance(val, list):
                    critical.append({"type": "bad_approaches_type", "docId": doc_id,
                                     "detail": "approaches 배열 아님"})
                    approach_counts.append(0)
                    candidate_chosen_counts.append(0)
                    continue
                approach_counts.append(len(val))
                if len(val) < 2:
                    critical.append({"type": "insufficient_approaches", "docId": doc_id,
                                     "detail": f"approaches={len(val)} — 2개 이상 필수"})
                chosen_count = _validate_approaches(
                    val, d.get("qaWeights") or {}, doc_id, critical, suggestions
                )
                candidate_chosen_counts.append(chosen_count)
                if chosen_count == 0:
                    critical.append({"type": "no_chosen_candidate", "docId": doc_id,
                                     "detail": "chosen=true 인 후보가 없음 — 최종 선정 미기록"})
            elif field == "relatedRequirements":
                if not isinstance(val, list):
                    critical.append({"type": "bad_relatedRequirements_type",
                                     "docId": doc_id,
                                     "detail": "배열 아님"})
                elif not val:
                    critical.append({"type": "no_requirement_link", "docId": doc_id,
                                     "detail": "Driver/Requirement 연결 없음"})
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
            elif field == "designLevel":
                if val not in VALID_LEVELS:
                    critical.append({"type": "bad_designLevel", "docId": doc_id,
                                     "detail": f"designLevel={val!r} (허용: {sorted(VALID_LEVELS)})"})
                elif val == "Architectural":
                    pass  # 아래 집계
                # 집계
            elif field == "mode":
                if val not in VALID_MODES:
                    critical.append({"type": "bad_mode", "docId": doc_id,
                                     "detail": f"mode={val!r}"})

        lvl = d.get("designLevel")
        if lvl == "Architectural":
            adsn_level += 1
        elif lvl == "Detailed":
            ddsn_level += 1

    # Driver 커버리지
    if driver_keys:
        uncovered = driver_keys - driver_hit
        for k in sorted(uncovered):
            critical.append({"type": "driver_uncovered", "docId": k,
                             "detail": "어느 DP 에도 매핑 안 됨"})

    # 문서 체크
    _validate_index_doc(index_doc, doc_ids, critical, suggestions)
    _validate_dp_files(designs, dp_files, critical, suggestions)

    total_bucket = max(1, 3 + len(designs) + len(driver_keys))
    score = max(0.0, 1.0 - len(critical) / total_bucket)
    score_effective = round(score - 0.5 * len(critical), 4)
    approach_avg = (sum(approach_counts) / len(approach_counts)) if approach_counts else 0

    return {
        "pass": len(critical) == 0 and score >= 0.8,
        "score": round(score, 4),
        "score_effective": score_effective,
        "critical": critical,
        "suggestions": suggestions,
        "stats": {
            "designs": len(designs),
            "architectural": adsn_level,
            "detailed": ddsn_level,
            "driverCoverage": f"{len(driver_hit)}/{len(driver_keys)}",
            "approachAvg": round(approach_avg, 2),
            "requirementCovered": len(req_hit),
            "totalCandidates": sum(approach_counts),
            "chosenCandidates": sum(candidate_chosen_counts),
            "dpFilesProvided": len(dp_files) if isinstance(dp_files, dict) else 0,
        },
    }


def _validate_approaches(
    approaches: list,
    qa_weights: dict,
    doc_id: str,
    critical: list,
    suggestions: list,
) -> int:
    """approaches 리스트 내부 검증. 반환값: chosen=true 인 후보 수."""
    chosen_count = 0
    ids_seen: list[str] = []
    weight_qas = set(qa_weights.keys()) if isinstance(qa_weights, dict) else set()

    for i, a in enumerate(approaches):
        if not isinstance(a, dict):
            critical.append({"type": "bad_approach_item", "docId": doc_id,
                             "detail": f"approaches[{i}] 가 dict 아님"})
            continue

        cid = a.get("candidateId", "")
        ids_seen.append(cid)

        for field in APPROACH_REQUIRED:
            val = a.get(field)
            if val is None:
                critical.append({"type": "approach_missing_field", "docId": doc_id,
                                 "detail": f"approaches[{cid or i}].{field} 누락"})
                continue
            if field in {"name", "pros", "cons", "eval", "candidateId"}:
                if not _has_text(val):
                    critical.append({"type": "approach_empty_field", "docId": doc_id,
                                     "detail": f"approaches[{cid or i}].{field} 비어있음"})
            elif field == "qaScores":
                if not isinstance(val, dict) or not val:
                    critical.append({"type": "bad_qaScores", "docId": doc_id,
                                     "detail": f"approaches[{cid or i}].qaScores 비어있음/비dict"})
                    continue
                for qa, score in val.items():
                    if not isinstance(score, (int, float)):
                        critical.append({"type": "bad_qaScore_type", "docId": doc_id,
                                         "detail": f"{cid}.qaScores[{qa}] 숫자 아님"})
                    elif not (1 <= score <= 5):
                        critical.append({"type": "qaScore_out_of_range",
                                         "docId": doc_id,
                                         "detail": f"{cid}.qaScores[{qa}]={score} (1~5)"})
                if weight_qas:
                    score_qas = set(val.keys())
                    missing = weight_qas - score_qas
                    extra = score_qas - weight_qas
                    if missing:
                        critical.append({"type": "qaScore_missing_qa",
                                         "docId": doc_id,
                                         "detail": f"{cid}.qaScores 에 {sorted(missing)} 누락"})
                    if extra:
                        suggestions.append({"type": "qaScore_extra_qa",
                                            "docId": doc_id,
                                            "detail": f"{cid}.qaScores 에 weights 없는 QA {sorted(extra)}"})
            elif field == "weightedScore":
                if not isinstance(val, (int, float)):
                    critical.append({"type": "bad_weightedScore_type",
                                     "docId": doc_id,
                                     "detail": f"{cid}.weightedScore 숫자 아님"})
            elif field == "chosen":
                if not isinstance(val, bool):
                    critical.append({"type": "bad_chosen_type", "docId": doc_id,
                                     "detail": f"{cid}.chosen 은 bool 필수"})
                elif val:
                    chosen_count += 1
                    note = a.get("decisionNote", "")
                    if not _has_text(note):
                        critical.append({"type": "chosen_without_note",
                                         "docId": doc_id,
                                         "detail": f"{cid} chosen=true 인데 decisionNote 비어있음"})

        # Diagram (Step 1.5 이후 필수: plantuml + @startuml/@enduml 쌍)
        for df in APPROACH_DIAGRAM_FIELDS:
            diag = a.get(df, "")
            if not isinstance(diag, str) or not pu.has_block(diag):
                critical.append({"type": "missing_diagram", "docId": doc_id,
                                 "detail": f"approaches[{cid or i}].{df} 에 ```plantuml 블록 없음 (Step 1.5 미수행?)"})
                continue
            blocks = pu.find_blocks(diag)
            if any(not pu.has_uml_pair(b) for b in blocks):
                critical.append({"type": "plantuml_missing_pair", "docId": doc_id,
                                 "detail": f"approaches[{cid or i}].{df} plantuml 블록에 @startuml/@enduml 쌍 누락"})

    # candidateId 중복 체크
    dup_cids = [k for k, v in Counter(ids_seen).items() if v > 1 and k]
    for cid in dup_cids:
        critical.append({"type": "duplicate_candidateId", "docId": doc_id,
                         "detail": f"candidateId={cid} 중복"})

    return chosen_count


def _validate_index_doc(
    index_doc: str, dp_doc_ids: list, critical: list, suggestions: list
) -> None:
    """상위 index 파일(design-decisions.md) 섹션 체크."""
    if not isinstance(index_doc, str) or not index_doc.strip():
        critical.append({"type": "index_missing", "docId": "(index)",
                         "detail": "design-decisions.md 내용 비어있음"})
        return

    section_patterns = [
        re.compile(r"#{1,3}.*1\..*Design Point", re.IGNORECASE),
        re.compile(r"#{1,3}.*2\..*Driver.*(매트릭스|Matrix)", re.IGNORECASE),
        re.compile(r"#{1,3}.*리스크|#{1,3}.*Risk", re.IGNORECASE),
    ]
    labels = ["Design Point 목록", "Driver 추적성", "리스크 요약"]
    for pat, label in zip(section_patterns, labels):
        if not pat.search(index_doc):
            critical.append({"type": "index_missing_section", "docId": "(index)",
                             "detail": f"섹션 누락: {label}"})

    # DP 키가 index 에 등장하는지
    for did in dp_doc_ids:
        if did and did not in index_doc:
            suggestions.append({"type": "dp_not_in_index", "docId": did,
                                "detail": f"{did} 가 index 에 노출 안 됨"})


def _validate_dp_files(
    designs: list, dp_files: dict, critical: list, suggestions: list
) -> None:
    """각 DP 파일 내용 체크."""
    if not isinstance(dp_files, dict):
        critical.append({"type": "dp_files_not_dict", "docId": "(dpFiles)",
                         "detail": "document.dpFiles 가 dict 아님"})
        return

    for d in designs:
        did = d.get("docId", "")
        if not did:
            continue
        body = dp_files.get(did)
        if body is None:
            critical.append({"type": "dp_file_missing", "docId": did,
                             "detail": f"{did} 파일 내용 누락 (document.dpFiles)"})
            continue
        if not isinstance(body, str) or not body.strip():
            critical.append({"type": "dp_file_empty", "docId": did,
                             "detail": f"{did} 파일 내용 비어있음"})
            continue
        # DP 파일 필수 섹션
        section_patterns = [
            re.compile(r"#{1,3}\s*1\..*Design Objective", re.IGNORECASE),
            re.compile(r"#{1,3}\s*2\..*(QA|가중치)", re.IGNORECASE),
            re.compile(r"#{1,3}\s*3\..*(후보|Candidate)", re.IGNORECASE),
            re.compile(r"#{1,3}\s*4\..*(최종 선정|Decision)", re.IGNORECASE),
        ]
        labels = ["Design Objective", "QA 가중치", "후보 대안", "최종 선정"]
        for pat, label in zip(section_patterns, labels):
            if not pat.search(body):
                critical.append({"type": "dp_file_missing_section", "docId": did,
                                 "detail": f"{did}: '{label}' 섹션 누락"})

        # 각 후보의 plantuml 블록이 문서에도 등장해야 함 (draft 와 이중 보장)
        approaches = d.get("approaches") or []
        plantuml_in_body = pu.find_blocks(body)
        expected_min = 2 * len(approaches)  # candidate 당 structural+dynamic
        if len(plantuml_in_body) < expected_min:
            suggestions.append({"type": "dp_file_diagram_count_low",
                                "docId": did,
                                "detail": f"{did}: plantuml 블록 {len(plantuml_in_body)} / 예상 {expected_min} (후보당 정적+동적)"})


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
