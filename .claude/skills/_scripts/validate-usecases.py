#!/usr/bin/env python3
"""validate-usecases.py — usecase-analyze2 의 Type A 검증.

stdin JSON:
{
  "vosKeys": ["<key>", ...],                 # Step 0 수집
  "existingFeatureKeys": ["<key>", ...],     # 기존 Feature (relink 시)
  "userStories": [
    {"id":"US-1", "actor":"...", "category":"Happy|Alternative|Exception",
     "story":"...", "vosKey":"<key>", "ucId":"UC_001"},
    ...
  ],
  "useCases": [
    {"id":"UC_001","name":"...","actor":"...","includes":[],"extends":[]},
    ...
  ],
  "features": [
    {"docId":"UC-1","summary":"...","actor":"...","basicflow":"...",
     "precondition":"","alternativeflow":"","exceptionflow":"","postcondition":"",
     "relatedVOS":["<key>"],"mode":"create","existingKey":""},
    ...
  ],
  "document": "<markdown>"                    # 작성한 use-cases.md 본문
}

stdout JSON:
{
  "pass": bool,
  "score": 0.0~1.0,
  "score_effective": number,
  "critical": [{"type":"...","docId":"...","detail":"..."}],
  "suggestions": [...],
  "stats": {"vosKeys":N,"storyVosCovered":V1,"featureVosCovered":V2,
            "features":F,"sequenceDiagramCount":SD,"useCaseDiagramFound":bool,
            "contextDiagramFound":bool,"plantumlBlocks":M,"plantumlBadPair":X}
}
"""
from __future__ import annotations

import json
import re
import sys
from collections import Counter, defaultdict


import plantuml_util as pu


FEATURE_REQUIRED = {"summary", "actor", "basicflow", "docId", "relatedVOS"}

EXT_IMAGE_RE = re.compile(r"!\[[^\]]*\]\([^\)]+\.(png|jpg|jpeg|gif|svg)\)", re.IGNORECASE)
# PlantUML sequence: participant 선언에 공백 있는 이름을 "..." 또는 as 없이 쓰면 실수
# 예: `participant My Service` (잘못) / `participant "My Service"` 또는 `participant Svc as "My Service"` (정상)
PARTICIPANT_BAD_NAME_RE = re.compile(
    r"^\s*(?:participant|boundary|control|entity|queue|collections)\s+"
    r"(?![\"'])[^\s\"']+\s+(?!as\b)[^\s\"'<][^\s\"']*",
    re.MULTILINE | re.IGNORECASE,
)


