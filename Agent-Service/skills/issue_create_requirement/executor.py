"""Requirement 이슈 생성 스킬."""

from skills.helpers import create_issue


_NFR_FIELDS = (
    "qa", "source", "stimulus", "environment", "artifact",
    "response", "measure", "responseScenario", "qai", "qad", "qas",
)


async def execute(params: dict, user: str) -> dict:
    req_type = params.get("reqType", "")
    if req_type not in ("FR", "NFR", "CNST"):
        raise ValueError("reqType은 FR, NFR, CNST 중 하나여야 합니다")

    extras = {"reqtype": req_type}

    # NFR일 경우 품질 속성 시나리오 필드 수집
    if req_type == "NFR":
        for f in _NFR_FIELDS:
            if params.get(f):
                extras[f] = params[f]

    return await create_issue("Requirement", params, user, extras=extras)
