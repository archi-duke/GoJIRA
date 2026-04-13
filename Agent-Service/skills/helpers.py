"""스킬 공용 헬퍼 함수."""

import re
from datetime import datetime
from typing import Optional

from core.database import issues, projects


def load_prompt(skill_dir: str, name: str) -> str:
    """스킬 디렉토리 내 `prompts/{name}.txt` 파일을 읽어 반환.

    사용 예:
        _SKILL_DIR = os.path.dirname(__file__)
        prompt = load_prompt(_SKILL_DIR, "generate").format(code=code, ...)

    텍스트 내 `{variable}`은 .format()으로 치환. JSON 리터럴 중괄호는 `{{`/`}}`로 이스케이프.
    """
    import os
    path = os.path.join(skill_dir, "prompts", f"{name}.txt")
    with open(path, "r", encoding="utf-8") as f:
        return f.read()


_TEMPLATE_PHRASES = (
    '기록하는 문서입니다', '기술하는 문서입니다',
    '반드시 작성해', '참조합니다', '작성 필요',
)


def is_template_only(content: str) -> bool:
    """문서 내용이 템플릿 수준(실질 내용 없음)인지 판정한다.

    스트립 대상: # 제목 라인, > 인용 라인, (작성 필요) 마커, 템플릿 안내 문구.
    스트립 후 30자 미만이면 템플릿으로 간주.
    """
    stripped = re.sub(r'#[^\n]*', '', content or '')
    stripped = re.sub(r'>.*', '', stripped)
    stripped = re.sub(r'\(작성 필요\)', '', stripped)
    for phrase in _TEMPLATE_PHRASES:
        stripped = stripped.replace(phrase, '')
    return len(stripped.strip()) < 30


def get_next_issue_num(code: str) -> int:
    """프로젝트 이슈 키에서 최대 번호 + 1 반환."""
    cursor = issues().find(
        {"key": {"$regex": f"^{re.escape(code)}-"}},
        {"key": 1},
    )
    prefix = f"{code}-"
    mx = 0
    for doc in cursor:
        k = doc.get("key", "")
        if len(k) > len(prefix):
            try:
                n = int(k[len(prefix):])
                if n > mx:
                    mx = n
            except ValueError:
                pass
    return mx + 1


def add_issue_to_project(code: str, issue_key: str):
    """프로젝트의 issues 배열에 이슈 키 추가."""
    projects().update_one(
        {"jiraCodes.main.jiraCode": code},
        {"$addToSet": {"issues": issue_key}},
    )


def create_issue(issue_type: str, params: dict, user: str, extras: Optional[dict] = None) -> dict:
    """공용 이슈 생성 — 타입을 고정해서 받고 공통 필드를 채운다.

    Args:
        issue_type: 이슈 타입 (VOS, Feature, Requirement, Architectural Design, Detailed Design)
        params: 사용자 입력 파라미터 (projectCode, summary, description, priority, assignee 등)
        user: 작성자
        extras: 타입별 추가 필드 (예: reqtype). None이면 무시.
    """
    code = params.get("projectCode", "")
    summary = params.get("summary", "")
    if not code or not summary:
        raise ValueError("projectCode, summary 필수")

    key = f"{code}-{get_next_issue_num(code)}"
    now = datetime.now().strftime("%Y-%m-%dT%H:%M:%S.000+0900")

    doc = {
        "key": key,
        "issueType": issue_type,
        "summary": summary,
        "description": params.get("description", ""),
        "status": "To Do",
        "priority": params.get("priority", "Medium"),
        "assignee": params.get("assignee", ""),
        "reporter": user,
        "created": now,
        "updated": now,
        "labels": [],
        "issueLinks": [],
        "comments": [],
        "_source": "gojira-agent",
    }
    if extras:
        doc.update(extras)

    issues().update_one({"key": key}, {"$set": doc}, upsert=True)
    add_issue_to_project(code, key)
    return {"key": key, "issueType": issue_type, "summary": summary, "status": "To Do"}


DELETION_PREFIX = "[Deletion Required] "
DELETION_LABEL = "del_req"


def _now() -> str:
    return datetime.now().strftime("%Y-%m-%dT%H:%M:%S.000+0900")


def add_comment(key: str, body: str, user: str) -> dict:
    """이슈에 코멘트를 추가한다.

    comments 필드가 없으면 update 시 자동 생성된다.
    """
    if not key or not body:
        raise ValueError("key, body 필수")
    comment = {"author": user, "body": body, "created": _now()}
    issues().update_one(
        {"key": key},
        {"$push": {"comments": comment}, "$set": {"updated": comment["created"]}},
    )
    return {"key": key, "comment": comment}


