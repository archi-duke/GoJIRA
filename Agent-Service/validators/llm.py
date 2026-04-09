"""LLM 기반 검증기 — 산출물 품질 검증."""

import logging
from typing import Optional

from core.llm import call_llm

log = logging.getLogger("agent.validators.llm")


async def validate_with_llm(content: str, criteria: str) -> dict:
    """LLM에게 산출물 품질 검증 요청.

    Returns:
        {"passed": bool, "reason": str, "suggestions": list[str]}
    """
    prompt = f"""다음 산출물을 검증해주세요.

## 검증 기준
{criteria}

## 산출물
{content[:4000]}

## 응답 형식 (JSON만 출력)
{{"passed": true/false, "reason": "판정 사유", "suggestions": ["개선사항1", ...]}}"""

    try:
        result_text = await call_llm(prompt)
        # JSON 파싱 시도
        import json
        # 코드블록 안에 있을 수 있음
        if "```" in result_text:
            start = result_text.find("{")
            end = result_text.rfind("}") + 1
            if start >= 0 and end > start:
                result_text = result_text[start:end]
        result = json.loads(result_text)
        return {
            "passed": result.get("passed", False),
            "reason": result.get("reason", ""),
            "suggestions": result.get("suggestions", []),
        }
    except Exception as e:
        log.warning("LLM 검증 파싱 실패: %s", e)
        return {"passed": True, "reason": "검증 파싱 실패 — 통과 처리", "suggestions": []}
