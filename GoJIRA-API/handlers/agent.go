package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"

	"gojira-api/config"
	"gojira-api/db"
	"gojira-api/skills"
)

type chatRequest struct {
	SessionID string                 `json:"sessionId"`
	Message   string                 `json:"message"`
	Context   map[string]interface{} `json:"context"`
}

type chatResponse struct {
	SessionID   string                   `json:"sessionId"`
	Reply       string                   `json:"reply"`
	Actions     []map[string]interface{} `json:"actions"`
	Suggestions []string                 `json:"suggestions"`
}

// POST /api/v{version}/Agent/chat
func AgentChatHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-Id")
	if userID == "" {
		userID = "anonymous"
	}

	var req chatRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if req.Message == "" {
		respondError(w, http.StatusBadRequest, "message is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	// 세션 관리
	if req.SessionID == "" {
		req.SessionID = uuid.New().String()
		db.CreateAgentSession(ctx, req.SessionID, userID, bson.M(req.Context))
	} else if req.Context != nil {
		db.UpdateAgentSessionContext(ctx, req.SessionID, bson.M(req.Context))
	}

	// 이전 대화 이력 로드 (현재 메시지 저장 전)
	session, _ := db.GetAgentSession(ctx, req.SessionID)
	messages := buildMessages(session)

	// 현재 사용자 메시지 추가
	messages = append(messages, map[string]interface{}{"role": "user", "content": req.Message})

	// 세션에 사용자 메시지 저장
	db.AppendAgentMessage(ctx, req.SessionID, bson.M{
		"role": "user", "content": req.Message, "timestamp": time.Now(),
	})

	// === Agentic Loop (Claude Code 패턴) ===
	reply, actions := agentLoop(ctx, messages, userID)

	// 세션에 어시스턴트 응답 저장
	db.AppendAgentMessage(ctx, req.SessionID, bson.M{
		"role": "assistant", "content": reply, "actions": actions, "timestamp": time.Now(),
	})

	respondJSON(w, http.StatusOK, chatResponse{
		SessionID:   req.SessionID,
		Reply:       reply,
		Actions:     actions,
		Suggestions: generateSuggestions(req.Context),
	})
}

// GET /api/v{version}/Agent/sessions — 세션 목록
func AgentSessionsHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-Id")
	query := r.URL.Query().Get("q")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var sessions []bson.M
	var err error
	if query != "" {
		sessions, err = db.SearchAgentSessions(ctx, userID, query)
	} else {
		sessions, err = db.ListAgentSessions(ctx, userID)
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// 응답 형태 정리: id, title(첫 메시지), updatedAt
	type SessionItem struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		UpdatedAt string `json:"updatedAt"`
	}
	items := make([]SessionItem, 0, len(sessions))
	for _, s := range sessions {
		id, _ := s["_id"].(string)
		updatedAt := formatTime(s["updatedAt"])
		title := "새 대화"
		// messages는 $slice:1로 첫 메시지만 포함
		var msgs []interface{}
		switch v := s["messages"].(type) {
		case []interface{}:
			msgs = v
		case bson.A:
			msgs = []interface{}(v)
		}
		if len(msgs) > 0 {
			msg := toBsonM(msgs[0])
			if msg != nil {
				if c, _ := msg["content"].(string); c != "" {
					title = c
					if len(title) > 50 {
						title = title[:50] + "..."
					}
				}
			}
		}
		items = append(items, SessionItem{ID: id, Title: title, UpdatedAt: updatedAt})
	}
	respondJSON(w, http.StatusOK, items)
}

// GET /api/v{version}/Agent/session/{id} — 세션 상세 (메시지 전체)
func AgentSessionDetailHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	session, err := db.GetAgentSession(ctx, sessionID)
	if err != nil {
		respondError(w, http.StatusNotFound, "세션을 찾을 수 없습니다")
		return
	}
	respondJSON(w, http.StatusOK, session)
}

// GET /api/v{version}/Agent/tools — MCP tool 목록
func AgentToolsHandler(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"tools": skills.Default.ListTools(),
	})
}

// ─── Agentic Loop ───
// Claude Code와 동일한 패턴:
// 1. LLM에 messages + tools 전달
// 2. stop_reason이 "tool_use"이면 도구 실행 → 결과를 messages에 추가 → 반복
// 3. stop_reason이 "end_turn" 또는 "stop"이면 루프 종료
// 4. 에러 발생 시 is_error: true로 LLM에 전달 → LLM이 재시도/대안 판단

