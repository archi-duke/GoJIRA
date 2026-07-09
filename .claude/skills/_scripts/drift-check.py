#!/usr/bin/env python3
"""drift-check.py — 워크플로우 단계 변경 진단 + deferredImpacts 관리.

각 단계의 핵심 필드를 정렬·정규화해 SHA256 해시로 baseline 을 저장하고,
재실행 시 baseline 과 비교해 변경 여부 + 하류 영향 단계를 보고한다.
LLM 호출 없음. archSettings 에 lastRunHash / lastRunAt / deferredImpacts 저장.

usage:
  drift-check.py <CODE> <USER> check
  drift-check.py <CODE> <USER> set-baseline [--stage <id>]
  drift-check.py <CODE> <USER> mark-deferred <stage> <reason...>
  drift-check.py <CODE> <USER> resolve-deferred <stage>

env:
  GOJIRA_API   required (launcher must inject)
"""
from __future__ import annotations

import hashlib
import json
import os
import sys
import urllib.error
import urllib.request
from datetime import datetime, timezone
from pathlib import Path

import issue_local


GRAPH_PATH = Path(__file__).resolve().parent / "workflow-graph.json"
GOJIRA_API = os.environ["GOJIRA_API"]


def http(method: str, url: str, headers=None, body=None):
    h = {"Content-Type": "application/json"}
    if headers:
        h.update(headers)
    data = json.dumps(body).encode("utf-8") if body is not None else None
    req = urllib.request.Request(url, data=data, headers=h, method=method)
    try:
        with urllib.request.urlopen(req, timeout=20) as resp:
            raw = resp.read().decode("utf-8") or "null"
            try:
                return json.loads(raw)
            except json.JSONDecodeError:
                return raw
    except urllib.error.HTTPError as e:
        return {"_error": f"HTTP {e.code}", "_body": e.read().decode("utf-8", errors="replace")[:500]}
    except Exception as exc:
        return {"_error": str(exc)}


def fetch_issues(code: str, user: str):
    """이슈는 로컬 clone data/issues/*.yaml 에서 읽는다 — 이슈 쓰기는 Mongo 를 건드리지
    않고 로컬에만 기록되고 Mongo 는 PR 머지 시점에만 갱신되므로, 세션 중 GET /Issue 로는
    방금 만든/수정한 이슈가 보이지 않아 drift 판정(baseline 해시)이 어긋난다."""
    return issue_local.load_local_issues()


def fetch_arch_settings(code: str, user: str):
    raw = http(
        "GET",
        f"{GOJIRA_API}/Project",
        headers={
            "XBody": json.dumps({"jiraCode": code}),
            "X-User-Id": user, "Authorization": "Bearer " + os.environ.get("GOJIRA_SESSION_TOKEN", ""),
        },
    ) or []
    if isinstance(raw, list) and raw:
        return raw[0].get("archSettings") or {}
    return {}


def patch_arch_settings(code: str, user: str, patch: dict):
    return http(
        "PATCH",
        f"{GOJIRA_API}/Project/{code}/archSettings",
        headers={"X-User-Id": user, "Authorization": "Bearer " + os.environ.get("GOJIRA_SESSION_TOKEN", "")},
        body=patch,
    )


def matches_filter(issue: dict, filt: dict | None) -> bool:
    if not filt:
        return True
    for k, v in filt.items():
        if issue.get(k) != v:
            return False
    return True


def matches_filter_not(issue: dict, filt: dict | None) -> bool:
    if not filt:
        return True
    for k, v in filt.items():
        if issue.get(k) == v:
            return False
    return True


# 정상 워크플로우 SKILL 들이 자동 추가하는 link 의 _addedBy 값 화이트리스트.
# 이 값들이 link 에 박혀있으면 hash 에서 제외 (false positive 차단).
# 사용자 수작업 link 는 _addedBy 없음 → 그대로 hash 에 포함되어 drift 감지.
_KNOWN_WORKFLOW_SOURCES = {
    "vos-collect",
    "usecase-analyze",
    "requirement-refine",
    "driver-select",
    "style-define",
    "design-decide",
    "design-integrate",
    "satam-analyze",
}


