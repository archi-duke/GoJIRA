"""아키텍처 스타일 추천 점수 엔진.

입력: 정규화된 요구사항 속성 dict ({attr: level, ...})
출력: 후보 스타일별 점수 + 근거 경로 + Primary/Secondary/Supporting 분류

LLM 무관 — 순수 함수. catalog.py 와 짝을 이룬다.
"""

from typing import Dict, List, Any, Optional, Tuple

# ── 레벨 비교 유틸 ───────────────────────────────────────────

# 레벨 어휘별 서열 — 같은 서열 내에서만 비교 가능
_LEVEL_ORDERS = [
    {"low": 1, "medium": 2, "high": 3, "required": 4},          # 일반 품질/요구
    {"small": 1, "medium": 2, "large": 3},                       # team_size
    {"eventual": 1, "strong": 3, "required": 4},                 # consistency
]


def _level_rank(v: str) -> Optional[int]:
    """v 가 속한 서열에서의 순위를 반환. 어느 서열에도 없으면 None."""
    for order in _LEVEL_ORDERS:
        if v in order:
            return order[v]
    return None


def _same_scale(a: str, b: str) -> bool:
    """두 값이 같은 서열에 있는지."""
    for order in _LEVEL_ORDERS:
        if a in order and b in order:
            return True
    return False


def _match_level(actual: str, expected: str) -> Optional[str]:
    """요구사항 actual(level) 이 기대 expected(level 또는 비교표현)와 매치되는지.

    반환: "exact" | "partial" | None
    - expected 가 "medium_or_higher" / "medium_or_lower" 같은 비교형이면 partial 간주
    - actual 과 expected 가 완전히 같으면 exact
    - level 순위 비교가 가능한데 actual 이 expected 를 충족하면 partial
    """
    if actual == expected:
        return "exact"
    # 명시적 비교 표현만 partial 매치 허용 — 서열 비교는 의미가 속성마다 다르므로
    # "low 이면 피하라"를 "low 이상이면 피하라"로 오해하지 않도록 strict mode 기본.
    if expected == "medium_or_higher":
        r = _level_rank(actual)
        return "partial" if (r is not None and r >= 2) else None
    if expected == "medium_or_lower":
        r = _level_rank(actual)
        return "partial" if (r is not None and r <= 2) else None
    if expected == "required":
        r = _level_rank(actual)
        return "exact" if actual == "required" else ("partial" if (r is not None and r >= 3) else None)
    return None


# ── 단일 스타일 점수 계산 ─────────────────────────────────────

