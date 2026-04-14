"""[개발/테스트 전용] 프로젝트 이슈 + 문서 초기화 스킬."""

import re
import httpx

from core.database import issues, projects
from clients.platform_api import PlatformClient
from config import settings


# Platform-API init-files 핸들러와 동일한 템플릿 (일관성 유지)
_TEMPLATES = {
    "business/project-description.md": (
        "# Project Description\n\n"
        "과제의 배경, 목적, 주요 사항을 기록하는 문서입니다.\n\n"
        "## 과제 배경\n\n(작성 필요)\n\n"
        "## 과제 목적\n\n(작성 필요)\n\n"
        "## 주요 사항\n\n(작성 필요)\n\n"
        "> VOS 자동생성 시 이 문서의 내용을 참조합니다. 반드시 작성해 주세요.\n"
    ),
    "business/use-cases.md": (
        "# Use Cases\n\n"
        "과제의 유즈케이스를 기술하는 문서입니다.\n\n"
        "## Actors\n\n(작성 필요)\n\n"
        "## Use Case 목록\n\n(작성 필요)\n\n"
        "## Use Case 상세\n\n(작성 필요)\n"
    ),
    "business/requirements.md": (
        "# Requirements\n\n"
        "과제의 요구사양을 기술하는 문서입니다.\n\n"
        "## Functional Requirements (FR)\n\n(작성 필요)\n\n"
        "## Quality Attributes / Non-Functional Requirements (NFR)\n\n(작성 필요)\n\n"
        "## Constraints (CNST)\n\n(작성 필요)\n"
    ),
    "architecture/architecture.md": (
        "# Architecture Design\n\n"
        "과제의 주요 설계 사항을 기술하는 문서입니다.\n"
        "Architecture의 스타일과 주요 디자인 포인트의 설계, "
        "다양한 관점의 통합 디자인 및 설계 평가(SATAM)를 포함합니다.\n\n"
        "## Architecture Style\n\n(작성 필요)\n\n"
        "## Key Design Points\n\n(작성 필요)\n\n"
        "## Integrated Design Views\n\n(작성 필요)\n\n"
        "## SATAM (Sensitivity, Architecture Tradeoff Analysis)\n\n(작성 필요)\n"
    ),
    "architecture/drivers.md": (
        "# Architecture Drivers\n\n"
        "아키텍처 결정에 영향이 큰 요구사항(Driver)의 선정 결과를 기록하는 문서입니다.\n"
        "driver_select 스킬이 생성합니다.\n\n"
        "## 선정 Driver 목록\n\n(작성 필요)\n"
    ),
    "architecture/architecture-style.md": (
        "# 시스템 아키텍처 스타일\n\n"
        "선정된 시스템 스타일(Primary/Secondary/Supporting)과 근거를 기록하는 문서입니다.\n"
        "style_define 스킬이 생성합니다.\n\n"
        "## 추천 요약\n\n(작성 필요)\n"
    ),
    "architecture/design-decisions.md": (
        "# 설계 결정 (Architectural & Detailed Design)\n\n"
        "Design Point 별 설계 대안 비교와 결정 결과를 기록하는 문서입니다.\n"
        "design_decide 스킬이 생성합니다.\n\n"
        "## Design Point 요약\n\n(작성 필요)\n"
    ),
    "architecture/architecture-integrated.md": (
        "# 통합 설계 뷰 (Integrated Design Views)\n\n"
        "논리/프로세스/배포/개발 관점의 통합 설계를 기록하는 문서입니다.\n"
        "design_integrate 스킬이 생성합니다.\n\n"
        "## 개요\n\n(작성 필요)\n"
    ),
}


async def execute(params: dict, user: str) -> dict:
    code = params.get("projectCode", "")
    if not code:
        raise ValueError("projectCode 필수")

    confirm = bool(params.get("confirm", False))
    req_user = params.get("user", user)
    do_issues = bool(params.get("resetIssues", True))
    do_docs = bool(params.get("resetDocs", True))

    # 영향 범위 산정
    issue_keys = []
    if do_issues:
        cursor = issues().find(
            {"key": {"$regex": f"^{re.escape(code)}-"}},
            {"key": 1, "_id": 0},
        )
        issue_keys = [d["key"] for d in cursor if d.get("key")]

    target_paths = list(_TEMPLATES.keys()) if do_docs else []

    # dry-run
    if not confirm:
        return {
            "projectCode": code,
            "mode": "dry-run",
            "wouldDelete": {
                "issueCount": len(issue_keys),
                "issueKeys": issue_keys[:50],  # 미리보기
                "issueKeysTruncated": len(issue_keys) > 50,
                "userBranchFiles": target_paths,
                "branch": f"docs/{req_user}" if do_docs else None,
                "mainInitFiles": do_docs,
            },
            "message": (
                f"DRY-RUN: 이슈 {len(issue_keys)}건, 파일 {len(target_paths)}개가 영향받습니다. "
                f"실제 실행하려면 confirm=true로 다시 호출하세요."
            ),
        }

    # 실제 실행
    deleted_issues = []
    failed_issues = []
    if do_issues and issue_keys:
        api_base = settings.gojira_api_url.rstrip("/")
        async with httpx.AsyncClient() as client:
            for key in issue_keys:
                try:
                    resp = await client.request(
                        "DELETE",
                        f"{api_base}/Issue",
                        json={"key": key, "prjID": code},
                        headers={"X-User-Id": req_user, "Content-Type": "application/json"},
                        timeout=10,
                    )
                    if resp.status_code < 400:
                        deleted_issues.append(key)
                    else:
                        failed_issues.append({"key": key, "status": resp.status_code, "body": resp.text[:200]})
                except Exception as e:
                    failed_issues.append({"key": key, "error": str(e)})

        # 안전망: API에서 삭제했더라도 DB에 잔여가 있으면 정리 + 프로젝트 issues 배열 비우기
        issues().delete_many({"key": {"$regex": f"^{re.escape(code)}-"}})
        projects().update_one(
            {"jiraCodes.main.jiraCode": code},
            {"$set": {"issues": []}, "$unset": {"archSettings": ""}},
        )

    saved_files = []
    failed_files = []
    main_init_result = None
    if do_docs:
        platform = PlatformClient()
        # 사용자 브랜치 덮어쓰기
        for path, content in _TEMPLATES.items():
            try:
                await platform.save_file(
                    code, path, content,
                    f"[reset_project] {path} 초기화",
                    req_user,
                )
                saved_files.append(path)
            except Exception as e:
                failed_files.append({"path": path, "error": str(e)})

        # main 브랜치 누락 파일 보충
        try:
            main_init_result = await platform.init_files(code)
        except Exception as e:
            main_init_result = {"error": str(e)}

    return {
        "projectCode": code,
        "mode": "executed",
        "issues": {
            "deletedCount": len(deleted_issues),
            "failedCount": len(failed_issues),
            "deletedKeys": deleted_issues,
            "failed": failed_issues,
        } if do_issues else None,
        "docs": {
            "userBranch": f"docs/{req_user}",
            "savedFiles": saved_files,
            "failedFiles": failed_files,
            "mainInitFiles": main_init_result,
        } if do_docs else None,
        "message": (
            f"리셋 완료 — 이슈 삭제 {len(deleted_issues)}건"
            f"{' (실패 ' + str(len(failed_issues)) + '건)' if failed_issues else ''}, "
            f"문서 {len(saved_files)}개 덮어쓰기"
            f"{' (실패 ' + str(len(failed_files)) + '건)' if failed_files else ''}"
        ),
    }
