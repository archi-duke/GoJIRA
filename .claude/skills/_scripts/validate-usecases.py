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
  "diagrams": {                               # Step 1.4 산출 (단계별 검증 시)
    "context": "@startuml...@enduml",
    "usecase": "@startuml...@enduml",
    "sequences": {"UC-1":"@startuml...@enduml", ...}
  },
  "document": "<markdown>"                    # 작성한 use-cases.md 본문 (final)
}

CLI:
  validate-usecases.py [--stage=entities|features|scenarios|diagrams|final]

기본 stage = `final` (기존 동작 유지). 단계별 validator 는 해당 단계가 책임지는
필드만 critical 검사하고 나머지 단계 산출은 부재해도 통과.

stdout JSON:
{
  "pass": bool,
  "stage": "<stage>",
  "score": 0.0~1.0,
  "score_effective": number,
  "critical": [{"type":"...","docId":"...","detail":"..."}],
  "suggestions": [...],
  "stats": {...}
}
"""
from __future__ import annotations

import argparse
import json
import re
import sys
from collections import Counter

import plantuml_util as pu


FEATURE_REQUIRED = {"summary", "actor", "basicflow", "docId", "relatedVOS"}

EXT_IMAGE_RE = re.compile(r"!\[[^\]]*\]\([^\)]+\.(png|jpg|jpeg|gif|svg)\)", re.IGNORECASE)
# PlantUML sequence: participant 선언에 공백 있는 이름을 "..." 또는 as 없이 쓰면 실수
PARTICIPANT_BAD_NAME_RE = re.compile(
    r"^\s*(?:participant|boundary|control|entity|queue|collections)\s+"
    r"(?![\"'])[^\s\"']+\s+(?!as\b)[^\s\"'<][^\s\"']*",
    re.MULTILINE | re.IGNORECASE,
)


# ─── 단계별 helper -----------------------------------------------------------

def _check_features_basic(features: list, vos_keys: set, critical: list) -> None:
    """features 배열 자체의 정합성 (필수 필드, docId 중복, relatedVOS 등) 검사."""
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
                # basicflow 빈 값은 scenarios/final 단계에서만 critical (entities/features 단계는 미허용)
                # 단, summary/actor 빈 값은 features 단계부터 critical
                critical.append({"type": "empty_field", "docId": doc_id,
                                 "detail": f"{field} 빈 값"})


def _check_plantuml_block(block: str, kind_hint: str | None,
                          critical: list, suggestions: list, label: str) -> str:
    """단일 PlantUML 텍스트 블록 검사 — kind 반환 ("usecase"/"sequence"/"class"/"other"/"badpair")."""
    if not block or not block.strip():
        return "empty"
    if not pu.has_uml_pair(block):
        critical.append({"type": "plantuml_missing_pair", "docId": label,
                         "detail": "@startuml / @enduml 쌍 누락"})
        return "badpair"
    kind = pu.detect_kind(block)
    if kind == "class":
        critical.append({"type": "class_diagram_for_uc", "docId": label,
                         "detail": "Use Case 를 class 다이어그램으로 표현 — usecase / sequence 다이어그램 사용"})
    if kind == "sequence" and PARTICIPANT_BAD_NAME_RE.search(block):
        suggestions.append({"type": "participant_no_as", "docId": label,
                            "detail": "participant 선언에 공백 있는 이름 — 따옴표 또는 `as` 사용"})
    return kind


# ─── stage 별 검증 -----------------------------------------------------------

def validate_entities(payload: dict) -> dict:
    """Step 1.1 산출: domain + userStories + useCases + vosKeys."""
    vos_keys = set(payload.get("vosKeys") or [])
    stories = payload.get("userStories") or []
    use_cases = payload.get("useCases") or []

    critical: list[dict] = []
    suggestions: list[dict] = []

    # VOS 커버리지 (User Story 기준)
    story_vos = set()
    for s in stories:
        vk = s.get("vosKey")
        if vk:
            story_vos.add(vk)
    for vk in sorted(vos_keys - story_vos):
        critical.append({"type": "vos_no_story", "docId": vk,
                         "detail": "VOS 가 어느 User Story 로도 파생되지 않음"})

    # 모든 UserStory 가 ucId 를 가져야
    uc_ids_in_use_cases = {u.get("id") for u in use_cases if u.get("id")}
    for s in stories:
        if not s.get("ucId"):
            critical.append({"type": "story_no_uc", "docId": s.get("id", "(missing)"),
                             "detail": "User Story 의 ucId 누락"})
        elif uc_ids_in_use_cases and s.get("ucId") not in uc_ids_in_use_cases:
            suggestions.append({"type": "story_uc_mismatch", "docId": s.get("id"),
                                "detail": f"ucId='{s.get('ucId')}' 가 useCases 에 없음"})

    if not use_cases:
        critical.append({"type": "no_use_cases", "docId": "(global)",
                         "detail": "Use Case 가 1개 이상 도출되어야 함"})

    total = max(1, 2 + len(vos_keys))
    score = max(0.0, 1.0 - len(critical) / total)
    return {
        "pass": len(critical) == 0 and score >= 0.8,
        "stage": "entities",
        "score": round(score, 4),
        "score_effective": round(score - 0.5 * len(critical), 4),
        "critical": critical,
        "suggestions": suggestions,
        "stats": {"vosKeys": len(vos_keys), "userStories": len(stories),
                  "useCases": len(use_cases), "storyVosCovered": len(story_vos)},
    }


def validate_features(payload: dict) -> dict:
    """Step 1.2 산출: features 의 docId/summary/actor/relatedVOS/mode (시나리오 빈 칸 허용)."""
    vos_keys = set(payload.get("vosKeys") or [])
    features = payload.get("features") or []

    critical: list[dict] = []
    suggestions: list[dict] = []

    if not features:
        critical.append({"type": "no_features", "docId": "(global)",
                         "detail": "Feature 가 1개 이상 도출되어야 함"})

    # docId 중복 + 필수 필드 (단, basicflow 는 scenarios 단계에서 채우므로 이 단계에서 면제)
    doc_ids = [f.get("docId", "") for f in features]
    dup_ids = [k for k, v in Counter(doc_ids).items() if v > 1 and k]
    for d in dup_ids:
        critical.append({"type": "duplicate_docId", "docId": d,
                         "detail": "Feature docId 중복"})
    for f in features:
        doc_id = f.get("docId", "<missing>")
        for field in {"summary", "actor", "docId", "relatedVOS"}:
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
            elif field in {"summary", "actor"} and not str(val).strip():
                critical.append({"type": "empty_field", "docId": doc_id,
                                 "detail": f"{field} 빈 값"})

        m = (f.get("mode") or "").strip()
        if m and m not in {"create", "reuse", "modify", "delete_mark"}:
            critical.append({"type": "bad_mode", "docId": doc_id,
                             "detail": f"mode='{m}' 잘못 — create/reuse/modify/delete_mark 중 하나"})

    # Feature 기준 VOS 커버리지
    feature_vos = set()
    for f in features:
        for vk in (f.get("relatedVOS") or []):
            if vk in vos_keys:
                feature_vos.add(vk)
    for vk in sorted(vos_keys - feature_vos):
        critical.append({"type": "vos_no_feature", "docId": vk,
                         "detail": "VOS 가 어느 Feature 의 relatedVOS 에도 포함 안 됨 (NFR 면제 없음)"})

    total = max(1, 2 + len(features) + len(vos_keys))
    score = max(0.0, 1.0 - len(critical) / total)
    return {
        "pass": len(critical) == 0 and score >= 0.8,
        "stage": "features",
        "score": round(score, 4),
        "score_effective": round(score - 0.5 * len(critical), 4),
        "critical": critical,
        "suggestions": suggestions,
        "stats": {"vosKeys": len(vos_keys), "features": len(features),
                  "featureVosCovered": len(feature_vos)},
    }


def validate_scenarios(payload: dict) -> dict:
    """Step 1.3 산출: features[*].basicflow 등 시나리오 채워졌는지."""
    features = payload.get("features") or []

    critical: list[dict] = []
    suggestions: list[dict] = []

    if not features:
        critical.append({"type": "no_features", "docId": "(global)",
                         "detail": "scenarios 단계 진입 시점에 features 가 비어있음"})

    for f in features:
        doc_id = f.get("docId", "<missing>")
        if not str(f.get("basicflow") or "").strip():
            critical.append({"type": "empty_basicflow", "docId": doc_id,
                             "detail": "basicflow 빈 값 — 시나리오 작성 필수"})
        if not str(f.get("precondition") or "").strip():
            suggestions.append({"type": "empty_precondition", "docId": doc_id,
                                "detail": "precondition 비어있음"})
        if not str(f.get("postcondition") or "").strip():
            suggestions.append({"type": "empty_postcondition", "docId": doc_id,
                                "detail": "postcondition 비어있음"})

    # 모든 Feature Actor 동일 체크 (3건 이상)
    actors = {(f.get("actor") or "").strip() for f in features if (f.get("actor") or "").strip()}
    if len(features) >= 3 and len(actors) == 1:
        critical.append({"type": "single_actor_all", "docId": "(global)",
                         "detail": f"Feature {len(features)}개가 모두 동일 Actor — 다른 Actor 누락 의심"})

    total = max(1, 2 + len(features))
    score = max(0.0, 1.0 - len(critical) / total)
    return {
        "pass": len(critical) == 0 and score >= 0.8,
        "stage": "scenarios",
        "score": round(score, 4),
        "score_effective": round(score - 0.5 * len(critical), 4),
        "critical": critical,
        "suggestions": suggestions,
        "stats": {"features": len(features), "distinctActors": len(actors)},
    }


def validate_diagrams(payload: dict) -> dict:
    """Step 1.4 산출: diagrams.context / usecase / sequences[docId]."""
    features = payload.get("features") or []
    diagrams = payload.get("diagrams") or {}

    critical: list[dict] = []
    suggestions: list[dict] = []

    ctx = diagrams.get("context") or ""
    uc = diagrams.get("usecase") or ""
    sequences = diagrams.get("sequences") or {}

    if not str(ctx).strip():
        critical.append({"type": "missing_context_diagram", "docId": "(global)",
                         "detail": "Context Diagram 누락"})
    else:
        _check_plantuml_block(ctx, "context", critical, suggestions, "context")

    if not str(uc).strip():
        critical.append({"type": "missing_usecase_diagram", "docId": "(global)",
                         "detail": "Use Case Diagram 누락"})
    else:
        kind = _check_plantuml_block(uc, "usecase", critical, suggestions, "usecase")
        if kind == "class":
            # 이미 critical 추가됨 (_check_plantuml_block 내부)
            pass

    seq_count = 0
    for f in features:
        doc_id = f.get("docId") or ""
        body = sequences.get(doc_id) or ""
        if not str(body).strip():
            critical.append({"type": "missing_sequence_diagram", "docId": doc_id,
                             "detail": f"{doc_id} 의 Sequence Diagram 누락"})
            continue
        kind = _check_plantuml_block(body, "sequence", critical, suggestions, doc_id)
        if kind == "sequence":
            seq_count += 1

    total = max(1, 2 + len(features))
    score = max(0.0, 1.0 - len(critical) / total)
    return {
        "pass": len(critical) == 0 and score >= 0.8,
        "stage": "diagrams",
        "score": round(score, 4),
        "score_effective": round(score - 0.5 * len(critical), 4),
        "critical": critical,
        "suggestions": suggestions,
        "stats": {"features": len(features), "sequenceDiagramCount": seq_count,
                  "contextDiagramFound": bool(str(ctx).strip()),
                  "useCaseDiagramFound": bool(str(uc).strip())},
    }


def validate_final(payload: dict) -> dict:
    """Step 1.5 결정적 조립 결과 (use-cases.md document) + 누적 도메인 자료 전체 검증.

    기존 단일 generation 동작과 호환 — payload 가 entities/features/scenarios/diagrams 단계의
    누적 결과이거나 단일 generation 산출이거나 둘 다 통과.
    """
    vos_keys = set(payload.get("vosKeys") or [])
    stories = payload.get("userStories") or []
    features = payload.get("features") or []
    doc = payload.get("document", "") or ""

    critical: list[dict] = []
    suggestions: list[dict] = []

    # User Story 기준 VOS 커버리지
    story_vos = set()
    for s in stories:
        vk = s.get("vosKey")
        if vk:
            story_vos.add(vk)
    for vk in sorted(vos_keys - story_vos):
        critical.append({"type": "vos_no_story", "docId": vk,
                         "detail": "VOS 가 어느 User Story 로도 파생되지 않음"})

    # Feature 기준 VOS 커버리지
    feature_vos = set()
    for f in features:
        for vk in (f.get("relatedVOS") or []):
            if vk in vos_keys:
                feature_vos.add(vk)
    for vk in sorted(vos_keys - feature_vos):
        critical.append({"type": "vos_no_feature", "docId": vk,
                         "detail": "VOS 가 어느 Feature 의 relatedVOS 에도 포함 안 됨 (NFR 면제 없음)"})

    # Feature 정합성 (모든 필수 필드 + basicflow 비어있는지 포함)
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

    if len(features) == 1:
        only = features[0]
        if len(only.get("relatedVOS") or []) >= 3:
            critical.append({"type": "over_integration",
                             "docId": only.get("docId", "(only)"),
                             "detail": "Feature 1건뿐인데 VOS 3건 이상 묶음 — 과도 통합"})

    actors = {(f.get("actor") or "").strip() for f in features if (f.get("actor") or "").strip()}
    if len(features) >= 3 and len(actors) == 1:
        critical.append({"type": "single_actor_all", "docId": "(global)",
                         "detail": f"Feature {len(features)}개가 모두 동일 Actor='{list(actors)[0]}' — 다른 Actor 누락 의심"})

    # PlantUML 블록 분류 (document 안)
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
        critical.append({"type": "plantuml_missing_pair", "docId": "(global)",
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

    if features and seq_count > 0 and seq_count < len(features):
        suggestions.append({"type": "sequence_diagram_count_low", "docId": "(global)",
                            "detail": f"Feature {len(features)}개인데 sequence 블록은 {seq_count}개"})

    for b in blocks:
        if pu.detect_kind(b) == "sequence" and PARTICIPANT_BAD_NAME_RE.search(b):
            suggestions.append({"type": "participant_no_as", "docId": "(global)",
                                "detail": "participant 선언에 공백 있는 이름 — 따옴표 감싸기 또는 `as` 키워드 사용"})
            break

    if EXT_IMAGE_RE.search(doc):
        critical.append({"type": "external_image", "docId": "(global)",
                         "detail": "외부 이미지 참조 금지 — PlantUML 인라인만 허용"})

    if re.search(r"\(NFR\)", doc):
        critical.append({"type": "nfr_placeholder", "docId": "(global)",
                         "detail": "coverageMatrix 에 '(NFR)' 플레이스홀더 사용 금지 — 실제 Feature 지정 필요"})

    total = max(1, 3 + len(features) + len(vos_keys))
    score = max(0.0, 1.0 - len(critical) / total)
    score_effective = round(score - 0.5 * len(critical), 4)

    return {
        "pass": len(critical) == 0 and score >= 0.8,
        "stage": "final",
        "score": round(score, 4),
        "score_effective": score_effective,
        "critical": critical,
        "suggestions": suggestions,
        "stats": {
            "vosKeys": len(vos_keys),
            "storyVosCovered": len(story_vos),
            "featureVosCovered": len(feature_vos),
            "features": len(features),
            "sequenceDiagramCount": seq_count,
            "useCaseDiagramFound": uc_diagram_found,
            "contextDiagramFound": context_found,
            "plantumlBlocks": len(blocks),
            "plantumlBadPair": bad_pair_count,
        },
    }


# ─── alias (구 API 호환) -----------------------------------------------------

def validate(payload: dict) -> dict:
    """기존 단일 generation 호환 진입점 — final stage 와 동일."""
    return validate_final(payload)


STAGES = {
    "entities": validate_entities,
    "features": validate_features,
    "scenarios": validate_scenarios,
    "diagrams": validate_diagrams,
    "final": validate_final,
}


def main() -> int:
    parser = argparse.ArgumentParser(add_help=True)
    parser.add_argument("--stage", default="final", choices=list(STAGES.keys()),
                        help="검증 단계 (기본: final)")
    args = parser.parse_args()

    try:
        payload = json.load(sys.stdin)
    except json.JSONDecodeError as exc:
        print(json.dumps({"pass": False, "stage": args.stage, "score": 0.0,
                          "score_effective": -1.0,
                          "critical": [{"type": "invalid_json",
                                        "docId": "(input)", "detail": str(exc)}],
                          "suggestions": [], "stats": {}},
                         ensure_ascii=False, indent=2))
        return 2

    fn = STAGES[args.stage]
    result = fn(payload)
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["pass"] else 1


if __name__ == "__main__":
    sys.exit(main())
