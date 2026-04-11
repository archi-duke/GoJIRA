"""UI 네비게이션 스킬."""


def execute(params: dict, user: str) -> dict:
    return {
        "_action": "navigate",
        "projectCode": params.get("projectCode", ""),
        "projectName": params.get("projectName", params.get("projectCode", "")),
        "page": params.get("page", ""),
        "message": f"{params.get('projectCode', '')} {params.get('page', '')} 화면을 열었습니다.",
    }
