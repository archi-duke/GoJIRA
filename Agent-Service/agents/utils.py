"""에이전트 공통 유틸리티."""

import re


def strip_reasoning(text: str) -> str:
    """LLM 응답에서 ReACT 추론 과정을 제거하고 사용자에게 보여줄 내용만 반환한다.

    전략:
    1. **Answer**: 가 있으면 그 이후만 반환
    2. 없으면 번호 목록/불릿 질문 직전까지의 추론 서술을 제거
    """
    if not text:
        return text

    # 1. **Answer**: 이후만 추출
    m = re.search(r'\*\*Answer\*\*\s*:\s*', text)
    if m:
        return text[m.end():].strip()

    # 2. 마커 접두어 제거
    cleaned = re.sub(r'\*\*(Thought|Action|Observation)\*\*\s*:\s*', '', text)

    # 3. 번호 목록이나 불릿 질문이 있으면, 그 직전까지의 서술을 제거
    question_start = re.search(
        r'\n\s*(?:\d+[\.\)]\s|[-•]\s|\|)',
        cleaned,
    )
    if question_start:
        before = cleaned[:question_start.start()]
        after = cleaned[question_start.start():]

        # 질문 직전 마지막 줄만 확인 — "~하겠습니다:", "~주세요:" 등 안내 문장이면 포함
        lines = before.rstrip().rsplit('\n', 1)
        last_line = lines[-1].strip() if lines else ''

        if re.search(r'(?:하겠습니다|주세요|알려주세요|답변해주세요)[.:\s]*$', last_line):
            return (last_line + after).strip()
        else:
            return after.strip()

    # 4. 패턴 매칭 안 되면 원본에서 마커만 제거
    cleaned = cleaned.strip()
    return cleaned if cleaned else text
