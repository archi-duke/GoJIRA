"""설계 이슈 생성 스킬 (Architectural / Detailed Design)."""

from skills.helpers import create_issue


_LEVEL_MAP = {
    "Architectural": "Architectural Design",
    "Detailed": "Detailed Design",
}

# ADSN+DDSN 공통
_COMMON_FIELDS = ("adiagram", "bdiagram")
# ADSN 전용
_ADSN_ONLY_FIELDS = (
    "dmodel", "archdiagram", "objectives", "pros", "cons",
    "score", "eval", "sensitivity", "mitigation", "risk", "decision",
)


def execute(params: dict, user: str) -> dict:
    level = params.get("designLevel", "")
    if level not in _LEVEL_MAP:
        raise ValueError("designLevel은 Architectural 또는 Detailed여야 합니다")

    extras = {f: params[f] for f in _COMMON_FIELDS if params.get(f)}
    if level == "Architectural":
        for f in _ADSN_ONLY_FIELDS:
            if params.get(f):
                extras[f] = params[f]

    return create_issue(_LEVEL_MAP[level], params, user, extras=extras or None)
