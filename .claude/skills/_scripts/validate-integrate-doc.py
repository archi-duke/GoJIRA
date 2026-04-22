#!/usr/bin/env python3
"""validate-integrate-doc.py — design-integrate2 자가검증 (Type A).

stdin JSON:
{
  "adsnSubkeys": ["ADSN_001", ...],          # 본문에 반드시 등장해야
  "ddsnSubkeys": ["DDSN_001", ...],          # 본문에 반드시 등장해야
  "chosenStyles": {
     "primary":   [{"id":"MICROSERVICES","name":"Microservices"}, ...],
     "secondary": [...],
     "supporting":[...]
  },
  "viewsIncluded": ["logical","process","deployment"],  # 에이전트가 주장한 뷰
  "document": "<markdown 전체>"
}

stdout JSON:
{
  "pass": bool,
  "score": 0.0~1.0,
  "score_effective": number,
  "critical":   [{"type":"...","detail":"..."}],
  "suggestions":[...],
  "stats": {
     "views": N, "mermaidBlocks": M,
     "adsnReferenced": A, "ddsnReferenced": D,
     "stylesReferenced": S,
     "sectionsFound": K
  }
}
"""
from __future__ import annotations

import json
import re
import sys


SECTION_PATTERNS = [
    re.compile(r"#{1,3}\s*1\.\s*개요", re.IGNORECASE),
    re.compile(r"#{1,3}\s*2\.\s*Logical\s*View", re.IGNORECASE),
    re.compile(r"#{1,3}\s*3\.\s*Process\s*View", re.IGNORECASE),
    re.compile(r"#{1,3}\s*6\.\s*관점\s*간\s*일관성", re.IGNORECASE),
    re.compile(r"#{1,3}\s*7\.\s*후속\s*단계|#{1,3}\s*7\.\s*SATAM", re.IGNORECASE),
]
OPTIONAL_SECTION_PATTERNS = [
    re.compile(r"#{1,3}\s*4\.\s*Deployment\s*View", re.IGNORECASE),
    re.compile(r"#{1,3}\s*5\.\s*Development\s*View", re.IGNORECASE),
]

MERMAID_BLOCK_RE = re.compile(r"```mermaid\s*\n([\s\S]*?)```", re.IGNORECASE)
EXTERNAL_IMAGE_RE = re.compile(r"!\[[^\]]*\]\([^)]+\.(png|jpg|jpeg|gif|svg)\)",
                               re.IGNORECASE)
# 첫 비공백 라인이 허용 헤더인지 체크
VALID_MERMAID_HEADERS = (
    "graph TB", "graph TD", "graph BT", "graph LR", "graph RL",
    "flowchart TB", "flowchart TD", "flowchart BT", "flowchart LR", "flowchart RL",
    "sequenceDiagram", "stateDiagram", "stateDiagram-v2",
    "erDiagram", "gantt", "classDiagram",
)
# Logical / Process / Deployment / Development 4개 뷰 매칭 (본문 탐지)
VIEW_HEADING_RES = {
    "logical":     re.compile(r"#{1,3}\s*2\.\s*Logical\s*View", re.IGNORECASE),
    "process":     re.compile(r"#{1,3}\s*3\.\s*Process\s*View", re.IGNORECASE),
    "deployment":  re.compile(r"#{1,3}\s*4\.\s*Deployment\s*View", re.IGNORECASE),
    "development": re.compile(r"#{1,3}\s*5\.\s*Development\s*View", re.IGNORECASE),
}


def _style_list(chosen: dict, key: str) -> list[dict]:
    arr = chosen.get(key) or []
    out = []
    for item in arr:
        if isinstance(item, dict):
            out.append(item)
        elif isinstance(item, str):
            out.append({"id": item, "name": item})
    return out