def validate(payload: dict) -> dict:
    vos_keys = set(payload.get("vosKeys") or [])
    stories = payload.get("userStories") or []
    features = payload.get("features") or []
    doc = payload.get("document", "") or ""

    critical: list[dict] = []
    suggestions: list[dict] = []

    # ── User Story 기준 VOS 커버리지
    story_vos_covered = set()
    for s in stories:
        vk = s.get("vosKey")
        if vk:
            story_vos_covered.add(vk)
    for vk in sorted(vos_keys - story_vos_covered):
        critical.append({"type": "vos_no_story", "docId": vk,
                         "detail": "VOS 가 어느 User Story 로도 파생되지 않음"})

    # ── Feature 기준 VOS 커버리지
    feature_vos_covered = set()
    for f in features:
        for vk in (f.get("relatedVOS") or []):
            if vk in vos_keys:
                feature_vos_covered.add(vk)
    for vk in sorted(vos_keys - feature_vos_covered):
        critical.append({"type": "vos_no_feature", "docId": vk,
                         "detail": "VOS 가 어느 Feature 의 relatedVOS 에도 포함 안 됨 (NFR 면제 없음)"})

    # ── Feature 검증
    doc_ids = [f.get("docId", "") for f in features]
    dup_ids = [k for k, v in Counter(doc_ids).items() if v > 1 and k]
    for d in dup_ids:
        critical.append({"type": "duplicate_docId", "docId": d,
                         "detail": "Feature docId 중복"})

    for f in features:
        doc_id = f.get("docId", "<missing>")
        for field in FEATURE_REQUIRED:
            val = f.get(field)
            if val is None:
                critical.append({"type": "missing_field", "docId": doc_id,
                                 "detail": f"{field} 누락"})
                continue
            if field == "relatedVOS":
                if not isinstance(val, list):
                    critical.append({"type": "bad_relatedVOS_type", "docId": doc_id,
                                     "detail": "relatedVOS 배열 아님"})
                elif not val:
                    critical.append({"type": "relatedVOS_empty", "docId": doc_id,
                                     "detail": "relatedVOS 는 최소 1개 VOS 키 필수"})
                else:
                    unknown = [k for k in val if k not in vos_keys]
                    if unknown:
                        critical.append({"type": "relatedVOS_hallucination", "docId": doc_id,
                                         "detail": f"VOS 목록에 없는 키: {unknown}"})
            elif field in {"summary", "actor", "basicflow"} and not str(val).strip():
                critical.append({"type": "empty_field", "docId": doc_id,
                                 "detail": f"{field} 빈 값"})

    # ── 과도 통합: Feature 1건인데 VOS 3건 이상
    if len(features) == 1:
        only = features[0]
        if len(only.get("relatedVOS") or []) >= 3:
            critical.append({"type": "over_integration",
                             "docId": only.get("docId", "(only)"),
                             "detail": "Feature 1건뿐인데 VOS 3건 이상 묶음 — 과도 통합"})

    # ── 모든 Feature Actor 동일
    actors = {(f.get("actor") or "").strip() for f in features if (f.get("actor") or "").strip()}
    if len(features) >= 3 and len(actors) == 1:
        critical.append({"type": "single_actor_all",
                         "docId": "(global)",
                         "detail": f"Feature {len(features)}개가 모두 동일 Actor='{list(actors)[0]}' — 다른 Actor 누락 의심"})

    # ── PlantUML 블록 분류
    blocks = pu.find_blocks(doc)
    uc_diagram_count = 0
    seq_count = 0
    class_count = 0
    bad_pair_count = 0
    for b in blocks:
        if not pu.has_uml_pair(b):
            bad_pair_count += 1
            continue
        kind = pu.detect_kind(b)
        if kind == "usecase":
            uc_diagram_count += 1
        elif kind == "sequence":
            seq_count += 1
        elif kind == "class":
            class_count += 1

    if bad_pair_count > 0:
        critical.append({"type": "plantuml_missing_pair",
                         "docId": "(global)",
                         "detail": f"{bad_pair_count}개 블록에 @startuml / @enduml 쌍 누락"})

    context_found = (
        any(("Context" in b) or ("컨텍스트" in b) for b in blocks)
        or uc_diagram_count >= 2
    )
    uc_diagram_found = uc_diagram_count >= 1

    if class_count > 0:
        critical.append({"type": "class_diagram_for_uc", "docId": "(global)",
                         "detail": "Use Case 를 class 다이어그램으로 표현 — usecase / sequence 다이어그램 사용"})
    if not uc_diagram_found:
        critical.append({"type": "missing_usecase_diagram", "docId": "(global)",
                         "detail": "Use Case Diagram (PlantUML usecase) 누락"})
    if seq_count == 0:
        critical.append({"type": "missing_sequence_diagram", "docId": "(global)",
                         "detail": "Sequence Diagram (PlantUML participant) 누락"})

    # ── 모든 Feature 에 Sequence Diagram 있어야
    if features and seq_count > 0 and seq_count < len(features):
        suggestions.append({"type": "sequence_diagram_count_low",
                            "docId": "(global)",
                            "detail": f"Feature {len(features)}개인데 sequence 블록은 {seq_count}개"})

    # ── PlantUML 구문 의심 패턴
    for b in blocks:
        if pu.detect_kind(b) == "sequence" and PARTICIPANT_BAD_NAME_RE.search(b):
            suggestions.append({"type": "participant_no_as",
                                "docId": "(global)",
                                "detail": "participant 선언에 공백 있는 이름 — 따옴표 감싸기 또는 `as` 키워드 사용"})
            break

    # ── 외부 이미지 참조 금지
    if EXT_IMAGE_RE.search(doc):
        critical.append({"type": "external_image",
                         "docId": "(global)",
                         "detail": "외부 이미지 참조 금지 — PlantUML 인라인만 허용"})

    # ── coverageMatrix 에 NFR 플레이스홀더 금지
    if re.search(r"\(NFR\)", doc):
        critical.append({"type": "nfr_placeholder",
                         "docId": "(global)",
                         "detail": "coverageMatrix 에 '(NFR)' 플레이스홀더 사용 금지 — 실제 Feature 지정 필요"})

    total = max(1, 3 + len(features) + len(vos_keys))
    score = max(0.0, 1.0 - len(critical) / total)
    score_effective = round(score - 0.5 * len(critical), 4)

    return {
        "pass": len(critical) == 0 and score >= 0.8,
        "score": round(score, 4),
        "score_effective": score_effective,
        "critical": critical,
        "suggestions": suggestions,
        "stats": {
            "vosKeys": len(vos_keys),
            "storyVosCovered": len(story_vos_covered),
            "featureVosCovered": len(feature_vos_covered),
            "features": len(features),
            "sequenceDiagramCount": seq_count,
            "useCaseDiagramFound": uc_diagram_found,
            "contextDiagramFound": context_found,
            "plantumlBlocks": len(blocks),
            "plantumlBadPair": bad_pair_count,
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
