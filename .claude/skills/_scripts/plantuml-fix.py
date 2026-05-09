#!/usr/bin/env python3
"""plantuml-fix.py — markdown 안 ```plantuml 블록의 흔한 LLM 실수를 결정적으로 수정.

대상 패턴:
  1. sequence destroy 화살표: `A x B:` → `A -x B:`
  2. component/database 안 `state "..."` 키워드 → `[...]` 로 변환
  3. sequence 의 잘못된 참여자 키워드: `file` → `participant`
  (3-2) sequence 의 `cloud` 도 sequence 에서 표현 어색하지만 PlantUML 은 받아들이므로 미수정

usage:
  plantuml-fix.py <file>...           # in-place 수정 (백업 .bak 자동 생성 안 함)
  plantuml-fix.py --check <file>...   # diff 만 출력
  plantuml-fix.py --dir <dir>         # 디렉터리 안 모든 .md
"""
from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path


PLANTUML_BLOCK_RE = re.compile(r"```plantuml\n(.*?)\n```", re.DOTALL)

# 이중 fence: ```plantuml\n```plantuml\n ... \n```\n``` — LLM 이 코드 블록을 두번 감싸는 실수
DOUBLE_FENCE_RE = re.compile(
    r"```plantuml\s*\n```plantuml\s*\n(.*?)\n```\s*\n\s*```",
    re.DOTALL,
)


def fix_double_plantuml_fence(text: str) -> tuple[str, int]:
    """``\\`plantuml`` 블록이 다시 ``\\`plantuml`` 안에 들어가 있는 패턴을 평탄화.

    LLM 이 가끔 PlantUML 코드 블록을 마크다운 코드 블록 안에 한 번 더 감싸서
    렌더가 실패하는 케이스 (외부 + 내부 두 쌍의 fence). 외부 fence 한 쌍을 제거.
    중첩이 3중 이상이어도 반복 적용으로 모두 풀린다.
    """
    total = 0
    while True:
        new, n = DOUBLE_FENCE_RE.subn("```plantuml\n\\1\n```", text)
        if not n:
            break
        text = new
        total += n
    return text, total


def detect_kind(body: str) -> str:
    """다이어그램 종류 식별. component/deployment 신호를 sequence 보다 우선.

    actor 와 package 가 함께 등장하는 deployment/component 다이어그램 (actor 가
    외부 사용자로 등장) 을 sequence 로 오인하면 자동 declare 가 위험하므로
    `package` / `node` / `[Component]` / `database` 가 있으면 component 로 분류.
    """
    # 0. 메시지 화살표 ("A -> B: text") 가 2개 이상이면 sequence 로 확정.
    # database/rectangle 같은 component 키워드를 sequence 안에서 아이콘으로 쓰는
    # 케이스 (legit) 가 있어서, component 신호만 보면 sequence 가 잘못 분류되고
    # fix_component_note_over 가 정상 sequence 의 `note over` 까지 floating note 로
    # 바꿔버리는 회귀 발생. 메시지 화살표 다수 + actor/participant 존재 = sequence.
    seq_arrows = re.findall(
        r"^\s*[\w\"][\w\".]*\s*(?:-->>|->>|-->|->|<<--|<--)\s*[\w\"][\w\".]*\s*:",
        body, re.MULTILINE,
    )
    has_actor_or_part = bool(re.search(r"^\s*(?:actor|participant)\s", body, re.MULTILINE))
    if has_actor_or_part and len(seq_arrows) >= 2:
        return "sequence"

    # 1. component/deployment 신호 (actor 보다 우선).
    # cloud 는 sequence 안에서도 잘못 등장하므로 단독 신호로 사용 안 함.
    # rectangle 은 context/component 다이어그램에서 actor 와 함께 흔히 등장 — 누락 시
    # sequence 로 오분류되어 fix_sequence_undefined_participant 가 잘못된 participant
    # 를 자동 declare 하는 회귀 발생 (rectangle alias 와 충돌해 렌더 실패).
    if re.search(
        r"^\s*(?:package|node|component|database|frame|artifact|rectangle)\s+\"",
        body, re.MULTILINE,
    ) or re.search(r"^\s*\[[^]]+\]\s+as\s+", body, re.MULTILINE):
        return "component"
    # 2. usecase
    if re.search(r"^\s*usecase\b", body, re.MULTILINE):
        return "usecase"
    # 3. sequence — actor/participant 가 있고 component 신호가 없을 때만
    if re.search(r"^\s*(?:actor|participant)\s", body, re.MULTILINE):
        return "sequence"
    return "unknown"


