#!/usr/bin/env python3
"""requirement-assemble.py — requirement-refine 결정적 문서 조립 (LLM 0회).

Requirement 이슈 (FR/NFR/CNST) + issueLinks (관련 Feature) 만으로 business/requirements.md 생성.
archSettings 보조 데이터 없음.

입력:
  --from-mongo CODE [--user USER]
출력:
  stdout: requirements.md 본문 (단일 파일)
  --out PATH: 파일로 저장
"""
from __future__ import annotations

import argparse
import json
import os
import sys
import urllib.error
import urllib.request

import issue_local


def load_issues() -> list:
    """이슈는 로컬 clone data/issues/*.yaml 에서 읽는다 — 이슈 쓰기는 Mongo 를
    건드리지 않고 로컬에만 기록되고 Mongo 는 PR 머지 시점에만 갱신되므로, 세션 중
    GET /Issue 로는 방금 만든 이슈가 보이지 않는다."""
    return issue_local.load_local_issues()


def http_get_project(api: str, code: str, user: str) -> dict:
    xbody = json.dumps({"jiraCode": code})
    req = urllib.request.Request(
        f"{api}/Project", headers={"X-User-Id": user, "Authorization": "Bearer " + os.environ.get("GOJIRA_SESSION_TOKEN", ""), "XBody": xbody}
    )
    arr = json.loads(urllib.request.urlopen(req, timeout=10).read().decode("utf-8"))
    return arr[0] if arr else {}


def related_features(req: dict, feature_keys: set) -> list[str]:
    """이슈의 issueLinks 에서 Contains 의 outwardIssue/inwardIssue 중 Feature 인 것 추출.

    Requirement 가 outward(in chain VOS->Feature->Requirement), VOS/Feature 가 inward.
    실제로는 Feature 가 inward (Feature contains Requirement) 일 수도, outward 일 수도 있어
    양쪽 다 검사."""
    out: list[str] = []
    for ln in req.get("issueLinks") or []:
        t = ((ln.get("type") or {}).get("name")) or ""
        if t != "Contains(WBSGantt)":
            continue
        inward = ((ln.get("inwardIssue") or {}).get("key")) or ""
        outward = ((ln.get("outwardIssue") or {}).get("key")) or ""
        for k in (inward, outward):
            if k and k in feature_keys and k not in out:
                out.append(k)
    return out


