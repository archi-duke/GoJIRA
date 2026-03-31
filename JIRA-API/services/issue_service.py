import json
from pathlib import Path
from datetime import datetime, timezone, timedelta
from typing import Optional

from config import ISSUES_DIR, BASE_URL, PROJECT_KEY

KST = timezone(timedelta(hours=9))

# Internal key to track source file per issue
_SOURCE_FILE_KEY = "__source_file__"


def _load_all_issues() -> list[dict]:
    issues = []
    for json_file in sorted(ISSUES_DIR.glob("*.json")):
        with open(json_file, "r", encoding="utf-8") as f:
            data = json.load(f)
        if isinstance(data, list):
            for item in data:
                item[_SOURCE_FILE_KEY] = json_file.name
                issues.append(item)
        elif isinstance(data, dict) and "fields" in data:
            data[_SOURCE_FILE_KEY] = json_file.name
            issues.append(data)
    return issues


def _save_issues(issues: list[dict]) -> None:
    grouped: dict[str, list[dict]] = {}
    for issue in issues:
        filename = issue.pop(_SOURCE_FILE_KEY, "issues.json")
        grouped.setdefault(filename, []).append(issue)

    # Write back each file
    for filename, issue_list in grouped.items():
        filepath = ISSUES_DIR / filename
        with open(filepath, "w", encoding="utf-8") as f:
            json.dump(issue_list, f, ensure_ascii=False, indent=4)

    # Remove files that no longer have issues
    for json_file in ISSUES_DIR.glob("*.json"):
        if json_file.name not in grouped:
            # Check if file was previously tracked - if so, write empty array
            # Only delete auto-created files, keep originals with empty array
            with open(json_file, "w", encoding="utf-8") as f:
                json.dump([], f, ensure_ascii=False, indent=4)


def _next_issue_key(issues: list[dict], project_key: str = PROJECT_KEY) -> str:
    max_num = 0
    for issue in issues:
        key = issue.get("key", "")
        if key.startswith(f"{project_key}-"):
            try:
                num = int(key.split("-")[1])
                max_num = max(max_num, num)
            except ValueError:
                pass
    return f"{project_key}-{max_num + 1}"


def _next_issue_id(issues: list[dict]) -> str:
    max_id = 10000
    for issue in issues:
        try:
            max_id = max(max_id, int(issue.get("id", 0)))
        except ValueError:
            pass
    return str(max_id + 1)


def _now_str() -> str:
    return datetime.now(KST).strftime("%Y-%m-%dT%H:%M:%S.%f")[:-3] + "+0900"


def get_issue(issue_id_or_key: str) -> Optional[dict]:
    issues = _load_all_issues()
    for issue in issues:
        if issue.get("key") == issue_id_or_key or issue.get("id") == issue_id_or_key:
            result = {k: v for k, v in issue.items() if k != _SOURCE_FILE_KEY}
            return result
    return None


def list_issues(start_at: int = 0, max_results: int = 50) -> dict:
    issues = _load_all_issues()
    total = len(issues)
    page = issues[start_at:start_at + max_results]
    clean_page = [{k: v for k, v in i.items() if k != _SOURCE_FILE_KEY} for i in page]
    return {
        "expand": "names,schema",
        "startAt": start_at,
        "maxResults": max_results,
        "total": total,
        "issues": clean_page,
    }


