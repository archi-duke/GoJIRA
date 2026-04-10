"""LLM 호출 모듈 — OpenAI SDK 기반. Go skills/helpers.go 대응."""

import logging
from typing import Optional, List, Dict

from openai import AsyncOpenAI

from config import settings, llm_config

log = logging.getLogger("agent.llm")

_client: Optional[AsyncOpenAI] = None


def _get_client() -> AsyncOpenAI:
    global _client
    if _client is None:
        kwargs = {}
        if llm_config.auth_type == "bearer":
            kwargs["api_key"] = settings.openai_api_key
        elif llm_config.auth_type == "header":
            kwargs["api_key"] = settings.openai_api_key
            kwargs["default_headers"] = {llm_config.auth_header: settings.openai_api_key}
        else:
            kwargs["api_key"] = "dummy"

        # endpoint에서 /chat/completions 제거하여 base_url 생성
        base_url = llm_config.endpoint
        if base_url.endswith("/chat/completions"):
            base_url = base_url[: -len("/chat/completions")]
        kwargs["base_url"] = base_url

        _client = AsyncOpenAI(**kwargs)
    return _client


async def call_llm(prompt: str) -> str:
    """단순 LLM 호출 (tool_use 없이). Go CallLLM() 대응."""
    client = _get_client()
    log.info("LLM call (simple): model=%s", llm_config.model)

    resp = await client.chat.completions.create(
        model=llm_config.model,
        messages=[{"role": "user", "content": prompt}],
        max_tokens=llm_config.max_tokens,
        temperature=llm_config.temperature,
    )

    text = resp.choices[0].message.content or ""
    # 코드블록 제거 (Go와 동일)
    text = text.strip()
    if text.startswith("```"):
        nl = text.find("\n")
        if nl != -1:
            text = text[nl + 1 :]
        if text.endswith("```"):
            text = text[:-3]
        text = text.strip()

    return text


async def call_llm_with_tools(
    messages: List[Dict],
    tools: List[Dict],
    max_tokens: Optional[int] = None,
    tool_choice: Optional[str] = None,
) -> dict:
    """Tool-use LLM 호출. Go CallLLMWithTools() 대응.

    Args:
        tool_choice: "none"이면 도구 호출 없이 텍스트만, "auto"(기본)이면 자동 판단

    Returns:
        {"message": ChatCompletionMessage, "finish_reason": str}
    """
    client = _get_client()
    mt = max_tokens or max(llm_config.max_tokens, 2000)

    kwargs = {
        "model": llm_config.model,
        "messages": messages,
        "tools": tools,
        "max_tokens": mt,
        "temperature": llm_config.temperature,
    }
    if tool_choice:
        kwargs["tool_choice"] = tool_choice

    log.info("LLM call (tools): model=%s, tools=%d, messages=%d, tool_choice=%s",
             llm_config.model, len(tools), len(messages), tool_choice or "auto")

    resp = await client.chat.completions.create(**kwargs)

    choice = resp.choices[0]
    msg = choice.message

    if msg.tool_calls:
        log.info("LLM tool_calls: %s", [tc.function.name for tc in msg.tool_calls])
    else:
        log.info("LLM no tool_calls, content: %.100s", msg.content or "")

    return {
        "message": msg,
        "finish_reason": choice.finish_reason,
    }
