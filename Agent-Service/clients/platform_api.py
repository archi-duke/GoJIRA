"""Platform-API REST 클라이언트 — Go skills/helpers.go fetchFileContent 등 대응."""

import base64
from typing import Optional
from urllib.parse import quote

import httpx

from config import settings


class PlatformClient:
    def __init__(self):
        self.base_url = settings.platform_api_url

    async def get_file_tree(self, code: str, ref: str = "main") -> dict:
        url = f"{self.base_url}/DocRepo/{code}/tree?ref={quote(ref)}"
        async with httpx.AsyncClient() as client:
            resp = await client.get(url, timeout=10)
            if resp.status_code >= 400:
                return {"tree": []}
            return resp.json()

    async def get_file_content(self, code: str, path: str, ref: str = "main") -> Optional[str]:
        """Go fetchFileContent 대응 — base64 디코딩 포함."""
        url = f"{self.base_url}/DocRepo/{code}/file?path={quote(path)}&ref={quote(ref)}"
        async with httpx.AsyncClient() as client:
            resp = await client.get(url, timeout=10)
            if resp.status_code >= 400:
                return None
            data = resp.json()
            content = data.get("content", "")
            encoding = data.get("encoding", "")
            if encoding == "base64" or content:
                try:
                    return base64.b64decode(content).decode("utf-8")
                except Exception:
                    return content
            return None

    async def list_prs(self, code: str, state: str = "open") -> list:
        url = f"{self.base_url}/DocRepo/{code}/prs?state={state}"
        async with httpx.AsyncClient() as client:
            resp = await client.get(url, timeout=10)
            if resp.status_code >= 400:
                return []
            return resp.json()

    async def list_commits(self, code: str, branch: str = "main", per_page: int = 20) -> list:
        """커밋 이력 조회 — GET /DocRepo/{code}/commits."""
        url = f"{self.base_url}/DocRepo/{code}/commits?branch={quote(branch)}&perPage={per_page}"
        async with httpx.AsyncClient() as client:
            resp = await client.get(url, timeout=10)
            if resp.status_code >= 400:
                return []
            return resp.json()

    async def save_file(self, code: str, path: str, content: str, message: str, user: str) -> dict:
        """사용자 브랜치에 파일 저장 (PUT /DocRepo/{code}/file)."""
        url = f"{self.base_url}/DocRepo/{code}/file"
        payload = {"path": path, "content": content, "message": message}
        async with httpx.AsyncClient() as client:
            resp = await client.put(
                url, json=payload,
                headers={"Content-Type": "application/json", "X-User-Id": user},
                timeout=30,
            )
            if resp.status_code >= 400:
                raise ValueError(f"파일 저장 실패: {resp.text}")
            return resp.json()

    async def init_files(self, code: str) -> dict:
        """초기 폴더/파일 생성 (main 브랜치, 없는 것만) — POST /DocRepo/{code}/init-files."""
        url = f"{self.base_url}/DocRepo/{code}/init-files"
        async with httpx.AsyncClient() as client:
            resp = await client.post(url, timeout=30)
            if resp.status_code >= 400:
                raise ValueError(f"init-files 실패: {resp.text}")
            return resp.json() if resp.text else {}

    async def delete_file(self, code: str, path: str, user: str) -> dict:
        """파일 삭제 (사용자 브랜치) — DELETE /DocRepo/{code}/file?path=..."""
        url = f"{self.base_url}/DocRepo/{code}/file"
        async with httpx.AsyncClient() as client:
            resp = await client.delete(
                url,
                params={"path": path},
                headers={"X-User-Id": user},
                timeout=10,
            )
            if resp.status_code >= 400 and resp.status_code != 404:
                raise ValueError(f"파일 삭제 실패: {resp.text}")
            return {"deleted": resp.status_code < 400, "path": path}

    async def create_pr(self, code: str, title: str, body: str, head: str, base: str, user: str) -> dict:
        url = f"{self.base_url}/DocRepo/{code}/pr"
        payload = {"title": title, "body": body, "head": head, "base": base}
        async with httpx.AsyncClient() as client:
            resp = await client.post(
                url, json=payload,
                headers={"X-User-Id": user},
                timeout=30,
            )
            if resp.status_code >= 400:
                raise ValueError(f"PR 생성 실패: {resp.text}")
            return resp.json()
