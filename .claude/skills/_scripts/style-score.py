#!/usr/bin/env python3
"""style-score.py — 정규화 속성(NORM) 으로 아키텍처 스타일 적합/회피 점수를 낸다.

점수 출처는 둘이고 **셈법은 동일**하다.

  ragaas  RAGaaS 지식그래프 조회 (`POST {api}/knowledge-bases/{kb}/agent/plan`).
          LLM 을 쓰지 않으므로 0.1~0.3s. 운영 KB 로 옮기기 쉽게 env 로 주소를 받는다.
  yaml    로컬 카탈로그(`style-define/knowledge/arch-styles/styles/*.yaml`) 직접 읽기.
          RAGaaS 에 못 붙거나 조회가 깨질 때의 대체 경로.

`source: "auto"`(기본) 는 ragaas 를 먼저 시도하고 실패하면 yaml 로 내려간다.
두 경로가 모두 막히면 exit 1 — SKILL 은 그때 카탈로그를 직접 읽어 판단한다.

2026-05-22 ~ 2026-11-16 사이 이 스크립트는 Agent-Service 의 `style_define` 엔진
wrapper 였고, 그 Agent-Service 가 커밋 89b379f9 에서 제거돼 줄곧 동작하지 않았다.

stdin JSON:
{
  "req": {"scalability":"high", "team_size":"small", ...},   # normalizedAttributes (필수)
  "source": "auto" | "ragaas" | "yaml",                      # 선택, 기본 auto
  "ragaas_api": "http://host:44300/ragaas/api/v2",           # 선택, 기본 env RAGAAS_API
  "kb": "<knowledge base id>",                               # 선택, 기본 env RAGAAS_STYLE_KB
  "knowledge_dir": "<arch-styles 경로>"                       # 선택, 기본 스킬 내장 카탈로그
}

stdout JSON:
{
  "source": "ragaas" | "yaml",
  "scoring": "signals_only",
  "scored": [ {id, name, scope, score, fit_exact, fit_partial, avoid, breakdown[]}, ... ],
  "primary": [...], "secondary": [...], "supporting": [...],
  "catalog_summary": {styles, names, scopes, fit_signals, avoid_signals},
  "ragaas": {...},          # source=ragaas 일 때만
  "warnings": [...]
}

exit code: 0 = 정상, 1 = 두 경로 모두 실패, 2 = 입력 오류
"""
from __future__ import annotations

import json
import os
import re
import sys
import urllib.error
import urllib.request
from pathlib import Path

DEFAULT_KB = "406b0eeb-8002-4a3f-8368-c580d397fa89"  # test-ontology
DEFAULT_RAGAAS_API = "http://localhost:44300/ragaas/api/v2"
HTTP_TIMEOUT = 10.0
# RAGaaS plan 은 조건 12개를 넘기면 reject 한다. NORM 속성이 많으면(스타일 10종 ×
# 비교 표기) 쉽게 넘으므로 나눠 던지고 결과를 합산한다. any(OR) 라서 분할해도 같다.
MAX_CONDITIONS = 12

# 신호 가중치 — index.yaml 의 weights 에서 읽고, 못 읽으면 이 기본값.
W_FIT_EXACT = 10
W_FIT_PARTIAL = 5
W_AVOID = -20
PRIMARY_MIN = 50
SECONDARY_MIN = 30

# 강도 사다리. 비교 표기(medium_or_higher 등)의 충족 판정에만 쓴다.
RANK = {
    "none": -1,
    "low": 0, "small": 0, "eventual": 0,
    "medium": 1, "strong": 1,
    "high": 2, "large": 2,
    "required": 3,
}


# ── 신호 충족 판정 ────────────────────────────────────────────────────────────
def satisfies(signal_value: str, norm_value: str) -> str:
    """NORM 값이 스타일의 신호 값을 충족하는가. "exact" | "partial" | "" 를 반환.

    exact    값이 같다.
    partial  비교 표기(medium_or_higher / medium_or_lower)를 만족하거나,
             required(강제) 가 high 신호를 넘어서 충족한다.
    """
    s, n = str(signal_value).strip(), str(norm_value).strip()
    if not s or not n:
        return ""
    if s == n:
        return "exact"
    nr = RANK.get(n)
    if s == "medium_or_higher":
        return "partial" if nr is not None and nr >= RANK["medium"] else ""
    if s == "medium_or_lower":
        return "partial" if nr is not None and nr <= RANK["medium"] else ""
    # required 는 high 보다 강하다 — high 를 요구하는 신호를 충족한다.
    if n == "required" and s == "high":
        return "partial"
    # 그 밖에는 정확 일치만 인정한다. fit_signals.scalability=medium 은
    # "중간이 적정" 이라는 뜻이므로 high 가 이를 충족한다고 보지 않는다.
    return ""


