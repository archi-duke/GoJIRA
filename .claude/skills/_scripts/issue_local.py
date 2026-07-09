"""issue_local.py — 로컬 clone의 data/issues/*.yaml 을 읽어 GoJIRA-API GET /Issue
응답과 동일한 dict 리스트 형태로 반환하는 공용 모듈.

배경: 이슈 쓰기(POST/PUT /Issue, X-Local-Write:true)는 Mongo 를 건드리지 않고 로컬
clone 에만 YAML 을 기록한다 — Mongo 는 PR 머지 시점에만 갱신된다. 그래서 세션 중
(PR 머지 전) GET /Issue 로 이슈를 조회하면 방금 만든 이슈가 전혀 안 보인다.
*-assemble.py / project-status.sh 의 이슈 기반 판정은 세션 중 바로 실행되므로 이슈
목록은 반드시 로컬 clone 파일에서 읽어야 한다.

(archSettings / usecase-artifacts 는 X-Local-Write 없이 직접 Mongo 에 쓰이므로
GET /Project, /Project/{code}/usecase-artifacts 는 그대로 API 조회를 유지한다 —
이 모듈이 다루는 대상이 아니다.)

지원 문법(GoJIRA-API 가 yaml.v3 로 쓰는 실제 출력 형태만 커버 — 범용 YAML 파서 아님):
  - 매핑 key: value (중첩 포함)
  - 빈 리스트 key: []
  - 블록 시퀀스 (- 로 시작, 각 항목은 스칼라 또는 중첩 매핑)
  - 블록 스칼라 key: | / |- / |+ / > / >- / >+ (여러 줄 텍스트)
  - 따옴표 문자열('...'/"..."), true/false, 그 외는 원문 문자열
"""
from __future__ import annotations

import os
import re
import subprocess
import sys
from pathlib import Path

_KV_RE = re.compile(r"^([^:\s][^:]*?):\s*(.*)$")


def find_repo_root() -> Path | None:
    root = os.environ.get("GOJIRA_REPO_ROOT")
    if root:
        p = Path(root)
        if p.is_dir():
            return p
    try:
        out = subprocess.run(
            ["git", "rev-parse", "--show-toplevel"],
            capture_output=True, text=True, timeout=5,
        )
        if out.returncode == 0 and out.stdout.strip():
            p = Path(out.stdout.strip())
            if p.is_dir():
                return p
    except Exception:
        pass
    return None


_DQ_ESCAPES = {"\\": "\\", '"': '"', "n": "\n", "t": "\t"}


def _unescape_double_quoted(inner: str) -> str:
    """단일 패스로 \\, \", \n, \t 만 처리 — 순차 str.replace 체인의 재해석 버그 회피."""
    out: list[str] = []
    i = 0
    n = len(inner)
    while i < n:
        c = inner[i]
        if c == "\\" and i + 1 < n and inner[i + 1] in _DQ_ESCAPES:
            out.append(_DQ_ESCAPES[inner[i + 1]])
            i += 2
        else:
            out.append(c)
            i += 1
    return "".join(out)


def _scalar(raw: str):
    s = raw.strip()
    if s in ("", "~", "null"):
        return None
    if s == "[]":
        return []
    if s in ("true", "True"):
        return True
    if s in ("false", "False"):
        return False
    if len(s) >= 2 and s[0] == "'" and s[-1] == "'":
        # yaml.v3 single-quoted style escapes ' as '' — unescape.
        return s[1:-1].replace("''", "'")
    if len(s) >= 2 and s[0] == '"' and s[-1] == '"':
        return _unescape_double_quoted(s[1:-1])
    return s


def _indent_of(line: str) -> int:
    return len(line) - len(line.lstrip(" "))


