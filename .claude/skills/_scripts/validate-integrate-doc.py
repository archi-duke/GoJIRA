#!/usr/bin/env python3
"""validate-integrate-doc.py — design-integrate2 자가검증 (Type A).

view 분할 + 결정적 통합 패턴(2026-04-25 개편) 을 검증한다. 4 view 본문이 각각
별도 문서로 들어오고, 상위 index 와 cross-view entity dictionary 도 입력에 포함된다.

stdin JSON:
{
  "views": {
    "logical":     "<logical.md 본문 마크다운>",
    "process":     "<process.md 본문>",
    "deployment":  "<deployment.md 본문>",
    "development": "<development.md 본문>"
  },
  "entityDictionary": [
    {"id":"WSS", "name":"WebSocket Server", "type":"component",
     "appearsIn":["logical","process","deployment"]},
    ...
  ],
  "indexDocument":  "<architecture-integrated.md 본문 (index, 짧음)>",

  "adsnSubkeys":  ["ADSN_001", ...],          # 4 view 통틀어 어딘가에 등장해야
  "ddsnSubkeys":  ["DDSN_001", ...],          # 4 view 통틀어 어딘가에 등장해야
  "chosenStyles": {
     "primary":   [{"id":"MICROSERVICES","name":"Microservices"}, ...],
     "secondary": [...],
     "supporting":[...]
  },
  "viewsIncluded": ["logical","process","deployment","development"]
}

stdout JSON:
{
  "pass": bool,
  "score": 0.0~1.0,
  "score_effective": number,
  "critical":   [{"type":"...","view":"...","detail":"..."}],
  "suggestions":[...],
  "stats": {
     "views": N,
     "viewsProvided": [...],
     "plantumlBlocks": M,           # 4 view + index 합계
     "plantumlBadPair": X,
     "adsnReferenced": A,
     "ddsnReferenced": D,
     "stylesReferenced": S,
     "entityCount": E,
     "entityInconsistent": I,
     "indexSectionsFound": K
  }
}
"""
from __future__ import annotations

import json
import re
import sys

import plantuml_util as pu


REQUIRED_VIEWS = ["logical", "process", "deployment", "development"]

# 각 view 의 추천 다이어그램 종류 (detect_kind 결과 기준).
# 위반 시 critical 아닌 suggestion (스타일에 따라 예외 있을 수 있음).
RECOMMENDED_KIND = {
    "logical":     {"component"},
    "process":     {"sequence", "activity"},
    "deployment":  {"deployment"},
    "development": {"package", "component"},
}

# 모든 view 에서 금지: class 다이어그램 (구조/배포/프로세스/개발 뷰 표현에 부적합)
FORBIDDEN_KIND = {"class"}

EXTERNAL_IMAGE_RE = re.compile(r"!\[[^\]]*\]\([^)]+\.(png|jpg|jpeg|gif|svg)\)",
                               re.IGNORECASE)

# 각 view 본문 §1 개요 / §3 다이어그램 헤딩 (validation 단순화 — 1~3 만 강제)
VIEW_SECTION_RES = [
    re.compile(r"#{1,3}\s*1\.\s*개요", re.IGNORECASE),
    re.compile(r"#{1,3}\s*3\.\s*다이어그램", re.IGNORECASE),
]

# Index 의 §1 View 구성 / §2 Cross-View Entity Dictionary 헤딩
INDEX_SECTION_RES = [
    re.compile(r"#{1,3}\s*1\.\s*View\s*구성", re.IGNORECASE),
    re.compile(r"#{1,3}\s*2\.\s*Cross[- ]?View\s*Entity\s*Dictionary",
               re.IGNORECASE),
]


def _style_list(chosen: dict, key: str) -> list[dict]:
    arr = chosen.get(key) or []
    out = []
    for item in arr:
        if isinstance(item, dict):
            out.append(item)
        elif isinstance(item, str):
            out.append({"id": item, "name": item})
    return out


