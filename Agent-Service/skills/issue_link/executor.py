"""이슈 링크 스킬."""

from core.database import issues


def execute(params: dict, user: str) -> dict:
    from_key = params.get("fromKey", "")
    to_key = params.get("toKey", "")
    link_type = params.get("linkType", "")
    if not from_key or not to_key or not link_type:
        raise ValueError("fromKey, toKey, linkType 필수")

    link_id = f"{from_key}_{link_type}_{to_key}"
    issues().update_one(
        {"key": from_key},
        {"$addToSet": {"issueLinks": {"id": link_id, "type": link_type, "outward": to_key}}},
    )
    issues().update_one(
        {"key": to_key},
        {"$addToSet": {"issueLinks": {"id": link_id, "type": link_type, "inward": from_key}}},
    )
    return {"linkId": link_id, "fromKey": from_key, "toKey": to_key, "linkType": link_type}
