"""스킬 레지스트리 — Claude 스타일 스킬 선별 지원.

스킬을 매 요청마다 전부 보내지 않고, 사용자 메시지에 맞는 스킬만
선별하여 LLM에 전달한다 (Claude Code의 description 기반 매칭과 동일 컨셉).
"""

import os
import re
import logging
from typing import Dict, List, Optional, Set

from skills.loader import SkillDef, load_skills_from_dir
from config import settings

log = logging.getLogger("agent.registry")

# 항상 포함되는 기본 스킬 (프로젝트 조회는 대부분의 작업에 필요)
ALWAYS_INCLUDE = {"project_query"}

# 스킬 그룹: 하나가 매칭되면 그룹 전체를 포함
_ISSUE_CREATE_ALL = [
    "issue_create_vos",
    "issue_create_feature",
    "issue_create_requirement",
    "issue_create_design",
]

SKILL_GROUPS = {
    "issue": ["issue_query", *_ISSUE_CREATE_ALL, "issue_update", "issue_delete", "issue_link", "issue_analyze"],
    "doc": ["doc_list", "doc_summarize", "doc_diff", "doc_history"],
    "pr": ["pr_list", "pr_create"],
    "design": ["satam_analyze", "issue_analyze", "issue_query", "issue_create_design", "issue_link"],
    "projdesc": ["projdesc_interview", "projdesc_generate"],
    "requirement_workflow": ["vos_collect", "usecase_analyze", "requirement_refine", "issue_query", "issue_analyze"],
}