def score_of(fit_exact: int, fit_partial: int, avoid: int) -> int:
    return fit_exact * W_FIT_EXACT + fit_partial * W_FIT_PARTIAL + avoid * W_AVOID


# ── 로컬 카탈로그 ─────────────────────────────────────────────────────────────
def default_knowledge_dir() -> Path:
    # .claude/skills/_scripts/style-score.py → .claude/skills/style-define/knowledge/arch-styles
    here = Path(__file__).resolve()
    return here.parent.parent / "style-define" / "knowledge" / "arch-styles"


def _load_yaml(path: Path) -> dict:
    """PyYAML 이 있으면 그걸로, 없으면 필요한 필드만 긁는 축소 파서로 읽는다.

    축소 파서가 보는 것: id / name / scope, 그리고 fit_signals / avoid_signals 의
    `  key: value` 한 단계. 점수에 쓰는 건 그게 전부다.
    """
    text = path.read_text(encoding="utf-8")
    try:
        import yaml  # noqa: PLC0415
        data = yaml.safe_load(text)
        if isinstance(data, dict):
            return data
    except Exception:  # noqa: BLE001 — PyYAML 없음 또는 파싱 실패 → 축소 파서
        pass
    return _load_yaml_mini(text)


def _load_yaml_mini(text: str) -> dict:
    """PyYAML 없이 id/name/scope + fit_signals/avoid_signals 만 긁는 축소 파서."""
    def scalar(v: str) -> str:
        # 끝에 붙은 주석을 떼어낸다 — `team_size: small   # 규율 유지 난이도`.
        return re.sub(r"\s+#.*$", "", v).strip().strip("\"'")

    out: dict = {}
    block = None
    for raw in text.splitlines():
        line = raw.rstrip()
        if not line or line.lstrip().startswith("#"):
            continue
        if not line.startswith((" ", "\t")):
            block = None
            m = re.match(r"^(id|name|scope):\s*(.+?)\s*$", line)
            if m:
                out[m.group(1)] = scalar(m.group(2))
                continue
            m = re.match(r"^(fit_signals|avoid_signals):\s*$", line)
            if m:
                block = m.group(1)
                out[block] = {}
            continue
        if block and re.match(r"^\s{2}\S", line):
            m = re.match(r"^\s+([A-Za-z0-9_]+):\s*(.+?)\s*$", line)
            if m:
                out[block][m.group(1)] = scalar(m.group(2))
    return out


def load_catalog(knowledge_dir: Path) -> dict:
    styles_dir = knowledge_dir / "styles"
    if not styles_dir.is_dir():
        raise FileNotFoundError(f"카탈로그 없음: {styles_dir}")
    styles: dict = {}
    for f in sorted(styles_dir.glob("*.yaml")):
        data = _load_yaml(f)
        sid = data.get("id") or f.stem
        styles[sid] = {
            "id": sid,
            "name": data.get("name") or sid,
            "scope": data.get("scope") or "",
            "fit_signals": data.get("fit_signals") or {},
            "avoid_signals": data.get("avoid_signals") or {},
        }
    if not styles:
        raise FileNotFoundError(f"카탈로그가 비었다: {styles_dir}")
    return styles


def load_weights(knowledge_dir: Path) -> list:
    """index.yaml 의 weights / engine 임계값을 전역에 반영. 실패는 무시(기본값 유지)."""
    global W_FIT_EXACT, W_FIT_PARTIAL, W_AVOID, PRIMARY_MIN, SECONDARY_MIN
    idx = knowledge_dir / "index.yaml"
    if not idx.is_file():
        return [f"index.yaml 없음 — 가중치 기본값 사용: {idx}"]
    try:
        data = _load_yaml(idx)
        w = data.get("weights") or {}
        W_FIT_EXACT = int(w.get("fit_signal_match", W_FIT_EXACT))
        W_FIT_PARTIAL = int(w.get("fit_signal_partial", W_FIT_PARTIAL))
        W_AVOID = int(w.get("avoid_signal_match", W_AVOID))
        so = ((data.get("engine") or {}).get("structured_output") or {})
        PRIMARY_MIN = int(so.get("primary_min_score", PRIMARY_MIN))
        SECONDARY_MIN = int(so.get("secondary_min_score", SECONDARY_MIN))
        return []
    except Exception as exc:  # noqa: BLE001
        return [f"index.yaml 가중치 읽기 실패 — 기본값 사용: {exc}"]


