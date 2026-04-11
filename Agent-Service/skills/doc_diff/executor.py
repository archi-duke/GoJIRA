"""문서 비교 스킬."""

from clients.platform_api import PlatformClient
from core.llm import call_llm


async def execute(params: dict, user: str) -> dict:
    code = params.get("projectCode", "")
    fp = params.get("filePath", "")
    if not code or not fp:
        raise ValueError("projectCode, filePath 필수")

    client = PlatformClient()
    mc = await client.get_file_content(code, fp, ref="main")
    uc = await client.get_file_content(code, fp, ref=f"docs/{user}")

    if mc == uc:
        return {"filePath": fp, "hasDiff": False, "summary": "차이 없음"}

    mc_trunc = (mc or "")[:4000]
    uc_trunc = (uc or "")[:4000]
    explanation = await call_llm(
        f"두 버전 비교:\n=== main ===\n{mc_trunc}\n=== user ===\n{uc_trunc}"
    )
    return {"filePath": fp, "hasDiff": True, "explanation": explanation}
