"""Mermaid 다이어그램 공용 유틸.

단일 소스 원칙:
- **규칙 문서**: `prompts/mermaid-syntax.md` (유일한 작성 기준 소스)
- **프롬프트 삽입**: 스킬 generate.txt 에서 `{mermaid_rules}` 변수로 치환하여 LLM에 전달
- **정적 검증**: 생성된 다이어그램을 코드 레벨로 추가 검증 (LLM 자기 판단 보강)
"""

import os
import re
from typing import List, Dict, Any

_RULES_PATH = os.path.join(os.path.dirname(__file__), "..", "prompts", "mermaid-syntax.md")

_MERMAID_BLOCK_RE = re.compile(r"```mermaid\s*\n(.*?)\n```", re.DOTALL)

# Mermaid 키워드 — 이 줄들은 node ID 검증에서 제외
_SKIP_LINE_PREFIXES = (
    "%%",                # 주석
    "graph ",
    "flowchart ",
    "sequenceDiagram",
    "classDiagram",
    "stateDiagram",
    "erDiagram",
    "gantt",
    "pie ",
    "journey",
    "classDef ",
    "class ",
    "subgraph ",
    "end",
    "direction ",
    "participant ",      # sequenceDiagram 참가자
    "actor ",
    "autonumber",
    "Note ",
    "note ",
    "loop ",
    "alt ",
    "opt ",
    "par ",
    "rect ",
    "title ",
    "style ",
    "linkStyle ",
    "click ",
)

# node ID가 나올 수 있는 토큰 경계 (라벨/관계)
_TOKEN_BOUNDARY_RE = re.compile(r"""
    (?P<id>[^\s\[\(\{<>\-=~\.|:;,]+)       # node ID 부분 (공백/괄호/화살표 전까지)
    (?:                                     # 뒤 토큰 (label 또는 arrow)
      \s*(?:\[|\(\(|\{\{|\{|<|\|)           # label 시작: [ (( {{ { < |
      | \s*(?::::[A-Za-z0-9_]+)?            # classDef applied
      \s*(?:-->|<--|-.->|<-\.|==>|<==|---|~~~|<-|->|<|>|-\.|\-\->)  # arrow
    )
""", re.VERBOSE)


def get_mermaid_rules() -> str:
    """`prompts/mermaid-syntax.md` 전체 내용을 반환 (프롬프트 인라인용).

    매 호출마다 파일을 읽어 규칙 변경 시 즉시 반영.
    """
    try:
        with open(_RULES_PATH, "r", encoding="utf-8") as f:
            return f.read()
    except FileNotFoundError:
        return ""


def find_mermaid_blocks(doc: str) -> List[str]:
    """Markdown 문서에서 ```mermaid ... ``` 코드블록 본문만 추출."""
    return _MERMAID_BLOCK_RE.findall(doc or "")


def _mask_labels(line: str) -> str:
    """label/메시지 내부를 공백으로 치환.

    대상:
    - `[...]`, `((...))`, `{{...}}`, `{...}` (flowchart 노드 label)
    - `|...|` (화살표 중간 label)
    - sequenceDiagram의 `: message` 뒤 전체 (콜론 이후는 메시지)
    """
    # sequenceDiagram message: `Actor->>System: 요청 내용` 의 `:` 뒤 부분
    # 단, `participant X as Y`, `Note right of X: ...`, `autonumber` 같은 줄은 그대로 유지됨
    # 콜론 앞이 화살표(`->>`, `-->>`, `-)`, `->`, etc)가 있을 때만 마스킹
    line = re.sub(
        r"((?:->>|-->>|-\)|--\)|-x|--x|->|-->)\s*[A-Za-z0-9_]+)\s*:\s*.*$",
        lambda m: m.group(1) + ": ",
        line,
    )
    # 중첩/다중 괄호를 안전하게 처리: 우선 긴 경계(`((` / `{{`)부터
    line = re.sub(r"\(\(.*?\)\)", lambda m: "((" + " " * (len(m.group()) - 4) + "))", line)
    line = re.sub(r"\{\{.*?\}\}", lambda m: "{{" + " " * (len(m.group()) - 4) + "}}", line)
    line = re.sub(r"\[.*?\]", lambda m: "[" + " " * (len(m.group()) - 2) + "]", line)
    line = re.sub(r"\{.*?\}", lambda m: "{" + " " * (len(m.group()) - 2) + "}", line)
    # 화살표 label: `-->|foo|` 내부도 마스킹
    line = re.sub(r"\|.*?\|", lambda m: "|" + " " * (len(m.group()) - 2) + "|", line)
    return line