# ── HTTP ─────────────────────────────────────────────────────────────────────
def ragaas_base() -> str:
    """RAGaaS API base. env RAGAAS_API → PLATFORM_API 에서 유추 → localhost."""
    api = (os.environ.get("RAGAAS_API") or "").strip().rstrip("/")
    if api:
        return api
    plat = (os.environ.get("PLATFORM_API") or "").strip().rstrip("/")
    if plat:
        # http://host:44300/platform/api/v2 → http://host:44300/ragaas/api/v2
        origin = re.sub(r"/platform/api/v\d+$", "", plat)
        if origin != plat:
            return origin + "/ragaas/api/v2"
    return DEFAULT_RAGAAS_API


def http_json(url: str, payload: dict | None = None) -> dict:
    data = None
    headers = {"Accept": "application/json"}
    if payload is not None:
        data = json.dumps(payload, ensure_ascii=False).encode("utf-8")
        headers["Content-Type"] = "application/json"
    req = urllib.request.Request(url, data=data, headers=headers,
                                 method="POST" if data else "GET")
    with urllib.request.urlopen(req, timeout=HTTP_TIMEOUT) as resp:  # noqa: S310
        return json.loads(resp.read().decode("utf-8"))


def norm_style_id(label: str) -> str:
    return re.sub(r"[\s_]+", "-", str(label).strip().lower())


# ── RAGaaS 경로 ──────────────────────────────────────────────────────────────
def signal_paths(describe: dict, kind: str) -> dict:
    """describe 응답 → {"fit_signals.team_size": {"small", ...}, ...} 허용값 집합.

    `in` 안에 목록에 없는 값이 하나라도 섞이면 RAGaaS 가 질의 전체를 zero 로
    돌려주므로, 조건을 만들기 전에 반드시 이 집합으로 걸러야 한다.
    """
    out: dict = {}
    for p in (describe.get("paths") or {}).get(kind) or []:
        key = p.get("key") or ""
        if not key.startswith(("fit_signals.", "avoid_signals.")):
            continue
        out[key] = {v.get("label") for v in (p.get("values") or []) if v.get("label")}
    return out


def build_conditions(req: dict, allowed: dict, cls: str) -> list:
    """NORM × 허용값 → (조건, 가중치종류) 목록. 조건 1개 = (path, value) 한 쌍."""
    conds = []
    for attr, nval in sorted(req.items()):
        path = f"{cls}.{attr}"
        for sval in sorted(allowed.get(path) or ()):
            kind = satisfies(sval, nval)
            if kind:
                conds.append({"path": path, "op": "eq", "value": sval,
                              "_kind": kind, "_attr": attr, "_norm": nval})
    return conds


def plan_query(base: str, kb: str, conds: list) -> dict:
    if not conds:
        return {"status": "zero", "entities": [], "total": 0, "elapsed_ms": 0}
    payload = {
        "kind": "ArchitectureStyle",
        "where": {"any": [{k: v for k, v in c.items() if not k.startswith("_")}
                          for c in conds]},
        "return": ["scope"],
        "evidence": "none",
        "limit": 50,
    }
    return http_json(f"{base}/knowledge-bases/{kb}/agent/plan", payload)


def score_via_ragaas(req: dict, base: str, kb: str) -> tuple:
    describe = http_json(f"{base}/knowledge-bases/{kb}/agent/describe?min_members=1")
    kinds = [k.get("name") for k in describe.get("kinds") or []]
    if "ArchitectureStyle" not in kinds:
        raise RuntimeError(f"KB {kb} 에 ArchitectureStyle kind 가 없다 (kinds={kinds})")
    allowed = signal_paths(describe, "ArchitectureStyle")
    if not allowed:
        raise RuntimeError(f"KB {kb} 에 fit_signals/avoid_signals 경로가 없다")

    members = []
    for k in describe.get("kinds") or []:
        if k.get("name") == "ArchitectureStyle":
            members = [norm_style_id(m) for m in (k.get("member_names") or [])]

    tally: dict = {}
    meta = {"kb": kb, "api": base, "schema_version": describe.get("schema_version"),
            "queries": [], "elapsed_ms": 0}
    warnings = []

    for cls in ("fit_signals", "avoid_signals"):
        conds = build_conditions(req, allowed, cls)
        batches = [conds[i:i + MAX_CONDITIONS]
                   for i in range(0, len(conds), MAX_CONDITIONS)] or [[]]
        for bi, batch in enumerate(batches):
            res = plan_query(base, kb, batch)
            meta["elapsed_ms"] += int(res.get("elapsed_ms") or 0)
            meta["queries"].append({
                "class": cls, "batch": bi + 1, "batches": len(batches),
                "status": res.get("status"), "conditions": len(batch),
                "matched_entities": res.get("total"),
            })
            if res.get("status") == "reject":
                raise RuntimeError(f"{cls} 조회 reject — {res.get('message')}")
            for z in res.get("zero_conditions") or []:
                warnings.append(f"{cls}: {z}")
            for ent in res.get("entities") or []:
                sid = norm_style_id(ent.get("label") or "")
                if not sid:
                    continue
                row = tally.setdefault(sid, {"fit_exact": 0, "fit_partial": 0,
                                             "avoid": 0, "breakdown": []})
                for ci in ent.get("matched_conditions") or []:
                    if not isinstance(ci, int) or ci >= len(batch):
                        continue
                    c = batch[ci]
                    if cls == "fit_signals":
                        row["fit_exact" if c["_kind"] == "exact" else "fit_partial"] += 1
                    else:
                        row["avoid"] += 1
                    row["breakdown"].append({
                        "class": cls, "attr": c["_attr"], "norm": c["_norm"],
                        "signal": c["value"], "match": c["_kind"],
                    })

    for sid in members:
        tally.setdefault(sid, {"fit_exact": 0, "fit_partial": 0,
                               "avoid": 0, "breakdown": []})
    return tally, meta, warnings


