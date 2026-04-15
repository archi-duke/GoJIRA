"""Feature 이슈 생성 스킬."""

from skills.helpers import create_issue


_USECASE_FIELDS = (
    "actor", "precondition", "basicflow",
    "alternativeflow", "exceptionflow", "postcondition",
)


async def execute(params: dict, user: str) -> dict:
    extras = {f: params[f] for f in _USECASE_FIELDS if params.get(f)}
    return await create_issue("Feature", params, user, extras=extras or None)
