"""이슈 YAML 스키마 — Git 기반 이슈 저장소의 진실 원천 모델.

docs/issue-yaml-schema.md 의 필드 정의를 pydantic 모델로 강제한다.
- forward compat: 미지 필드는 `extra` 로 보존 (경고 로그)
- 필수 필드: key, subkey, issueType, summary
- 로더는 YAML 문자열/Path → IssueDoc 인스턴스로 변환하고, 직렬화는 그 역
"""

from __future__ import annotations

import logging
from pathlib import Path
from typing import Any, List, Optional, Literal, Union

import yaml
from pydantic import BaseModel, ConfigDict, Field, ValidationError

log = logging.getLogger("agent.issue_schema")


IssueType = Literal["VOS", "Feature", "Requirement", "Architectural Design", "Detailed Design"]


class IssueLinkRef(BaseModel):
    """issueLinks 내부의 inward/outward 참조."""
    model_config = ConfigDict(extra="allow")
    key: str


class IssueLinkType(BaseModel):
    model_config = ConfigDict(extra="allow")
    name: str


class IssueLink(BaseModel):
    model_config = ConfigDict(extra="allow")
    id: Optional[str] = None
    type: IssueLinkType
    inwardIssue: Optional[IssueLinkRef] = None
    outwardIssue: Optional[IssueLinkRef] = None


class Comment(BaseModel):
    model_config = ConfigDict(extra="allow")
    id: Optional[str] = None
    author: Optional[str] = None
    body: str = ""
    created: Optional[str] = None
    updated: Optional[str] = None


class ApproachItem(BaseModel):
    """ADSN/DDSN 의 대안 비교 항목."""
    model_config = ConfigDict(extra="allow")
    name: str
    pros: str = ""
    cons: str = ""


class IssueDoc(BaseModel):
    """이슈 1건 = 1 YAML 파일.

    공통 + 유형별 필드를 단일 flat 모델로 병합 (YAML 편집성 우선).
    유형별 필드는 Optional — 해당 유형이 아니면 비워둔다.
    """
    model_config = ConfigDict(extra="allow", str_strip_whitespace=False)

    # ── 공통 필드 ──
    key: str
    subkey: str
    issueType: IssueType
    summary: str
    description: str = ""
    status: str = "To Do"
    priority: Union[str, float, int] = "Medium"
    reporter: str = ""
    assignee: str = ""
    labels: List[str] = Field(default_factory=list)
    created: Optional[str] = None
    updated: Optional[str] = None

    issueLinks: List[IssueLink] = Field(default_factory=list)
    comments: List[Comment] = Field(default_factory=list)

    # ── VOS ──
    source: Optional[str] = None
    objectives: Optional[str] = None

    # ── Feature (Use Case) ──
    actor: Optional[str] = None
    precondition: Optional[str] = None
    postcondition: Optional[str] = None
    basicflow: Optional[str] = None
    alternativeflow: Optional[str] = None
    exceptionflow: Optional[str] = None

    # ── Requirement ──
    reqtype: Optional[str] = None  # 권장 값: FR | NFR | CNST (legacy 허용)
    driverSelected: Optional[bool] = None
    stimulus: Optional[str] = None
    response: Optional[str] = None
    responseScenario: Optional[str] = None
    environment: Optional[str] = None
    artifact: Optional[str] = None
    measure: Optional[str] = None
    measureAuto: Optional[bool] = None
    qas: Optional[str] = None
    qai: Union[str, int, float, None] = None
    qad: Union[str, int, float, None] = None

    # ── Architectural / Detailed Design ──
    designLevel: Optional[Literal["Architectural", "Detailed"]] = None
    designPoint: Optional[str] = None
    decision: Optional[str] = None
    decisionRationale: Optional[str] = None
    approaches: Union[str, List[ApproachItem], None] = None  # 문자열(legacy JSON) 또는 리스트

    # ── SATAM (ADSN/DDSN 확장) ──
    risk: Optional[str] = None
    sensitivity: Optional[str] = None
    mitigation: Optional[str] = None
    tradeoffs: Optional[str] = None
    nonrisk: Optional[str] = None

    # ── metadata (optional wrapper for any source-of-origin) ──
    metadata: dict = Field(default_factory=dict)


# ──────────────────────────────────────────────────────────────────────────
# 로드 / 직렬화 유틸
# ──────────────────────────────────────────────────────────────────────────


