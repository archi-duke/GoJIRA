"""Mongo → Git YAML export — 과제별 이슈를 data/issues/<subkey>.yaml 로 덤프.

사용 예:
    # 로컬 출력 (기본)
    python -m scripts.export_issues_to_yaml GOJIRA3 --out /tmp/gojira3-data

    # 전 과제
    python -m scripts.export_issues_to_yaml --all --out /tmp/gojira-data

    # Gitea user 브랜치로 push (Platform-API save_file 경유)
    python -m scripts.export_issues_to_yaml GOJIRA3 --push --user batman

파일 규칙:
    <out>/<projectCode>/data/issues/<subkey>.yaml
"""

from __future__ import annotations

import argparse
import asyncio
import logging
import re
import sys
from pathlib import Path
from typing import Iterable, List

from core.database import issues, projects
from core.issue_schema import from_mongo, dump_issue_yaml, dump_issue_file, IssueDoc

log = logging.getLogger("export_issues")


def _iter_project_codes(only_codes: List[str] | None) -> Iterable[str]:
    if only_codes:
        for c in only_codes:
            yield c
        return
    for p in projects().find({}, {"jiraCodes.main.jiraCode": 1, "_id": 0}):
        code = (((p.get("jiraCodes") or {}).get("main") or {}).get("jiraCode"))
        if code:
            yield code


def _sanitize_subkey(subkey: str, fallback: str) -> str:
    s = (subkey or "").strip()
    if not s:
        s = fallback
    # 파일명 안전화
    return re.sub(r"[^A-Za-z0-9_\-]+", "_", s)


def export_project_local(code: str, out_root: Path, *, strict: bool) -> dict:
    prefix = {"key": {"$regex": f"^{re.escape(code)}-"}}
    docs_dir = out_root / code / "data" / "issues"
    docs_dir.mkdir(parents=True, exist_ok=True)

    ok = fail = 0
    fail_details: list = []
    seen_files: set = set()

    for raw in issues().find(prefix):
        try:
            doc: IssueDoc = from_mongo(raw)
        except Exception as e:
            fail += 1
            fail_details.append({"key": raw.get("key"), "error": f"parse: {e}"})
            if strict:
                raise
            continue

        subkey = _sanitize_subkey(doc.subkey, doc.key or f"issue-{ok}")
        fname = f"{subkey}.yaml"
        # 중복 subkey 방어 (드문 케이스) — key 접미
        if fname in seen_files:
            fname = f"{subkey}__{doc.key}.yaml"
        seen_files.add(fname)

        target = docs_dir / fname
        try:
            dump_issue_file(doc, target)
            ok += 1
        except Exception as e:
            fail += 1
            fail_details.append({"key": doc.key, "error": f"write: {e}"})

    return {"projectCode": code, "exported": ok, "failed": fail, "dir": str(docs_dir),
            "failDetails": fail_details[:10]}


async def export_project_push(code: str, *, user: str, strict: bool) -> dict:
    """Platform-API save_file 로 각 YAML 을 user 브랜치에 커밋."""
    from clients.platform_api import PlatformClient
    client = PlatformClient()

    prefix = {"key": {"$regex": f"^{re.escape(code)}-"}}
    ok = fail = 0
    fail_details: list = []
    seen: set = set()

    for raw in issues().find(prefix):
        try:
            doc: IssueDoc = from_mongo(raw)
            subkey = _sanitize_subkey(doc.subkey, doc.key or "issue")
            fname = f"{subkey}.yaml"
            if fname in seen:
                fname = f"{subkey}__{doc.key}.yaml"
            seen.add(fname)

            text = dump_issue_yaml(doc)
            await client.save_file(
                code,
                f"data/issues/{fname}",
                text,
                f"[export] 이슈 YAML 마이그레이션: {doc.key}",
                user,
            )
            ok += 1
        except Exception as e:
            fail += 1
            fail_details.append({"key": raw.get("key"), "error": str(e)})
            if strict:
                raise

    return {"projectCode": code, "pushed": ok, "failed": fail,
            "failDetails": fail_details[:10]}


def main(argv=None):
    parser = argparse.ArgumentParser(description="Mongo → Git YAML export")
    parser.add_argument("codes", nargs="*", help="과제 코드 목록 (비우고 --all 사용 가능)")
    parser.add_argument("--all", action="store_true", help="전 과제 대상")
    parser.add_argument("--out", type=Path, default=Path("./export"), help="로컬 출력 루트 (default: ./export)")
    parser.add_argument("--push", action="store_true", help="Gitea user 브랜치로 커밋")
    parser.add_argument("--user", default="anonymous", help="--push 시 사용자 브랜치 소유자")
    parser.add_argument("--strict", action="store_true", help="검증 실패 시 즉시 중단")
    parser.add_argument("-v", "--verbose", action="store_true")
    args = parser.parse_args(argv)

    logging.basicConfig(
        level=logging.DEBUG if args.verbose else logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
    )

    codes = list(_iter_project_codes(args.codes if not args.all else None))
    if not codes:
        print("대상 과제 없음", file=sys.stderr)
        return 1

    results = []
    for code in codes:
        if args.push:
            r = asyncio.run(export_project_push(code, user=args.user, strict=args.strict))
        else:
            r = export_project_local(code, args.out, strict=args.strict)
        results.append(r)
        print(r)

    # 요약
    total_ok = sum(r.get("exported", r.get("pushed", 0)) for r in results)
    total_fail = sum(r.get("failed", 0) for r in results)
    print(f"\n== 요약: 프로젝트 {len(results)} / 성공 {total_ok} / 실패 {total_fail} ==")
    return 0 if total_fail == 0 else 2


if __name__ == "__main__":
    sys.exit(main())
