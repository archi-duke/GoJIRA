#!/usr/bin/env python3
"""validate-style-doc.py — style-define2 자가검증 (Type A).

stdin JSON:
{
  "normalizedAttributes": {"scalability":"high", ...},
  "engine_output": { ... style-score.py 출력 ... },
  "chosen": {"primary":[...], "secondary":[...], "supporting":[...]},
  "document": "<markdown 전체 문자열>"
}

stdout JSON:
{
  "pass": bool,
  "score": 0.0~1.0,
  "score_effective": number,
  "critical": [{"type":"...","detail":"..."}],
  "suggestions": [...],
  "stats": {
    "attributes": N, "attributesInDoc": M,
    "chosenStyles": K, "stylesInDoc": L,
    "mermaidBlocks": B
  }
}
"""
from __future__ import annotations

import json
import re
import sys


REQUIRED_SECTION_RE = [
    re.compile(r"#{1,3}.*1\..*추천", re.IGNORECASE),
    re.compile(r"#{1,3}.*2\..*정규화|#{1,3}.*2\..*속성", re.IGNORECASE),
    re.compile(r"#{1,3}.*3\..*선정|#{1,3}.*3\..*근거", re.IGNORECASE),
    re.compile(r"#{1,3}.*4\..*조합", re.IGNORECASE),
    re.compile(r"#{1,3}.*5\..*영향|#{1,3}.*5\..*결정", re.IGNORECASE),
]

MERMAID_BLOCK_RE = re.compile(r"```mermaid[\s\S]*?```", re.IGNORECASE)


def validate(payload: dict) -> dict:
    attrs = payload.get("normalizedAttributes") or {}
    engine = payload.get("engine_output") or {}
    chosen = payload.get("chosen") or {}
    doc = payload.get("document", "") or ""

    cat_summary = engine.get("catalog_summary", {}) or {}
    catalog_ids = set(cat_summary.get("styles") or [])
    scopes = cat_summary.get("scopes") or {}

    primary = list(chosen.get("primary") or [])
    secondary = list(chosen.get("secondary") or [])
    supporting = list(chosen.get("supporting") or [])
    all_chosen = primary + secondary + supporting

    critical: list[dict] = []
    suggestions: list[dict] = []

    # 1. 카탈로그에 없는 스타일 선택 금지
    unknown = [sid for sid in all_chosen if sid not in catalog_ids]
    for u in unknown:
        critical.append({"type": "style_not_in_catalog",
                         "detail": f"{u} 는 카탈로그(id) 에 없음"})

    # 2. 동일 scope 에 Primary 2개 이상 금지
    seen_scopes = {}
    for sid in primary:
        sc = scopes.get(sid)
        if sc in seen_scopes:
            critical.append({"type": "primary_scope_conflict",
                             "detail": f"scope '{sc}' 에 Primary {seen_scopes[sc]} / {sid} 중복"})
        else:
            seen_scopes[sc] = sid

    # 3. engine 추천과의 차이 — secondary 변경은 허용, 하지만 Primary 누락은 critical
    engine_primary = set(engine.get("primary") or [])
    if engine_primary and not engine_primary.intersection(primary):
        critical.append({"type": "primary_ignores_engine",
                         "detail": f"engine Primary={sorted(engine_primary)} 인데 chosen.primary={primary} — 엔진 결정을 뒤집음"})

    # 4. 선택 스타일이 없음
    if not primary:
        critical.append({"type": "no_primary", "detail": "Primary 가 비어있음"})

    # 5. Driver 커버리지 — normalizedAttributes 의 모든 속성명이 문서 어딘가에 등장해야
    attrs_in_doc = []
    attrs_not_in_doc = []
    for attr in attrs.keys():
        if attr in doc:
            attrs_in_doc.append(attr)
        else:
            attrs_not_in_doc.append(attr)
    if attrs and len(attrs_not_in_doc) > max(2, len(attrs) // 3):
        critical.append({"type": "driver_coverage_gap",
                         "detail": f"{len(attrs_not_in_doc)}/{len(attrs)} 속성이 문서에 언급 안 됨: {attrs_not_in_doc[:10]}"})
    elif attrs_not_in_doc:
        suggestions.append({"type": "driver_coverage_partial",
                            "detail": f"미언급 속성 {len(attrs_not_in_doc)}건: {attrs_not_in_doc}"})

    # 6. 선택된 스타일이 문서 본문에 등장해야
    styles_in_doc = []
    styles_missing = []
    names = cat_summary.get("names") or {}
    for sid in all_chosen:
        nm = names.get(sid, sid)
        if sid in doc or nm in doc:
            styles_in_doc.append(sid)
        else:
            styles_missing.append(sid)
    for sid in styles_missing:
        critical.append({"type": "chosen_style_not_in_doc",
                         "detail": f"{sid} ({names.get(sid, sid)}) 가 문서 어디에도 등장 안 함"})

    # 7. 필수 섹션 1~5 존재
    for i, regex in enumerate(REQUIRED_SECTION_RE, start=1):
        if not regex.search(doc):
            critical.append({"type": "missing_section",
                             "detail": f"섹션 {i} 헤딩 누락"})

    # 8. Mermaid 블록 존재
    mermaid_blocks = MERMAID_BLOCK_RE.findall(doc)
    if len(mermaid_blocks) < 1:
        critical.append({"type": "no_mermaid",
                         "detail": "Mermaid 다이어그램이 하나도 없음 (최소 1개 필요)"})
    elif len(mermaid_blocks) < 2:
        suggestions.append({"type": "mermaid_low",
                            "detail": f"Mermaid {len(mermaid_blocks)}개 — 구조/조합 관점 2개 이상 권장"})

    # 9. 엔진이 준 점수 순위를 뒤집었는지 (top1 미선택 시)
    scored = engine.get("scored") or []
    if scored:
        top_id = scored[0].get("id")
        if top_id and top_id not in all_chosen:
            critical.append({"type": "top_score_ignored",
                             "detail": f"점수 1위 {top_id} 가 chosen 에 없음 — Primary/Secondary/Supporting 어디에도 미포함"})

    total_checks = max(1, 3 + len(all_chosen) + len(attrs))
    score = max(0.0, 1.0 - len(critical) / total_checks)
    score_effective = round(score - 0.5 * len(critical), 4)

    return {
        "pass": len(critical) == 0 and score >= 0.8,
        "score": round(score, 4),
        "score_effective": score_effective,
        "critical": critical,
        "suggestions": suggestions,
        "stats": {
            "attributes": len(attrs),
            "attributesInDoc": len(attrs_in_doc),
            "chosenStyles": len(all_chosen),
            "stylesInDoc": len(styles_in_doc),
            "mermaidBlocks": len(mermaid_blocks),
        },
    }


def main() -> int:
    try:
        payload = json.load(sys.stdin)
    except json.JSONDecodeError as exc:
        print(json.dumps({"pass": False, "score": 0.0, "score_effective": -1.0,
                          "critical": [{"type": "invalid_json", "detail": str(exc)}],
                          "suggestions": [], "stats": {}},
                         ensure_ascii=False, indent=2))
        return 2
    result = validate(payload)
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["pass"] else 1


if __name__ == "__main__":
    sys.exit(main())