def load_issue_yaml(text: str, *, strict: bool = False) -> IssueDoc:
    """YAML 문자열 → IssueDoc. strict=True 면 검증 실패 시 예외."""
    data = yaml.safe_load(text) or {}
    try:
        return IssueDoc.model_validate(data)
    except ValidationError as e:
        if strict:
            raise
        log.warning("issue YAML 검증 경고 (key=%s): %s",
                    (data or {}).get("key", "?"), e.errors()[:3])
        # 느슨한 복구: 필수 필드 누락이면 여전히 실패
        return IssueDoc.model_validate(data)


def load_issue_file(path: Union[str, Path], *, strict: bool = False) -> IssueDoc:
    """파일 경로 → IssueDoc."""
    p = Path(path)
    return load_issue_yaml(p.read_text(encoding="utf-8"), strict=strict)


def _str_representer(dumper, data):
    """멀티라인 문자열은 literal block (`|`) 로 덤프."""
    if isinstance(data, str) and "\n" in data:
        return dumper.represent_scalar("tag:yaml.org,2002:str", data, style="|")
    return dumper.represent_scalar("tag:yaml.org,2002:str", data)


class _IssueYamlDumper(yaml.SafeDumper):
    pass


_IssueYamlDumper.add_representer(str, _str_representer)


def dump_issue_yaml(doc: IssueDoc) -> str:
    """IssueDoc → YAML 문자열. 키 순서는 모델 선언 순서 유지.

    - None 필드는 제외 (편집성 우선)
    - 빈 컬렉션은 명시적으로 남김 (`labels: []`)
    - 멀티라인 문자열은 literal block (`|`) 로 출력
    """
    raw = doc.model_dump(mode="json", exclude_none=True)
    return yaml.dump(
        raw,
        Dumper=_IssueYamlDumper,
        allow_unicode=True,
        sort_keys=False,
        default_flow_style=False,
        width=120,
    )


def dump_issue_file(doc: IssueDoc, path: Union[str, Path]) -> None:
    p = Path(path)
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text(dump_issue_yaml(doc), encoding="utf-8")


# ──────────────────────────────────────────────────────────────────────────
# Mongo 문서 ↔ YAML 변환 (마이그레이션/sync 훅 용)
# ──────────────────────────────────────────────────────────────────────────


# Mongo 에 남아있으나 YAML 로 옮기지 않는 내부 필드
_MONGO_INTERNAL_FIELDS = {
    "_id", "_source", "_syncedAt", "_dirty", "_deletedLinks",
}


def from_mongo(doc: dict) -> IssueDoc:
    """Mongo issues 도큐먼트 → IssueDoc (마이그레이션용).

    JIRA-DC sync 로 들어온 legacy 형태 정규화:
    - status/priority: {id, name} dict → name 문자열
    - reporter/assignee: user dict → name 문자열
    - description None → ""
    """
    clean = {k: v for k, v in (doc or {}).items() if k not in _MONGO_INTERNAL_FIELDS}

    # 중첩 dict → name 추출
    for f in ("status", "priority", "reporter", "assignee"):
        v = clean.get(f)
        if isinstance(v, dict):
            clean[f] = v.get("name") or v.get("displayName") or ""
        elif v is None and f in ("reporter", "assignee"):
            clean[f] = ""

    # description None 안전 처리
    if clean.get("description") is None:
        clean["description"] = ""

    # subkey 누락 시 key 에서 파생 (legacy 이슈 대응)
    if not clean.get("subkey") and clean.get("key"):
        clean["subkey"] = str(clean["key"]).replace("-", "_")

    # metadata 흡수
    if doc.get("_source"):
        clean.setdefault("metadata", {})["source"] = doc["_source"]

    return IssueDoc.model_validate(clean)


def to_mongo(doc: IssueDoc) -> dict:
    """IssueDoc → Mongo upsert 용 dict. 내부 필드는 호출 측이 채움."""
    return doc.model_dump(mode="json", exclude_none=True)


__all__ = [
    "IssueDoc",
    "IssueLink",
    "IssueLinkType",
    "IssueLinkRef",
    "Comment",
    "ApproachItem",
    "load_issue_yaml",
    "load_issue_file",
    "dump_issue_yaml",
    "dump_issue_file",
    "from_mongo",
    "to_mongo",
]
