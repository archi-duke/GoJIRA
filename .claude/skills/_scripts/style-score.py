#!/usr/bin/env python3
"""style-score.py — Agent-Service 의 style_define 엔진을 얇게 감싼 CLI wrapper.

stdin JSON:
{
  "req": {"scalability":"high", "team_size":"large", ...},     # normalizedAttributes
  "knowledge_dir": "<optional absolute path>"                   # 없으면 Agent-Service/knowledge/arch-styles 자동
}

stdout JSON:
{
  "scored": [ {id, score, breakdown, unmet_prerequisites}, ... ],  # 내림차순
  "primary": ["<id>", ...],
  "secondary": ["<id>", ...],
  "supporting": ["<id>", ...],
  "combinations_applied": [ {from, to, delta, type, reason}, ... ],
  "catalog_summary": {
    "styles": ["<id>", ...],
    "scopes": {"<id>": "<scope>", ...},
    "names":  {"<id>": "<name>", ...},
    "fit_signals": {"<id>": {...}, ...},
    "avoid_signals": {"<id>": {...}, ...},
    "quality_attributes": {"<id>": [...], ...},
    "tradeoffs": {"<id>": {"pros":[...], "cons":[...]}, ...},
    "prerequisites": {"<id>": {...}, ...},
    "combines_well_with": {"<id>": [...], ...},
    "conflicts_with": {"<id>": [...], ...}
  }
}

exit code: 0 = 정상, 1 = 카탈로그 오류, 2 = 입력 오류
"""
from __future__ import annotations

import json
import os
import sys
from pathlib import Path


def _default_agent_service_dir() -> Path:
    here = Path(__file__).resolve()
    # .claude/skills/_scripts/ → repo root
    repo_root = here.parent.parent.parent.parent
    return repo_root / "Agent-Service"


def _load_engine(agent_service_dir: Path):
    sys.path.insert(0, str(agent_service_dir))
    from skills.style_define import catalog as cat_mod
    from skills.style_define import engine as eng_mod
    return cat_mod, eng_mod


def _catalog_summary(catalog: dict) -> dict:
    styles = catalog["styles"]
    return {
        "styles": sorted(styles.keys()),
        "scopes": {sid: s.get("scope") for sid, s in styles.items()},
        "names": {sid: s.get("name", sid) for sid, s in styles.items()},
        "fit_signals": {sid: s.get("fit_signals") or {} for sid, s in styles.items()},
        "avoid_signals": {sid: s.get("avoid_signals") or {} for sid, s in styles.items()},
        "quality_attributes": {sid: s.get("quality_attributes") or [] for sid, s in styles.items()},
        "tradeoffs": {sid: s.get("tradeoffs") or {} for sid, s in styles.items()},
        "prerequisites": {sid: s.get("prerequisites") or {} for sid, s in styles.items()},
        "combines_well_with": {sid: s.get("combines_well_with") or [] for sid, s in styles.items()},
        "conflicts_with": {sid: s.get("conflicts_with") or [] for sid, s in styles.items()},
    }


def main() -> int:
    try:
        payload = json.load(sys.stdin)
    except json.JSONDecodeError as exc:
        print(json.dumps({"error": "invalid_json", "detail": str(exc)},
                         ensure_ascii=False), file=sys.stderr)
        return 2

    req = payload.get("req")
    if not isinstance(req, dict) or not req:
        print(json.dumps({"error": "empty_req",
                          "detail": "req (normalizedAttributes) is required"},
                         ensure_ascii=False), file=sys.stderr)
        return 2

    agent_service_dir = Path(payload.get("agent_service_dir") or _default_agent_service_dir())
    if not (agent_service_dir / "skills" / "style_define").is_dir():
        print(json.dumps({
            "error": "agent_service_not_found",
            "detail": f"expected Agent-Service dir: {agent_service_dir}"
        }, ensure_ascii=False), file=sys.stderr)
        return 1

    knowledge_dir = payload.get("knowledge_dir")
    cat_mod, eng_mod = _load_engine(agent_service_dir)
    try:
        catalog = cat_mod.load_catalog(knowledge_dir=knowledge_dir) if knowledge_dir else cat_mod.load_catalog()
    except Exception as exc:
        print(json.dumps({"error": "catalog_load_failed",
                          "detail": str(exc)},
                         ensure_ascii=False), file=sys.stderr)
        return 1

    problems = cat_mod.validate_catalog(catalog) if hasattr(cat_mod, "validate_catalog") else []
    if problems:
        print(json.dumps({"error": "catalog_invalid", "problems": problems},
                         ensure_ascii=False), file=sys.stderr)
        return 1

    rec = eng_mod.recommend(catalog, req)
    rec["catalog_summary"] = _catalog_summary(catalog)
    print(json.dumps(rec, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    sys.exit(main())