# 키워드 → 스킬 이름 매핑 (description 보완)
KEYWORD_MAP = {
    # 이슈 관련
    "이슈": ["issue_query", *_ISSUE_CREATE_ALL, "issue_update", "issue_delete"],
    "issue": ["issue_query", *_ISSUE_CREATE_ALL, "issue_update", "issue_delete"],
    "생성": _ISSUE_CREATE_ALL,
    "수정": ["issue_update"],
    "삭제": ["issue_delete"],
    "관계": ["issue_link", "issue_analyze"],
    "link": ["issue_link"],
    "contain": ["issue_link", "issue_analyze"],
    "고아": ["issue_analyze"],
    # 타입별 — 해당 타입의 생성 스킬만 선별
    "vos": ["issue_query", "issue_create_vos", "vos_collect"],
    "feature": ["issue_query", "issue_create_feature", "usecase_analyze"],
    "유즈케이스": ["issue_query", "issue_create_feature", "usecase_analyze"],
    "usecase": ["issue_query", "issue_create_feature", "usecase_analyze"],
    "use case": ["issue_query", "issue_create_feature", "usecase_analyze"],
    "use-case": ["issue_query", "usecase_analyze"],
    "유즈 케이스": ["usecase_analyze"],
    "actor": ["usecase_analyze"],
    "시나리오": ["usecase_analyze"],
    "requirement": ["issue_query", "issue_create_requirement", "requirement_refine"],
    "요구사항": ["issue_query", "issue_create_requirement", "vos_collect", "requirement_refine"],
    "정제": ["requirement_refine"],
    "user story": ["requirement_refine"],
    "유저 스토리": ["requirement_refine"],
    "qas": ["requirement_refine"],
    "fr": ["issue_create_requirement"],
    "nfr": ["issue_create_requirement"],
    "qa": ["issue_create_requirement"],
    "품질속성": ["issue_create_requirement"],
    "품질 속성": ["issue_create_requirement"],
    "cnst": ["issue_create_requirement"],
    "제약": ["issue_create_requirement"],
    # 설계
    "설계": ["issue_query", "issue_create_design", "satam_analyze", "driver_select"],
    "design": ["issue_query", "issue_create_design", "driver_select"],
    "adsn": ["issue_create_design"],
    "ddsn": ["issue_create_design"],
    "아키텍처": ["issue_create_design", "driver_select"],
    "architectural": ["issue_create_design", "driver_select"],
    "detailed": ["issue_create_design"],
    "driver": ["driver_select"],
    "드라이버": ["driver_select"],
    "qad": ["driver_select"],
    "qai": ["driver_select"],
    "asr": ["driver_select"],
    "architecture driver": ["driver_select"],
    "design point": ["design_decide"],
    "디자인 포인트": ["design_decide"],
    "설계 결정": ["design_decide"],
    "설계안": ["design_decide"],
    "approach": ["design_decide"],
    "trade-off": ["design_decide"],
    "스타일": ["style_define"],
    "style": ["style_define"],
    "architecture style": ["style_define"],
    "아키텍처 스타일": ["style_define"],
    "시스템 스타일": ["style_define"],
    "microservices": ["style_define"],
    "monolith": ["style_define"],
    "hexagonal": ["style_define"],
    "event-driven": ["style_define"],
    "layered": ["style_define"],
    "cqrs": ["style_define"],
    "serverless": ["style_define"],
    "service mesh": ["style_define"],
    "micro-frontend": ["style_define"],
    # 문서 관련
    "문서": ["doc_list", "doc_summarize", "doc_diff", "doc_history"],
    "doc": ["doc_list", "doc_summarize", "doc_diff", "doc_history"],
    "설계문서": ["doc_list", "doc_summarize", "doc_diff"],
    "요약": ["doc_summarize"],
    "diff": ["doc_diff"],
    "커밋": ["doc_history"],
    "이력": ["doc_history"],
    # PR 관련
    "pr": ["pr_list", "pr_create"],
    "pull request": ["pr_list", "pr_create"],
    "발행": ["pr_create"],
    # 분석
    "satam": ["satam_analyze"],
    "리스크": ["satam_analyze"],
    "risk": ["satam_analyze"],
    "sensitivity": ["satam_analyze"],
    "mitigation": ["satam_analyze"],
    "일정": ["schedule_analyze", "project_query", "issue_query"],
    "마일스톤": ["schedule_analyze", "project_query"],
    "스케줄": ["schedule_analyze", "project_query"],
    "schedule": ["schedule_analyze", "project_query"],
    # 프로젝트 설명
    "project-description": ["projdesc_interview", "projdesc_generate"],
    "projdesc": ["projdesc_interview", "projdesc_generate"],
    "인터뷰": ["projdesc_interview", "projdesc_generate"],
    "과제 소개": ["projdesc_interview", "projdesc_generate"],
    "과제 설명": ["projdesc_interview", "projdesc_generate"],
    "문서 생성": ["projdesc_generate", "projdesc_interview"],
    "문서 작성": ["projdesc_generate", "projdesc_interview"],
    "문서를 생성": ["projdesc_generate"],
    "문서를 작성": ["projdesc_generate"],
    # 네비게이션
    "이동": ["ui_navigate"],
    "열어": ["ui_navigate"],
    "보여줘": ["ui_navigate"],
    # 프로젝트
    "프로젝트": ["project_query", "issue_query"],
    "project": ["project_query", "issue_query"],
    "목록": ["project_query"],
    "몇개": ["project_query", "issue_query"],
    "몇 개": ["project_query", "issue_query"],
}

# 매칭 결과의 최소/최대 스킬 수
MIN_TOOLS = 3
MAX_TOOLS = 10