# ── YAML 경로 ────────────────────────────────────────────────────────────────
def score_via_yaml(req: dict, catalog: dict) -> dict:
    tally: dict = {}
    for sid, s in catalog.items():
        row = {"fit_exact": 0, "fit_partial": 0, "avoid": 0, "breakdown": []}
        for cls, key in (("fit_signals", "fit_signals"), ("avoid_signals", "avoid_signals")):
            for attr, sval in (s.get(key) or {}).items():
                nval = req.get(attr)
                if nval is None:
                    continue
                kind = satisfies(sval, nval)
                if not kind:
                    continue
                if cls == "fit_signals":
                    row["fit_exact" if kind == "exact" else "fit_partial"] += 1
                else:
                    row["avoid"] += 1
                row["breakdown"].append({"class": cls, "attr": attr, "norm": nval,
                                         "signal": str(sval), "match": kind})
        tally[sid] = row
    return tally


# ── 조립 ─────────────────────────────────────────────────────────────────────
def assemble(tally: dict, catalog: dict | None) -> dict:
    scored = []
    for sid, row in tally.items():
        meta = (catalog or {}).get(sid) or {}
        scored.append({
            "id": sid,
            "name": meta.get("name") or sid,
            "scope": meta.get("scope") or "",
            "score": score_of(row["fit_exact"], row["fit_partial"], row["avoid"]),
            "fit_exact": row["fit_exact"],
            "fit_partial": row["fit_partial"],
            "avoid": row["avoid"],
            "breakdown": row["breakdown"],
        })
    scored.sort(key=lambda r: (-r["score"], r["id"]))
    return {
        "scored": scored,
        "primary": [r["id"] for r in scored if r["score"] >= PRIMARY_MIN],
        "secondary": [r["id"] for r in scored
                      if SECONDARY_MIN <= r["score"] < PRIMARY_MIN],
        "supporting": [r["id"] for r in scored if 0 < r["score"] < SECONDARY_MIN],
    }


def catalog_summary(catalog: dict | None, tally: dict) -> dict:
    if not catalog:
        return {"styles": sorted(tally.keys()), "names": {}, "scopes": {},
                "fit_signals": {}, "avoid_signals": {}}
    return {
        "styles": sorted(catalog.keys()),
        "names": {k: v["name"] for k, v in catalog.items()},
        "scopes": {k: v["scope"] for k, v in catalog.items()},
        "fit_signals": {k: v["fit_signals"] for k, v in catalog.items()},
        "avoid_signals": {k: v["avoid_signals"] for k, v in catalog.items()},
    }