def _parse_block_scalar(lines: list[str], i: int, parent_indent: int, style: str) -> tuple[str, int]:
    n = len(lines)
    collected: list[str] = []
    block_indent = None
    while i < n:
        line = lines[i]
        if not line.strip():
            collected.append("")
            i += 1
            continue
        cur_indent = _indent_of(line)
        if cur_indent <= parent_indent:
            break
        if block_indent is None:
            block_indent = cur_indent
        collected.append(line[block_indent:])
        i += 1
    while collected and collected[-1] == "":
        collected.pop()
    sep = "\n" if style[0] == "|" else " "
    text = sep.join(collected)
    if style.endswith("-"):
        return text, i
    return text + "\n", i


def _parse_seq(lines: list[str], i: int, indent: int) -> tuple[list, int]:
    items: list = []
    n = len(lines)
    while i < n:
        line = lines[i]
        if not line.strip():
            i += 1
            continue
        cur_indent = _indent_of(line)
        if cur_indent < indent:
            break
        content = line[indent:]
        if not content.startswith("- "):
            break
        after_dash = content[2:]
        item_indent = indent + 2
        if after_dash.strip() == "":
            sub, i = _parse_block(lines, i + 1, item_indent)
            items.append(sub)
            continue
        m = _KV_RE.match(after_dash)
        if m:
            fake_lines = [(" " * item_indent) + after_dash] + lines[i + 1:]
            sub, consumed = _parse_block(fake_lines, 0, item_indent)
            items.append(sub)
            i = i + consumed
        else:
            items.append(_scalar(after_dash))
            i += 1
    return items, i


def _parse_block(lines: list[str], i: int, indent: int) -> tuple[dict, int]:
    result: dict = {}
    n = len(lines)
    while i < n:
        line = lines[i]
        if not line.strip():
            i += 1
            continue
        cur_indent = _indent_of(line)
        if cur_indent < indent:
            break
        if cur_indent > indent:
            i += 1
            continue
        content = line[indent:]
        if content.startswith("- "):
            break
        m = _KV_RE.match(content)
        if not m:
            i += 1
            continue
        key, rest = m.group(1), m.group(2)
        rest_stripped = rest.strip()
        if rest_stripped in ("|", "|-", "|+", ">", ">-", ">+"):
            text, i = _parse_block_scalar(lines, i + 1, indent, rest_stripped)
            result[key] = text
        elif rest_stripped == "":
            if i + 1 < n and lines[i + 1].strip():
                next_indent = _indent_of(lines[i + 1])
                if next_indent > indent and lines[i + 1].strip().startswith("- "):
                    seq, i = _parse_seq(lines, i + 1, next_indent)
                    result[key] = seq
                elif next_indent > indent:
                    sub, i = _parse_block(lines, i + 1, next_indent)
                    result[key] = sub
                else:
                    result[key] = None
                    i += 1
            else:
                result[key] = None
                i += 1
        else:
            result[key] = _scalar(rest_stripped)
            i += 1
    return result, i


def parse_issue_yaml(path: Path) -> dict:
    text = path.read_text(encoding="utf-8")
    lines = text.split("\n")
    result, _ = _parse_block(lines, 0, 0)
    return result


def load_local_issues(root: Path | None = None) -> list[dict]:
    """data/issues/*.yaml 전체를 GET /Issue 응답과 동일한 dict 리스트로 반환."""
    if root is None:
        root = find_repo_root()
    elif not isinstance(root, Path):
        root = Path(root)
    if root is None:
        print("WARN: repo 루트 미확인 — GOJIRA_REPO_ROOT 설정 또는 clone 안에서 실행 (이슈 0건으로 처리됨)", file=sys.stderr)
        return []
    issues_dir = root / "data" / "issues"
    if not issues_dir.is_dir():
        print(f"WARN: {issues_dir} 없음 — 잘못된 repo 루트이거나 신규 프로젝트(이슈 0건으로 처리됨)", file=sys.stderr)
        return []
    out: list[dict] = []
    for p in sorted(issues_dir.glob("*.yaml")):
        try:
            out.append(parse_issue_yaml(p))
        except Exception as e:
            print(f"WARN: {p} 파싱 실패 — 스킵 ({e})", file=sys.stderr)
    return out
