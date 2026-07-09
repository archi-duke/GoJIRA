#!/usr/bin/env python3
"""driver-assemble.py — driver-select 결정적 문서 조립 (LLM 0회).

driverSelected=true Requirement 이슈 + archSettings.normalizedAttributes 만으로
architecture/drivers.md 생성.

입력:
  --from-mongo CODE [--user USER]
출력:
  stdout: drivers.md 본문
"""
from __future__ import annotations

import argparse
import json
import os
import sys
import urllib.error
import urllib.request

import issue_local


def http_get(api: str, code: str, user: str) -> tuple[list, dict]:
    # 이슈는 로컬 clone data/issues/*.yaml 에서 읽는다 — 이슈 쓰기는 Mongo 를 건드리지
    # 않고 로컬에만 기록되고 Mongo 는 PR 머지 시점에만 갱신되므로, 세션 중 GET /Issue 로는
    # 방금 만든 이슈가 보이지 않는다.
    issues = issue_local.load_local_issues()

    pxbody = json.dumps({"jiraCode": code})
    preq = urllib.request.Request(
        f"{api}/Project", headers={"X-User-Id": user, "Authorization": "Bearer " + os.environ.get("GOJIRA_SESSION_TOKEN", ""), "XBody": pxbody}
    )
    parr = json.loads(urllib.request.urlopen(preq, timeout=10).read().decode("utf-8"))
    arch = (parr[0].get("archSettings") if parr else {}) or {}
    return issues, arch


def is_driver(req: dict) -> bool:
    if req.get("driverSelected") is True:
        return True
    labels = req.get("labels") or []
    if isinstance(labels, list) and "ASR" in labels:
        return True
    # qad/qai 둘 다 채워진 Requirement 도 driver 추정
    if (req.get("qad") or "").strip() and (req.get("qai") or "").strip():
        return True
    return False


def build_doc(drivers: list, arch: dict) -> str:
    drivers.sort(key=lambda x: x.get("subkey") or x.get("key") or "")

    L: list[str] = []
    L.append("# Architecture Drivers")
    L.append("")

    # Section 1 선정 Driver 목록
    L.append("## 1. 선정 Driver 목록")
    L.append("")
    if drivers:
        L.append("| # | 이슈 키 | 타입 | QA | 요약 | QAD | QAI | QAS | Priority |")
        L.append("|---|---|---|---|---|---|---|---|---|")
        for i, d in enumerate(drivers, start=1):
            sub = d.get("subkey") or d.get("key") or "?"
            rt = d.get("reqtype") or "-"
            qa_field = d.get("qa") or []
            qa = ", ".join(qa_field) if isinstance(qa_field, list) else str(qa_field or "-")
            summary = (d.get("summary") or "").replace("|", "\\|")
            L.append(f"| {i} | {sub} | {rt} | {qa} | {summary} | {d.get('qad') or '-'} | {d.get('qai') or '-'} | {d.get('qas') or '-'} | {d.get('priority') or '-'} |")
    else:
        L.append("_(Driver 0건 — driver-select full mode 1회 실행 필요)_")
    L.append("")

    # Section 2 Driver 별 상세 근거
    L.append("## 2. Driver 별 상세 근거")
    L.append("")
    for d in drivers:
        sub = d.get("subkey") or "?"
        summary = d.get("summary") or ""
        L.append(f"### {sub} — {summary}")
        L.append("")
        rt = d.get("reqtype") or ""
        if rt in ("NFR", "QA"):
            L.append("**QAS 6요소**:")
            L.append(f"- Source: {d.get('source') or '_(미기록)_'}")
            L.append(f"- Stimulus: {d.get('stimulus') or '_(미기록)_'}")
            L.append(f"- Environment: {d.get('environment') or '_(미기록)_'}")
            L.append(f"- Artifact: {d.get('artifact') or '_(미기록)_'}")
            L.append(f"- Response: {d.get('response') or '_(미기록)_'}")
            L.append(f"- Measure: {d.get('measure') or '_(미기록)_'}")
            L.append(f"- Response Scenario: {d.get('responseScenario') or '_(미기록)_'}")
            L.append("")
        L.append(f"- **QAD**: {d.get('qad') or '-'}")
        L.append(f"- **QAI**: {d.get('qai') or '-'}")
        L.append(f"- **QAS**: {d.get('qas') or '-'}")
        if d.get("priority"):
            L.append(f"- **Priority**: {d['priority']}")
        desc = (d.get("description") or "").strip()
        if desc:
            L.append("")
            L.append(f"**Description**: {desc}")
        L.append("")
        L.append("---")
        L.append("")
    while L and L[-1] in ("", "---"):
        L.pop()
    L.append("")

    # Section 3 정규화된 시스템 속성
    L.append("## 3. 정규화된 시스템 속성 (style_define 입력)")
    L.append("")
    norm = arch.get("normalizedAttributes") or {}
    if norm:
        L.append("| 속성 | 값 |")
        L.append("|---|---|")
        for k, v in sorted(norm.items()):
            L.append(f"| {k} | {v} |")
    else:
        L.append("_(archSettings.normalizedAttributes 미저장)_")
    L.append("")

    # Section 4 카테고리 분포 (선정 편향 점검 — 결정적 통계)
    L.append("## 4. 선정 편향 점검 (카테고리 분포)")
    L.append("")
    type_count: dict[str, int] = {}
    qa_count: dict[str, int] = {}
    for d in drivers:
        rt = d.get("reqtype") or "-"
        type_count[rt] = type_count.get(rt, 0) + 1
        qa_field = d.get("qa") or []
        if isinstance(qa_field, list):
            for q in qa_field:
                qa_count[q] = qa_count.get(q, 0) + 1
    if drivers:
        L.append("**Type 분포**: " + ", ".join(f"{k}={v}" for k, v in sorted(type_count.items())))
        L.append("")
        if qa_count:
            L.append("**QA 분포**: " + ", ".join(f"{k}={v}" for k, v in sorted(qa_count.items(), key=lambda kv: -kv[1])))
        else:
            L.append("_(QA 정보 없음)_")
    else:
        L.append("_(Driver 0건)_")

    return "\n".join(L) + "\n"


def main() -> int:
    p = argparse.ArgumentParser()
    p.add_argument("--from-mongo", dest="code", metavar="CODE", required=True)
    p.add_argument("--user", default=os.environ.get("USER", "system"))
    p.add_argument("--api", default=os.environ["GOJIRA_API"])
    p.add_argument("--out", default=None)
    args = p.parse_args()

    try:
        issues, arch = http_get(args.api, args.code, args.user)
    except urllib.error.HTTPError as e:
        print(f"ERROR: HTTP {e.code} — {e.reason}", file=sys.stderr)
        return 4
    except urllib.error.URLError as e:
        print(f"ERROR: connect — {e}", file=sys.stderr)
        return 4

    drivers = [
        i for i in issues
        if i.get("issueType") == "Requirement" and is_driver(i)
    ]
    if not drivers:
        print("ERROR: Driver (driverSelected=true) Requirement 0건 — driver-select full mode 1회 실행 필요", file=sys.stderr)
        return 3

    md = build_doc(drivers, arch)
    if args.out:
        with open(args.out, "w", encoding="utf-8") as fh:
            fh.write(md)
        print(f"saved to {args.out}")
    else:
        sys.stdout.write(md)
    return 0


if __name__ == "__main__":
    sys.exit(main())
