"""SKILL.md 파서 + executor 로더."""

import os
import importlib
import asyncio
from typing import Any, Callable, Dict, List, Optional

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
    prompt: str = ""
    dir: str = ""
    executor: Optional[Callable] = None  # execute(params, user) 함수 참조

    model_config = {"arbitrary_types_allowed": True}


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

    skill_dir = os.path.dirname(path)

    # executor.py 로드
    executor_fn = _load_executor(skill_dir, name)

    return SkillDef(
        name=name,
        description=raw.get("description", ""),
        parameters=params,
        prompt=body.strip(),
        dir=skill_dir,
        executor=executor_fn,
    )


def _load_executor(skill_dir: str, skill_name: str) -> Optional[Callable]:
    """스킬 디렉토리에서 executor.py의 execute() 함수를 로드."""
    executor_path = os.path.join(skill_dir, "executor.py")
    if not os.path.isfile(executor_path):
        return None

    module_name = f"skills.{skill_name}.executor"
    try:
        module = importlib.import_module(module_name)
        fn = getattr(module, "execute", None)
        if fn and callable(fn):
            return fn
        print(f"[skills] {skill_name}/executor.py에 execute() 함수 없음")
    except Exception as e:
        print(f"[skills] {skill_name}/executor.py 로드 실패: {e}")
    return None


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
                exec_mark = "+" if skill.executor else "-"
                print(f"[skills] 로드: {skill.name} [{exec_mark}] ({skill.description[:40]}...)")
            except Exception as e:
                print(f"[skills] 로드 실패 ({skill_file}): {e}")
    return skills