class SkillRegistry:
    def __init__(self):
        self._skills: Dict[str, SkillDef] = {}
        self._order: List[str] = []

    def register(self, skill: SkillDef):
        if skill.name not in self._skills:
            self._order.append(skill.name)
        self._skills[skill.name] = skill

    def get(self, name: str) -> Optional[SkillDef]:
        return self._skills.get(name)

    def list_all(self) -> List[SkillDef]:
        return [self._skills[n] for n in self._order if n in self._skills]

    def list_openai_tools(self) -> List[dict]:
        """전체 도구 목록 (레거시 호환용)."""
        return [_to_openai_tool(s) for s in self.list_all()]

    def list_tools_info(self) -> List[dict]:
        """프론트엔드 /Agent/tools 응답용."""
        return [_to_tool_info(s) for s in self.list_all()]

    def select_tools(self, message: str, extra_context: str = "") -> List[dict]:
        """사용자 메시지 기반으로 관련 스킬만 선별하여 OpenAI tool 형식 반환.

        1. 키워드 매칭으로 후보 스킬 선별
        2. description 매칭으로 추가 발견
        3. 스킬 그룹으로 관련 스킬 확장
        4. 항상 포함 스킬 추가
        5. MIN_TOOLS ~ MAX_TOOLS 범위 보정
        """
        text = (message + " " + extra_context).lower()
        matched: Set[str] = set()

        # 1. 키워드 매칭
        for keyword, skill_names in KEYWORD_MAP.items():
            if keyword in text:
                matched.update(skill_names)

        # 2. description 매칭 — 스킬 description에서 메시지 단어와 겹치는 것
        # 일반적인 단어(프로젝트, 분석 등)를 제외하고 매칭
        _STOPWORDS = {"프로젝트", "분석", "조회", "목록", "합니다", "수행", "이슈", "상태", "코드"}
        words = set(re.findall(r'[가-힣]{2,}|[a-zA-Z_]{3,}', text)) - _STOPWORDS
        for skill in self.list_all():
            desc_lower = skill.description.lower()
            overlap = sum(1 for w in words if w in desc_lower)
            if overlap >= 2:
                matched.add(skill.name)

        # 3. 스킬 그룹 확장 — 매칭된 스킬이 속한 그룹의 다른 스킬도 포함
        expanded: Set[str] = set(matched)
        for group_name, group_skills in SKILL_GROUPS.items():
            if matched & set(group_skills):
                expanded.update(group_skills)

        # 4. 항상 포함 스킬
        expanded.update(ALWAYS_INCLUDE)

        # 5. 범위 보정
        if len(expanded) < MIN_TOOLS:
            # 너무 적으면 전체 반환
            expanded = {s.name for s in self.list_all()}
        elif len(expanded) > MAX_TOOLS:
            # 그룹 확장 전 원본 매칭 우선, 나머지는 잘라냄
            priority = list(matched | ALWAYS_INCLUDE)
            rest = [s for s in expanded if s not in priority]
            expanded = set(priority[:MAX_TOOLS]) | ALWAYS_INCLUDE
            remaining = MAX_TOOLS - len(expanded)
            if remaining > 0:
                expanded.update(rest[:remaining])

        # 순서 유지하면서 필터링
        selected = [s for s in self.list_all() if s.name in expanded]

        log.info("select_tools: %d/%d 스킬 선별 — %s",
                 len(selected), len(self._order),
                 [s.name for s in selected])

        return [_to_openai_tool(s) for s in selected]


def _to_openai_tool(skill: SkillDef) -> dict:
    """Go ToTool() 대응 — OpenAI function calling 형식."""
    props = {}
    required = []
    for k, p in skill.parameters.items():
        prop = {"type": p.type, "description": p.description}
        if p.enum:
            prop["enum"] = p.enum
        props[k] = prop
        if p.required:
            required.append(k)

    schema = {"type": "object", "properties": props}
    if required:
        schema["required"] = required

    return {
        "type": "function",
        "function": {
            "name": skill.name,
            "description": skill.description,
            "parameters": schema,
        },
    }


def _to_tool_info(skill: SkillDef) -> dict:
    """Go AgentToolsHandler 응답 형식."""
    return {
        "name": skill.name,
        "description": skill.description,
        "input_schema": {
            "type": "object",
            "properties": {
                k: {"type": p.type, "description": p.description, **({"enum": p.enum} if p.enum else {})}
                for k, p in skill.parameters.items()
            },
            "required": [k for k, p in skill.parameters.items() if p.required],
        },
    }


def auto_load_skills() -> SkillRegistry:
    """스킬 자동 로드."""
    registry = SkillRegistry()

    skills_dir = settings.skills_dir
    if not skills_dir:
        skills_dir = os.path.dirname(os.path.abspath(__file__))

    if skills_dir:
        for skill in load_skills_from_dir(skills_dir):
            registry.register(skill)

    print(f"[skills] 총 {len(registry._order)}개 스킬 로드 완료")
    return registry


# 싱글턴
default_registry: Optional[SkillRegistry] = None


def get_registry() -> SkillRegistry:
    global default_registry
    if default_registry is None:
        default_registry = auto_load_skills()
    return default_registry
