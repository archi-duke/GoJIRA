package handlers

import (
	"log"
	"net/http"
	"time"

	"platform-api/config"
	"platform-api/llm"
)

// LLMCallRequest: GoJIRA-API 등 다른 서비스가 Platform-API 의 LLM 기능을 호출할 때 사용.
//
//	{
//	  "messages": [{"role": "user", "content": "..."}], // OR "prompt"
//	  "prompt":   "...",                                 // 단일 user message 단축형
//	  "options":  { "model": "", "maxTokens": 0, "temperature": null }
//	}
type LLMCallRequest struct {
	Messages []llm.Message `json:"messages"`
	Prompt   string        `json:"prompt"`
	Options  *struct {
		Model       string   `json:"model"`
		MaxTokens   int      `json:"maxTokens"`
		Temperature *float64 `json:"temperature"`
	} `json:"options"`
}

type LLMCallResponse struct {
	Text  string `json:"text"`
	Error string `json:"error,omitempty"`
}

// LLMCallHandler POST /api/v2/LLM/call — LLM 호출 게이트웨이.
// Platform-API 가 OPENAI_API_KEY / 모델 / 엔드포인트 정책을 모두 관리하는
// 단일 진입점이다. 다른 서비스는 키 없이 HTTP 만으로 LLM 호출 가능.
func LLMCallHandler(w http.ResponseWriter, r *http.Request) {
	if config.LLM == nil {
		respondError(w, http.StatusInternalServerError, "LLM 설정 미로드")
		return
	}

	var req LLMCallRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	messages := req.Messages
	if len(messages) == 0 && req.Prompt != "" {
		messages = []llm.Message{{Role: "user", Content: req.Prompt}}
	}
	if len(messages) == 0 {
		respondError(w, http.StatusBadRequest, "messages 또는 prompt 필요")
		return
	}

	var opts *llm.Options
	if req.Options != nil {
		opts = &llm.Options{
			Model:       req.Options.Model,
			MaxTokens:   req.Options.MaxTokens,
			Temperature: req.Options.Temperature,
			Ctx:         r.Context(),
		}
	}

	text, err := llm.Call(config.LLM, messages, opts)
	if err != nil {
		log.Printf("LLM 호출 실패: %v", err)
		respondJSON(w, http.StatusBadGateway, LLMCallResponse{Error: err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, LLMCallResponse{Text: text})
}

// LLMChatHandler GET|POST /api/v2/LLM/chat — LLM 연결 검증용 단순 질의응답.
// llm.json 설정(endpoint/auth/모델)이 실제로 동작하는지 curl/브라우저로 바로 확인한다.
//   GET  /LLM/chat?q=안녕
//   POST /LLM/chat {"q": "안녕"}  (또는 {"message": "..."} / {"prompt": "..."})
func LLMChatHandler(w http.ResponseWriter, r *http.Request) {
	if config.LLM == nil {
		respondError(w, http.StatusInternalServerError, "LLM 설정 미로드")
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" && r.Method == http.MethodPost {
		var req struct {
			Q       string `json:"q"`
			Message string `json:"message"`
			Prompt  string `json:"prompt"`
		}
		if err := decodeJSON(r, &req); err == nil {
			switch {
			case req.Q != "":
				query = req.Q
			case req.Message != "":
				query = req.Message
			default:
				query = req.Prompt
			}
		}
	}
	if query == "" {
		respondError(w, http.StatusBadRequest, "질의가 없습니다 — ?q= 또는 body {\"q\": \"...\"} 로 전달")
		return
	}

	start := time.Now()
	text, err := llm.Call(config.LLM, []llm.Message{{Role: "user", Content: query}},
		&llm.Options{Ctx: r.Context()})
	elapsed := time.Since(start).Round(time.Millisecond)
	if err != nil {
		log.Printf("LLM chat 실패 (%s): %v", elapsed, err)
		respondJSON(w, http.StatusBadGateway, map[string]interface{}{
			"error":     err.Error(),
			"endpoint":  config.LLM.Endpoint,
			"model":     config.LLM.Model,
			"elapsedMs": elapsed.Milliseconds(),
		})
		return
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"answer":    text,
		"endpoint":  config.LLM.Endpoint,
		"model":     config.LLM.Model,
		"elapsedMs": elapsed.Milliseconds(),
	})
}

// decodeJSON / respondError / respondJSON 은 Platform-API/handlers 의 공용 헬퍼 사용.
