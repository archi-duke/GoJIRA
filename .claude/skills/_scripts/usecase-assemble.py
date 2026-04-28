#!/usr/bin/env python3
"""usecase-assemble.py — usecase-analyze2 Step 1.5 결정적 조립 (LLM 0회).

stdin: 누적 draft JSON (Step 1.1 entities + 1.2 features + 1.3 scenarios + 1.4 diagrams)
stdout: business/use-cases.md 본문 (UTF-8 markdown)

입력 스키마 (필요한 키만 사용):
{
  "vosKeys": ["X-1", ...],
  "domain": {
    "scope": "...", "concepts": [{"name":"...","desc":"..."}, ...],
    "primaryActors": [{"name":"...","role":"..."}],
    "supportingActors": [{"name":"...","role":"..."}],
    "rules": ["..."]
  },
  "userStories": [{"id":"US-1","actor":"...","category":"Happy|Alternative|Exception",
                  "story":"...","vosKey":"X-1","ucId":"UC-1"}],
  "useCases": [{"id":"UC-1","summary":"...","stories":["US-1"],
                "feature":true,"note":"..."}],
  "features": [{"docId":"UC-1","summary":"...","actor":"...",
                "precondition":"...","basicflow":"...","alternativeflow":"...",
                "exceptionflow":"...","postcondition":"...","relatedVOS":["X-1"],
                "mode":"create","existingKey":""}],
  "diagrams": {
    "context": "@startuml\n...\n@enduml",
    "usecase": "@startuml\n...\n@enduml",
    "sequences": {"UC-1": "@startuml\n...\n@enduml"}
  }
}

template (선택): --template <path> 로 marker 기반 치환. 미지정 시 내장 템플릿 사용.
지원 marker (HTML 주석):
  <!-- INSERT business_domain HERE -->
  <!-- INSERT user_stories_table HERE -->
  <!-- INSERT use_cases_table HERE -->
  <!-- INSERT context_diagram HERE -->
  <!-- INSERT usecase_diagram HERE -->
  <!-- INSERT features_section HERE -->
  <!-- INSERT coverage_matrix HERE -->
"""
from __future__ import annotations

import argparse
import json
import sys


# ─── 섹션 빌더 ----------------------------------------------------------------

def build_business_domain(domain: dict) -> str:
    if not domain:
        return "_(domain 정보 없음)_"
    lines: list[str] = []
    lines.append("### 1.1 도메인 경계와 범위")
    scope = domain.get("scope") or "_(범위 미정)_"
    lines.append(f"- **시스템 책임 영역**: {scope}")
    if domain.get("boundaries"):
        for b in domain["boundaries"]:
            lines.append(f"- {b}")
    lines.append("")
    lines.append("### 1.2 핵심 도메인 개념")
    concepts = domain.get("concepts") or []
    if concepts:
        for c in concepts:
            n = c.get("name") or "?"
            d = c.get("desc") or ""
            lines.append(f"- **{n}**: {d}")
    else:
        lines.append("- _(미수집)_")
    lines.append("")
    lines.append("### 1.3 Actor 식별")
    pa = domain.get("primaryActors") or []
    sa = domain.get("supportingActors") or []
    lines.append("- **Primary Actor (가치 요청 트리거)**:")
    if pa:
        for a in pa:
            lines.append(f"  - {a.get('name','?')}: {a.get('role','')}")
    else:
        lines.append("  - _(미수집)_")
    lines.append("- **Supporting Actor (협력)**:")
    if sa:
        for a in sa:
            lines.append(f"  - {a.get('name','?')}: {a.get('role','')}")
    else:
        lines.append("  - _(없음)_")
    lines.append("")
    lines.append("### 1.4 도메인 규칙/제약")
    rules = domain.get("rules") or []
    if rules:
        for r in rules:
            lines.append(f"- {r}")
    else:
        lines.append("- _(미수집)_")
    return "\n".join(lines)