def fix_sequence_destroy_arrow(body: str) -> tuple[str, int]:
    """`A x B:` → `A -x B:` (sequence 에서)."""
    pattern = re.compile(
        r"^(\s*)([A-Za-z_][\w]*|\"[^\"]+\")\s+x\s+([A-Za-z_][\w]*|\"[^\"]+\")(\s*:)",
        re.MULTILINE,
    )
    new, n = pattern.subn(r"\1\2 -x \3\4", body)
    return new, n


def fix_component_note_over(body: str) -> tuple[str, int]:
    """component 다이어그램에서 sequence 문법인 `note over X, Y : <text>` 를
    floating note (`note as N\\n...\\nend note`) 로 변환.

    `note over` 는 sequence 전용. component 에 multi-target note 직접 대응 없음 →
    floating note 로 의미 보존. 라벨 안 `\\n` literal 은 실제 줄바꿈으로 풀어
    multi-line note 본문으로 변환.
    """
    pattern = re.compile(
        r"^(\s*)note\s+over\s+[^:]+:\s*(.+)$",
        re.MULTILINE,
    )

    counter = [0]

    def repl(m: re.Match) -> str:
        counter[0] += 1
        indent = m.group(1)
        label = m.group(2).rstrip()
        lines = label.replace("\\n", "\n").splitlines()
        body_text = "\n".join(indent + line for line in lines)
        return f"{indent}note as N_auto{counter[0]}\n{body_text}\n{indent}end note"

    new, n = pattern.subn(repl, body)
    return new, n


def fix_state_in_component(body: str) -> tuple[str, int]:
    """component/database/package 안의 `state "..." as X` → `[...] as X`."""
    pattern = re.compile(
        r"^(\s*)state\s+(\"[^\"]+\")\s+as\s+([A-Za-z_][\w]*)\s*$",
        re.MULTILINE,
    )

    def repl(m: re.Match) -> str:
        indent, label, alias = m.group(1), m.group(2), m.group(3)
        inner = label.strip('"')
        return f"{indent}[{inner}] as {alias}"

    new, n = pattern.subn(repl, body)
    return new, n


def fix_sequence_file_keyword(body: str) -> tuple[str, int]:
    """sequence 다이어그램에서 `file "..." as X` → `participant "..." as X`."""
    pattern = re.compile(
        r"^(\s*)file\s+(\"[^\"]+\")\s+as\s+([A-Za-z_][\w]*)\s*$",
        re.MULTILINE,
    )
    new, n = pattern.subn(r"\1participant \2 as \3", body)
    return new, n


def fix_sequence_cloud_keyword(body: str) -> tuple[str, int]:
    """sequence 다이어그램에서 `cloud "..." as X` → `participant "..." as X`.

    PlantUML 의 sequence 는 cloud 키워드를 지원하지 않음 (deployment 전용).
    """
    pattern = re.compile(
        r"^(\s*)cloud\s+(\"[^\"]+\")\s+as\s+([A-Za-z_][\w]*)\s*$",
        re.MULTILINE,
    )
    new, n = pattern.subn(r"\1participant \2 as \3", body)
    return new, n


_DECL_KEYWORDS = (
    # sequence 선언
    "actor", "participant", "boundary", "control", "entity",
    "database", "collections", "queue",
    # component/deployment 선언 — sequence 다이어그램 안에 함께 등장하는 경우
    # (예: Context 가 sequence 로 오분류된 경우) 이미 선언된 alias 를 다시
    # participant 로 자동 declare 하지 않도록 인식.
    "rectangle", "package", "node", "component", "frame", "artifact",
    # use case 선언 — Use Case Diagram 이 sequence 로 오분류되는 경우 대비
    "usecase",
)


def fix_sequence_undefined_participant(body: str) -> tuple[str, int]:
    """sequence 다이어그램의 메시지에서 등장하지만 미정의된 식별자를 자동 declare.

    @startuml 직후에 `participant <name>` 추가. quote 라벨 없이 식별자 그대로.
    """
    # 1. 이미 선언된 alias 수집
    # `actor "label" as X` (quoted), `actor label as X` (unquoted, 한글 라벨 포함),
    # `actor X` 단순 선언 모두 커버.
    declared: set[str] = set()
    decl_with_label = re.compile(
        rf"^\s*(?:{'|'.join(_DECL_KEYWORDS)})\s+(?:\"[^\"]+\"|\S+)\s+as\s+([A-Za-z_][\w]*)",
        re.MULTILINE,
    )
    decl_simple = re.compile(
        rf"^\s*(?:{'|'.join(_DECL_KEYWORDS)})\s+([A-Za-z_][\w]*)(?:\s+as\s+\"[^\"]+\")?\s*$",
        re.MULTILINE,
    )
    declared |= set(decl_with_label.findall(body))
    declared |= set(decl_simple.findall(body))

    # 2. 메시지 양 끝의 식별자 추출
    arrow_re = re.compile(
        r"^\s*([A-Za-z_][\w]*)\s*(?:-+(?:>>?|x>?)|<<?-+|<<?x-+|->>?x?|<<?-+>?)\s*"
        r"([A-Za-z_][\w]*)\s*:",
        re.MULTILINE,
    )
    used: set[str] = set()
    for m in arrow_re.finditer(body):
        used.add(m.group(1))
        used.add(m.group(2))

    undefined = sorted(used - declared)
    if not undefined:
        return body, 0

    insertion = "\n".join(f"participant {n}" for n in undefined) + "\n"
    new = re.sub(
        r"(@startuml[^\n]*\n)",
        lambda m: m.group(1) + insertion,
        body,
        count=1,
    )
    return new, len(undefined)


