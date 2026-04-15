"""Issue Store — Git(YAML) 원천 + MongoDB 캐시 듀얼 경로 어댑터.

진실 원천: 각 과제의 docs repo `data/issues/<subkey>.yaml`
캐시:      MongoDB `issues` 컬렉션 (쿼리/카운트 성능 보존)

## API 원칙

- **쓰기 경로 단일화** — 모든 이슈 생성/수정/삭제는 반드시 이 모듈을 경유. 스킬/라우터가
  `issues().update_one(...)` 을 직접 호출하지 않는다.
- **읽기는 캐시** — `get_issue`, `list_issues`, `count_issues` 는 Mongo 를 읽는다.
  캐시 손상/누락 시 `rebuild_cache` 로 git 원천에서 재구성.
- **쓰기 순서** — git YAML 저장 → 성공 시 Mongo 캐시 upsert. 중간 실패 시 Mongo 를
  건드리지 않아 다음 rebuild 에서 복구 가능.

docs/issue-yaml-schema.md, docs/design-platform.md §6 참조.
"""

from __future__ import annotations

import logging
import re
from typing import Any, Dict, Iterable, List, Optional

from core.database import issues as _issues
from core.issue_schema import (
    IssueDoc,
    dump_issue_yaml,
    load_issue_yaml,
    from_mongo,
    to_mongo,
)

log = logging.getLogger("agent.issuestore")


# ──────────────────────────────────────────────────────────────────────────
# 경로/프리픽스 유틸
# ──────────────────────────────────────────────────────────────────────────


def _yaml_path(subkey: str) -> str:
    safe = re.sub(r"[^A-Za-z0-9_\-]+", "_", subkey or "")
    if not safe:
        raise ValueError("subkey 비어있음")
    return f"data/issues/{safe}.yaml"


def _key_to_project_code(key: str) -> str:
    """이슈 key (GOJIRA3-23) → 프로젝트 코드 (GOJIRA3)."""
    if not key or "-" not in key:
        raise ValueError(f"잘못된 이슈 key: {key!r}")
    return key.rsplit("-", 1)[0]


# ──────────────────────────────────────────────────────────────────────────
# Read — Mongo 캐시 경유
# ──────────────────────────────────────────────────────────────────────────


def get_issue(key: str) -> Optional[IssueDoc]:
    raw = _issues().find_one({"key": key})
    if not raw:
        return None
    try:
        return from_mongo(raw)
    except Exception as e:
        log.warning("get_issue 파싱 실패 key=%s: %s", key, e)
        return None


def list_issues(project_code: str, **filters: Any) -> List[IssueDoc]:
    """과제별 이슈 목록. 추가 필터(issueType, reqtype 등)는 kwargs 로 전달."""
    q: Dict[str, Any] = {"key": {"$regex": f"^{re.escape(project_code)}-"}}
    q.update(filters)
    out: List[IssueDoc] = []
    for raw in _issues().find(q):
        try:
            out.append(from_mongo(raw))
        except Exception as e:
            log.warning("list_issues 파싱 실패 key=%s: %s", raw.get("key"), e)
    return out


def count_issues(project_code: str, **filters: Any) -> int:
    q: Dict[str, Any] = {"key": {"$regex": f"^{re.escape(project_code)}-"}}
    q.update(filters)
    return _issues().count_documents(q)


# ──────────────────────────────────────────────────────────────────────────
# Write — git YAML 저장 → Mongo 캐시 upsert
# ──────────────────────────────────────────────────────────────────────────


async def save_issue(doc: IssueDoc, user: str, *, commit_message: str | None = None) -> Dict[str, Any]:
    """이슈 1건 저장. YAML 커밋 후 Mongo 캐시 upsert.

    Args:
        doc: 저장할 IssueDoc (key, subkey 필수)
        user: Gitea user 브랜치 대상
        commit_message: git commit 메시지. None 이면 자동 생성.

    Returns:
        {"key", "subkey", "yamlPath", "cached": bool}
    """
    if not doc.key or not doc.subkey:
        raise ValueError("IssueDoc.key/subkey 필수")
    code = _key_to_project_code(doc.key)
    path = _yaml_path(doc.subkey)
    text = dump_issue_yaml(doc)
    msg = commit_message or f"[issue] {doc.key} {doc.summary[:40]}"

    # 1) git YAML 저장 (user 브랜치 커밋)
    from clients.platform_api import PlatformClient
    client = PlatformClient()
    await client.save_file(code, path, text, msg, user)

    # 2) Mongo 캐시 upsert — 실패해도 git 이 진실이므로 경고만
    cached = False
    try:
        mongo_doc = to_mongo(doc)
        _issues().update_one(
            {"key": doc.key},
            {"$set": mongo_doc},
            upsert=True,
        )
        cached = True
    except Exception as e:
        log.warning("캐시 upsert 실패 key=%s: %s", doc.key, e)

    return {"key": doc.key, "subkey": doc.subkey, "yamlPath": path, "cached": cached}