def build_user_stories_table(stories: list) -> str:
    if not stories:
        return "_(User Story 미도출)_"
    rows = ["| ID | Actor | 분류 | Story | 원본 VOS | 매핑 UC |",
            "|----|-------|------|-------|----------|---------|"]
    for s in stories:
        cat = s.get("category") or ""
        # 단일 글자 (A/B/C) 인 경우 풀어주기
        cat = {"A": "Happy", "B": "Alternative", "C": "Exception"}.get(cat, cat)
        rows.append("| {id} | {actor} | {cat} | {story} | {vos} | {uc} |".format(
            id=s.get("id", ""),
            actor=s.get("actor", ""),
            cat=cat,
            story=(s.get("story") or "").replace("|", "\\|"),
            vos=s.get("vosKey", ""),
            uc=s.get("ucId", ""),
        ))
    return "\n".join(rows)


def build_use_cases_table(use_cases: list, features: list) -> str:
    if not use_cases:
        return "_(Use Case 미도출)_"
    feature_doc_ids = {f.get("docId") for f in features if f.get("docId")}
    rows = ["| UC ID | Use Case 명 | 관련 User Story | Feature 선정 | 비고 |",
            "|-------|------------|----------------|:------------:|------|"]
    for u in use_cases:
        uid = u.get("id", "")
        nm = u.get("summary") or u.get("name") or ""
        stories = ", ".join(u.get("stories", []) or [])
        feat = "✅" if (u.get("feature") or uid in feature_doc_ids) else "—"
        note = u.get("note") or ""
        rows.append(f"| {uid} | {nm} | {stories} | {feat} | {note} |")
    return "\n".join(rows)


def build_features_section(features: list, sequences: dict) -> str:
    if not features:
        return "_(Feature 미도출)_"
    lines: list[str] = []
    for idx, f in enumerate(features, start=1):
        doc_id = f.get("docId", f"UC-{idx}")
        existing = f.get("existingKey") or "(생성 예정)"
        title = f.get("summary") or doc_id
        lines.append(f"### 5.{idx} {doc_id}: {title} (Feature 이슈: {existing})")
        lines.append("")
        lines.append(f"**Actor**: {f.get('actor') or '_(미정)_'}")
        if f.get("precondition"):
            lines.append(f"**Precondition**: {f['precondition']}")
        else:
            lines.append("**Precondition**: _(없음)_")
        lines.append("")
        lines.append("**Basic Flow**:")
        bf = (f.get("basicflow") or "").rstrip()
        if bf:
            lines.append(bf)
        else:
            lines.append("_(미작성)_")
        lines.append("")
        af = (f.get("alternativeflow") or "").strip()
        lines.append("**Alternative Flow**:")
        lines.append(af if af else "- (없음)")
        lines.append("")
        ef = (f.get("exceptionflow") or "").strip()
        lines.append("**Exception Flow**:")
        lines.append(ef if ef else "- (없음)")
        lines.append("")
        pc = (f.get("postcondition") or "").strip()
        lines.append(f"**Postcondition**: {pc if pc else '_(없음)_'}")
        lines.append("")
        lines.append("#### Sequence Diagram")
        lines.append("")
        seq = sequences.get(doc_id) if sequences else None
        if seq and seq.strip():
            lines.append("```plantuml")
            lines.append(seq.strip())
            lines.append("```")
        else:
            lines.append("_(Sequence Diagram 미작성 — critical)_")
        lines.append("")
        lines.append("---")
        lines.append("")
    # 마지막 trailing --- 제거
    while lines and lines[-1] in {"", "---"}:
        lines.pop()
    return "\n".join(lines)


def build_diagram_block(body: str | None, kind_label: str) -> str:
    if not body or not str(body).strip():
        return f"_({kind_label} 미작성 — critical)_"
    return "```plantuml\n" + body.strip() + "\n```"


