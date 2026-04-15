"""사업 분석 → 설계 통합 워크플로우 정의 (9 stages).

docs/agent-workflow-evolution.md §3.2 참조.

각 stage:
- id:        stage 식별자
- skill:     해당 stage 를 처리하는 주 스킬 이름 (engine 이 호출)
- completion: 완료 판정 기준 (코드 검증 — workflow_state 의 completedSteps 와 동일 의미)
- role:      의미 구분 (business / architecture / evaluation)
"""

BUSINESS_DESIGN = {
    "id": "business_design",
    "name": "사업 분석 → 설계 통합",
    "description": "project-description 인터뷰부터 SATAM 평가까지 9단계 선형 워크플로우",
    "stages": [
        {
            "id": "projdesc",
            "skill": "projdesc_interview",
            "role": "business",
            "completion": "projdesc_real",
            "multi_turn": True,
        },
        {
            "id": "vos",
            "skill": "vos_collect",
            "role": "business",
            "completion": "vos_count > 0",
        },
        {
            "id": "usecase",
            "skill": "usecase_analyze",
            "role": "business",
            "completion": "feature_count > 0",
        },
        {
            "id": "requirement",
            "skill": "requirement_refine",
            "role": "business",
            "completion": "requirement_count > 0",
        },
        {
            "id": "driver",
            "skill": "driver_select",
            "role": "architecture",
            "completion": "driver_selected",
        },
        {
            "id": "style",
            "skill": "style_define",
            "role": "architecture",
            "completion": "chosen_styles",
        },
        {
            "id": "design_decide",
            "skill": "design_decide",
            "role": "architecture",
            "completion": "adsn_or_ddsn",
        },
        {
            "id": "design_integrate",
            "skill": "design_integrate",
            "role": "architecture",
            "completion": "design_integrated",
        },
        {
            "id": "satam",
            "skill": "satam_analyze",
            "role": "evaluation",
            "completion": "satam_done",
        },
    ],
    "branches": [],  # 분기 — Phase C 에서 확장
}