def _check_view(view_name: str, body: str) -> tuple[list[dict], list[dict],
                                                    int, int]:
    """단일 view 본문 검증. (critical, suggestions, plantuml_count, bad_pair)."""
    critical: list[dict] = []
    suggestions: list[dict] = []

    if not body or not body.strip():
        critical.append({"type": "view_missing", "view": view_name,
                         "detail": f"{view_name} view 본문 비어있음"})
        return critical, suggestions, 0, 0

    # 1. 필수 섹션 (§1 개요, §3 다이어그램)
    for i, regex in enumerate(VIEW_SECTION_RES):
        if not regex.search(body):
            label = ["1. 개요", "3. 다이어그램"][i]
            critical.append({"type": "view_missing_section", "view": view_name,
                             "detail": f"{view_name} view §{label} 헤딩 누락"})

    # 2. PlantUML 블록 1+ 개
    blocks = pu.find_blocks(body)
    if len(blocks) == 0:
        critical.append({"type": "view_no_plantuml", "view": view_name,
                         "detail": f"{view_name} view 에 PlantUML 블록 없음"})

    # 3. 각 블록의 @startuml/@enduml 쌍 + class 다이어그램 금지 + 추천 종류 안내
    bad_pair = 0
    for i, blk in enumerate(blocks):
        if not pu.has_uml_pair(blk):
            critical.append({"type": "plantuml_missing_pair", "view": view_name,
                             "detail": f"{view_name} view 블록 #{i+1} 에 "
                                       "@startuml/@enduml 쌍 누락"})
            bad_pair += 1
            continue
        kind = pu.detect_kind(blk)
        if kind in FORBIDDEN_KIND:
            critical.append({"type": "class_diagram_for_view", "view": view_name,
                             "detail": f"{view_name} view 블록 #{i+1} 에 "
                                       "class 다이어그램 사용 — 금지"})
        elif kind not in RECOMMENDED_KIND.get(view_name, set()) and kind != "unknown":
            suggestions.append({"type": "view_kind_mismatch", "view": view_name,
                                "detail": f"{view_name} view 블록 #{i+1} 종류="
                                f"{kind} (추천: "
                                f"{','.join(sorted(RECOMMENDED_KIND[view_name]))})"})

    # 4. 외부 이미지 금지
    for m in EXTERNAL_IMAGE_RE.findall(body):
        critical.append({"type": "external_image", "view": view_name,
                         "detail": f"{view_name} view 외부 이미지 ({m}) — "
                                   "PlantUML 인라인만 허용"})

    return critical, suggestions, len(blocks), bad_pair


def _check_entity_consistency(entities: list[dict]) -> tuple[list[dict],
                                                              list[dict], int]:
    """entity_dictionary 의 view 간 이름 일관성 검증."""
    critical: list[dict] = []
    suggestions: list[dict] = []

    by_id: dict[str, list[dict]] = {}
    for e in entities:
        if not isinstance(e, dict):
            continue
        eid = (e.get("id") or "").strip()
        if not eid:
            critical.append({"type": "entity_missing_id", "view": "",
                             "detail": f"entity 항목에 id 없음: {e}"})
            continue
        by_id.setdefault(eid, []).append(e)

    inconsistent = 0
    for eid, group in by_id.items():
        names = {(g.get("name") or "").strip() for g in group}
        names.discard("")
        if len(names) > 1:
            critical.append({"type": "entity_inconsistent", "view": "",
                             "detail": f"entity {eid} 가 여러 이름으로 등장: "
                                       f"{sorted(names)}"})
            inconsistent += 1

    return critical, suggestions, inconsistent