def build_coverage_matrix(vos_keys: list, stories: list, features: list) -> str:
    rows = ["| VOS 키 | 파생 User Story | 매핑된 Feature | 비고 |",
            "|--------|----------------|---------------|------|"]
    # VOS → US 인덱스
    vos_to_us: dict[str, list[str]] = {k: [] for k in vos_keys}
    for s in stories:
        vk = s.get("vosKey")
        if vk in vos_to_us:
            vos_to_us[vk].append(s.get("id", ""))
    # VOS → feature docId 인덱스
    vos_to_feat: dict[str, list[str]] = {k: [] for k in vos_keys}
    for f in features:
        for vk in (f.get("relatedVOS") or []):
            if vk in vos_to_feat:
                vos_to_feat[vk].append(f.get("docId", ""))
    for vk in vos_keys:
        us_list = ", ".join(filter(None, vos_to_us.get(vk, []))) or "_(없음)_"
        feat_list = ", ".join(filter(None, vos_to_feat.get(vk, []))) or "_(없음)_"
        rows.append(f"| {vk} | {us_list} | {feat_list} | |")
    return "\n".join(rows)


# ─── 본 조립 ------------------------------------------------------------------

DEFAULT_TEMPLATE = """# Use Cases

## 1. Business Domain 분석

<!-- INSERT business_domain HERE -->

---

## 2. User Story

### 2.1 User Story 목록

<!-- INSERT user_stories_table HERE -->

⚠️ **모든 VOS 는 최소 1개 User Story 로 파생되어야 함** — VOS 누락 시 critical.

---

## 3. Use Case 목록

### 3.1 Use Case 요약

<!-- INSERT use_cases_table HERE -->

⚠️ Feature 로 선정되지 않은 Use Case 도 **삭제하지 않음** — 목록과 다이어그램에 그대로 유지.

---

## 4. 다이어그램 (전체)

### 4.1 Context Diagram

<!-- INSERT context_diagram HERE -->

### 4.2 Use Case Diagram

⚠️ **모든 Use Case 포함** — Feature 선정 여부와 무관.

<!-- INSERT usecase_diagram HERE -->

---

## 5. Feature 상세

⚠️ **모든 Feature 에 시나리오 + Sequence Diagram 필수** — 누락 시 critical.

<!-- INSERT features_section HERE -->

---

## 6. VOS 커버리지 매트릭스

<!-- INSERT coverage_matrix HERE -->

⚠️ **VOS → User Story → Use Case → Feature** 경로가 **모든** VOS 에 대해 추적 가능해야 함.
"""


def assemble(payload: dict, template: str) -> str:
    vos_keys = payload.get("vosKeys") or []
    domain = payload.get("domain") or {}
    stories = payload.get("userStories") or []
    use_cases = payload.get("useCases") or []
    features = payload.get("features") or []
    diagrams = payload.get("diagrams") or {}
    sequences = diagrams.get("sequences") or {}

    out = template
    replacements = {
        "<!-- INSERT business_domain HERE -->": build_business_domain(domain),
        "<!-- INSERT user_stories_table HERE -->": build_user_stories_table(stories),
        "<!-- INSERT use_cases_table HERE -->": build_use_cases_table(use_cases, features),
        "<!-- INSERT context_diagram HERE -->": build_diagram_block(diagrams.get("context"), "Context Diagram"),
        "<!-- INSERT usecase_diagram HERE -->": build_diagram_block(diagrams.get("usecase"), "Use Case Diagram"),
        "<!-- INSERT features_section HERE -->": build_features_section(features, sequences),
        "<!-- INSERT coverage_matrix HERE -->": build_coverage_matrix(vos_keys, stories, features),
    }
    for marker, body in replacements.items():
        out = out.replace(marker, body)
    return out


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--template", help="템플릿 파일 (marker 기반). 미지정 시 내장 템플릿 사용")
    args = parser.parse_args()

    try:
        payload = json.load(sys.stdin)
    except json.JSONDecodeError as exc:
        print(f"ERROR: invalid JSON: {exc}", file=sys.stderr)
        return 2

    template = DEFAULT_TEMPLATE
    if args.template:
        with open(args.template, encoding="utf-8") as fh:
            template = fh.read()

    sys.stdout.write(assemble(payload, template))
    return 0


if __name__ == "__main__":
    sys.exit(main())
