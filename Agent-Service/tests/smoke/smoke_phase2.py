"""Phase 2 스킬 smoke test — driver_select → style_define 연쇄를 실제 LLM + DB 로 검증.

사용:
    cd Agent-Service
    .venv/bin/python scripts/smoke_phase2.py [PROJECT_CODE] [USER]
    # 기본: PROJECT_CODE=GOJIRA3, USER=batman

주의: 실제 OpenAI API 호출 발생. 1회에 수십만 토큰 소모 가능.
"""

import sys
import asyncio
import json


async def main(code: str, user: str):
    print(f"\n========== driver_select 호출 (project={code}, user={user}) ==========")
    from skills.driver_select.executor import execute as ds_execute
    ds = await ds_execute({"projectCode": code, "user": user}, user)
    print(f"message: {ds.get('message')}")
    print(f"drivers: {len(ds.get('drivers') or [])}건")
    for d in (ds.get("drivers") or [])[:5]:
        print(f"  - {d.get('subkey') or d.get('key')}: qad={d.get('qad')} qai={d.get('qai')} priority={d.get('priority')}")
    print(f"normalizedAttributes: {json.dumps(ds.get('normalizedAttributes') or {}, ensure_ascii=False)}")
    print(f"documentSaved: {ds.get('documentSaved')}")
    if ds.get("saveError"):
        print(f"saveError: {ds['saveError']}")

    print(f"\n========== style_define 호출 ==========")
    from skills.style_define.executor import execute as sd_execute
    sd = await sd_execute({"projectCode": code, "user": user}, user)
    print(f"message: {sd.get('message')}")
    rec = sd.get("recommendation") or {}
    print(f"엔진 순위 (top):")
    for s in rec.get("topScored") or []:
        print(f"  - {s['id']:22s} score={s['score']}")
    chosen = sd.get("chosen") or {}
    print(f"\nLLM chosen:")
    print(f"  Primary:    {chosen.get('primary')}")
    print(f"  Secondary:  {chosen.get('secondary')}")
    print(f"  Supporting: {chosen.get('supporting')}")
    print(f"documentSaved: {sd.get('documentSaved')}")
    if sd.get("saveError"):
        print(f"saveError: {sd['saveError']}")


if __name__ == "__main__":
    code = sys.argv[1] if len(sys.argv) > 1 else "GOJIRA3"
    user = sys.argv[2] if len(sys.argv) > 2 else "batman"
    asyncio.run(main(code, user))