async def delete_issue(key: str, user: str) -> Dict[str, Any]:
    """이슈 삭제. YAML 파일 제거 + Mongo 캐시 제거.

    캐시에서 subkey 를 조회해 파일 경로 결정. 캐시 miss 시에도 Mongo 는 정리.
    """
    cached = _issues().find_one({"key": key}, {"subkey": 1, "_id": 0})
    subkey = (cached or {}).get("subkey")
    code = _key_to_project_code(key)

    yaml_removed = False
    if subkey:
        try:
            from clients.platform_api import PlatformClient
            client = PlatformClient()
            await client.delete_file(code, _yaml_path(subkey), user)
            yaml_removed = True
        except Exception as e:
            log.warning("YAML 삭제 실패 key=%s: %s", key, e)

    del_res = _issues().delete_one({"key": key})
    return {"key": key, "yamlRemoved": yaml_removed, "cacheDeleted": del_res.deleted_count}


async def save_many(docs: Iterable[IssueDoc], user: str) -> Dict[str, Any]:
    """여러 이슈를 순차 저장. 각 건 개별 커밋 (git 이력 해상도 유지).

    대량 마이그레이션/동기화 용도. 실패 건은 failed 에 누적.
    """
    ok: list = []
    failed: list = []
    for d in docs:
        try:
            r = await save_issue(d, user)
            ok.append(r["key"])
        except Exception as e:
            failed.append({"key": d.key, "error": str(e)})
    return {"saved": ok, "failed": failed, "total": len(ok) + len(failed)}


# ──────────────────────────────────────────────────────────────────────────
# Rebuild — git → Mongo 캐시 전체 재구성
# ──────────────────────────────────────────────────────────────────────────


async def rebuild_cache(project_code: str, *, ref: str = "main") -> Dict[str, Any]:
    """git 의 data/issues/*.yaml 을 읽어 Mongo 캐시를 완전 재구성.

    절차:
    1. 기존 Mongo 에서 해당 과제 이슈 전부 삭제
    2. DocRepo 트리에서 `data/issues/*.yaml` 목록 조회
    3. 각 파일 content 가져와 파싱 → Mongo upsert

    사용처: 초기 마이그레이션 직후, Mongo 손상/소실 복구, 브랜치 전환 후 동기화.
    """
    from clients.platform_api import PlatformClient
    client = PlatformClient()
    tree = await client.get_file_tree(project_code, ref=ref)
    entries = tree.get("tree") if isinstance(tree, dict) else tree
    paths = [
        e.get("path") for e in (entries or [])
        if isinstance(e, dict) and e.get("type") == "blob"
        and (e.get("path") or "").startswith("data/issues/")
        and (e.get("path") or "").endswith(".yaml")
    ]

    prefix = {"key": {"$regex": f"^{re.escape(project_code)}-"}}
    removed = _issues().delete_many(prefix).deleted_count

    loaded = 0
    failed: list = []
    for p in paths:
        try:
            text = await client.get_file_content(project_code, p, ref=ref) or ""
            doc = load_issue_yaml(text)
            _issues().update_one({"key": doc.key}, {"$set": to_mongo(doc)}, upsert=True)
            loaded += 1
        except Exception as e:
            failed.append({"path": p, "error": str(e)})
            log.warning("rebuild 파일 로드 실패 path=%s: %s", p, e)

    return {
        "projectCode": project_code,
        "ref": ref,
        "removedFromCache": removed,
        "loaded": loaded,
        "failed": failed,
        "totalYamlFiles": len(paths),
    }


__all__ = [
    "get_issue",
    "list_issues",
    "count_issues",
    "save_issue",
    "save_many",
    "delete_issue",
    "rebuild_cache",
]
