"""문서 목록 조회 스킬."""

from clients.platform_api import PlatformClient


async def execute(params: dict, user: str) -> dict:
    code = params.get("projectCode", "")
    if not code:
        raise ValueError("projectCode 필수")

    client = PlatformClient()
    tree = await client.get_file_tree(code, ref="main")
    items = tree.get("tree", [])
    files = [item["path"] for item in items if item.get("type") != "tree" and item.get("path")]
    return {"projectCode": code, "files": files, "count": len(files)}
