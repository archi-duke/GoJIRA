package config

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"runtime"
)

// LLMConfig LLM API 호출 설정
type LLMConfig struct {
	Endpoint    string            `json:"endpoint"`
	Model       string            `json:"model"`
	APIKey		string			  `json:"apiKey"` 
	MaxTokens   int               `json:"maxTokens"`
	Temperature float64           `json:"temperature"`
	Stream		bool			  `json:"stream"` 
	Headers     map[string]string `json:"headers"`
	AuthType    string            `json:"authType"` // "bearer" | "header" | "none"
	AuthHeader  string            `json:"authHeader"` // authType이 "header"일 때 사용할 헤더 이름
	ResponseFormat struct {
		ChoicesPath string `json:"choicesPath"`
		ContentPath string `json:"contentPath"`
		StreamPrefix string `json:"streamPrefix"`
		StreamDone string `json:"streamDone"`
	} `json:"responseFormat"`
}

var LLM LLMConfig

func init() {
	LLM = LLMConfig{
		Endpoint:    "https://api.openai.com/v1/chat/completions",
		Model:       "gpt-4o-mini",
		MaxTokens:   500,
		Temperature: 0.3,
		Stream : false,
		Headers:     map[string]string{"Content-Type": "application/json"},
		AuthType:    "bearer",
	}
	LLM.ResponseFormat.ChoicesPath = "choices"
	LLM.ResponseFormat.ContentPath = "message.content"
	LLM.ResponseFormat.StreamPrefix = "data: "
	LLM.ResponseFormat.StreamDone = "[DONE]"
}

// LoadLLMConfig llm.json 파일에서 LLM 설정을 로드
func LoadLLMConfig() {
	paths := []string{}

	// 1. 환경변수
	if p := os.Getenv("GOJIRA_LLM_CONFIG"); p != "" {
		paths = append(paths, p)
	}

	// 2. 소스 파일 기준 (개발 환경)
	if _, filename, _, ok := runtime.Caller(0); ok {
		paths = append(paths, filepath.Join(filepath.Dir(filename), "llm.json"))
	}

	// 3. 현재 디렉토리 기준
	paths = append(paths, "config/llm.json", "llm.json")

	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if err := json.Unmarshal(data, &LLM); err != nil {
			log.Printf("llm.json 파싱 실패 (%s): %v", p, err)
			continue
		}
		// Headers가 nil이면 기본값
		if LLM.Headers == nil {
			LLM.Headers = map[string]string{"Content-Type": "application/json"}
		}
		log.Printf("LLM 설정 로드 완료: %s (model: %s, endpoint: %s)", p, LLM.Model, LLM.Endpoint)
		return
	}
	log.Println("llm.json 파일을 찾을 수 없어 기본값을 사용합니다")
}