def normalize_links(links):
    if not isinstance(links, list):
        return []
    out = []
    for link in links:
        if not isinstance(link, dict):
            continue
        # 정상 워크플로우 SKILL 자동 생성 link 는 hash 에서 제외 (false positive 차단)
        added_by = link.get("_addedBy")
        if added_by and added_by in _KNOWN_WORKFLOW_SOURCES:
            continue
        out.append(
            {
                "in": ((link.get("inwardIssue") or {}).get("key")) or "",
                "out": ((link.get("outwardIssue") or {}).get("key")) or "",
                "type": ((link.get("type") or {}).get("name")) or "",
            }
        )
    out.sort(key=lambda x: (x["in"], x["out"], x["type"]))
    return out


def compute_stage_hash(stage: dict, all_issues: list, arch_settings: dict):
    types = stage.get("issueTypes") or []
    fields = stage.get("fields") or []
    filt = stage.get("filter") or {}
    filt_not = stage.get("filterNot") or {}
    arch_keys = stage.get("archSettingsKeys") or []
    link_fields = stage.get("linkFields") or []

    rows = []
    for it in all_issues:
        if types and it.get("issueType") not in types:
            continue
        if not matches_filter(it, filt):
            continue
        if not matches_filter_not(it, filt_not):
            continue
        row = {f: it.get(f) for f in fields}
        for lf in link_fields:
            row[lf] = normalize_links(it.get(lf))
        rows.append(row)

    rows.sort(key=lambda r: (r.get("key") or ""))
    arch_part = {k: arch_settings.get(k) for k in arch_keys}

    payload = {"rows": rows, "arch": arch_part}
    canonical = json.dumps(payload, sort_keys=True, ensure_ascii=False, default=str)
    return hashlib.sha256(canonical.encode("utf-8")).hexdigest(), len(rows)


def now_iso() -> str:
    return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def cmd_check(code: str, user: str) -> int:
    graph = json.loads(GRAPH_PATH.read_text(encoding="utf-8"))
    arch = fetch_arch_settings(code, user)
    issues = fetch_issues(code, user)
    if isinstance(issues, dict) and issues.get("_error"):
        print(f"ERROR fetching issues: {issues}")
        return 2

    last_hashes = arch.get("lastRunHash") or {}
    last_at = arch.get("lastRunAt") or {}
    deferred = arch.get("deferredImpacts") or []
    deps = graph["dependencies"]

    drifted_stages: list[str] = []
    no_baseline_stages: list[str] = []
    stage_status: dict[str, str] = {}

    print(f"=== Workflow Drift Check — {code} ===\n")
    print(f"{'stage':<20}{'status':<14}{'count':<8}{'last @':<22}note")
    print("-" * 100)

    for stage in graph["stages"]:
        sid = stage["id"]
        h, count = compute_stage_hash(stage, issues, arch)
        last_h = last_hashes.get(sid)
        if not last_h:
            status = "NO-BASELINE"
            note = "set-baseline 으로 저장 필요"
            no_baseline_stages.append(sid)
        elif last_h == h:
            status = "CLEAN"
            note = f"hash {h[:8]}"
        else:
            status = "DRIFT"
            note = f"baseline {last_h[:8]} → now {h[:8]}"
            drifted_stages.append(sid)
        stage_status[sid] = status
        at = last_at.get(sid, "-")
        print(f"{sid:<20}{status:<14}{count:<8}{at:<22}{note}")

    if drifted_stages:
        print()
        print("⚠ 변경 감지 — 하류 단계 영향 분석:")
        impacted: set[str] = set()

        def trace(sid: str):
            for child in deps.get(sid, []):
                if child not in impacted:
                    impacted.add(child)
                    trace(child)

        for s in drifted_stages:
            trace(s)

        already_drift = [s for s in impacted if stage_status.get(s) == "DRIFT"]
        clean_impacted = [
            s for s in impacted if stage_status.get(s) == "CLEAN"
        ]
        if already_drift:
            print(f"  · 이미 DRIFT: {', '.join(already_drift)}")
        if clean_impacted:
            print(
                f"  · CLEAN 이지만 영향 가능 (재실행/연기 결정 필요): "
                f"{', '.join(clean_impacted)}"
            )
        if not (already_drift or clean_impacted):
            print("  · 하류 단계 없음")
    elif no_baseline_stages:
        print(
            f"\nℹ baseline 미설정 단계 {len(no_baseline_stages)}건 — "
            f"`set-baseline` 으로 현재 상태를 저장한 뒤 다시 check 하세요."
        )
    else:
        print(
            "\n✓ 모든 단계 CLEAN — 이슈 변경 없음. "
            "문서/다이어그램만 재렌더 안전."
        )

    active_deferred = [d for d in deferred if not d.get("resolvedAt")]
    if active_deferred:
        print()
        print("📌 등록된 deferredImpacts (미해결):")
        for d in active_deferred:
            print(
                f"  - {d.get('stage')} (trigger={d.get('trigger', '?')}, "
                f"since={d.get('since', '?')}): {d.get('reason', '')}"
            )
    return 0