def score_style(style: dict, req: Dict[str, str], weights: dict) -> dict:
    """하나의 스타일에 대해 요구사항 req 기준 점수를 계산.

    Returns:
        {
          "id": style_id,
          "score": float,
          "breakdown": [ {category, delta, note}, ... ],
          "unmet_prerequisites": {technical: [...], design: [...], organizational: [...]},
        }
    """
    sid = style["id"]
    score = 0.0
    breakdown: List[Dict[str, Any]] = []

    w_fit = weights.get("fit_signal_match", 10)
    w_fit_partial = weights.get("fit_signal_partial", 5)
    w_avoid = weights.get("avoid_signal_match", -20)
    w_qa_pos = weights.get("qa_positive_strength", {"low": 3, "medium": 6, "high": 10})
    w_qa_neg = weights.get("qa_negative_strength", {"low": -2, "medium": -5, "high": -10})
    w_pre = weights.get("prerequisite_unmet", {"technical": 0, "design": -5, "organizational": -15})

    # fit_signals
    for attr, expected in (style.get("fit_signals") or {}).items():
        actual = req.get(attr)
        if actual is None:
            continue
        m = _match_level(actual, expected)
        if m == "exact":
            score += w_fit
            breakdown.append({"category": "fit", "delta": w_fit,
                              "note": f"fit_signals.{attr}={expected} 충족 (actual={actual})"})
        elif m == "partial":
            score += w_fit_partial
            breakdown.append({"category": "fit_partial", "delta": w_fit_partial,
                              "note": f"fit_signals.{attr}={expected} 부분 충족 (actual={actual})"})

    # avoid_signals — 매치되면 감점
    for attr, expected in (style.get("avoid_signals") or {}).items():
        actual = req.get(attr)
        if actual is None:
            continue
        m = _match_level(actual, expected)
        if m:
            score += w_avoid
            breakdown.append({"category": "avoid", "delta": w_avoid,
                              "note": f"avoid_signals.{attr}={expected} 매치 (actual={actual})"})

    # quality_attributes — 요구 품질속성이 req 에 존재할 때 effect×strength 반영
    for q in (style.get("quality_attributes") or []):
        attr = q.get("attribute")
        if not attr or attr not in req:
            continue
        actual = req[attr]
        # 요구가 low 면 positive/negative 양쪽 모두 낮은 가중
        # 여기서는 단순화: 요구가 high/required 일 때만 강하게 반영, medium 은 절반, low 는 1/3
        factor = {"low": 0.33, "medium": 0.5, "high": 1.0, "required": 1.0}.get(actual, 0.0)
        strength = q.get("strength", "medium")
        if q.get("effect") == "positive":
            delta = w_qa_pos.get(strength, 5) * factor
        elif q.get("effect") == "negative":
            delta = w_qa_neg.get(strength, -5) * factor
        else:
            continue
        if delta:
            score += delta
            breakdown.append({"category": "qa", "delta": round(delta, 2),
                              "note": f"QA {attr} {q.get('effect')}/{strength} × req={actual}"})

    # decision_rules
    for i, rule in enumerate(style.get("decision_rules") or []):
        cond = rule.get("if") or {}
        if not cond:
            continue
        matched = all(
            _match_level(req.get(k, ""), v) is not None
            for k, v in cond.items()
        )
        if matched:
            then = rule.get("then") or {}
            delta = float(then.get("score", 0))
            score += delta
            breakdown.append({"category": "rule", "delta": delta,
                              "note": f"rule[{i}] {then.get('reason', '')}"})

    # prerequisites — organizational 만 req 와 대조 (tech/design 는 실행 컨텍스트 필요)
    unmet = {"technical": [], "design": [], "organizational": []}
    org = ((style.get("prerequisites") or {}).get("organizational") or {})
    for attr, expected in (org or {}).items():
        actual = req.get(attr)
        if actual is None:
            # 정보 없음 — 경고만
            continue
        m = _match_level(actual, expected)
        if not m:
            unmet["organizational"].append(f"{attr} 요구 {expected} / 실제 {actual}")
            delta = w_pre.get("organizational", -15)
            score += delta
            breakdown.append({"category": "prereq_unmet", "delta": delta,
                              "note": f"organizational.{attr} 요구 {expected} 미달 (actual={actual})"})

    return {
        "id": sid,
        "score": round(score, 2),
        "breakdown": breakdown,
        "unmet_prerequisites": unmet,
    }


# ── 전체 추천 ───────────────────────────────────────────────

def recommend(catalog: dict, req: Dict[str, str]) -> dict:
    """카탈로그 전체에 대해 점수를 매기고 Primary/Secondary/Supporting 으로 분류.

    Args:
        catalog: catalog.load_catalog() 결과
        req: 정규화된 요구사항 속성 {attr: level}

    Returns:
        {
          "scored": [ {id, score, breakdown, unmet_prerequisites}, ... ]  (내림차순),
          "primary": [style_id, ...],       # 최상위 점수(동점 임계값 이내)
          "secondary": [style_id, ...],     # primary 외 top_n 이내
          "supporting": [style_id, ...],    # 결합 가능한 보조 스타일 (scope 다른 상위 후보)
          "combinations_applied": [ {from, to, bonus}, ... ],
        }
    """
    index = catalog["index"]
    styles = catalog["styles"]
    weights = index.get("weights") or {}
    engine_cfg = index.get("engine") or {}
    top_n = int(engine_cfg.get("top_n", 5))
    tie = float(engine_cfg.get("tie_threshold", 3))

    scored = [score_style(s, req, weights) for s in styles.values()]

    # combines_bonus / conflict_penalty — 상위 후보 간 상호 조정
    w_combine = weights.get("combines_bonus", 5)
    w_conflict = weights.get("conflict_penalty", -25)
    combos_applied: List[Dict[str, Any]] = []
    by_id = {s["id"]: s for s in scored}
    # 상위 top_n 기준으로 쌍별 조정
    ranked = sorted(scored, key=lambda x: x["score"], reverse=True)[:top_n]
    ranked_ids = {r["id"] for r in ranked}
    for r in ranked:
        style = styles[r["id"]]
        for rel in (style.get("combines_well_with") or []):
            tgt = rel.get("style")
            if tgt in ranked_ids and tgt != r["id"]:
                by_id[r["id"]]["score"] += w_combine
                combos_applied.append({"from": r["id"], "to": tgt,
                                       "delta": w_combine, "type": "combine",
                                       "reason": rel.get("reason", "")})
        for rel in (style.get("conflicts_with") or []):
            tgt = rel.get("style")
            if tgt in ranked_ids and tgt != r["id"]:
                by_id[r["id"]]["score"] += w_conflict
                combos_applied.append({"from": r["id"], "to": tgt,
                                       "delta": w_conflict, "type": "conflict",
                                       "reason": rel.get("reason", "")})

    scored = sorted(by_id.values(), key=lambda x: x["score"], reverse=True)

    # Primary/Secondary 분류
    primary: List[str] = []
    secondary: List[str] = []
    supporting: List[str] = []
    if scored:
        top_score = scored[0]["score"]
        # primary: 최상위 스타일 중 system_structure / data / integration scope 우선
        # 단순화: tie 임계값 이내 전부 primary 후보 → scope 당 1개씩만 채택
        tied = [s for s in scored if top_score - s["score"] <= tie]
        seen_scope = set()
        for s in tied:
            scope = styles[s["id"]].get("scope")
            if scope not in seen_scope:
                primary.append(s["id"])
                seen_scope.add(scope)
        # secondary: primary 외 top_n 내 양의 점수
        for s in scored[:top_n]:
            if s["id"] in primary:
                continue
            if s["score"] > 0:
                secondary.append(s["id"])
        # supporting: primary scope 와 다르고 결합 가능한 module_boundary / infrastructure 스타일
        aux_scopes = {"module_boundary", "infrastructure"}
        for s in scored[:top_n]:
            if s["id"] in primary or s["id"] in secondary:
                continue
            if styles[s["id"]].get("scope") in aux_scopes and s["score"] > 0:
                supporting.append(s["id"])

    return {
        "scored": scored,
        "primary": primary,
        "secondary": secondary,
        "supporting": supporting,
        "combinations_applied": combos_applied,
    }


