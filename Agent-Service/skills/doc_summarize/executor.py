"""문서 요약 스킬."""

from clients.platform_api import PlatformClient
from core.llm import call_llm


async def execute(params: dict, user: str) -> dict:
    code = params.get("projectCode", "")
    fp = params.get("filePath", "")
    if not code or not fp:
        raise ValueError("projectCode, filePath 필수")

    client = PlatformClient()
    content = await client.get_file_content(code, fp, ref="main")
    if not content:
        content = await client.get_file_content(code, fp, ref=f"docs/{user}")
    if not content:
        return {"summary": "파일이 비어 있습니다."}

    truncated = content[:8000] if len(content) > 8000 else content
    summary = await call_llm(f"다음 문서를 한국어로 요약:\n\n{truncated}")
    return {"projectCode": code, "filePath": fp, "summary": summary}
