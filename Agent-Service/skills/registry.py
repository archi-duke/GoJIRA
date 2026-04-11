"""스킬 레지스트리 — Go skills/registry.go SkillRegistry + ToTool 대응."""

import os
from typing import Dict, List, Optional

from skills.loader import SkillDef, load_skills_from_dir
from config import settings


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
        """OpenAI function calling 형식 도구 목록 반환."""
        return [_to_openai_tool(s) for s in self.list_all()]

    def list_tools_info(self) -> List[dict]:
        """프론트엔드 /Agent/tools 응답용."""
        return [_to_tool_info(s) for s in self.list_all()]


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
    """스킬 자동 로드 — skills/defs/ 디렉토리에서 SKILL.md 로드."""
    registry = SkillRegistry()

    # 1. 환경변수 우선
    skills_dir = settings.skills_dir
    if not skills_dir:
        # 2. Agent-Service/skills/ (SKILL.md + executor.py가 함께 있는 디렉토리)
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
