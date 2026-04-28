"""OpenCode 브리지 — Agent-Service 를 OpenCode 서버(:4096)에 연결하는 얇은 어댑터.

- 세션 매핑 (Mongo agent_sessions.openCodeSessionId)
- UI SSE 계약(event: session/reasoning/tool_call/tool_result/content/done) 유지
- OpenCode /event SSE 이벤트 번역
"""