def cmd_set_baseline(code: str, user: str, only_stage: str | None) -> int:
    graph = json.loads(GRAPH_PATH.read_text(encoding="utf-8"))
    arch = fetch_arch_settings(code, user)
    issues = fetch_issues(code, user)
    if isinstance(issues, dict) and issues.get("_error"):
        print(f"ERROR fetching issues: {issues}")
        return 2

    new_hashes = dict(arch.get("lastRunHash") or {})
    new_at = dict(arch.get("lastRunAt") or {})
    now = now_iso()
    updated = []

    for stage in graph["stages"]:
        sid = stage["id"]
        if only_stage and sid != only_stage:
            continue
        h, count = compute_stage_hash(stage, issues, arch)
        new_hashes[sid] = h
        new_at[sid] = now
        updated.append((sid, h, count))

    res = patch_arch_settings(
        code, user, {"lastRunHash": new_hashes, "lastRunAt": new_at}
    )
    print(f"=== set-baseline — {code} ===")
    for sid, h, count in updated:
        print(f"  {sid:<20} hash={h[:12]}  items={count}")
    if isinstance(res, dict) and res.get("_error"):
        print(f"⚠ PATCH 실패: {res}")
        return 2
    print(f"PATCH ok ({json.dumps(res, ensure_ascii=False)[:120]}…)")
    return 0


def cmd_mark_deferred(code: str, user: str, stage: str, reason: str) -> int:
    arch = fetch_arch_settings(code, user)
    deferred = list(arch.get("deferredImpacts") or [])
    deferred = [
        d
        for d in deferred
        if not (d.get("stage") == stage and not d.get("resolvedAt"))
    ]
    deferred.append(
        {
            "stage": stage,
            "trigger": "manual",
            "since": now_iso()[:10],
            "reason": reason,
        }
    )
    res = patch_arch_settings(code, user, {"deferredImpacts": deferred})
    print(f"deferred 등록: {stage} — {reason}")
    if isinstance(res, dict) and res.get("_error"):
        print(f"⚠ PATCH 실패: {res}")
        return 2
    return 0


def cmd_resolve_deferred(code: str, user: str, stage: str) -> int:
    arch = fetch_arch_settings(code, user)
    deferred = list(arch.get("deferredImpacts") or [])
    now = now_iso()[:10]
    found = False
    for d in deferred:
        if d.get("stage") == stage and not d.get("resolvedAt"):
            d["resolvedAt"] = now
            found = True
    if not found:
        print(f"resolve 대상 없음: {stage}")
        return 1
    res = patch_arch_settings(code, user, {"deferredImpacts": deferred})
    print(f"resolved: {stage}")
    if isinstance(res, dict) and res.get("_error"):
        print(f"⚠ PATCH 실패: {res}")
        return 2
    return 0


def main() -> int:
    if len(sys.argv) < 4:
        print(__doc__)
        return 1
    code, user, sub = sys.argv[1], sys.argv[2], sys.argv[3]
    args = sys.argv[4:]

    if sub == "check":
        return cmd_check(code, user)
    if sub == "set-baseline":
        only = None
        if "--stage" in args:
            i = args.index("--stage")
            only = args[i + 1] if i + 1 < len(args) else None
        return cmd_set_baseline(code, user, only)
    if sub == "mark-deferred":
        if len(args) < 2:
            print("usage: ... mark-deferred <stage> <reason...>")
            return 1
        return cmd_mark_deferred(code, user, args[0], " ".join(args[1:]))
    if sub == "resolve-deferred":
        if len(args) < 1:
            print("usage: ... resolve-deferred <stage>")
            return 1
        return cmd_resolve_deferred(code, user, args[0])

    print(f"unknown subcommand: {sub}")
    return 1


if __name__ == "__main__":
    sys.exit(main())
