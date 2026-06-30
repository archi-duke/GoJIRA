#!/usr/bin/env python3
"""design-decide-assemble.py — design-decide2 결정적 문서 조립 (LLM 0회).

ADSN/DDSN 이슈(designPoint != "시스템 스타일 결정") + archSettings.designDecideQAWeights
만으로 design-decisions.md (index) + design-decisions/DP-NN-{slug}.md (DP별) 생성.

DP 그룹핑: 이슈의 designPoint 필드. DP-NN 번호 = 그룹 내 가장 작은 subkey 의 alphabetical
순서로 안정 정렬.

입력:
  --from-mongo CODE [--user USER]
  --out-dir DIR (선택, 미지정 시 stdout 으로 index 만)
"""
from __future__ import annotations

import argparse
import json
import os
import re
import sys
import urllib.error
import urllib.request
from pathlib import Path


def slugify(name: str) -> str:
    s = re.sub(r"[^0-9A-Za-z가-힣+\-]+", "-", name).strip("-").lower()
    return s[:60] if s else "dp"


def http_get_json(url: str, user: str) -> dict:
    req = urllib.request.Request(url, headers={"X-User-Id": user, "Authorization": "Bearer " + os.environ.get("GOJIRA_SESSION_TOKEN", "")})
    with urllib.request.urlopen(req, timeout=10) as resp:
        return json.loads(resp.read().decode("utf-8"))


def http_get_issues(api: str, code: str, user: str) -> list:
    xbody = json.dumps({"prjID": code, "infoType": ""})
    req = urllib.request.Request(
        f"{api}/Issue", headers={"X-User-Id": user, "Authorization": "Bearer " + os.environ.get("GOJIRA_SESSION_TOKEN", ""), "XBody": xbody}
    )
    with urllib.request.urlopen(req, timeout=15) as resp:
        return json.loads(resp.read().decode("utf-8"))


def fetch_inputs(code: str, user: str, api: str) -> tuple[list, dict]:
    """ADSN/DDSN 이슈 (스타일 제외) + archSettings 반환."""
    issues = http_get_issues(api, code, user)
    design_issues = [
        i for i in issues
        if i.get("issueType") in ("Architectural Design", "Detailed Design")
        and (i.get("designPoint") or "") != "시스템 스타일 결정"
        and (i.get("designPoint") or "")  # designPoint 비어있으면 제외
    ]
    design_issues.sort(key=lambda x: x.get("subkey") or x.get("key") or "")

    pxbody = json.dumps({"jiraCode": code})
    preq = urllib.request.Request(
        f"{api}/Project", headers={"X-User-Id": user, "Authorization": "Bearer " + os.environ.get("GOJIRA_SESSION_TOKEN", ""), "XBody": pxbody}
    )
    parr = json.loads(urllib.request.urlopen(preq, timeout=10).read().decode("utf-8"))
    arch = (parr[0].get("archSettings") if parr else {}) or {}

    return design_issues, arch


def group_by_designpoint(issues: list) -> list[tuple[str, list]]:
    """designPoint 별 그룹. 그룹 내 정렬은 subkey 오름차순. 그룹 순서는 그룹 내
    최소 subkey 의 알파벳순으로 안정 정렬."""
    groups: dict[str, list] = {}
    for i in issues:
        dp = i.get("designPoint") or "(미지정)"
        groups.setdefault(dp, []).append(i)
    # 그룹 정렬: 내부 subkey 오름차순
    for dp, lst in groups.items():
        lst.sort(key=lambda x: x.get("subkey") or x.get("key") or "")
    # 그룹 순서: 각 그룹의 첫 subkey 로 정렬
    return sorted(groups.items(), key=lambda kv: kv[1][0].get("subkey") or "")


def get_dp_qa_weights(arch: dict, design_point: str) -> dict | None:
    """archSettings.designDecideQAWeights 는 [{designPoint, qaWeights}, ...] 형태."""
    arr = arch.get("designDecideQAWeights") or []
    if not isinstance(arr, list):
        return None
    for entry in arr:
        if isinstance(entry, dict) and entry.get("designPoint") == design_point:
            return entry.get("qaWeights")
    return None


