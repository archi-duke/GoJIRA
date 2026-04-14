"""아키텍처 스타일 카탈로그 로더 + 스키마 검증.

knowledge/arch-styles/ 의 index.yaml 과 styles/*.yaml 을 로드하고 구조/참조/어휘 일관성을 검증한다.
LLM 무관 — 순수 함수.
"""

import os
import yaml
from typing import Iterable, Optional, List, Dict

_KNOWLEDGE_DIR = os.path.join(
    os.path.dirname(__file__), "..", "..", "knowledge", "arch-styles"
)

_ALLOWED_EFFECT = {"positive", "negative"}
_ALLOWED_STRENGTH = {"low", "medium", "high"}
_ALLOWED_LEVEL_SUFFIX = {
    "low", "medium", "high", "required",
    "medium_or_higher", "medium_or_lower",
    "small", "large",            # team_size 등
    "eventual", "strong",         # consistency
    "none",
    "enabled", "disabled",        # 플래그형 (decision_rules의 부가 조건용)
}


class CatalogError(ValueError):
    pass


def _load_yaml(path: str) -> dict:
    with open(path, "r", encoding="utf-8") as f:
        return yaml.safe_load(f) or {}


def load_catalog(knowledge_dir: Optional[str] = None) -> dict:
    """카탈로그 전체를 로드해 구조화된 딕셔너리로 반환.

    Returns:
        {
          "index": {...index.yaml 내용...},
          "styles": {id: {...style yaml...}, ...},
          "knowledge_dir": str,
        }
    """
    root = knowledge_dir or _KNOWLEDGE_DIR
    index_path = os.path.join(root, "index.yaml")
    styles_dir = os.path.join(root, "styles")
    if not os.path.isfile(index_path):
        raise CatalogError(f"index.yaml 없음: {index_path}")
    if not os.path.isdir(styles_dir):
        raise CatalogError(f"styles/ 디렉토리 없음: {styles_dir}")

    index = _load_yaml(index_path)
    styles = {}
    for fname in sorted(os.listdir(styles_dir)):
        if not fname.endswith(".yaml"):
            continue
        data = _load_yaml(os.path.join(styles_dir, fname))
        sid = data.get("id", "")
        if not sid:
            raise CatalogError(f"{fname}: id 필드 누락")
        if f"{sid}.yaml" != fname:
            raise CatalogError(f"{fname}: id({sid}) 가 파일명과 일치해야 함")
        styles[sid] = data

    return {"index": index, "styles": styles, "knowledge_dir": root}