def build_doc(reqs: list, features: list, project_name: str) -> str:
    feature_keys = {f.get("key") for f in features if f.get("key")}

    # 그룹: FR / NFR(QA) / CNST
    fr = [r for r in reqs if (r.get("reqtype") or "") == "FR"]
    nfr = [r for r in reqs if (r.get("reqtype") or "") in ("NFR", "QA")]
    cnst = [r for r in reqs if (r.get("reqtype") or "") == "CNST"]

    for lst in (fr, nfr, cnst):
        lst.sort(key=lambda x: x.get("subkey") or x.get("key") or "")

    L: list[str] = []
    L.append("# Requirements")
    L.append("")
    L.append("## 1. 개요")
    L.append("")
    L.append(f"본 문서는 **{project_name or '(이름 없음)'}** 의 기능적 요구사항(FR), 비기능적 요구사항(NFR), 제약사항(CNST)을 정의한다.")
    L.append("")

    # Section 2 FR
    L.append("## 2. 기능적 요구사항 (FR)")
    L.append("")
    if fr:
        L.append("| ID | 요구사항 | 관련 Feature |")
        L.append("|---|---|---|")
        for r in fr:
            sub = r.get("subkey") or r.get("key") or "?"
            summary = (r.get("summary") or "").replace("|", "\\|")
            feats = related_features(r, feature_keys)
            L.append(f"| {sub} | {summary} | {', '.join(feats) or '_(없음)_'} |")
    else:
        L.append("_(FR 미도출)_")
    L.append("")

    # Section 3 NFR
    L.append("## 3. 비기능적 요구사항 (NFR)")
    L.append("")
    if nfr:
        L.append("| ID | 품질 속성 | 요구사항 | 관련 Feature |")
        L.append("|---|---|---|---|")
        for r in nfr:
            sub = r.get("subkey") or r.get("key") or "?"
            qa_field = r.get("qa") or []
            qa = ", ".join(qa_field) if isinstance(qa_field, list) else str(qa_field or "")
            summary = (r.get("summary") or "").replace("|", "\\|")
            feats = related_features(r, feature_keys)
            L.append(f"| {sub} | {qa or '-'} | {summary} | {', '.join(feats) or '_(없음)_'} |")
    else:
        L.append("_(NFR 미도출)_")
    L.append("")

    # Section 4 CNST
    L.append("## 4. 제약사항 (CNST)")
    L.append("")
    if cnst:
        L.append("| ID | 요구사항 | 관련 Feature |")
        L.append("|---|---|---|")
        for r in cnst:
            sub = r.get("subkey") or r.get("key") or "?"
            summary = (r.get("summary") or "").replace("|", "\\|")
            feats = related_features(r, feature_keys)
            L.append(f"| {sub} | {summary} | {', '.join(feats) or '_(없음)_'} |")
    else:
        L.append("_(CNST 미도출)_")
    L.append("")

    # Section 5 상세
    L.append("## 5. 요구사항 상세")
    L.append("")
    for r in fr + nfr + cnst:
        sub = r.get("subkey") or r.get("key") or "?"
        summary = r.get("summary") or ""
        feats = related_features(r, feature_keys)
        feat_str = ", ".join(feats) or "_(없음)_"
        L.append(f"### {sub}: {summary} (관련 Feature: {feat_str})")
        L.append("")
        desc = (r.get("description") or "").strip()
        if desc:
            L.append(f"**Description**: {desc}")
            L.append("")
        rt = (r.get("reqtype") or "").upper()
        if rt in ("NFR", "QA"):
            L.append("**QAS 6요소**:")
            L.append(f"- **Source**: {r.get('source') or '_(미기록)_'}")
            L.append(f"- **Stimulus**: {r.get('stimulus') or '_(미기록)_'}")
            L.append(f"- **Environment**: {r.get('environment') or '_(미기록)_'}")
            L.append(f"- **Artifact**: {r.get('artifact') or '_(미기록)_'}")
            L.append(f"- **Response**: {r.get('response') or '_(미기록)_'}")
            L.append(f"- **Measure**: {r.get('measure') or '_(미기록)_'}")
            L.append(f"- **Response Scenario**: {r.get('responseScenario') or '_(미기록)_'}")
            qad = r.get("qad")
            qai = r.get("qai")
            qas = r.get("qas")
            if qad or qai or qas:
                L.append("")
                L.append(f"**평가**: QAD={qad or '-'} / QAI={qai or '-'} / QAS={qas or '-'}")
        L.append("")
        L.append("---")
        L.append("")
    while L and L[-1] in ("", "---"):
        L.pop()
    L.append("")

    # Section 6 Feature 커버리지
    L.append("## 6. Feature 커버리지 매트릭스")
    L.append("")
    feature_to_reqs: dict[str, list[str]] = {f.get("key"): [] for f in features if f.get("key")}
    for r in reqs:
        sub = r.get("subkey") or r.get("key") or "?"
        for fk in related_features(r, feature_keys):
            feature_to_reqs.setdefault(fk, []).append(sub)

    L.append("| Feature 키 | 매핑 Requirement |")
    L.append("|---|---|")
    for f in features:
        fk = f.get("key") or ""
        f_sub = f.get("subkey") or fk
        reqs_for_f = feature_to_reqs.get(fk) or []
        L.append(f"| {f_sub} ({fk}) | {', '.join(reqs_for_f) or '_(미연결)_'} |")

    return "\n".join(L) + "\n"


def main() -> int:
    p = argparse.ArgumentParser()
    p.add_argument("--from-mongo", dest="code", metavar="CODE", required=True)
    p.add_argument("--user", default=os.environ.get("USER", "system"))
    p.add_argument("--api", default=os.environ["GOJIRA_API"])
    p.add_argument("--out", default=None, help="저장 경로. 미지정 시 stdout")
    args = p.parse_args()

    try:
        issues = load_issues()
        prj = http_get_project(args.api, args.code, args.user)
    except urllib.error.HTTPError as e:
        print(f"ERROR: HTTP {e.code} — {e.reason}", file=sys.stderr)
        return 4
    except urllib.error.URLError as e:
        print(f"ERROR: connect — {e}", file=sys.stderr)
        return 4

    reqs = [i for i in issues if i.get("issueType") == "Requirement"]
    features = [i for i in issues if i.get("issueType") == "Feature"]
    if not reqs:
        print("ERROR: Requirement 이슈 0건 — full mode 1회 실행 필요", file=sys.stderr)
        return 3

    md = build_doc(reqs, features, prj.get("name") or "")
    if args.out:
        with open(args.out, "w", encoding="utf-8") as fh:
            fh.write(md)
        print(f"saved to {args.out}")
    else:
        sys.stdout.write(md)
    return 0


if __name__ == "__main__":
    sys.exit(main())