# ─── index 빌더 ----------------------------------------------------------------

def build_index(grouped: list[tuple[str, list]], arch: dict) -> str:
    lines: list[str] = []
    lines.append("# 설계 결정 Index")
    lines.append("")
    lines.append("## 개요")
    lines.append("")
    lines.append(
        "이 문서는 프로젝트의 Architectural / Detailed Design Decision 전체 목차이며, "
        "각 Design Point 의 상세는 `design-decisions/DP-NN-*.md` 에 수록됩니다."
    )
    lines.append("")

    # Section 1 DP 목록
    lines.append("## 1. Design Point 목록")
    lines.append("")
    lines.append("| DP | 주제 | designLevel | 후보 수 | 선정 | 상세 |")
    lines.append("|---|---|---|---|---|---|")
    for n, (dp_name, group) in enumerate(grouped, start=1):
        num = f"{n:02d}"
        slug = slugify(dp_name)
        first = group[0]
        level = first.get("designLevel") or "Architectural"
        candidates = len(group)
        chosen = [g for g in group if (g.get("decision") or "").strip()]
        chosen_str = "Mix" if len(chosen) > 1 else (chosen[0].get("subkey") or chosen[0].get("key") or "?") if chosen else "—"
        link = f"[DP-{num}](./design-decisions/DP-{num}-{slug}.md)"
        lines.append(f"| DP-{num} | {dp_name} | {level} | {candidates} | {chosen_str} | {link} |")
    lines.append("")

    # Section 2 Driver 추적성
    lines.append("## 2. Driver 추적성 매트릭스")
    lines.append("")
    driver_to_dps: dict[str, list[str]] = {}
    for n, (dp_name, group) in enumerate(grouped, start=1):
        chosen = [g for g in group if (g.get("decision") or "").strip()]
        for c in chosen:
            for ln in c.get("issueLinks") or []:
                if ((ln.get("type") or {}).get("name")) != "Contains(WBSGantt)":
                    continue
                d_key = ((ln.get("inwardIssue") or {}).get("key")) or ""
                if not d_key:
                    continue
                driver_to_dps.setdefault(d_key, []).append(f"DP-{n:02d}")
    if driver_to_dps:
        lines.append("| Driver | 담당 DP | 비고 |")
        lines.append("|---|---|---|")
        for d_key, dps in sorted(driver_to_dps.items()):
            lines.append(f"| {d_key} | {', '.join(sorted(set(dps)))} | (Contains 링크) |")
    else:
        lines.append("_(Contains 링크된 Driver 없음)_")
    lines.append("")

    # Section 3 QA 가중치 요약
    lines.append("## 3. QA 가중치 요약")
    lines.append("")
    lines.append("DP 별 QA 가중치는 각 DP 파일 내부에 기록. 여기는 프로젝트 전반 경향만 요약.")
    lines.append("")
    qa_freq: dict[str, int] = {}
    for _, (dp_name, _) in enumerate(grouped):
        weights = get_dp_qa_weights(arch, dp_name) or {}
        # 최고 가중 QA 카운트
        if weights:
            top_qa = max(weights.items(), key=lambda kv: float((kv[1] or {}).get("weight", 0) or 0))[0]
            qa_freq[top_qa] = qa_freq.get(top_qa, 0) + 1
    if qa_freq:
        lines.append("- **DP 별 최고 가중 QA 분포**:")
        for qa, n in sorted(qa_freq.items(), key=lambda kv: -kv[1]):
            lines.append(f"  - {qa}: {n} DP")
    else:
        lines.append("_(QA 가중치 미저장 — full mode 1회 실행 필요)_")
    lines.append("")

    # Section 4 리스크 요약
    lines.append("## 4. 리스크 요약")
    lines.append("")
    lines.append("| DP | 주요 리스크 | 영향 QA |")
    lines.append("|---|---|---|")
    has_risk = False
    for n, (dp_name, group) in enumerate(grouped, start=1):
        chosen = [g for g in group if (g.get("decision") or "").strip()]
        for c in chosen:
            risk = (c.get("risk") or "").strip()
            if not risk:
                continue
            has_risk = True
            risk_short = risk.replace("\n", " ")[:80]
            lines.append(f"| DP-{n:02d} | {risk_short} | (SATAM 단계에서 매핑) |")
    if not has_risk:
        lines.append("| — | _(SATAM 미실행 또는 risk 미기록)_ | — |")

    return "\n".join(lines) + "\n"


