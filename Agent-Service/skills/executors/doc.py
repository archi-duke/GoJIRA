"""문서 스킬 실행기 — Go executor.go doc 관련 함수 대응."""

import logging

from clients.platform_api import PlatformClient
from core.llm import call_llm

log = logging.getLogger("agent.skills.doc")


async def exec_doc_list(params: dict, user: str) -> dict:
    code = params.get("projectCode", "")
    if not code:
        raise ValueError("projectCode 필수")

    client = PlatformClient()
    tree = await client.get_file_tree(code, ref="main")
    items = tree.get("tree", [])
    files = [item["path"] for item in items if item.get("type") != "tree" and item.get("path")]
    return {"projectCode": code, "files": files, "count": len(files)}


async def exec_doc_summarize(params: dict, user: str) -> dict:
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


async def exec_doc_diff(params: dict, user: str) -> dict:
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
