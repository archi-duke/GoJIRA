"""SKILL.md 파서 — Go skills/registry.go ParseSkillFile/splitFrontmatter 대응."""

import os
from typing import Dict, List, Optional

import yaml
from pydantic import BaseModel


class ParameterDef(BaseModel):
    type: str = "string"
    description: str = ""
    required: bool = False
    enum: Optional[List[str]] = None


class SkillDef(BaseModel):
    name: str
    description: str = ""
    parameters: Dict[str, ParameterDef] = {}
    prompt: str = ""  # markdown body
    dir: str = ""     # SKILL.md가 위치한 디렉토리


def parse_skill_file(path: str) -> SkillDef:
    """SKILL.md 파일 파싱 (YAML frontmatter + markdown body)."""
    with open(path, "r", encoding="utf-8") as f:
        content = f.read()

    frontmatter, body = _split_frontmatter(content)

    raw = yaml.safe_load(frontmatter) or {}
    params = {}
    for k, v in (raw.get("parameters") or {}).items():
        if isinstance(v, dict):
            params[k] = ParameterDef(**v)

    name = raw.get("name", "")
    if not name:
        name = os.path.basename(os.path.dirname(path))

    return SkillDef(
        name=name,
        description=raw.get("description", ""),
        parameters=params,
        prompt=body.strip(),
        dir=os.path.dirname(path),
    )


def _split_frontmatter(content: str):
    """--- 로 구분된 YAML frontmatter와 markdown body 분리."""
    if not content.startswith("---"):
        return "", content

    end = content.find("---", 3)
    if end == -1:
        return "", content

    fm = content[3:end].strip()
    body = content[end + 3:]
    return fm, body


def load_skills_from_dir(directory: str) -> List[SkillDef]:
    """디렉토리에서 모든 SKILL.md 파일 로드."""
    skills = []
    if not os.path.isdir(directory):
        return skills

    for entry in sorted(os.listdir(directory)):
        skill_file = os.path.join(directory, entry, "SKILL.md")
        if os.path.isfile(skill_file):
            try:
                skill = parse_skill_file(skill_file)
                skills.append(skill)
                print(f"[skills] 로드: {skill.name} ({skill.description[:40]}...)")
            except Exception as e:
                print(f"[skills] 로드 실패 ({skill_file}): {e}")
    return skills