# ─── DP 파일 빌더 -------------------------------------------------------------

def build_dp_file(dp_name: str, group: list, arch: dict, dp_num: int) -> str:
    lines: list[str] = []
    num = f"{dp_num:02d}"
    lines.append(f"# DP-{num}: {dp_name}")
    lines.append("")
    lines.append("> 상위 index: [../design-decisions.md](../design-decisions.md)")
    lines.append("")

    first = group[0]
    chosen = [g for g in group if (g.get("decision") or "").strip()]

    # 1 Design Objective
    lines.append("## 1. Design Objective")
    lines.append("")
    lines.append((first.get("objectives") or "").strip() or "_(미작성)_")
    lines.append("")
    lines.append(f"- **designLevel**: `{first.get('designLevel') or 'Architectural'}`")
    if first.get("dmodel"):
        lines.append(f"- **적용 영역**: {first.get('dmodel')}")
    # 관련 Driver
    drivers: list[str] = []
    if chosen:
        for c in chosen:
            for ln in c.get("issueLinks") or []:
                if ((ln.get("type") or {}).get("name")) == "Contains(WBSGantt)":
                    d_key = ((ln.get("inwardIssue") or {}).get("key")) or ""
                    if d_key and d_key not in drivers:
                        drivers.append(d_key)
    if drivers:
        lines.append(f"- **관련 Driver/Requirement**: {', '.join(drivers)}")
    lines.append("")

    # 2 QA 가중치
    lines.append("## 2. QA 가중치")
    lines.append("")
    weights = get_dp_qa_weights(arch, dp_name) or {}
    if weights:
        lines.append("| QA | Weight | 선정 근거 |")
        lines.append("|---|---|---|")
        total = 0.0
        for qa, info in weights.items():
            if not isinstance(info, dict):
                continue
            w = info.get("weight", 0)
            r = info.get("rationale", "")
            try:
                total += float(w)
            except (TypeError, ValueError):
                pass
            lines.append(f"| {qa} | {w} | {r} |")
        lines.append(f"| **합계** | **{round(total, 2)}** | |")
    else:
        lines.append("_(archSettings.designDecideQAWeights 미저장)_")
    lines.append("")

    # 3 후보 대안
    lines.append("## 3. 후보 대안 비교")
    lines.append("")
    for idx, cand in enumerate(group, start=1):
        subkey = cand.get("subkey") or cand.get("key") or "?"
        # candidate name: summary 가 "DP명: NAME" → NAME 추출
        cand_name = _candidate_name(cand, dp_name)
        is_chosen = bool((cand.get("decision") or "").strip())
        marker = " ✅" if is_chosen else ""
        lines.append(f"### 3.{idx} {subkey}: {cand_name}{marker}")
        lines.append("")
        lines.append(f"- **ADSN 키**: {cand.get('key','?')}")
        lines.append("")
        # Structural
        lines.append("**Structural View (정적)**")
        lines.append("")
        adiagram = (cand.get("adiagram") or "").strip()
        if adiagram:
            lines.append("```plantuml")
            lines.append(adiagram)
            lines.append("```")
        else:
            lines.append("_(미작성)_")
        lines.append("")
        # Dynamic
        lines.append("**Dynamic View (동적)**")
        lines.append("")
        bdiagram = (cand.get("bdiagram") or "").strip()
        if bdiagram:
            lines.append("```plantuml")
            lines.append(bdiagram)
            lines.append("```")
        else:
            lines.append("_(미작성)_")
        lines.append("")
        # Pros / Cons
        lines.append(f"**Pros**: {(cand.get('pros') or '').strip() or '_(없음)_'}")
        lines.append("")
        lines.append(f"**Cons**: {(cand.get('cons') or '').strip() or '_(없음)_'}")
        lines.append("")
        # Score / eval
        lines.append(f"**가중 점수**: {cand.get('score') or '-'}")
        lines.append("")
        eval_text = (cand.get("eval") or "").strip()
        if eval_text:
            lines.append(eval_text)
            lines.append("")
        lines.append("---")
        lines.append("")
    while lines and lines[-1] in ("", "---"):
        lines.pop()
    lines.append("")

    # 4 최종 선정
    lines.append("## 4. 최종 선정")
    lines.append("")
    if not chosen:
        lines.append("_(선정 후보 없음)_")
    else:
        if len(chosen) > 1:
            keys = ", ".join(c.get("subkey") or "?" for c in chosen)
            lines.append(f"**Mix 채택**: {keys}")
        else:
            c = chosen[0]
            lines.append(f"**선정**: {c.get('subkey') or '?'} — {_candidate_name(c, dp_name)}")
        lines.append("")
        for c in chosen:
            rationale = (c.get("decisionRationale") or "").strip()
            if rationale:
                lines.append(f"**근거** ({c.get('subkey') or '?'}): {rationale}")
                lines.append("")
            decision = (c.get("decision") or "").strip()
            if decision:
                lines.append(f"**결정 노트** ({c.get('subkey') or '?'}): {decision}")
                lines.append("")
    lines.append("")

    # 5 트레이드오프
    lines.append("## 5. 트레이드오프")
    lines.append("")
    if chosen:
        any_t = False
        for c in chosen:
            t = (c.get("tradeoffs") or "").strip()
            if t:
                any_t = True
                lines.append(f"- **{c.get('subkey') or '?'}**: {t}")
        if not any_t:
            lines.append("_(기록 없음)_")
    else:
        lines.append("_(선정 후보 없음)_")
    lines.append("")

    return "\n".join(lines)


