package config

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"text/template"
)

var promptDir string

func init() {
	// 실행 파일 기준으로 prompts 디렉토리 탐색
	// 1. 환경변수
	if d := os.Getenv("GOJIRA_PROMPTS_DIR"); d != "" {
		promptDir = d
		return
	}
	// 2. 소스 파일 기준 (개발 환경)
	_, filename, _, ok := runtime.Caller(0)
	if ok {
		d := filepath.Join(filepath.Dir(filename), "..", "prompts")
		if info, err := os.Stat(d); err == nil && info.IsDir() {
			promptDir = d
			return
		}
	}
	// 3. 현재 디렉토리 기준
	promptDir = "prompts"
}

// LoadPrompt 프롬프트 파일을 읽고 템플릿 변수를 치환하여 반환
func LoadPrompt(name string, data map[string]string) string {
	path := filepath.Join(promptDir, name)
	raw, err := os.ReadFile(path)
	if err != nil {
		log.Printf("프롬프트 파일 로드 실패 (%s): %v", path, err)
		return ""
	}

	tmpl, err := template.New(name).Parse(string(raw))
	if err != nil {
		log.Printf("프롬프트 템플릿 파싱 실패 (%s): %v", name, err)
		return string(raw)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		log.Printf("프롬프트 템플릿 실행 실패 (%s): %v", name, err)
		return string(raw)
	}
	return buf.String()
}