def validate(payload: dict) -> dict:
    adsn = list(payload.get("adsnSubkeys") or [])
    ddsn = list(payload.get("ddsnSubkeys") or [])
    chosen = payload.get("chosenStyles") or {}
    views_claimed = [v.lower() for v in (payload.get("viewsIncluded") or [])]
    doc = payload.get("document", "") or ""

    critical: list[dict] = []
    suggestions: list[dict] = []

    # 1. ADSN / DDSN subkey 본문 참조
    adsn_ref = [k for k in adsn if k and k in doc]
    adsn_miss = [k for k in adsn if k and k not in doc]
    for k in adsn_miss:
        critical.append({"type": "upstream_not_referenced",
                         "detail": f"{k} (ADSN) 가 문서 본문에 등장 안 함 — upstream 미반영"})
    ddsn_ref = [k for k in ddsn if k and k in doc]
    ddsn_miss = [k for k in ddsn if k and k not in doc]
    for k in ddsn_miss:
        critical.append({"type": "upstream_not_referenced",
                         "detail": f"{k} (DDSN) 가 문서 본문에 등장 안 함 — upstream 미반영"})

    # 2. 선정 스타일 본문 언급 (primary/secondary/supporting 합쳐 전체)
    all_styles = (_style_list(chosen, "primary")
                  + _style_list(chosen, "secondary")
                  + _style_list(chosen, "supporting"))
    styles_ref = 0
    for s in all_styles:
        sid = (s.get("id") or "").strip()
        nm = (s.get("name") or "").strip()
        hit = (sid and sid in doc) or (nm and nm in doc)
        if hit:
            styles_ref += 1
        else:
            label = sid or nm or "(unknown)"
            critical.append({"type": "style_not_referenced",
                             "detail": f"선정 스타일 {label} 가 문서 본문에 등장 안 함"})

    # 3. 관점 수 (viewsIncluded ≥ 2) + 본문 실제 뷰 헤딩
    views_in_doc = [name for name, rx in VIEW_HEADING_RES.items() if rx.search(doc)]
    if len(views_claimed) < 2:
        critical.append({"type": "insufficient_views",
                         "detail": f"viewsIncluded={views_claimed} — 최소 2개 뷰 필요"})
    # viewsIncluded 가 본문과 불일치
    claimed_set = set(views_claimed)
    found_set = set(views_in_doc)
    missing_in_doc = claimed_set - found_set
    extra_in_doc = found_set - claimed_set
    if missing_in_doc:
        critical.append({"type": "views_mismatch",
                         "detail": f"viewsIncluded={sorted(claimed_set)} 인데 본문에 "
                                   f"{sorted(missing_in_doc)} 뷰 헤딩 없음"})
    if extra_in_doc:
        suggestions.append({"type": "views_mismatch_extra",
                            "detail": f"본문에 {sorted(extra_in_doc)} 뷰가 있는데 "
                                      f"viewsIncluded 에서 누락 — 메타데이터 갱신 권장"})

    # 4. Mermaid 블록 수 / 시작 헤더
    mermaid_blocks = MERMAID_BLOCK_RE.findall(doc)
    if len(mermaid_blocks) < 2:
        critical.append({"type": "insufficient_mermaid",
                         "detail": f"Mermaid 블록 {len(mermaid_blocks)}개 — 최소 2개 필요"})
    bad_headers = 0
    class_on_non_class_view = 0
    for i, body in enumerate(mermaid_blocks):
        first = ""
        for line in body.splitlines():
            s = line.strip()
            if s and not s.startswith("%%"):
                first = s
                break
        if not any(first.startswith(h) for h in VALID_MERMAID_HEADERS):
            critical.append({"type": "mermaid_bad_header",
                             "detail": f"Mermaid 블록 #{i+1} 시작 '{first[:40]}' — "
                                       "표준 헤더 아님 (graph/flowchart/sequenceDiagram/...)"})
            bad_headers += 1
        # classDiagram 은 Logical/Deployment/Process/Development 어디서도 컴포넌트/배포 표현 금지
        if first.startswith("classDiagram"):
            critical.append({"type": "class_diagram_for_component",
                             "detail": f"Mermaid 블록 #{i+1} 에 classDiagram 사용 — "
                                       "컴포넌트/배포/프로세스 뷰 표현에 금지"})
            class_on_non_class_view += 1

    # 5. 외부 이미지 참조 금지
    for m in EXTERNAL_IMAGE_RE.findall(doc):
        critical.append({"type": "external_image",
                         "detail": f"외부 이미지 참조 ({m}) — Mermaid 인라인만 허용"})

    # 6. 필수 섹션 1/2/3/6/7
    sections_found = 0
    for i, regex in enumerate(SECTION_PATTERNS, start=1):
        if regex.search(doc):
            sections_found += 1
        else:
            label = ["1. 개요", "2. Logical View", "3. Process View",
                    "6. 관점 간 일관성", "7. 후속 단계"][i - 1]
            critical.append({"type": "missing_section",
                             "detail": f"섹션 '{label}' 헤딩 누락"})

    # 7. 관점 일관성 섹션에 체크표 존재 (suggestion)
    if not re.search(r"\|.*체크.*\|", doc) and not re.search(
            r"\|.*체크\s*항목.*\|", doc):
        suggestions.append({"type": "no_consistency_table",
                            "detail": "§6 관점 간 일관성 체크표가 마크다운 표로 안 보임"})

    total_checks = max(1, 5 + len(adsn) + len(ddsn) + len(all_styles))
    score = max(0.0, 1.0 - len(critical) / total_checks)
    score_effective = round(score - 0.5 * len(critical), 4)

    return {
        "pass": len(critical) == 0 and score >= 0.8,
        "score": round(score, 4),
        "score_effective": score_effective,
        "critical": critical,
        "suggestions": suggestions,
        "stats": {
            "views": len(views_claimed),
            "mermaidBlocks": len(mermaid_blocks),
            "adsnReferenced": len(adsn_ref),
            "ddsnReferenced": len(ddsn_ref),
            "stylesReferenced": styles_ref,
            "sectionsFound": sections_found,
            "viewsInDoc": views_in_doc,
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