def search_issues(jql: str = "", start_at: int = 0, max_results: int = 50) -> dict:
    issues = _load_all_issues()
    filtered = issues
    if jql:
        jql_lower = jql.lower()

        # Filter by project
        if "project" in jql_lower:
            import re
            pm = re.search(r'project\s*=\s*["\']?(\w+)["\']?', jql, re.IGNORECASE)
            if pm:
                project_key = pm.group(1).upper()
                filtered = [
                    i for i in filtered
                    if i.get("fields", {}).get("project", {}).get("key", "").upper() == project_key
                    or i.get("key", "").startswith(project_key + "-")
                ]

        # Filter by issuetype in (...)
        if "issuetype" in jql_lower:
            import re
            itm = re.search(r'issuetype\s+in\s*\(([^)]+)\)', jql, re.IGNORECASE)
            if itm:
                raw_types = [t.strip().strip('"').strip("'") for t in itm.group(1).split(",")]
                type_set = {t.lower() for t in raw_types}
                filtered = [
                    i for i in filtered
                    if i.get("fields", {}).get("issuetype", {}).get("name", "").lower() in type_set
                ]
            else:
                # issuetype = "xxx"
                itm2 = re.search(r'issuetype\s*=\s*["\']?([^"\'&]+)["\']?', jql, re.IGNORECASE)
                if itm2:
                    it_name = itm2.group(1).strip().lower()
                    filtered = [
                        i for i in filtered
                        if i.get("fields", {}).get("issuetype", {}).get("name", "").lower() == it_name
                    ]

    total = len(filtered)
    page = filtered[start_at:start_at + max_results]
    # Strip internal keys from response
    clean_page = [{k: v for k, v in i.items() if k != _SOURCE_FILE_KEY} for i in page]
    return {
        "expand": "names,schema",
        "startAt": start_at,
        "maxResults": max_results,
        "total": total,
        "issues": clean_page,
    }


def create_issue(payload: dict) -> dict:
    issues = _load_all_issues()
    fields = payload.get("fields", {})

    prj_key = fields.get("project", {}).get("key", PROJECT_KEY)
    new_key = _next_issue_key(issues, prj_key)
    new_id = _next_issue_id(issues)
    now = _now_str()

    # Determine which file to store in based on issuetype
    issue_type_name = fields.get("issuetype", {}).get("name", "Task")
    type_to_file = {
        "Architectueral Design": "designs.json",
        "Detaied Design": "designs.json",
        "Feature": "feature.json",
        "Requirement": "requirement.json",
        "VOS": "vos.json",
    }
    target_file = type_to_file.get(issue_type_name, "issues.json")

    new_issue = {
        _SOURCE_FILE_KEY: target_file,
        "expand": "renderedFields,names,schema,operations,editmeta,changelog,versionedRepresentations",
        "id": new_id,
        "self": f"{BASE_URL}/rest/api/2/issue/{new_id}",
        "key": new_key,
        "fields": {
            "issuetype": fields.get("issuetype", {
                "self": f"{BASE_URL}/rest/api/2/issuetype/10003",
                "id": "10003",
                "name": "Task",
                "subtask": False,
            }),
            "components": fields.get("components", []),
            "project": fields.get("project", {
                "self": f"{BASE_URL}/rest/api/2/project/10000",
                "id": "10000",
                "key": "KAN",
                "name": "Matrix",
                "projectTypeKey": "software",
            }),
            "description": fields.get("description"),
            "resolution": None,
            "summary": fields.get("summary", ""),
            "watches": {"self": f"{BASE_URL}/rest/api/2/issue/{new_key}/watchers", "watchCount": 0, "isWatching": False},
            "lastViewed": now,
            "creator": fields.get("reporter", {
                "self": f"{BASE_URL}/rest/api/2/user?accountId=anonymous",
                "accountId": "anonymous",
                "displayName": "Anonymous",
                "active": True,
            }),
            "subtasks": [],
            "created": now,
            "reporter": fields.get("reporter", {
                "self": f"{BASE_URL}/rest/api/2/user?accountId=anonymous",
                "accountId": "anonymous",
                "displayName": "Anonymous",
                "active": True,
            }),
            "priority": fields.get("priority", {
                "self": f"{BASE_URL}/rest/api/2/priority/3",
                "name": "Medium",
                "id": "3",
            }),
            "labels": fields.get("labels", []),
            "environment": fields.get("environment"),
            "duedate": fields.get("duedate"),
            "issuelinks": [],
            "assignee": fields.get("assignee"),
            "updated": now,
            "status": {
                "self": f"{BASE_URL}/rest/api/2/status/10000",
                "description": "",
                "name": "To Do",
                "id": "10000",
                "statusCategory": {
                    "self": f"{BASE_URL}/rest/api/2/statuscategory/2",
                    "id": 2,
                    "key": "new",
                    "colorName": "blue-gray",
                    "name": "To Do",
                },
            },
        },
    }

    # Copy custom fields
    for k, v in fields.items():
        if k.startswith("customfield_"):
            new_issue["fields"][k] = v

    issues.append(new_issue)
    _save_issues(issues)

    return {"id": new_id, "key": new_key, "self": f"{BASE_URL}/rest/api/2/issue/{new_id}"}


