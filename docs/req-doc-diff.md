🚀 Claude Code 실행 프롬프트 (Short Version)

Git 기반 Markdown 설계 문서 협업 시스템에서  문서내용보기 화면애 Diff UX를 추가

🎯 목표

문서보기 화면 우상단에 'Diff 분석' 버튼을 클릭하면 문서 보기창에아래 3개 탭을 구현하라:

* Diff
기존 unified/split diff 그대로 제공

* Semantic Diff (기본 탭)
Markdown 문서를 구조 기반으로 비교
line이 아닌 섹션/문단/문장 단위 diff
다음 지원:
문단 추가/삭제/수정
문장/단어 단위 강조
섹션(heading) 기준 비교
UI:
side-by-side 또는 inline 강조
비개발자도 이해 가능해야 함

* Summary (AI)
변경사항을 자연어로 요약
예:
“성능 요구사항 섹션이 추가됨”
“인증 방식이 세션 → OAuth2로 변경됨”

🧠 핵심 요구사항
Semantic Diff 처리
Markdown → AST 파싱
heading 기준 섹션 매칭
문단 단위 비교
수정된 문단은 단어 diff 수행

출력 JSON 구조 (Semantic Diff)
{
  "summary": {
    "sectionsAdded": 1,
    "sectionsModified": 2
  },
  "sections": [
    {
      "title": "인증 방식",
      "changeType": "modified",
      "blocks": [
        {
          "type": "paragraph",
          "changeType": "modified",
          "before": "세션 기반 인증",
          "after": "OAuth2 기반 인증",
          "inlineDiff": [
            { "type": "removed", "text": "세션 기반" },
            { "type": "added", "text": "OAuth2 기반" }
          ]
        }
      ]
    }
  ]
}

🖥️ UI 요구사항
탭 구조:
Diff | Semantic | Summary
기본 탭: Semantic 
Semantic Diff 화면:
변경 통계 표시
섹션 네비게이션
문단 단위 diff 카드 또는 side-by-side


⚠️ 주의
Git diff를 대체하지 말고 병행 제공
Semantic Diff는 “이해하기 쉬움”이 핵심
Markdown 구조(heading/paragraph)를 반드시 활용할 것