def format_report(rec: dict, catalog: dict, top: int = 10) -> str:
    """사람/LLM이 읽기 편한 텍스트 보고서."""
    lines: List[str] = []
    styles = catalog["styles"]
    lines.append("=== 점수 순위 ===")
    for i, s in enumerate(rec["scored"][:top]):
        name = styles[s["id"]].get("name", s["id"])
        scope = styles[s["id"]].get("scope", "-")
        lines.append(f"{i+1:2d}. {s['id']:22s} [{scope:18s}] score={s['score']}")
        for b in s["breakdown"][:8]:
            lines.append(f"      {b['delta']:+6.2f}  {b['note']}")
        if s["unmet_prerequisites"]["organizational"]:
            lines.append(f"      ⚠ 조직 전제 미충족: {s['unmet_prerequisites']['organizational']}")
    lines.append("")
    lines.append(f"Primary:    {rec['primary']}")
    lines.append(f"Secondary:  {rec['secondary']}")
    lines.append(f"Supporting: {rec['supporting']}")
    if rec["combinations_applied"]:
        lines.append("\n조합 가감산:")
        for c in rec["combinations_applied"]:
            lines.append(f"  {c['from']} → {c['to']}  {c['delta']:+d}  [{c['type']}]  {c['reason']}")
    return "\n".join(lines)


# ── CLI smoke test ─────────────────────────────────────────

if __name__ == "__main__":
    from skills.style_define.catalog import load_catalog, validate_catalog

    cat = load_catalog()
    probs = validate_catalog(cat)
    if probs:
        print("카탈로그 검증 실패:")
        for p in probs:
            print(f"  - {p}")
        raise SystemExit(1)

    # 합성 시나리오 1: 대규모 분산 시스템 적합 조건
    scenario1 = {
        "scalability": "high",
        "availability": "high",
        "release_frequency": "high",
        "team_independent_deploy": "high",
        "domain_complexity": "high",
        "team_size": "large",
        "ops_maturity": "high",
        "event_volume": "high",
        "auditability": "high",
    }
    # 시나리오 2: 소규모 팀의 단순 시스템
    scenario2 = {
        "scalability": "low",
        "team_size": "small",
        "ops_maturity": "low",
        "simple_crud": "high",
        "strong_consistency": "high",
        "domain_complexity": "low",
    }
    # 시나리오 3: 저지연 강일관성 요구
    scenario3 = {
        "synchronous_low_latency": "high",
        "strong_consistency": "high",
        "team_size": "medium",
        "ops_maturity": "medium",
        "domain_complexity": "medium",
    }

    for i, sc in enumerate([scenario1, scenario2, scenario3], 1):
        print(f"\n########## 시나리오 {i} ##########")
        print(f"입력: {sc}")
        rec = recommend(cat, sc)
        print(format_report(rec, cat, top=10))
