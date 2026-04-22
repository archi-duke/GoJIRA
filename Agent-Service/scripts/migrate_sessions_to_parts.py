"""기존 agent_sessions 를 OpenCode native parts 포맷으로 마이그레이션.

변환:
- 기존 `process.steps[]` + `assistant.content` 쌍 → OpenCode 의 `{info, parts}` 메시지 하나
- `user` 메시지는 content 기반 유지 (ocMessageId 는 OC 에서 복원 가능하면 채움)

실행:
  cd Agent-Service && python3 -m scripts.migrate_sessions_to_parts [--dry-run]
"""

from __future__ import annotations

import asyncio
import sys
from datetime import datetime, timezone

sys.path.insert(0, ".")

from core.database import agent_sessions  # noqa: E402
from opencode_bridge import client as oc  # noqa: E402


def _ts_from_info(info):
    t = (info or {}).get("time") or {}
    created = t.get("created")
    if created:
        try:
            return datetime.fromtimestamp(created / 1000, tz=timezone.utc)
        except Exception:
            pass
    return None


async def migrate_one(doc, dry_run: bool):
    sid = doc["_id"]
    oc_sid = doc.get("openCodeSessionId")
    messages = doc.get("messages") or []
    if not oc_sid:
        return {"sid": sid, "status": "skip_no_oc"}

    try:
        oc_messages = await oc.get_session_messages(oc_sid)
    except Exception as e:
        return {"sid": sid, "status": f"oc_err:{e}"}
    if not oc_messages:
        return {"sid": sid, "status": "oc_empty"}

    # OpenCode 메시지를 시간순 정렬 후 role 별로 처리
    def _sort_key(m):
        info = m.get("info", {}) if isinstance(m, dict) else {}
        return (info.get("time") or {}).get("created") or 0

    oc_messages.sort(key=_sort_key)

    # user 메시지는 기존 Mongo 것을 순서대로 소비 (OpenCode 가 user part 의 텍스트를 동일하게 갖고 있지만
    # 우리 쪽 content 를 우선 사용해서 리스트 UI 의 title 일관성 유지)
    existing_users = [m for m in messages if m.get("role") == "user"]
    user_cursor = 0

    new_messages = []
    for m in oc_messages:
        info = m.get("info") or {}
        role = info.get("role")
        mid = info.get("id")
        ts = _ts_from_info(info) or datetime.now(timezone.utc)
        if role == "user":
            text = ""
            for p in m.get("parts") or []:
                if p.get("type") == "text":
                    text = p.get("text") or ""
                    break
            entry = {"role": "user", "content": text, "timestamp": ts}
            if user_cursor < len(existing_users):
                prev = existing_users[user_cursor]
                # 기존 timestamp 유지 + content 일치 시 복사
                if prev.get("content"):
                    entry["content"] = prev["content"]
                if prev.get("timestamp"):
                    entry["timestamp"] = prev["timestamp"]
                user_cursor += 1
            if mid:
                entry["ocMessageId"] = mid
            new_messages.append(entry)
        elif role == "assistant":
            new_messages.append({
                "role": "assistant",
                "ocMessageId": mid,
                "info": info,
                "parts": m.get("parts") or [],
                "timestamp": ts,
            })

    if not new_messages:
        return {"sid": sid, "status": "nothing_to_migrate"}

    # title: 첫 user 메시지 content
    first_user = next((m["content"] for m in new_messages if m.get("role") == "user" and m.get("content")), "")

    if dry_run:
        return {
            "sid": sid,
            "status": "dry_run_ok",
            "before_count": len(messages),
            "after_count": len(new_messages),
            "title_preview": first_user[:40],
        }

    agent_sessions().update_one(
        {"_id": sid},
        {"$set": {"messages": new_messages, "updatedAt": datetime.now(timezone.utc)}},
    )
    return {
        "sid": sid,
        "status": "migrated",
        "before_count": len(messages),
        "after_count": len(new_messages),
    }


async def main():
    dry_run = "--dry-run" in sys.argv
    docs = list(agent_sessions().find({}))
    print(f"총 세션: {len(docs)} (dry_run={dry_run})")
    stats = {"migrated": 0, "dry_run_ok": 0, "skip_no_oc": 0, "oc_empty": 0, "nothing_to_migrate": 0, "err": 0}
    for d in docs:
        try:
            r = await migrate_one(d, dry_run)
        except Exception as e:
            r = {"sid": d["_id"], "status": f"exception:{e}"}
        status = r["status"]
        key = status if status in stats else ("err" if status.startswith(("oc_err", "exception")) else "err")
        stats[key] = stats.get(key, 0) + 1
        marker = "✓" if status in ("migrated", "dry_run_ok") else "·"
        extra = ""
        if "before_count" in r:
            extra = f"  msgs {r.get('before_count')} → {r.get('after_count')}"
            if r.get("title_preview"):
                extra += f"  '{r['title_preview']}'"
        print(f"  {marker} {r['sid'][:8]} {status}{extra}")
    print("")
    print("요약:", stats)


if __name__ == "__main__":
    asyncio.run(main())