func agentLoop(ctx context.Context, messages []map[string]interface{}, user string) (string, []map[string]interface{}) {
	oaiTools := buildToolDefs()
	var allActions []map[string]interface{}

	maxTurns := 10 // 최대 turn 수

	for turn := 0; turn < maxTurns; turn++ {
		// LLM 호출
		result, err := skills.CallLLMWithTools(messages, oaiTools)
		if err != nil {
			log.Printf("Agent LLM error at turn %d: %v", turn, err)
			return "LLM 호출 중 오류가 발생했습니다: " + err.Error(), allActions
		}

		choices, _ := result["choices"].([]interface{})
		if len(choices) == 0 {
			return "응답을 생성할 수 없습니다.", allActions
		}

		choice, _ := choices[0].(map[string]interface{})
		message, _ := choice["message"].(map[string]interface{})
		finishReason, _ := choice["finish_reason"].(string)
		content, _ := message["content"].(string)
		toolCalls, _ := message["tool_calls"].([]interface{})

		// stop_reason 확인: tool_calls가 없으면 루프 종료 (end_turn)
		if len(toolCalls) == 0 || finishReason == "stop" {
			if content == "" && len(allActions) > 0 {
				content = "작업이 완료되었습니다."
			}
			return content, allActions
		}

		// assistant message를 대화 기록에 추가 (tool_calls 포함)
		messages = append(messages, message)

		// 도구 실행: 각 tool_call 처리
		for _, tc := range toolCalls {
			tcMap, ok := tc.(map[string]interface{})
			if !ok {
				continue
			}
			fn, _ := tcMap["function"].(map[string]interface{})
			fnName, _ := fn["name"].(string)
			fnArgsStr, _ := fn["arguments"].(string)
			tcID, _ := tcMap["id"].(string)

			var fnArgs map[string]interface{}
			json.Unmarshal([]byte(fnArgsStr), &fnArgs)

			log.Printf("Agent turn=%d tool=%s args=%v", turn, fnName, fnArgs)

			// 도구 실행
			skillResult, execErr := skills.Default.Execute(ctx, fnName, fnArgs, user)

			toolResultMsg := map[string]interface{}{
				"role":         "tool",
				"tool_call_id": tcID,
			}

			if execErr != nil {
				// Claude Code 패턴: is_error로 에러 전달 → LLM이 재시도/대안 판단
				log.Printf("Agent tool error: %s — %v", fnName, execErr)
				toolResultMsg["content"] = execErr.Error()
				// OpenAI는 is_error를 직접 지원 안 하므로 content에 에러 표시
				allActions = append(allActions, map[string]interface{}{
					"skill": fnName, "params": fnArgs, "error": execErr.Error(),
				})
			} else {
				resultJSON, _ := json.Marshal(skillResult)
				toolResultMsg["content"] = string(resultJSON)
				allActions = append(allActions, map[string]interface{}{
					"skill": fnName, "params": fnArgs, "result": skillResult,
				})
			}

			messages = append(messages, toolResultMsg)
		}

		// 다음 turn: LLM이 tool_result를 보고 판단 (추가 tool_call 또는 최종 응답)
	}

	return "최대 처리 단계를 초과했습니다. 질문을 좁혀서 다시 시도해주세요.", allActions
}

// buildToolDefs OpenAI function calling 형식 도구 정의
func buildToolDefs() []map[string]interface{} {
	tools := skills.Default.ListTools()
	oaiTools := make([]map[string]interface{}, len(tools))
	for i, t := range tools {
		oaiTools[i] = map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        t["name"],
				"description": t["description"],
				"parameters":  t["input_schema"],
			},
		}
	}
	return oaiTools
}

// buildMessages 세션 이력에서 LLM messages 배열 구성
func buildMessages(session bson.M) []map[string]interface{} {
	systemPrompt := config.LoadPrompt("agent-system.txt", nil)
	if systemPrompt == "" {
		systemPrompt = defaultSystemPrompt()
	}

	msgs := []map[string]interface{}{
		{"role": "system", "content": systemPrompt},
	}

	if session == nil {
		return msgs
	}

	history, ok := session["messages"].([]interface{})
	if !ok {
		return msgs
	}

	// 최근 20개 메시지만 (컨텍스트 윈도우 관리)
	start := 0
	if len(history) > 20 {
		start = len(history) - 20
	}

	for _, m := range history[start:] {
		msg := toBsonM(m)
		if msg == nil {
			continue
		}
		role, _ := msg["role"].(string)
		content, _ := msg["content"].(string)
		if role == "" {
			continue
		}
		// user/assistant 메시지만 이력에 포함 (tool 메시지는 턴 내에서만 유효)
		if role == "user" || (role == "assistant" && content != "") {
			msgs = append(msgs, map[string]interface{}{"role": role, "content": content})
		}
	}

	return msgs
}

func toBsonM(v interface{}) bson.M {
	switch m := v.(type) {
	case bson.M:
		return m
	case bson.D:
		r := bson.M{}
		for _, e := range m {
			r[e.Key] = e.Value
		}
		return r
	case map[string]interface{}:
		return bson.M(m)
	}
	return nil
}

func formatTime(v interface{}) string {
	switch t := v.(type) {
	case time.Time:
		return t.Format("2006-01-02 15:04")
	case bson.DateTime:
		return t.Time().Format("2006-01-02 15:04")
	case string:
		return t
	}
	return ""
}

func defaultSystemPrompt() string {
	return `당신은 GoJIRA 프로젝트 관리 시스템의 AI 에이전트입니다.
사용자의 요청을 이해하고, 제공된 도구를 사용하여 실제 데이터를 조회하고 작업을 수행합니다.
항상 한국어로 응답합니다. 추측하지 말고 도구를 통해 확인하세요.`
}

func generateSuggestions(ctx map[string]interface{}) []string {
	if ctx == nil {
		return []string{"프로젝트 목록 보여줘", "이슈 현황 알려줘"}
	}
	page, _ := ctx["page"].(string)
	switch page {
	case "docs":
		return []string{"문서 요약해줘", "Diff 분석해줘", "PR 생성해줘"}
	case "issues":
		return []string{"이슈 현황 정리해줘", "관계도 분석해줘"}
	default:
		return []string{"이슈 현황 알려줘", "일정 분석해줘"}
	}
}