def update_issue_with_history(key: str, updates: dict, reason: str, user: str) -> dict:
    """이슈를 수정하고 변경 사유를 자동 코멘트로 남긴다.

    Args:
        key: 이슈 키
        updates: $set 대상 필드 dict (summary/description/필드 등)
        reason: 수정 사유 (코멘트로 기록됨)
        user: 수정자
    """
    if not key or not updates:
        raise ValueError("key, updates 필수")
    if "key" in updates:
        del updates["key"]  # key 변경 금지
    updates["updated"] = _now()
    issues().update_one({"key": key}, {"$set": updates})

    fields = ", ".join(k for k in updates.keys() if k != "updated")
    body = f"[자동수정] {reason}\n변경 필드: {fields}"
    add_comment(key, body, user)
    return {"key": key, "updated": list(updates.keys()), "reason": reason}


def mark_for_deletion(key: str, reason: str, user: str) -> dict:
    """이슈를 삭제 마킹한다 (실제 삭제는 하지 않음).

    - summary 앞에 "[Deletion Required] " 접두 추가 (이미 있으면 유지)
    - labels에 "del_req" 추가
    - 사유를 코멘트로 기록
    """
    if not key or not reason:
        raise ValueError("key, reason 필수")
    issue = issues().find_one({"key": key})
    if not issue:
        raise ValueError(f"이슈 없음: {key}")

    summary = issue.get("summary", "")
    new_summary = summary if summary.startswith(DELETION_PREFIX) else DELETION_PREFIX + summary

    issues().update_one(
        {"key": key},
        {
            "$set": {"summary": new_summary, "updated": _now()},
            "$addToSet": {"labels": DELETION_LABEL},
        },
    )
    add_comment(key, f"[삭제마킹] {reason}", user)
    return {"key": key, "marked": True, "reason": reason}


def dispatch_by_mode(
    items: list,
    existing_keys: set,
    create_fn,
    update_fields_fn,
    user: str,
) -> dict:
    """LLM이 도출한 항목들을 mode에 따라 분기 처리한다.

    각 item은 다음 키를 갖는다:
        - mode: "create" | "reuse" | "modify" | "delete_mark"
        - existingKey: reuse/modify/delete_mark일 때 대상 이슈 키
        - modifyReason: modify일 때 사유
        - deleteReason: delete_mark일 때 사유
        - 기타 도메인 필드 (summary, description, ...)

    Args:
        create_fn(item) -> dict: 신규 이슈 생성. result["key"] 반환 필수.
        update_fields_fn(item) -> dict: modify 시 $set할 필드 dict.

    Returns:
        {"created": [...], "reused": [...], "modified": [...], "markedForDeletion": [...]}
        각 entry는 {"key", "item", ...} 형태.
    """
    buckets = {"created": [], "reused": [], "modified": [], "markedForDeletion": []}

    for item in items:
        mode = (item.get("mode") or "create").lower()
        existing_key = (item.get("existingKey") or "").strip()
        try:
            if mode == "reuse" and existing_key in existing_keys:
                buckets["reused"].append({
                    "key": existing_key,
                    "item": item,
                    "summary": item.get("summary", ""),
                })

            elif mode == "modify" and existing_key in existing_keys:
                updates = update_fields_fn(item) or {}
                reason = item.get("modifyReason") or "내용 보강"
                if updates:
                    update_issue_with_history(existing_key, updates, reason, user)
                buckets["modified"].append({
                    "key": existing_key,
                    "item": item,
                    "reason": reason,
                    "updatedFields": list(updates.keys()),
                })

            elif mode == "delete_mark" and existing_key in existing_keys:
                reason = item.get("deleteReason") or "분석 결과 폐기 대상"
                mark_for_deletion(existing_key, reason, user)
                buckets["markedForDeletion"].append({
                    "key": existing_key,
                    "item": item,
                    "reason": reason,
                })

            else:
                # 기본: create (mode 미지정 또는 잘못된 existingKey 포함)
                r = create_fn(item)
                buckets["created"].append({
                    "key": r["key"],
                    "item": item,
                    "summary": item.get("summary", ""),
                })
        except Exception:
            # 단일 항목 실패가 전체를 막지 않도록 무시 (요약 메시지에서 건수 차이로 드러남)
            continue

    return buckets


def link_issues(from_key: str, to_key: str, link_type: str = "contains") -> dict:
    """두 이슈를 양방향 link로 연결한다 (contains / relates / blocks 등).

    `from_key`가 outward(부모), `to_key`가 inward(자식) 관점.
    """
    if not from_key or not to_key or not link_type:
        raise ValueError("from_key, to_key, link_type 필수")
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