def fix_quoted_label_newline(body: str) -> tuple[str, int]:
    """Quoted string 안의 raw newline 을 \\n escape 로 변환.

    LLM 이 `participant "Socket.IO\\nServer"` 로 써야 할 걸 raw newline 으로
    풀어버리는 케이스. PlantUML 의 quoted string 은 한 줄이어야 함.
    다이어그램 종류 무관 발동.
    """
    out: list[str] = []
    in_quote = False
    count = 0
    for ch in body:
        if ch == '"':
            in_quote = not in_quote
            out.append(ch)
        elif ch == "\n" and in_quote:
            out.append("\\n")
            count += 1
        else:
            out.append(ch)
    return "".join(out), count


def fix_block(body: str) -> tuple[str, dict[str, int]]:
    """블록 한 개 수정. (수정본, 패턴별 적용 횟수) 반환."""
    counts: dict[str, int] = {}

    # 1단계: 종류 무관 — quoted label 안 raw newline 우선 정리.
    # detect_kind 가 multi-line label 때문에 오인할 수 있어 가장 먼저.
    body, n0 = fix_quoted_label_newline(body)
    counts["quoted_label_newline"] = n0

    kind = detect_kind(body)

    if kind == "sequence":
        body, n1 = fix_sequence_destroy_arrow(body)
        counts["seq_destroy_arrow"] = n1
        body, n3 = fix_sequence_file_keyword(body)
        counts["seq_file_keyword"] = n3
        body, n4 = fix_sequence_cloud_keyword(body)
        counts["seq_cloud_keyword"] = n4
        # cloud → participant 후 미정의가 남아 있을 수 있으므로 마지막에 declare 추가
        body, n5 = fix_sequence_undefined_participant(body)
        counts["seq_undefined_participant"] = n5
    elif kind == "component":
        body, n2 = fix_state_in_component(body)
        counts["component_state_keyword"] = n2
        body, n6 = fix_component_note_over(body)
        counts["component_note_over"] = n6

    return body, counts


def fix_file(path: Path, check: bool = False) -> dict[str, int]:
    """파일 단위 수정. counts 합계 반환."""
    text = path.read_text(encoding="utf-8")
    total: dict[str, int] = {}

    # 1단계: 이중 fence 평탄화 — 블록 단위 수정 전에 외부 fence 정리해야 본문이 정확히 잡힘
    text, n_double = fix_double_plantuml_fence(text)
    total["double_fence"] = n_double

    def repl(m: re.Match) -> str:
        body = m.group(1)
        new_body, counts = fix_block(body)
        for k, v in counts.items():
            total[k] = total.get(k, 0) + v
        return f"```plantuml\n{new_body}\n```"

    new_text = PLANTUML_BLOCK_RE.sub(repl, text)

    original = path.read_text(encoding="utf-8")
    if new_text == original:
        return total

    if not check:
        path.write_text(new_text, encoding="utf-8")
    return total


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("paths", nargs="*", help="markdown files")
    ap.add_argument("--dir", help="recurse this directory for .md")
    ap.add_argument("--check", action="store_true", help="don't write, only report")
    args = ap.parse_args()

    files: list[Path] = []
    if args.dir:
        files.extend(sorted(Path(args.dir).rglob("*.md")))
    files.extend(Path(p) for p in args.paths)

    if not files:
        ap.print_help()
        return 1

    grand: dict[str, int] = {}
    changed = 0
    for f in files:
        if not f.is_file():
            continue
        counts = fix_file(f, check=args.check)
        if any(counts.values()):
            changed += 1
            print(f"{f}: " + ", ".join(f"{k}={v}" for k, v in counts.items() if v))
            for k, v in counts.items():
                grand[k] = grand.get(k, 0) + v

    print()
    print(f"파일 변경: {changed} / 검사: {len(files)}")
    print("패턴별 수정 횟수:", grand)
    return 0


if __name__ == "__main__":
    sys.exit(main())
