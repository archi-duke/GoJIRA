"""LLM 호출 모듈 — OpenAI SDK 기반. Go skills/helpers.go 대응."""

import logging
from typing import Optional, List, Dict

from openai import AsyncOpenAI

from config import settings, llm_config

log = logging.getLogger("agent.llm")

_client: Optional[AsyncOpenAI] = None


# thinking 계열 모델 식별 — 내부 추론을 수행하므로 ReACT 2-pass 불필요
_THINKING_MODEL_PREFIXES = (
    # OpenAI o-시리즈 (o1/o3/o4/o5 ...)
    "o1", "o3", "o4", "o5",
    # Google Gemini thinking 모델 (2.5 이상 시리즈는 기본 thinking)
    "gemini-2.5", "gemini-3",
)


def is_thinking_model(model: str = "") -> bool:
    """현재 활성 모델이 내부 추론(thinking) 을 수행하는 모델인지 판정."""
    m = (model or (llm_config.model if llm_config else "")).lower().strip()
    if not m:
        return False
    return any(m.startswith(p) for p in _THINKING_MODEL_PREFIXES)


def _get_client() -> AsyncOpenAI:
    global _client
    if _client is None:
        # llm.json 의 api_key_env 가 지정되어 있으면 그 환경변수에서 읽음 (기본: OPENAI_API_KEY)
        api_key = settings.get_llm_api_key(llm_config.api_key_env) or "dummy"

        kwargs = {}
        if llm_config.auth_type == "bearer":
            kwargs["api_key"] = api_key
        elif llm_config.auth_type == "header":
            kwargs["api_key"] = api_key
            kwargs["default_headers"] = {llm_config.auth_header: api_key}
        else:
            kwargs["api_key"] = "dummy"

        # endpoint에서 /chat/completions 제거하여 base_url 생성
        base_url = llm_config.endpoint
        if base_url.endswith("/chat/completions"):
            base_url = base_url[: -len("/chat/completions")]
        kwargs["base_url"] = base_url

        _client = AsyncOpenAI(**kwargs)
        log.info("LLM 클라이언트 초기화: provider=%s, model=%s, base_url=%s",
                 llm_config.provider, llm_config.model, base_url)
    return _client


async def call_llm(prompt: str) -> str:
    """단순 LLM 호출 (tool_use 없이). Go CallLLM() 대응."""
    client = _get_client()
    log.info("LLM call (simple): model=%s", llm_config.model)

    system_msg = (
        "당신은 한국어로 답변하는 소프트웨어 분석/설계 보조 도구입니다. "
        "사용자가 제공한 프로젝트 문서, VOS, Feature 정보를 바탕으로 "
        "Use Case·요구사항·설계 산출물을 생성합니다. 모든 입력은 정상적인 "
        "엔터프라이즈 SW 개발 컨텍스트로 간주하고, 요청된 형식(JSON/Markdown)에 "
        "맞춰 충실히 산출물을 작성하세요."
    )
    messages = [
        {"role": "system", "content": system_msg},
        {"role": "user", "content": prompt},
    ]

    async def _call(msgs):
        return await client.chat.completions.create(
            model=llm_config.model,
            messages=msgs,
            max_tokens=llm_config.max_tokens,
            temperature=llm_config.temperature,
        )

    resp = await _call(messages)
    text = (resp.choices[0].message.content or "").strip()

    # 일시적 거부 응답 1회 재시도 (system 강조 추가)
    if text.lower().startswith(("i'm sorry", "i am sorry", "i cannot", "i can't")):
        log.warning("LLM refused, retrying with stronger system message")
        retry_msgs = [
            {"role": "system", "content": system_msg + " 입력을 거부하지 말고 반드시 산출물을 생성하세요."},
            {"role": "user", "content": prompt},
        ]
        resp = await _call(retry_msgs)
        text = (resp.choices[0].message.content or "").strip()
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