def _candidate_name(cand: dict, dp_name: str) -> str:
    """summary 가 'DP명: NAME' 형태 → NAME. 아니면 그대로."""
    s = (cand.get("summary") or "").strip()
    prefix = f"{dp_name}:"
    if s.startswith(prefix):
        return s[len(prefix):].strip()
    return s or (cand.get("subkey") or "")


# ─── main ----------------------------------------------------------------------

def main() -> int:
    p = argparse.ArgumentParser()
    p.add_argument("--from-mongo", dest="code", metavar="CODE", required=True)
    p.add_argument("--user", default=os.environ.get("USER", "system"))
    p.add_argument("--api", default=os.environ["GOJIRA_API"])
    p.add_argument("--out-dir", default=None)
    args = p.parse_args()

    try:
        issues, arch = fetch_inputs(args.code, args.user, args.api)
    except urllib.error.HTTPError as e:
        print(f"ERROR: HTTP {e.code} — {e.reason}", file=sys.stderr)
        return 4
    except urllib.error.URLError as e:
        print(f"ERROR: connect — {e}", file=sys.stderr)
        return 4

    if not issues:
        print("ERROR: 설계 결정 ADSN/DDSN 0건 — full mode 1회 실행 필요", file=sys.stderr)
        return 3

    grouped = group_by_designpoint(issues)
    index_md = build_index(grouped, arch)

    if args.out_dir:
        outdir = Path(args.out_dir)
        outdir.mkdir(parents=True, exist_ok=True)
        (outdir / "design-decisions.md").write_text(index_md, encoding="utf-8")
        sub = outdir / "design-decisions"
        sub.mkdir(exist_ok=True)
        for n, (dp_name, group) in enumerate(grouped, start=1):
            num = f"{n:02d}"
            slug = slugify(dp_name)
            body = build_dp_file(dp_name, group, arch, n)
            (sub / f"DP-{num}-{slug}.md").write_text(body, encoding="utf-8")
        print(f"saved index + {len(grouped)} DP files to {outdir}")
    else:
        sys.stdout.write(index_md)
    return 0


if __name__ == "__main__":
    sys.exit(main())
