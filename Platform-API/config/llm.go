package config

// llm.go: 공유 llm 모듈로 위임. config.LLM 은 *llm.Config 로, 호출은 llm.Call/CallPrompt 사용.
//
// translateRepoName 등 Platform-API 의 LLM 사용 지점은 config.LLM 을 그대로 참조한다.

import (
	"log"
	"os"
	"path/filepath"
	"runtime"

	"platform-api/llm"
)

// LLM 은 공유 패키지의 Config 포인터. init/LoadLLMConfig 가 채운다.
var LLM *llm.Config

// LoadLLMConfig 는 llm.json 을 찾아 LLM 변수에 할당한다.
// 미발견 시 default 값을 사용 (gpt-4o-mini, OpenAI 엔드포인트).
func LoadLLMConfig() {
	paths := []string{}

	if p := os.Getenv("PLATFORM_LLM_CONFIG"); p != "" {
		paths = append(paths, p)
	}
	if _, filename, _, ok := runtime.Caller(0); ok {
		paths = append(paths, filepath.Join(filepath.Dir(filename), "llm.json"))
	}
	paths = append(paths, "config/llm.json", "llm.json")

	for _, p := range paths {
		cfg, err := llm.LoadConfig(p)
		if err != nil {
			continue
		}
		LLM = cfg
		log.Printf("LLM 설정 로드 완료: %s (model: %s, endpoint: %s)", p, LLM.Model, LLM.Endpoint)
		return
	}
	LLM = llm.DefaultConfig()
	log.Println("llm.json 파일을 찾을 수 없어 기본값을 사용합니다")
}
