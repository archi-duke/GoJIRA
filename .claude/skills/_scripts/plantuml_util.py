"""PlantUML 블록 파싱 / 종류 판별 공통 유틸.

Markdown 문서 안의 ` ```plantuml ... ``` ` 블록을 찾아 구조(@startuml/@enduml) 가
온전한지 확인하고, 내부 키워드로 다이어그램 종류를 추정한다.

사용 측 (validator) 는 이 모듈을 `import plantuml_util as pu` 로 쓴다.
"""
from __future__ import annotations

import re


PLANTUML_BLOCK_RE = re.compile(r"```plantuml\s*\n([\s\S]*?)```", re.IGNORECASE)
START_UML_RE = re.compile(r"^\s*@startuml\b", re.MULTILINE)
END_UML_RE = re.compile(r"^\s*@enduml\b", re.MULTILINE)

# 다이어그램 종류 판별용 키워드 regex (우선순위 순).
# - usecase: `usecase "..." as UC1` 또는 `(...) as UC1` 축약
# - sequence: participant/boundary/control/entity/queue/collections 선언
# - class: class/interface/abstract/enum 선언
# - deployment: node/artifact/cloud/frame/database
# - package: package/folder (개발 뷰)
# - component: component 단독
# - state/activity: state 선언 / :label;
_USECASE_KEYWORD_RE = re.compile(r"^\s*usecase\s+", re.MULTILINE | re.IGNORECASE)
_USECASE_ALIAS_RE = re.compile(r"\(\s*[^)]+\s*\)\s*as\s+\w+", re.IGNORECASE)
_SEQUENCE_RE = re.compile(
    r"^\s*(participant|boundary|control|entity|queue|collections)\b",
    re.MULTILINE | re.IGNORECASE,
)
_CLASS_RE = re.compile(
    r"^\s*(class|interface|abstract|enum)\s+\w+",
    re.MULTILINE | re.IGNORECASE,
)
_DEPLOY_RE = re.compile(
    r"^\s*(node|artifact|cloud|frame|database)\b",
    re.MULTILINE | re.IGNORECASE,
)
_PACKAGE_RE = re.compile(
    r"^\s*(package|folder)\b", re.MULTILINE | re.IGNORECASE
)
_COMPONENT_RE = re.compile(r"^\s*component\s+", re.MULTILINE | re.IGNORECASE)
_STATE_RE = re.compile(r"^\s*state\s+", re.MULTILINE | re.IGNORECASE)
_ACTIVITY_RE = re.compile(r"^\s*:[^\n]+;", re.MULTILINE)


def find_blocks(text: str) -> list[str]:
    """Markdown 안의 plantuml code fence 본문들을 리스트로 반환."""
    if not isinstance(text, str):
        return []
    return PLANTUML_BLOCK_RE.findall(text)


def has_block(text: str) -> bool:
    return bool(find_blocks(text))


def has_uml_pair(body: str) -> bool:
    """블록 본문에 @startuml / @enduml 쌍이 모두 있는지."""
    if not isinstance(body, str):
        return False
    return bool(START_UML_RE.search(body)) and bool(END_UML_RE.search(body))


def detect_kind(body: str) -> str:
    """블록 내부 키워드로 다이어그램 종류 추정.

    우선순위: usecase > sequence > class > deployment > package > component >
    state > activity > unknown.
    """
    if not isinstance(body, str):
        return "unknown"
    if _USECASE_KEYWORD_RE.search(body) or _USECASE_ALIAS_RE.search(body):
        return "usecase"
    if _SEQUENCE_RE.search(body):
        return "sequence"
    if _CLASS_RE.search(body):
        return "class"
    if _DEPLOY_RE.search(body):
        return "deployment"
    if _PACKAGE_RE.search(body):
        return "package"
    if _COMPONENT_RE.search(body):
        return "component"
    if _STATE_RE.search(body):
        return "state"
    if _ACTIVITY_RE.search(body):
        return "activity"
    return "unknown"