# ── 자가점검 ──────────────────────────────────────────────────────────────────
def selftest() -> int:
    """`--selftest` — 신호 충족 표와 축소 YAML 파서를 점검한다. 네트워크 불필요."""
    cases = [
        # (신호값, NORM값, 기대)
        ("small", "small", "exact"), ("small", "medium", ""),
        ("high", "high", "exact"), ("high", "medium", ""),
        ("high", "required", "partial"),      # required 는 high 보다 강하다
        ("required", "high", ""),             # 반대는 충족하지 않는다
        ("medium_or_higher", "medium", "partial"),
        ("medium_or_higher", "high", "partial"),
        ("medium_or_higher", "large", "partial"),
        ("medium_or_higher", "low", ""),
        ("medium_or_lower", "low", "partial"),
        ("medium_or_lower", "small", "partial"),
        ("medium_or_lower", "high", ""),
        ("medium", "high", ""),               # medium 적정 신호에 high 는 가점 없음
        ("low", "low", "exact"), ("low", "", ""),
    ]
    fails = []
    for sig, norm, want in cases:
        got = satisfies(sig, norm)
        if got != want:
            fails.append(f"satisfies({sig!r},{norm!r}) = {got!r}, 기대 {want!r}")

    # 축소 파서가 PyYAML 과 같은 신호를 내는지 — 끝주석·따옴표 처리 포함.
    kd = default_knowledge_dir()
    try:
        import yaml  # noqa: PLC0415
        for f in sorted((kd / "styles").glob("*.yaml")):
            text = f.read_text(encoding="utf-8")
            ref = yaml.safe_load(text) or {}
            mini = _load_yaml_mini(text)
            for cls in ("fit_signals", "avoid_signals"):
                a = {k: str(v) for k, v in (ref.get(cls) or {}).items()}
                b = {k: str(v) for k, v in (mini.get(cls) or {}).items()}
                if a != b:
                    fails.append(f"{f.name} {cls}: 축소파서={b} PyYAML={a}")
            if str(ref.get("name") or "") != str(mini.get("name") or ""):
                fails.append(f"{f.name} name: 축소파서={mini.get('name')!r} PyYAML={ref.get('name')!r}")
    except ImportError:
        print("PyYAML 없음 — 축소 파서 대조는 생략", file=sys.stderr)

    for f in fails:
        print("FAIL " + f, file=sys.stderr)
    print(f"selftest: {len(cases)} 충족판정 + 카탈로그 대조 — "
          + ("통과" if not fails else f"실패 {len(fails)}건"))
    return 1 if fails else 0


def main() -> int:
    if "--selftest" in sys.argv[1:]:
        return selftest()

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

    source = (payload.get("source") or "auto").strip().lower()
    if source not in ("auto", "ragaas", "yaml"):
        print(json.dumps({"error": "bad_source", "detail": source},
                         ensure_ascii=False), file=sys.stderr)
        return 2

    knowledge_dir = Path(payload.get("knowledge_dir") or default_knowledge_dir())
    warnings = load_weights(knowledge_dir)

    catalog = None
    try:
        catalog = load_catalog(knowledge_dir)
    except Exception as exc:  # noqa: BLE001 — 메타데이터용. ragaas 경로는 이것 없이도 된다.
        warnings.append(f"로컬 카탈로그 읽기 실패: {exc}")

    base = (payload.get("ragaas_api") or ragaas_base()).rstrip("/")
    kb = (payload.get("kb") or os.environ.get("RAGAAS_STYLE_KB") or DEFAULT_KB).strip()

    out: dict = {}
    errors = []

    if source in ("auto", "ragaas"):
        try:
            tally, meta, warns = score_via_ragaas(req, base, kb)
            out = {"source": "ragaas", "scoring": "signals_only", "ragaas": meta}
            warnings += warns
        except (urllib.error.URLError, urllib.error.HTTPError, TimeoutError, OSError) as exc:
            errors.append(f"ragaas 연결 실패 ({base}, kb={kb}): {exc}")
        except Exception as exc:  # noqa: BLE001
            errors.append(f"ragaas 조회 실패 ({base}, kb={kb}): {exc}")

    if not out and source in ("auto", "yaml"):
        if catalog:
            tally = score_via_yaml(req, catalog)
            out = {"source": "yaml", "scoring": "signals_only"}
        else:
            errors.append("yaml 경로 불가 — 카탈로그를 읽지 못했다")

    if not out:
        print(json.dumps({"error": "no_source_available", "detail": errors},
                         ensure_ascii=False, indent=2), file=sys.stderr)
        return 1

    if out["source"] == "yaml" and source == "auto":
        warnings.insert(0, "RAGaaS 조회 실패 → 로컬 카탈로그로 대체: " + "; ".join(errors))

    out.update(assemble(tally, catalog))
    out["catalog_summary"] = catalog_summary(catalog, tally)
    out["weights"] = {"fit_signal_match": W_FIT_EXACT,
                      "fit_signal_partial": W_FIT_PARTIAL,
                      "avoid_signal_match": W_AVOID,
                      "primary_min_score": PRIMARY_MIN,
                      "secondary_min_score": SECONDARY_MIN}
    out["warnings"] = warnings
    print(json.dumps(out, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    sys.exit(main())