def delete_issue(issue_id_or_key: str) -> bool:
    issues = _load_all_issues()
    new_issues = [
        i for i in issues
        if i.get("key") != issue_id_or_key and i.get("id") != issue_id_or_key
    ]
    if len(new_issues) == len(issues):
        return False
    _save_issues(new_issues)
    return True


def update_issue(issue_id_or_key: str, payload: dict) -> Optional[dict]:
    issues = _load_all_issues()
    target = None
    for issue in issues:
        if issue.get("key") == issue_id_or_key or issue.get("id") == issue_id_or_key:
            target = issue
            break
    if target is None:
        return None

    fields_update = payload.get("fields", {})
    for k, v in fields_update.items():
        target["fields"][k] = v

    # Support "update" format (add/remove/set operations)
    update_ops = payload.get("update", {})
    for field_name, operations in update_ops.items():
        for op in operations:
            if "add" in op:
                current = target["fields"].get(field_name, [])
                if not isinstance(current, list):
                    current = []
                current.append(op["add"])
                target["fields"][field_name] = current
            elif "remove" in op:
                current = target["fields"].get(field_name, [])
                if isinstance(current, list):
                    target["fields"][field_name] = [
                        item for item in current if item != op["remove"]
                    ]
            elif "set" in op:
                target["fields"][field_name] = op["set"]

    target["fields"]["updated"] = _now_str()
    _save_issues(issues)
    result = {k: v for k, v in target.items() if k != _SOURCE_FILE_KEY}
    return result


# --- Issue Links ---

def _next_link_id() -> str:
    issues = _load_all_issues()
    max_id = 10000
    for issue in issues:
        for link in issue.get("fields", {}).get("issuelinks", []):
            try:
                max_id = max(max_id, int(link.get("id", 0)))
            except (ValueError, TypeError):
                pass
    return str(max_id + 1)


def create_issue_link(payload: dict) -> bool:
    link_type = payload.get("type", {})
    inward_key = payload.get("inwardIssue", {}).get("key", "")
    outward_key = payload.get("outwardIssue", {}).get("key", "")

    issues = _load_all_issues()
    inward_issue = None
    outward_issue = None
    for issue in issues:
        if issue.get("key") == inward_key:
            inward_issue = issue
        if issue.get("key") == outward_key:
            outward_issue = issue

    if inward_issue is None or outward_issue is None:
        return False

    link_id = _next_link_id()

    inward_issue.setdefault("fields", {}).setdefault("issuelinks", []).append({
        "id": link_id,
        "type": link_type,
        "outwardIssue": {
            "id": outward_issue["id"],
            "key": outward_issue["key"],
            "self": outward_issue.get("self", ""),
            "fields": {
                "summary": outward_issue.get("fields", {}).get("summary", ""),
                "status": outward_issue.get("fields", {}).get("status", {}),
                "priority": outward_issue.get("fields", {}).get("priority", {}),
                "issuetype": outward_issue.get("fields", {}).get("issuetype", {}),
            },
        },
    })

    outward_issue.setdefault("fields", {}).setdefault("issuelinks", []).append({
        "id": link_id,
        "type": link_type,
        "inwardIssue": {
            "id": inward_issue["id"],
            "key": inward_issue["key"],
            "self": inward_issue.get("self", ""),
            "fields": {
                "summary": inward_issue.get("fields", {}).get("summary", ""),
                "status": inward_issue.get("fields", {}).get("status", {}),
                "priority": inward_issue.get("fields", {}).get("priority", {}),
                "issuetype": inward_issue.get("fields", {}).get("issuetype", {}),
            },
        },
    })

    _save_issues(issues)
    return True


def delete_issue_link(link_id: str) -> bool:
    issues = _load_all_issues()
    found = False
    for issue in issues:
        links = issue.get("fields", {}).get("issuelinks", [])
        new_links = [l for l in links if str(l.get("id")) != str(link_id)]
        if len(new_links) != len(links):
            issue["fields"]["issuelinks"] = new_links
            found = True
    if found:
        _save_issues(issues)
    return found
