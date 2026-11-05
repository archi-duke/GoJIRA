#!/usr/bin/env bash
# build-catalog.sh — 서비스 카탈로그 검증·서명·게시 (설계 §10.6).
#
#   validate   스키마 / 경로 충돌 / prefix 규칙 / 패키지 sha256 검사 (키 불필요 — PR 에서 실행)
#   sign       issuedAt·expiresAt·commit·runId 기록 후 ed25519 서명 (CATALOG_SIGNING_KEY 필요)
#   publish    catalog.json + .sig 를 launcher-dist 에 배치
#
# 통제 구조: 사람은 PR 리뷰(CODEOWNERS)로 통제하고, 서명은 main 머지 후 자동으로 돈다(D11).
# 서명 대상은 **파일 바이트 그대로**이므로 sign 이후에는 재포맷하지 말 것.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/_lib.sh"

ROOT_DIR="$(cd "${DEPLOY_DIR}/.." && pwd)"
CATALOG="${DEPLOY_DIR}/launcher-dist/catalog.json"
EXPIRE_DAYS="${CATALOG_EXPIRE_DAYS:-30}"

cmd="${1:-}"
case "$cmd" in
  validate) ;;
  sign) ;;
  publish) ;;
  *) die "사용: build-catalog.sh validate|sign|publish" ;;
esac

[ -f "$CATALOG" ] || die "카탈로그 없음: $CATALOG"

# ── validate ────────────────────────────────────────────────────────────────
# 경로 충돌은 런타임이 아니라 여기서 막는다(§7.2) — 런처는 최장 prefix 일치로 중계하므로
# 중첩 prefix + 메서드 교집합이 있으면 어느 서비스가 받을지 등록 순서에 좌우된다.
validate() {
  python3 - "$CATALOG" "${DEPLOY_DIR}/launcher-dist" <<'PY'
import json, sys, os, re

path, dist = sys.argv[1], sys.argv[2]
try:
    cat = json.load(open(path, encoding="utf-8"))
except Exception as e:
    print(f"catalog.json 파싱 실패: {e}", file=sys.stderr); sys.exit(1)

errors, warns = [], []
services = cat.get("services")
if not isinstance(services, list):
    print("services 배열 없음", file=sys.stderr); sys.exit(1)

seen_names = set()
routes = []            # (prefix, methods, service)
VER = re.compile(r"^\d+\.\d+\.\d+$")

for i, s in enumerate(services):
    where = f"services[{i}]"
    name = s.get("name")
    if not name:
        errors.append(f"{where}: name 없음"); continue
    if name in seen_names:
        errors.append(f"{where}: name 중복 — {name}")
    seen_names.add(name)

    if not VER.match(str(s.get("version", ""))):
        errors.append(f"{name}: version 이 semver(x.y.z) 아님 — {s.get('version')!r}")
    if s.get("policy") not in ("required", "optional"):
        errors.append(f"{name}: policy 는 required|optional — {s.get('policy')!r}")

    pkgs = s.get("packages") or {}
    if not pkgs:
        errors.append(f"{name}: packages 없음")
    for plat, p in pkgs.items():
        f, sha = p.get("file"), p.get("sha256")
        if not f or not sha:
            errors.append(f"{name}/{plat}: file·sha256 필수"); continue
        if not re.fullmatch(r"[0-9a-f]{64}", sha):
            errors.append(f"{name}/{plat}: sha256 형식 오류")
        # 패키지가 이미 게시돼 있으면 실제 해시와 대조한다(게시 전이면 건너뜀).
        local = os.path.join(dist, "services", f)
        if os.path.exists(local):
            import hashlib
            h = hashlib.sha256(open(local, "rb").read()).hexdigest()
            if h != sha:
                errors.append(f"{name}/{plat}: sha256 불일치 — 카탈로그 {sha[:12]}… vs 실제 {h[:12]}…")
        else:
            warns.append(f"{name}/{plat}: 패키지 미게시 — sha256 대조 생략 ({f})")

    for r in s.get("routes") or []:
        pre, methods = r.get("prefix"), r.get("methods") or []
        if not pre:
            errors.append(f"{name}: route.prefix 없음"); continue
        # D8 — 서비스 경로는 /v{n}/ 접두사 필수. /launcher/* 는 런처 예약(§5).
        if not re.match(r"^/v\d+/", pre):
            errors.append(f"{name}: prefix 는 /v{{버전}}/ 로 시작해야 함 — {pre}")
        if pre.startswith("/launcher/"):
            errors.append(f"{name}: /launcher/* 는 런처 예약 이름공간 — {pre}")
        if pre.endswith("/"):
            errors.append(f"{name}: prefix 끝에 / 금지 — {pre}")
        routes.append((pre, set(m.upper() for m in methods) or {"*"}, name))

# 세그먼트 경계 기준 중첩 검사 — /v1/docs/pptx 와 /v1/docs/pptxx 는 무관하지만
# /v1/docs 와 /v1/docs/pptx 는 중첩이다.
def nested(a, b):
    return a == b or b.startswith(a + "/") or a.startswith(b + "/")

for i in range(len(routes)):
    for j in range(i + 1, len(routes)):
        p1, m1, s1 = routes[i]
        p2, m2, s2 = routes[j]
        if s1 == s2 or not nested(p1, p2):
            continue
        if "*" in m1 or "*" in m2 or (m1 & m2):
            errors.append(f"경로 충돌: {s1} {p1} {sorted(m1)} ↔ {s2} {p2} {sorted(m2)}")

for w in warns:
    print(f"  경고: {w}")
if errors:
    print("\n카탈로그 검증 실패:", file=sys.stderr)
    for e in errors:
        print(f"  - {e}", file=sys.stderr)
    sys.exit(1)
print(f"  검증 통과 — 서비스 {len(services)}개, 경로 {len(routes)}개")
PY
}

