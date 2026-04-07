package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"

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

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	// 세션 관리
	if req.SessionID == "" {
		req.SessionID = uuid.New().String()
		db.CreateAgentSession(ctx, req.SessionID, userID, bson.M(req.Context))
	} else {
		// 기존 세션에 컨텍스트 업데이트
		if req.Context != nil {
			db.UpdateAgentSessionContext(ctx, req.SessionID, bson.M(req.Context))
		}
	}

	// 대화 이력 로드 (현재 메시지 저장 전 — 이전 턴만)
	session, _ := db.GetAgentSession(ctx, req.SessionID)
	messages := buildLLMMessages(session, req.Message)
	// 현재 사용자 메시지 명시적으로 추가
	messages = append(messages, map[string]interface{}{"role": "user", "content": req.Message})

	// 이제 현재 메시지를 세션에 저장
	db.AppendAgentMessage(ctx, req.SessionID, bson.M{
		"role": "user", "content": req.Message, "timestamp": time.Now(),
	})

	// Agent Router: LLM에 tools와 함께 요청
	tools := skills.Default.ListTools()
	reply, actions, err := routeAgent(ctx, messages, tools, userID)
	if err != nil {
		log.Printf("Agent error: %v", err)
		reply = "죄송합니다, 처리 중 오류가 발생했습니다: " + err.Error()
	}

	// 어시스턴트 메시지 저장
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

// GET /api/v{version}/Agent/tools — MCP tool 목록
func AgentToolsHandler(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"tools": skills.Default.ListTools(),
	})
}

// buildLLMMessages 세션 이력으로 LLM messages 배열 구성
func buildLLMMessages(session bson.M, currentMessage string) []map[string]interface{} {
	msgs := []map[string]interface{}{
		{"role": "system", "content": agentSystemPrompt()},
	}

	// 이전 대화 이력 (최근 10턴)
	if session != nil {
		if history, ok := session["messages"].([]interface{}); ok {
			start := 0
			if len(history) > 20 {
				start = len(history) - 20
			}
			for _, m := range history[start:] {
				switch msg := m.(type) {
				case bson.M:
					role, _ := msg["role"].(string)
					content, _ := msg["content"].(string)
					if role != "" && content != "" {
						msgs = append(msgs, map[string]interface{}{"role": role, "content": content})
					}
				case bson.D:
					mm := bson.M{}
					for _, e := range msg {
						mm[e.Key] = e.Value
					}
					role, _ := mm["role"].(string)
					content, _ := mm["content"].(string)
					if role != "" && content != "" {
						msgs = append(msgs, map[string]interface{}{"role": role, "content": content})
					}
				}
			}
		}
	}

	return msgs
}

// routeAgent LLM에 tool_use 요청 → Skill 실행 → 최종 응답 (재귀적 tool_call 처리)
func routeAgent(ctx context.Context, messages []map[string]interface{}, tools []map[string]interface{}, user string) (string, []map[string]interface{}, error) {
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

	var allActions []map[string]interface{}
	maxRounds := 5 // 무한루프 방지

	for round := 0; round < maxRounds; round++ {
		result, err := skills.CallLLMWithTools(messages, oaiTools)
		if err != nil {
			return "", allActions, err
		}

		choices, _ := result["choices"].([]interface{})
		if len(choices) == 0 {
			return "응답을 생성할 수 없습니다.", allActions, nil
		}

		choice, _ := choices[0].(map[string]interface{})
		message, _ := choice["message"].(map[string]interface{})
		content, _ := message["content"].(string)
		toolCalls, _ := message["tool_calls"].([]interface{})

		// tool_call이 없으면 최종 텍스트 응답
		if len(toolCalls) == 0 {
			if content == "" && len(allActions) > 0 {
				content = "작업이 완료되었습니다."
			}
			return content, allActions, nil
		}

		// assistant message를 messages에 추가 (tool_calls 포함)
		messages = append(messages, message)

		// 모든 tool_call 실행
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

			log.Printf("Agent round=%d skill=%s args=%v", round, fnName, fnArgs)

			skillResult, err := skills.Default.Execute(ctx, fnName, fnArgs, user)
			if err != nil {
				skillResult = map[string]string{"error": err.Error()}
			}

			resultJSON, _ := json.Marshal(skillResult)
			allActions = append(allActions, map[string]interface{}{
				"skill": fnName, "params": fnArgs, "result": skillResult,
			})

			messages = append(messages, map[string]interface{}{
				"role": "tool", "tool_call_id": tcID, "content": string(resultJSON),
			})
		}
		// 다음 라운드에서 LLM이 tool_result를 보고 최종 응답 또는 추가 tool_call
	}

	return "처리 라운드 초과", allActions, nil
}

func agentSystemPrompt() string {
	return `당신은 GoJIRA 프로젝트 관리 시스템의 AI 도우미입니다.
사용자의 요청에 대해 반드시 제공된 도구(function)를 호출하여 실제 데이터를 조회하고 작업을 수행해야 합니다.

중요 규칙:
- 사용자가 프로젝트, 이슈, 문서, 일정에 대해 질문하면 반드시 해당 도구를 호출하세요
- 추측하거나 가정하지 마세요. 항상 도구를 통해 실제 데이터를 확인하세요
- 도구 호출 결과를 한국어로 사용자가 이해하기 쉽게 요약하세요
- 이슈/프로젝트 목록은 마크다운 표 형식으로 정리하세요
- 프로젝트 코드가 불명확하면 먼저 project_query로 목록을 확인하세요

사용 가능한 도구:
- project_query: 프로젝트 목록 조회 (프로젝트 수, 이름, 상태)
- issue_query: 프로젝트별 이슈 조회 (타입별 필터 가능)
- issue_manage: 이슈 관리 (action: create/update/delete/link)
- doc_manage: 문서 관리 (action: list/summarize/diff)
- pr_manage: PR 관리 (action: create/list)
- schedule_analyze: 프로젝트 일정 분석`
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