def validate(payload: dict) -> dict:
    views_in = payload.get("views") or {}
    entities = payload.get("entityDictionary") or []
    index_doc = payload.get("indexDocument", "") or ""
    adsn = list(payload.get("adsnSubkeys") or [])
    ddsn = list(payload.get("ddsnSubkeys") or [])
    chosen = payload.get("chosenStyles") or {}
    views_claimed = [v.lower() for v in (payload.get("viewsIncluded") or [])]

    critical: list[dict] = []
    suggestions: list[dict] = []

    # 1. 4 view 모두 존재 확인
    views_provided = [k for k in REQUIRED_VIEWS
                      if (views_in.get(k) or "").strip()]
    for k in REQUIRED_VIEWS:
        if k not in views_provided:
            critical.append({"type": "view_missing", "view": k,
                             "detail": f"{k} view 가 누락됨 (4 view 모두 필요)"})

    # 2. 각 view 본문 검증
    plantuml_total = 0
    bad_pair_total = 0
    for k in REQUIRED_VIEWS:
        body = views_in.get(k) or ""
        if not body.strip():
            continue  # 위에서 view_missing 으로 잡힘
        v_crit, v_sugg, n_blocks, n_bad = _check_view(k, body)
        critical.extend(v_crit)
        suggestions.extend(v_sugg)
        plantuml_total += n_blocks
        bad_pair_total += n_bad

    # 3. ADSN/DDSN subkey 가 4 view 통틀어 어딘가에 등장 확인 (index 도 합산)
    combined_doc = "\n".join(views_in.get(k) or "" for k in REQUIRED_VIEWS)
    combined_doc += "\n" + index_doc

    adsn_ref = [k for k in adsn if k and k in combined_doc]
    adsn_miss = [k for k in adsn if k and k not in combined_doc]
    for k in adsn_miss:
        critical.append({"type": "upstream_not_referenced", "view": "",
                         "detail": f"{k} (ADSN) 가 어떤 view 에도 등장 안 함 — "
                                   "upstream 미반영"})
    ddsn_ref = [k for k in ddsn if k and k in combined_doc]
    ddsn_miss = [k for k in ddsn if k and k not in combined_doc]
    for k in ddsn_miss:
        critical.append({"type": "upstream_not_referenced", "view": "",
                         "detail": f"{k} (DDSN) 가 어떤 view 에도 등장 안 함 — "
                                   "upstream 미반영"})

    # 4. 선정 스타일 본문 언급 (4 view + index 통합 본문 기준)
    all_styles = (_style_list(chosen, "primary")
                  + _style_list(chosen, "secondary")
                  + _style_list(chosen, "supporting"))
    styles_ref = 0
    for s in all_styles:
        sid = (s.get("id") or "").strip()
        nm = (s.get("name") or "").strip()
        hit = (sid and sid in combined_doc) or (nm and nm in combined_doc)
        if hit:
            styles_ref += 1
        else:
            label = sid or nm or "(unknown)"
            critical.append({"type": "style_not_referenced", "view": "",
                             "detail": f"선정 스타일 {label} 가 어떤 view/index "
                                       "에도 등장 안 함"})

    # 5. viewsIncluded ≥ 2 + claimed 와 provided 일치
    if len(views_claimed) < 2:
        critical.append({"type": "insufficient_views", "view": "",
                         "detail": f"viewsIncluded={views_claimed} — "
                                   "최소 2개 뷰 필요"})
    claimed_set = set(views_claimed)
    provided_set = set(views_provided)
    missing_in_doc = claimed_set - provided_set
    if missing_in_doc:
        critical.append({"type": "views_mismatch", "view": "",
                         "detail": f"viewsIncluded={sorted(claimed_set)} 인데 "
                                   f"본문 입력 누락: {sorted(missing_in_doc)}"})

    # 6. entity_dictionary 검증
    if not entities:
        critical.append({"type": "dictionary_missing", "view": "",
                         "detail": "entityDictionary 비어있음 (Step 1 view 들에서 "
                                   "누적 산출 누락)"})
        ent_critical, ent_sugg, inconsistent = [], [], 0
    else:
        ent_critical, ent_sugg, inconsistent = _check_entity_consistency(entities)
        critical.extend(ent_critical)
        suggestions.extend(ent_sugg)

    # 7. Index 문서 — 짧은 index 검증 (§1, §2 헤딩만 강제)
    if not index_doc.strip():
        critical.append({"type": "index_missing", "view": "",
                         "detail": "indexDocument 비어있음 (architecture-"
                                   "integrated.md 결정적 통합 누락)"})
        index_sections_found = 0
    else:
        index_sections_found = 0
        for i, regex in enumerate(INDEX_SECTION_RES):
            if regex.search(index_doc):
                index_sections_found += 1
            else:
                label = ["1. View 구성", "2. Cross-View Entity Dictionary"][i]
                critical.append({"type": "index_missing_section", "view": "",
                                 "detail": f"index §{label} 헤딩 누락"})
        # index 의 view 링크 4개 모두 존재
        for k in REQUIRED_VIEWS:
            link_pat = re.compile(rf"\(architecture-integrated/{k}\.md\)",
                                  re.IGNORECASE)
            if not link_pat.search(index_doc):
                suggestions.append({"type": "index_link_missing", "view": k,
                                    "detail": f"index 에 {k}.md 링크 없음"})

    # 점수 계산
    total_checks = max(1, len(REQUIRED_VIEWS) * 3   # view 별 3 체크
                       + len(adsn) + len(ddsn) + len(all_styles)
                       + 4)  # entity, index 등
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
            "viewsProvided": views_provided,
            "plantumlBlocks": plantuml_total,
            "plantumlBadPair": bad_pair_total,
            "adsnReferenced": len(adsn_ref),
            "ddsnReferenced": len(ddsn_ref),
            "stylesReferenced": styles_ref,
            "entityCount": len(entities),
            "entityInconsistent": inconsistent,
            "indexSectionsFound": index_sections_found,
        },
    }


def main() -> int:
    try:
        payload = json.load(sys.stdin)
    except json.JSONDecodeError as exc:
        print(json.dumps({"pass": False, "score": 0.0, "score_effective": -1.0,
                          "critical": [{"type": "invalid_json",
                                        "detail": str(exc)}],
                          "suggestions": [], "stats": {}},
                         ensure_ascii=False, indent=2))
        return 2
    result = validate(payload)
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["pass"] else 1


if __name__ == "__main__":
    sys.exit(main())