def validate_catalog(catalog: dict) -> list[str]:
    """카탈로그 전체를 검증하고 문제점 목록을 반환한다.

    문제 없으면 빈 리스트. 발견된 모든 문제를 수집(첫 오류에서 멈추지 않음).
    """
    problems: List[str] = []
    index = catalog["index"]
    styles = catalog["styles"]

    scopes = set((index.get("scopes") or {}).keys())
    if not scopes:
        problems.append("index.scopes 가 비어있음")

    attributes = index.get("attributes") or {}
    allowed_attr_keys = set(attributes.keys())
    # attributes 값은 리스트 형태 — 어휘 존재 여부만 검증 (개별 값 범위는 level 공통 규칙 적용)

    for sid, s in styles.items():
        prefix = f"[{sid}]"
        # 필수 필드
        for req in ("id", "name", "scope", "summary"):
            if not s.get(req):
                problems.append(f"{prefix} 필수 필드 누락: {req}")
        # scope
        if s.get("scope") and s["scope"] not in scopes:
            problems.append(f"{prefix} scope '{s['scope']}' 가 index.scopes 에 없음")

        # fit_signals / avoid_signals — attr 어휘 검증
        for field in ("fit_signals", "avoid_signals"):
            sig = s.get(field) or {}
            if not isinstance(sig, dict):
                problems.append(f"{prefix} {field} 는 dict 이어야 함")
                continue
            for k, v in sig.items():
                if allowed_attr_keys and k not in allowed_attr_keys:
                    problems.append(f"{prefix} {field}.{k} 어휘가 index.attributes 에 없음")
                if v not in _ALLOWED_LEVEL_SUFFIX:
                    problems.append(f"{prefix} {field}.{k}={v} 허용 레벨 아님")

        # prerequisites — dict with technical/design/organizational keys
        pre = s.get("prerequisites") or {}
        if not isinstance(pre, dict):
            problems.append(f"{prefix} prerequisites 는 dict 이어야 함")
        else:
            for k in pre.keys():
                if k not in {"technical", "design", "organizational"}:
                    problems.append(f"{prefix} prerequisites.{k} 허용 키 아님 (technical/design/organizational)")

        # quality_attributes — list of {attribute, effect, strength}
        qa = s.get("quality_attributes") or []
        if not isinstance(qa, list):
            problems.append(f"{prefix} quality_attributes 는 list 이어야 함")
        else:
            for i, q in enumerate(qa):
                loc = f"{prefix} quality_attributes[{i}]"
                if not isinstance(q, dict):
                    problems.append(f"{loc} dict 이어야 함")
                    continue
                if not q.get("attribute"):
                    problems.append(f"{loc}.attribute 누락")
                if q.get("effect") not in _ALLOWED_EFFECT:
                    problems.append(f"{loc}.effect={q.get('effect')} 허용값 아님")
                if q.get("strength") not in _ALLOWED_STRENGTH:
                    problems.append(f"{loc}.strength={q.get('strength')} 허용값 아님")

        # combines_well_with / conflicts_with — 참조 유효성 + conflicts scope 규칙
        for field in ("combines_well_with", "conflicts_with"):
            rels = s.get(field) or []
            if not isinstance(rels, list):
                problems.append(f"{prefix} {field} 는 list 이어야 함")
                continue
            for i, r in enumerate(rels):
                loc = f"{prefix} {field}[{i}]"
                if not isinstance(r, dict):
                    problems.append(f"{loc} dict {{style, reason}} 이어야 함")
                    continue
                tgt = r.get("style")
                if not tgt:
                    problems.append(f"{loc}.style 누락")
                    continue
                # 참조는 카탈로그 내 id 여야 — 단 외부 참조(shared-database-centric 등) 허용을 위한 whitelist 이후 확장 가능
                if tgt in styles:
                    if field == "conflicts_with":
                        if styles[tgt].get("scope") != s.get("scope"):
                            problems.append(
                                f"{loc} '{tgt}' 는 다른 scope ({styles[tgt].get('scope')} vs {s.get('scope')}) — "
                                f"conflicts_with 는 같은 scope 내에서만 허용"
                            )

        # decision_rules — list of {if: dict, then: {score, reason}}
        rules = s.get("decision_rules") or []
        if not isinstance(rules, list):
            problems.append(f"{prefix} decision_rules 는 list 이어야 함")
        else:
            for i, rule in enumerate(rules):
                loc = f"{prefix} decision_rules[{i}]"
                if not isinstance(rule, dict) or "if" not in rule or "then" not in rule:
                    problems.append(f"{loc} {{if, then}} 구조 아님")
                    continue
                cond = rule["if"] or {}
                then = rule["then"] or {}
                if not isinstance(cond, dict):
                    problems.append(f"{loc}.if dict 이어야 함")
                else:
                    for k, v in cond.items():
                        if allowed_attr_keys and k not in allowed_attr_keys:
                            problems.append(f"{loc}.if.{k} 어휘가 index.attributes 에 없음")
                        if v not in _ALLOWED_LEVEL_SUFFIX:
                            problems.append(f"{loc}.if.{k}={v} 허용 레벨 아님")
                if not isinstance(then, dict) or "score" not in then:
                    problems.append(f"{loc}.then.score 누락")

    return problems


def list_styles(catalog: dict) -> List[str]:
    return sorted(catalog["styles"].keys())


def styles_by_scope(catalog: dict) -> Dict[str, List[str]]:
    out: Dict[str, List[str]] = {}
    for sid, s in catalog["styles"].items():
        out.setdefault(s.get("scope", "unknown"), []).append(sid)
    for k in out:
        out[k].sort()
    return out


if __name__ == "__main__":
    cat = load_catalog()
    probs = validate_catalog(cat)
    print(f"styles: {len(cat['styles'])}")
    print(f"scopes: {styles_by_scope(cat)}")
    if probs:
        print(f"\n문제 {len(probs)}건:")
        for p in probs:
            print(f"  - {p}")
    else:
        print("검증 통과")