def validate_mermaid(code: str) -> List[str]:
    """단일 Mermaid 코드블록을 정적 검증. 위반 사항 리스트 반환 (비어 있으면 유효).

    검증 항목:
    - 빈 코드블록
    - node ID에 한글 포함 (label 내부 한글은 허용, ID 자리만 체크)
    """
    errors: List[str] = []
    src = (code or "").strip()
    if not src:
        errors.append("빈 mermaid 블록")
        return errors

    # 첫 비어있지 않은 줄 = 다이어그램 종류
    first_line = next((l.strip() for l in src.split("\n") if l.strip()), "")

    # classDiagram 감지 — Use Case 표현에는 사용 금지 (graph TB 권장)
    # 또한 classDiagram을 쓰더라도 `class X[...]` 대괄호 문법은 무조건 오류
    if first_line.startswith("classDiagram"):
        errors.append(
            "classDiagram 사용 감지 — Use Case Diagram은 'graph TB'로 작성하세요 "
            "(classDiagram에서 `class X[라벨]` 같은 대괄호 문법은 렌더 실패)"
        )
        # 추가로 잘못된 class 문법 / 한글 메서드명 점검
        for lineno, raw in enumerate(src.split("\n"), 1):
            line = raw.strip()
            if re.match(r"class\s+\w+\s*\[", line):
                errors.append(
                    f"line {lineno}: classDiagram에서 `class X[...]` 문법 오류 — "
                    f"label은 큰따옴표 `class X[\"라벨\"]` 사용"
                )
            # 메서드/속성 줄에 한글
            if re.match(r"^[+\-#~]", line) and re.search(r"[가-힣]", line):
                errors.append(
                    f"line {lineno}: classDiagram 멤버에 한글 사용: '{line}' — 영문만 허용"
                )

    for lineno, raw in enumerate(src.split("\n"), 1):
        line = raw.strip()
        if not line:
            continue
        # 스킵: 키워드/주석/제어문 줄
        if any(line.startswith(p) for p in _SKIP_LINE_PREFIXES):
            continue

        # label 내부 문자를 공백으로 마스킹 (한글 라벨은 허용해야 함)
        masked = _mask_labels(line)

        # 마스킹된 라인에 남아있는 한글 = ID 자리의 한글
        if re.search(r"[가-힣]", masked):
            # 어떤 한글 토큰이 문제인지 추출 (첫 번째)
            m = re.search(r"[^\s\[\(\{<>\|]*[가-힣][^\s\[\(\{<>\|]*", masked)
            bad_token = m.group() if m else "(unknown)"
            errors.append(
                f"line {lineno}: 노드 ID 자리에 한글 사용: '{bad_token}' — 영문 ID + [표시명] 형태로 감싸세요"
            )

    # 중복 제거
    seen = set()
    unique = []
    for e in errors:
        if e not in seen:
            seen.add(e)
            unique.append(e)
    return unique


_IMAGE_REF_RE = re.compile(
    r"!\[[^\]]*\]\(\s*([^)\s]+\.(?:png|jpg|jpeg|svg|gif|webp))\s*\)",
    re.IGNORECASE,
)


def validate_document_mermaid(doc: str) -> Dict[str, Any]:
    """Markdown 문서 내 모든 Mermaid 블록을 일괄 검증 + 외부 이미지 참조 탐지."""
    blocks = find_mermaid_blocks(doc)
    all_errors: List[str] = []
    for idx, block in enumerate(blocks, 1):
        errs = validate_mermaid(block)
        for e in errs:
            all_errors.append(f"Block #{idx}: {e}")

    # 외부 이미지 참조는 다이어그램을 Mermaid 대신 PNG/SVG로 대체한 패턴 — 금지
    image_refs = _IMAGE_REF_RE.findall(doc or "")
    for ref in image_refs:
        all_errors.append(
            f"외부 이미지 참조 금지 (Mermaid 인라인 코드블록으로 작성해야 함): {ref}"
        )

    return {
        "blockCount": len(blocks),
        "imageRefs": image_refs,
        "errors": all_errors,
        "valid": len(all_errors) == 0,
    }


def build_mermaid_fix_prompt(doc: str, errors: List[str]) -> str:
    """정적 검증 실패 시, 문서 내 Mermaid 블록만 수정하도록 LLM에 지시하는 프롬프트."""
    errors_text = "\n".join(f"- {e}" for e in errors)
    return f"""다음 Markdown 문서에 포함된 Mermaid 다이어그램에 아래 구문 오류가 있습니다.
**문서 본문은 그대로 두고, Mermaid 코드블록의 오류만 수정**하여 전체 문서를 출력하세요.

## 오류 목록
{errors_text}

## Mermaid 구문 규칙
{get_mermaid_rules()}

## 원본 문서
{doc}

## 출력 규칙
- 수정한 전체 문서 (Markdown 원문)만 출력. 설명/코드펜스로 전체를 감싸지 말 것
- 오류와 무관한 부분은 절대 변경 금지
- 모든 기존 다이어그램 코드블록 개수와 위치 유지
"""