# ── sign ────────────────────────────────────────────────────────────────────
sign() {
  [ -n "${CATALOG_SIGNING_KEY:-}" ] || die "CATALOG_SIGNING_KEY 미설정 (environment secret)"
  local now expires
  now="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  expires="$(python3 -c "import datetime,os;print((datetime.datetime.utcnow()+datetime.timedelta(days=int(os.environ['D']))).strftime('%Y-%m-%dT%H:%M:%SZ'))" D="$EXPIRE_DAYS")"

  # 메타 주입 — 서명 대상 원문을 여기서 확정한다(이후 재포맷 금지).
  python3 - "$CATALOG" "$now" "$expires" "${COMMIT:-}" "${RUN_ID:-}" <<'PY'
import json, sys
path, now, exp, commit, run = sys.argv[1:6]
cat = json.load(open(path, encoding="utf-8"))
cat["issuedAt"], cat["expiresAt"] = now, exp
if commit: cat["commit"] = commit
if run: cat["runId"] = run
with open(path, "w", encoding="utf-8") as f:
    json.dump(cat, f, ensure_ascii=False, indent=2, sort_keys=True)
    f.write("\n")
PY
  log "메타 기록 — issuedAt=$now expiresAt=$expires"

  local signer="${RUNNER_TEMP:-/tmp}/gojira-sign"
  ( cd "${ROOT_DIR}/tools/gojira-sign" && GOWORK=off go build -o "$signer" . )
  SIGNING_KEY="$CATALOG_SIGNING_KEY" "$signer" sign -in "$CATALOG"
  ok "카탈로그 서명 완료 → ${CATALOG}.sig"
}

# ── publish ─────────────────────────────────────────────────────────────────
# 카탈로그 원본이 이미 launcher-dist 안에 있으므로 여기서는 존재 확인만 한다.
# 게시 경로(배포 서버 반영)는 §15 미해결 2번 — 확정되면 이 단계에 연결한다.
publish() {
  [ -f "${CATALOG}.sig" ] || die "서명(.sig) 없음 — sign 을 먼저 실행"
  ok "게시 대상 준비됨: $(basename "$CATALOG") + .sig"
  warn "TODO(§15-2): Platform-API 호스트 게시 경로 확정 후 연결 (bind mount 또는 rsync)"
}

case "$cmd" in
  validate) log "카탈로그 검증 — $CATALOG"; validate ;;
  sign)     log "카탈로그 서명 — $CATALOG"; validate; sign ;;
  publish)  publish ;;
esac
