#!/usr/bin/env python3
"""validate-style-doc.py — style-define2 v2 (통합 스타일 조합 후보).

v2 변경점: "Primary/Secondary/Supporting 각각 ADSN" → "통합 스타일 조합 후보 N개 비교".
각 후보는 (Primary + Secondary + Supporting) 세트이며, QA 가중 점수로 비교.
최종 선정 후보만 `chosen=true`. 후보 ADSN 1건씩.

stdin JSON:
{
  "driverKeys": ["<key>", ...],
  "styleCatalogIds": ["modular-monolith", "layered", ...],  # 카탈로그 id 풀 (선택)
  "qaWeights": {
    "Performance":     {"weight": 0.4, "rationale": "..."},
    "Maintainability": {"weight": 0.3, "rationale": "..."},
    "Security":        {"weight": 0.3, "rationale": "..."}
  },
  "candidates": [
    {
      "candidateId": "A",
      "docId": "ST-1",
      "name": "Modular Monolith + Layered + Event-Driven",
      "composition": {
        "primary":    [{"id":"modular-monolith","name":"...","role":"..."}],
        "secondary":  [{"id":"layered","name":"...","role":"..."}],
        "supporting": [{"id":"event-driven","name":"...","role":"..."}]
      },
      "objectives": "...",
      "pros": "...",
      "cons": "...",
      "qaScores": {"Performance": 4, "Maintainability": 4, "Security": 3},
      "weightedScore": 3.7,
      "eval": "...",
      "structuralDiagram": "```mermaid\\ngraph TB\\n...\\n```",
      "dynamicDiagram": "```mermaid\\nsequenceDiagram\\n...\\n```",
      "chosen": false,
      "decisionNote": ""
    }
  ],
  "decisionSummary": "...",
  "decisionRationale": "...",
  "tradeoffs": "",
  "relatedDrivers": ["<key>", ...],                           # 선택
  "document": {
    "index": "<architecture-style.md>",
    "candidateFiles": {"ST-1": "<...>", "ST-2": "<...>"}
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
import math
import re
import sys
from collections import Counter


CANDIDATE_REQUIRED = {
    "candidateId", "docId", "name", "composition", "objectives",
    "pros", "cons", "qaScores", "weightedScore", "eval", "chosen",
}
APPROACH_DIAGRAM_FIELDS = {"structuralDiagram", "dynamicDiagram"}

ST_ID_RE = re.compile(r"^ST-\d+$")
MERMAID_BLOCK_RE = re.compile(r"```mermaid\s*\n(.*?)```", re.DOTALL)

ROLE_KEYS = ["primary", "secondary", "supporting"]


def _has_text(v) -> bool:
    return isinstance(v, str) and bool(v.strip())


def _mermaid_has_block(text: str) -> bool:
    if not isinstance(text, str):
        return False
    return bool(MERMAID_BLOCK_RE.search(text))


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
    catalog_ids = set(payload.get("styleCatalogIds") or [])
    qa_weights = payload.get("qaWeights") or {}
    candidates = payload.get("candidates") or []
    doc = payload.get("document") or {}

    if isinstance(doc, str):
        doc = {"index": doc, "candidateFiles": {}}

    index_doc = doc.get("index", "") if isinstance(doc, dict) else ""
    cand_files = doc.get("candidateFiles", {}) if isinstance(doc, dict) else {}

    critical: list[dict] = []
    suggestions: list[dict] = []

    # ── QA 가중치 검증
    if not isinstance(qa_weights, dict) or not qa_weights:
        critical.append({"type": "bad_qaWeights", "docId": "(global)",
                         "detail": "qaWeights 필수 — {QA:{weight,rationale}}"})
    else:
        total = _weights_sum(qa_weights)
        if not math.isclose(total, 1.0, abs_tol=0.02):
            critical.append({"type": "qaWeights_sum_not_one", "docId": "(global)",
                             "detail": f"QA 가중치 합={total:.3f} (1.0 ± 0.02 필수)"})
        for qa, meta in qa_weights.items():
            if not isinstance(meta, dict):
                critical.append({"type": "bad_qaWeight_item", "docId": "(global)",
                                 "detail": f"{qa} 항목이 dict 아님"})
                continue
            if not _has_text(meta.get("rationale")):
                suggestions.append({"type": "qaWeight_rationale_empty",
                                    "docId": "(global)",
                                    "detail": f"{qa} 가중치 근거 비어있음"})

    # ── 후보 수 검증
    if len(candidates) < 2:
        critical.append({"type": "insufficient_candidates", "docId": "(global)",
                         "detail": f"후보 조합 {len(candidates)}개 — 2개 이상 필수 (권장 2~3)"})
    if len(candidates) > 3:
        suggestions.append({"type": "too_many_candidates", "docId": "(global)",
                            "detail": f"후보 {len(candidates)}개 — 정말 검토 가치 있는 2~3개로 좁히는 편이 낫습니다"})

    doc_ids = [c.get("docId", "") for c in candidates]
    dup_ids = [k for k, v in Counter(doc_ids).items() if v > 1 and k]
    for d in dup_ids:
        critical.append({"type": "duplicate_docId", "docId": d,
                         "detail": "후보 docId 중복"})

    weight_qa_set = set(qa_weights.keys())
    chosen_count = 0

    for i, c in enumerate(candidates):
        doc_id = c.get("docId", f"(candidate-{i})")

        if doc_id and not ST_ID_RE.match(doc_id):
            critical.append({"type": "bad_docId_format", "docId": doc_id,
                             "detail": "docId 는 ST-N 형식"})

        for field in CANDIDATE_REQUIRED:
            val = c.get(field)
            if val is None:
                critical.append({"type": "candidate_missing_field", "docId": doc_id,
                                 "detail": f"{field} 누락"})
                continue
            if field in {"candidateId", "name", "objectives", "pros", "cons", "eval"}:
                if not _has_text(val):
                    critical.append({"type": "candidate_empty_field", "docId": doc_id,
                                     "detail": f"{field} 비어있음"})
            elif field == "composition":
                _check_composition(val, doc_id, catalog_ids, critical, suggestions)
            elif field == "qaScores":
                _check_qa_scores(val, doc_id, weight_qa_set, critical, suggestions)
            elif field == "weightedScore":
                if not isinstance(val, (int, float)):
                    critical.append({"type": "bad_weightedScore_type", "docId": doc_id,
                                     "detail": "weightedScore 숫자 아님"})
            elif field == "chosen":
                if not isinstance(val, bool):
                    critical.append({"type": "bad_chosen_type", "docId": doc_id,
                                     "detail": "chosen 은 bool 필수"})
                elif val:
                    chosen_count += 1
                    if not _has_text(c.get("decisionNote", "")):
                        critical.append({"type": "chosen_without_note", "docId": doc_id,
                                         "detail": "chosen=true 인데 decisionNote 비어있음"})

        # Step 1.5 이후: mermaid 필수
        for df in APPROACH_DIAGRAM_FIELDS:
            diag = c.get(df, "")
            if not isinstance(diag, str) or not _mermaid_has_block(diag):
                critical.append({"type": "missing_diagram", "docId": doc_id,
                                 "detail": f"{df} 에 ```mermaid 블록 없음 (Step 1.5 미수행?)"})

    if candidates and chosen_count == 0:
        critical.append({"type": "no_chosen_candidate", "docId": "(global)",
                         "detail": "chosen=true 인 후보가 없음 — 최종 선정 미기록"})

    if not _has_text(payload.get("decisionSummary")):
        critical.append({"type": "empty_decisionSummary", "docId": "(global)",
                         "detail": "decisionSummary 비어있음"})
    if not _has_text(payload.get("decisionRationale")):
        critical.append({"type": "empty_rationale", "docId": "(global)",
                         "detail": "decisionRationale 비어있음"})

    rel_drivers = payload.get("relatedDrivers") or []
    if driver_keys and rel_drivers:
        uncovered = driver_keys - set(rel_drivers)
        for k in sorted(uncovered):
            suggestions.append({"type": "driver_not_cited", "docId": k,
                                "detail": "선정 근거에 Driver 추적 미제시"})

    _validate_index_doc(index_doc, doc_ids, critical, suggestions)
    _validate_candidate_files(candidates, cand_files, critical, suggestions)

    total_bucket = max(1, 3 + len(candidates))
    score = max(0.0, 1.0 - len(critical) / total_bucket)
    score_effective = round(score - 0.5 * len(critical), 4)
    selected = [c.get("docId") for c in candidates if c.get("chosen")]

    return {
        "pass": len(critical) == 0 and score >= 0.8,
        "score": round(score, 4),
        "score_effective": score_effective,
        "critical": critical,
        "suggestions": suggestions,
        "stats": {
            "candidates": len(candidates),
            "chosen": chosen_count,
            "selectedDocIds": selected,
            "qaWeightsTotal": round(_weights_sum(qa_weights), 4),
            "candidateFilesProvided": len(cand_files) if isinstance(cand_files, dict) else 0,
            "indexProvided": bool(_has_text(index_doc)),
        },
    }


def _check_composition(comp, doc_id, catalog_ids, critical, suggestions):
    if not isinstance(comp, dict):
        critical.append({"type": "bad_composition_type", "docId": doc_id,
                         "detail": "composition 은 dict 필수 (primary/secondary/supporting 키)"})
        return
    missing_roles = [r for r in ROLE_KEYS if r not in comp]
    if missing_roles:
        critical.append({"type": "composition_missing_role", "docId": doc_id,
                         "detail": f"composition 에 {missing_roles} 키 누락"})

    primary = comp.get("primary") or []
    if not isinstance(primary, list) or len(primary) < 1:
        critical.append({"type": "composition_no_primary", "docId": doc_id,
                         "detail": "primary 에 최소 1개 스타일 필수"})
    elif len(primary) > 1:
        suggestions.append({"type": "composition_multiple_primary", "docId": doc_id,
                            "detail": f"primary {len(primary)}개 — 원칙상 scope 당 1개 권장"})

    for role in ROLE_KEYS:
        items = comp.get(role) or []
        if not isinstance(items, list):
            critical.append({"type": "composition_bad_role_type", "docId": doc_id,
                             "detail": f"composition.{role} 배열 아님"})
            continue
        for i, s in enumerate(items):
            if not isinstance(s, dict):
                critical.append({"type": "composition_bad_item", "docId": doc_id,
                                 "detail": f"composition.{role}[{i}] dict 아님"})
                continue
            sid = s.get("id", "")
            if not _has_text(sid):
                critical.append({"type": "composition_missing_id", "docId": doc_id,
                                 "detail": f"composition.{role}[{i}].id 누락"})
            elif catalog_ids and sid not in catalog_ids:
                suggestions.append({"type": "style_id_not_in_catalog", "docId": doc_id,
                                    "detail": f"{role}[{i}].id={sid} 가 카탈로그에 없음"})
            if not _has_text(s.get("name")):
                critical.append({"type": "composition_missing_name", "docId": doc_id,
                                 "detail": f"composition.{role}[{i}].name 누락"})
            if not _has_text(s.get("role")):
                suggestions.append({"type": "composition_empty_role_desc", "docId": doc_id,
                                    "detail": f"composition.{role}[{i}].role 설명 비어있음"})


def _check_qa_scores(val, doc_id, weight_qa_set, critical, suggestions):
    if not isinstance(val, dict) or not val:
        critical.append({"type": "bad_qaScores", "docId": doc_id,
                         "detail": "qaScores 비어있음/비dict"})
        return
    for qa, score in val.items():
        if not isinstance(score, (int, float)):
            critical.append({"type": "bad_qaScore_type", "docId": doc_id,
                             "detail": f"qaScores[{qa}] 숫자 아님"})
        elif not (1 <= score <= 5):
            critical.append({"type": "qaScore_out_of_range", "docId": doc_id,
                             "detail": f"qaScores[{qa}]={score} (1~5)"})
    if weight_qa_set:
        score_qas = set(val.keys())
        missing = weight_qa_set - score_qas
        extra = score_qas - weight_qa_set
        if missing:
            critical.append({"type": "qaScore_missing_qa", "docId": doc_id,
                             "detail": f"qaScores 에 {sorted(missing)} 누락"})
        if extra:
            suggestions.append({"type": "qaScore_extra_qa", "docId": doc_id,
                                "detail": f"qaScores 에 weights 없는 QA {sorted(extra)}"})


def _validate_index_doc(index_doc, doc_ids, critical, suggestions):
    if not isinstance(index_doc, str) or not index_doc.strip():
        critical.append({"type": "index_missing", "docId": "(index)",
                         "detail": "architecture-style.md 내용 비어있음"})
        return

    section_patterns = [
        re.compile(r"#{1,3}\s*1\..*(후보|Candidate)", re.IGNORECASE),
        re.compile(r"#{1,3}\s*2\..*(QA|가중치)", re.IGNORECASE),
        re.compile(r"#{1,3}\s*3\..*(최종|Decision)", re.IGNORECASE),
    ]
    labels = ["후보 조합 목록", "QA 가중치", "최종 선정"]
    for pat, label in zip(section_patterns, labels):
        if not pat.search(index_doc):
            critical.append({"type": "index_missing_section", "docId": "(index)",
                             "detail": f"섹션 누락: {label}"})

    for did in doc_ids:
        if did and did not in index_doc:
            suggestions.append({"type": "candidate_not_in_index", "docId": did,
                                "detail": f"{did} 가 index 에 노출 안 됨"})


def _validate_candidate_files(candidates, cand_files, critical, suggestions):
    if not isinstance(cand_files, dict):
        critical.append({"type": "candidate_files_not_dict", "docId": "(candidateFiles)",
                         "detail": "document.candidateFiles 가 dict 아님"})
        return

    for c in candidates:
        did = c.get("docId", "")
        if not did:
            continue
        body = cand_files.get(did)
        if body is None:
            critical.append({"type": "candidate_file_missing", "docId": did,
                             "detail": f"{did} 파일 내용 누락 (document.candidateFiles)"})
            continue
        if not isinstance(body, str) or not body.strip():
            critical.append({"type": "candidate_file_empty", "docId": did,
                             "detail": f"{did} 파일 내용 비어있음"})
            continue

        section_patterns = [
            re.compile(r"#{1,3}\s*1\..*(조합|Composition)", re.IGNORECASE),
            re.compile(r"#{1,3}\s*2\..*(Design Objective|목표)", re.IGNORECASE),
            re.compile(r"#{1,3}\s*[3-6]\..*(Structural|정적)", re.IGNORECASE),
            re.compile(r"#{1,3}\s*[3-6]\..*(Dynamic|동적)", re.IGNORECASE),
            re.compile(r"#{1,3}\s*[4-7]\..*(QA|평가)", re.IGNORECASE),
        ]
        labels = ["조합 구성", "Design Objective", "Structural View", "Dynamic View", "QA 평가"]
        for pat, label in zip(section_patterns, labels):
            if not pat.search(body):
                critical.append({"type": "candidate_file_missing_section", "docId": did,
                                 "detail": f"{did}: '{label}' 섹션 누락"})

        mermaid_in_body = MERMAID_BLOCK_RE.findall(body)
        if len(mermaid_in_body) < 2:
            suggestions.append({"type": "candidate_file_diagram_count_low", "docId": did,
                                "detail": f"{did}: mermaid 블록 {len(mermaid_in_body)} / 예상 2 (정적+동적)"})


def main() -> int:
    try:
        payload = json.load(sys.stdin)
    except json.JSONDecodeError as exc:
        print(json.dumps({"pass": False, "score": 0.0, "score_effective": -1.0,
                          "critical": [{"type": "invalid_json", "docId": "(input)",
                                        "detail": str(exc)}],
                          "suggestions": [], "stats": {}},
                         ensure_ascii=False, indent=2))
        return 2
    result = validate(payload)
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["pass"] else 1


if __name__ == "__main__":
    sys.exit(main())
