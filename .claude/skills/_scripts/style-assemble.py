#!/usr/bin/env python3
"""style-assemble.py — style-define2 결정적 문서 조립 (LLM 0회).

ADSN 이슈(designPoint="시스템 스타일 결정") + archSettings 만으로 architecture-style.md (index)
+ architecture-style/{subkey}-{slug}.md (후보별 상세) 를 생성한다.

입력 모드 (택1):
  (a) --from-mongo CODE [--user USER]: GoJIRA-API 에서 ADSN 이슈 + archSettings fetch
  (b) --draft PATH: 기존 draft.json (구버전 호환, 점진 deprecation)

출력:
  --out-dir DIR: index 와 후보 파일을 디렉토리에 저장 (DIR/architecture-style.md + DIR/architecture-style/<subkey>-<slug>.md)
  --print-index: index 본문만 stdout (단일 검증용)

각 후보 ADSN 의 approaches 필드는 모든 후보의 composition + score 를 담은 JSON 문자열.
파싱해 "이 후보의 composition" 을 찾아 사용.
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
    return s[:60] if s else "adsn"


def http_get_json(url: str, user: str) -> dict:
    req = urllib.request.Request(url, headers={"X-User-Id": user})
    with urllib.request.urlopen(req, timeout=10) as resp:
        return json.loads(resp.read().decode("utf-8"))


def http_get_issues(api: str, code: str, user: str) -> list:
    xbody = json.dumps({"prjID": code, "infoType": ""})
    req = urllib.request.Request(
        f"{api}/Issue", headers={"X-User-Id": user, "XBody": xbody}
    )
    with urllib.request.urlopen(req, timeout=15) as resp:
        return json.loads(resp.read().decode("utf-8"))


def fetch_inputs(code: str, user: str, api: str) -> tuple[list, dict]:
    """ADSN(스타일 결정) 이슈 목록 + archSettings 반환."""
    issues = http_get_issues(api, code, user)
    style_adsns = [
        i for i in issues
        if i.get("issueType") == "Architectural Design"
        and (i.get("designPoint") or "") == "시스템 스타일 결정"
    ]

    # subkey 오름차순 안정 정렬 (ADSN_001, ADSN_002, ...)
    style_adsns.sort(key=lambda x: x.get("subkey") or x.get("key") or "")

    # archSettings 가져오기 — Project GET (XBody)
    pxbody = json.dumps({"jiraCode": code})
    preq = urllib.request.Request(
        f"{api}/Project", headers={"X-User-Id": user, "XBody": pxbody}
    )
    parr = json.loads(urllib.request.urlopen(preq, timeout=10).read().decode("utf-8"))
    arch = (parr[0].get("archSettings") if parr else {}) or {}

    return style_adsns, arch


# ─── index 본문 빌더 -------------------------------------------------------------

def build_index(style_adsns: list, arch: dict) -> str:
    lines: list[str] = []
    lines.append("# 시스템 스타일 결정 (Integrated Architecture Style)")
    lines.append("")
    lines.append("## 개요")
    lines.append("")
    lines.append(
        "프로젝트의 시스템 스타일은 2~3개 통합 스타일 후보 (Primary + Secondary + Supporting "
        "조합) 를 QA(NFR) 가중 평가로 비교하여 최종 선정합니다."
    )
    lines.append("")

    # Section 1 후보 표
    lines.append("## 1. 후보 조합 스타일")
    lines.append("")
    lines.append("| 후보 | Primary | Secondary | Supporting | 총점 | 선정 | 상세 |")
    lines.append("|---|---|---|---|---|---|---|")
    for adsn in style_adsns:
        subkey = adsn.get("subkey") or adsn.get("key") or "?"
        name = _candidate_name(adsn)
        comp = _composition_for(adsn)
        prim = _join_styles(comp.get("primary"))
        sec  = _join_styles(comp.get("secondary"))
        supp = _join_styles(comp.get("supporting"))
        score = adsn.get("score") or "-"
        chosen = "✓" if (adsn.get("decision") or "").strip() else ""
        slug = slugify(name)
        link = f"[{subkey}](./architecture-style/{subkey}-{slug}.md)"
        lines.append(f"| {subkey} | {prim} | {sec} | {supp} | {score} | {chosen} | {link} |")
    lines.append("")

    # Section 2 QA 가중치
    lines.append("## 2. QA 가중치")
    lines.append("")
    qaw = arch.get("styleQAWeights") or {}
    if qaw:
        lines.append("| QA | Weight | 가중치 근거 |")
        lines.append("|---|---|---|")
        total = 0.0
        for qa, info in qaw.items():
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
        lines.append("_(archSettings.styleQAWeights 미저장 — full mode 1회 실행 필요)_")
    lines.append("")

    # Section 3 최종 선정
    lines.append("## 3. 최종 선정")
    lines.append("")
    chosen_list = [a for a in style_adsns if (a.get("decision") or "").strip()]
    if not chosen_list:
        lines.append("_(선정 ADSN 없음)_")
    else:
        lines.append(f"**채택 ADSN 키**: {', '.join(a.get('key','?') for a in chosen_list)}")
        lines.append("")
        for a in chosen_list:
            subkey = a.get("subkey") or "?"
            name = _candidate_name(a)
            lines.append(f"- **{subkey} — {name}**")
            rationale = (a.get("decisionRationale") or "").strip()
            if rationale:
                lines.append(f"  - 근거: {rationale}")
            decision = (a.get("decision") or "").strip()
            if decision:
                lines.append(f"  - 결정: {decision}")
    lines.append("")

    # Section 4 Driver 추적성
    lines.append("## 4. Driver 추적성 매트릭스")
    lines.append("")
    drivers = _collect_drivers(style_adsns)
    if drivers:
        lines.append("| Driver | 담당 ADSN | 역할 |")
        lines.append("|---|---|---|")
        for d_key, adsn_keys in drivers.items():
            lines.append(f"| {d_key} | {', '.join(adsn_keys)} | (Contains 링크) |")
    else:
        lines.append("_(Contains 링크된 Driver 없음)_")
    lines.append("")

    # Section 5 트레이드오프
    lines.append("## 5. 트레이드오프")
    lines.append("")
    tradeoff_lines = []
    for a in chosen_list:
        t = (a.get("tradeoffs") or "").strip()
        if t:
            subkey = a.get("subkey") or "?"
            tradeoff_lines.append(f"- **{subkey}**: {t}")
    if tradeoff_lines:
        lines.extend(tradeoff_lines)
    else:
        lines.append("_(기록 없음)_")

    return "\n".join(lines) + "\n"


# ─── 후보별 상세 빌더 -----------------------------------------------------------

def build_candidate(adsn: dict) -> str:
    subkey = adsn.get("subkey") or adsn.get("key") or "?"
    name = _candidate_name(adsn)
    comp = _composition_for(adsn)
    chosen = bool((adsn.get("decision") or "").strip())

    lines: list[str] = []
    lines.append(f"# {subkey}: {name}")
    lines.append("")
    lines.append("> 상위 index: [../architecture-style.md](../architecture-style.md)")
    lines.append("")

    # 1 조합 구성
    lines.append("## 1. 조합 구성")
    lines.append("")
    lines.append("| 역할 | 스타일 이름 | 스타일 ID | 조합 내 역할 |")
    lines.append("|---|---|---|---|")
    for role_label, role_key in (("Primary", "primary"), ("Secondary", "secondary"), ("Supporting", "supporting")):
        for s in comp.get(role_key) or []:
            lines.append(f"| {role_label} | {s.get('name','?')} | {s.get('id','?')} | {s.get('role','')} |")
    lines.append("")

    # 2 Design Objective
    lines.append("## 2. Design Objective")
    lines.append("")
    obj = (adsn.get("objectives") or "").strip()
    lines.append(obj if obj else "_(미작성)_")
    lines.append("")

    # 3 Structural
    lines.append("## 3. Structural View (정적)")
    lines.append("")
    adiagram = (adsn.get("adiagram") or "").strip()
    if adiagram:
        lines.append("```plantuml")
        lines.append(adiagram)
        lines.append("```")
    else:
        lines.append("_(미작성 — critical)_")
    lines.append("")

    # 4 Dynamic
    lines.append("## 4. Dynamic View (동적)")
    lines.append("")
    bdiagram = (adsn.get("bdiagram") or "").strip()
    if bdiagram:
        lines.append("```plantuml")
        lines.append(bdiagram)
        lines.append("```")
    else:
        lines.append("_(미작성)_")
    lines.append("")

    # 5 Pros / Cons
    lines.append("## 5. Pros / Cons")
    lines.append("")
    lines.append(f"**Pros**: {(adsn.get('pros') or '').strip() or '_(없음)_'}")
    lines.append("")
    lines.append(f"**Cons**: {(adsn.get('cons') or '').strip() or '_(없음)_'}")
    lines.append("")

    # 6 QA 평가
    lines.append("## 6. QA 평가")
    lines.append("")
    score = adsn.get("score") or "-"
    eval_text = (adsn.get("eval") or "").strip()
    lines.append(f"**총점 (가중합)**: {score}")
    lines.append("")
    lines.append(eval_text if eval_text else "_(평가 narrative 미작성)_")
    lines.append("")

    # 7 트레이드오프 (chosen 만)
    if chosen:
        lines.append("## 7. 트레이드오프")
        lines.append("")
        lines.append((adsn.get("tradeoffs") or "").strip() or "_(없음)_")
        lines.append("")

    # 8 선정 여부
    lines.append("## 8. 선정 여부")
    lines.append("")
    if chosen:
        lines.append("✅ **선정됨**")
        lines.append("")
        rationale = (adsn.get("decisionRationale") or "").strip()
        if rationale:
            lines.append(f"**근거**: {rationale}")
            lines.append("")
        decision = (adsn.get("decision") or "").strip()
        if decision:
            lines.append(f"**결정 노트**: {decision}")
    else:
        lines.append("❌ 미선정")

    return "\n".join(lines) + "\n"


# ─── 헬퍼 ----------------------------------------------------------------------

def _candidate_name(adsn: dict) -> str:
    """summary 가 '시스템 스타일 조합: NAME' 형태 → NAME 추출. 아니면 그대로."""
    s = (adsn.get("summary") or "").strip()
    m = re.match(r"^시스템 스타일 조합:\s*(.+)$", s)
    return m.group(1).strip() if m else s or (adsn.get("subkey") or "")


def _composition_for(adsn: dict) -> dict:
    """approaches 필드(JSON 문자열) 에서 이 후보의 composition 찾기.

    approaches 는 모든 후보 정보를 담고 있어 candidateId / name 으로 매칭 시도."""
    approaches_raw = adsn.get("approaches") or ""
    name = _candidate_name(adsn)
    if not approaches_raw:
        return {}
    try:
        all_cands = json.loads(approaches_raw)
    except (json.JSONDecodeError, TypeError):
        return {}
    if not isinstance(all_cands, list):
        return {}
    # 1) name 일치
    for c in all_cands:
        if isinstance(c, dict) and c.get("name") == name:
            return c.get("composition") or {}
    # 2) summary 의 docId 패턴 (description 에 'ADSN-N — ...' 형태)
    desc = adsn.get("description") or ""
    m = re.match(r"^(ADSN-\d+)\b", desc.strip())
    if m:
        target = m.group(1)
        for c in all_cands:
            if isinstance(c, dict) and c.get("docId") == target:
                return c.get("composition") or {}
    return {}


def _join_styles(items: list | None) -> str:
    if not items:
        return "—"
    parts = []
    for s in items:
        if isinstance(s, dict):
            parts.append(s.get("name") or s.get("id") or "?")
        else:
            parts.append(str(s))
    return " + ".join(parts)


def _collect_drivers(adsns: list) -> dict[str, list[str]]:
    """chosen ADSN 의 issueLinks 에서 Contains 의 inwardIssue.key (Driver/Requirement) 수집."""
    out: dict[str, list[str]] = {}
    for a in adsns:
        if not (a.get("decision") or "").strip():
            continue
        sub = a.get("subkey") or a.get("key") or "?"
        for ln in a.get("issueLinks") or []:
            t = ((ln.get("type") or {}).get("name")) or ""
            if t != "Contains(WBSGantt)":
                continue
            d_key = ((ln.get("inwardIssue") or {}).get("key")) or ""
            if not d_key:
                continue
            out.setdefault(d_key, []).append(sub)
    return out


# ─── main ----------------------------------------------------------------------

def main() -> int:
    p = argparse.ArgumentParser()
    p.add_argument("--from-mongo", dest="code", metavar="CODE",
                   help="GoJIRA-API 에서 ADSN + archSettings fetch")
    p.add_argument("--user", default=os.environ.get("USER", "system"))
    p.add_argument("--api", default=os.environ["GOJIRA_API"])
    p.add_argument("--out-dir", default=None,
                   help="저장 디렉토리 — index + per-candidate. 미지정 시 stdout (index 만)")
    args = p.parse_args()

    if not args.code:
        print("ERROR: --from-mongo CODE required", file=sys.stderr)
        return 2

    try:
        adsns, arch = fetch_inputs(args.code, args.user, args.api)
    except urllib.error.HTTPError as e:
        print(f"ERROR: HTTP {e.code} — {e.reason}", file=sys.stderr)
        return 4
    except urllib.error.URLError as e:
        print(f"ERROR: connect — {e}", file=sys.stderr)
        return 4

    if not adsns:
        print("ERROR: 시스템 스타일 ADSN 0건 — full mode 1회 실행 필요", file=sys.stderr)
        return 3

    index_md = build_index(adsns, arch)

    if args.out_dir:
        outdir = Path(args.out_dir)
        outdir.mkdir(parents=True, exist_ok=True)
        idxpath = outdir / "architecture-style.md"
        idxpath.write_text(index_md, encoding="utf-8")
        sub_dir = outdir / "architecture-style"
        sub_dir.mkdir(exist_ok=True)
        for a in adsns:
            subkey = a.get("subkey") or a.get("key") or "?"
            name = _candidate_name(a)
            slug = slugify(name)
            (sub_dir / f"{subkey}-{slug}.md").write_text(build_candidate(a), encoding="utf-8")
        print(f"saved index + {len(adsns)} candidate files to {outdir}")
    else:
        sys.stdout.write(index_md)

    return 0


if __name__ == "__main__":
    sys.exit(main())
