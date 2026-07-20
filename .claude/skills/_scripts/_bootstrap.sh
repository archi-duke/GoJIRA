# _bootstrap.sh — 모든 에이전트 bash 호출 시작 시 자동 실행(launcher 가 BASH_ENV 로 지정).
#
# 역할: launcher 가 GOJIRA_* 접두사로 주입한 값을, 스킬/스크립트가 참조하는 bare 이름
#       (CODE / USER / PROJECT_ROOT)으로 결정적으로 매핑한다. 에이전트(LLM)가 export 를
#       빠뜨리거나 CODE 를 오판해도 여기서 올바른 값이 채워진다.
#
# ⚠️ 이 파일은 매 bash -c 호출 전에 source 되므로 **stdout 출력 절대 금지**(명령 출력 오염).
#    export 만 수행. 실패해도 조용히 통과(가드).

# clone 루트 — launcher 가 GOJIRA_REPO_ROOT 주입. 없으면 현재 git toplevel.
export PROJECT_ROOT="${PROJECT_ROOT:-${GOJIRA_REPO_ROOT:-$(git rev-parse --show-toplevel 2>/dev/null || true)}}"

# 사용자(knoxId) — launcher 가 GOJIRA_USER 주입. 이미 셸에 있으면 GOJIRA_USER 우선(정확).
export USER="${GOJIRA_USER:-$USER}"

# 프로젝트 코드 — launcher 가 GOJIRA_CODE 주입. 없으면 clone 디렉토리명 = 코드.
if [ -z "${CODE:-}" ]; then
  export CODE="${GOJIRA_CODE:-$(basename "${GOJIRA_REPO_ROOT:-}" 2>/dev/null || true)}"
fi
