"""스키마 검증기 — 스킬 입출력 Pydantic 검증."""

import logging
from typing import Dict, List, Any, Optional

log = logging.getLogger("agent.validators")


def validate_skill_params(skill_name: str, params: Dict[str, Any], required_fields: List[str]) -> Optional[str]:
    """스킬 파라미터 필수 필드 검증. 실패 시 에러 메시지 반환."""
    missing = [f for f in required_fields if not params.get(f)]
    if missing:
        return f"{skill_name}: 필수 파라미터 누락 — {', '.join(missing)}"
    return None


def validate_stage_result(stage_id: str, result: Any, min_length: int = 0) -> Optional[str]:
    """단계 결과 기본 검증."""
    if result is None:
        return f"{stage_id}: 결과가 비어 있습니다"
    if isinstance(result, str) and min_length > 0 and len(result) < min_length:
        return f"{stage_id}: 결과 길이 부족 ({len(result)} < {min_length})"
    if isinstance(result, dict):
        if result.get("error"):
            return f"{stage_id}: 결과에 에러 포함 — {result['error']}"
    return None


def validate_requirements_output(data: Any) -> Optional[str]:
    """요구사항 수집 결과 검증."""
    if not isinstance(data, dict):
        return "요구사항 결과가 dict가 아닙니다"
    issues = data.get("issues", [])
    if not issues:
        return "수집된 요구사항이 없습니다"
    return None


def validate_document_output(content: str, required_sections: List[str] = None) -> Optional[str]:
    """문서 작성 결과 검증."""
    if not content or len(content) < 100:
        return "문서 내용이 너무 짧습니다 (최소 100자)"
    if required_sections:
        missing = [s for s in required_sections if s not in content]
        if missing:
            return f"필수 섹션 누락: {', '.join(missing)}"
    return None
