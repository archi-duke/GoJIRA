"""MongoDB 연결 — Go db/mongo.go와 동일한 컬렉션명 사용."""

from typing import Optional

from pymongo import MongoClient
from pymongo.database import Database
from pymongo.collection import Collection

from config import settings

_client: Optional[MongoClient] = None
_db: Optional[Database] = None


def get_client() -> MongoClient:
    global _client
    if _client is None:
        _client = MongoClient(settings.mongo_uri)
        print(f"[db] MongoDB 연결: {settings.mongo_uri}/{settings.mongo_db}")
    return _client


def get_db() -> Database:
    global _db
    if _db is None:
        _db = get_client()[settings.mongo_db]
    return _db


# --- 컬렉션 접근 ---

def issues() -> Collection:
    return get_db()["issues"]


def projects() -> Collection:
    return get_db()["projects"]


def agent_sessions() -> Collection:
    return get_db()["agent_sessions"]


def agent_jobs() -> Collection:
    return get_db()["agent_jobs"]


def agent_logs() -> Collection:
    return get_db()["agent_logs"]


def ensure_indexes():
    """TTL 인덱스 등 필수 인덱스 생성."""
    from pymongo import ASCENDING
    from datetime import timedelta

    # agent_sessions: 7일 TTL (Go와 동일)
    agent_sessions().create_index(
        [("updatedAt", ASCENDING)],
        expireAfterSeconds=int(timedelta(days=7).total_seconds()),
        background=True,
    )

    # agent_jobs: 30일 TTL
    agent_jobs().create_index(
        [("updatedAt", ASCENDING)],
        expireAfterSeconds=int(timedelta(days=30).total_seconds()),
        background=True,
    )

    # agent_logs: 30일 TTL
    agent_logs().create_index(
        [("timestamp", ASCENDING)],
        expireAfterSeconds=int(timedelta(days=30).total_seconds()),
        background=True,
    )

    print("[db] 인덱스 생성 완료")
