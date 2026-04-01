package config

import (
	"bufio"
	"log"
	"os"
	"strings"
)

var (
	ServerPort  = envOrDefault("GOJIRA_SERVER_PORT", ":9000")
	JiraMockURL = envOrDefault("GOJIRA_JIRA_URL", "http://localhost:8000")
	JiraAPIBase = envOrDefault("GOJIRA_JIRA_API_BASE", "/rest/api/2")
	JiraUser    string
	JiraPass    string
	MongoURI    = envOrDefault("GOJIRA_MONGO_URI", "mongodb://localhost:27017")
	MongoDB     = envOrDefault("GOJIRA_MONGO_DB", "gojira")
)

func init() {
	loadEnvFile(".env")

	raw := envOrDefault("GOJIRA_JIRA_USER", "")
	if val, err := Decrypt(raw); err == nil {
		JiraUser = val
	} else {
		log.Printf("JIRA_USER 복호화 실패: %v", err)
	}

	raw = envOrDefault("GOJIRA_JIRA_PASS", "")
	if val, err := Decrypt(raw); err == nil {
		JiraPass = val
	} else {
		log.Printf("JIRA_PASS 복호화 실패: %v", err)
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// loadEnvFile reads a .env file and sets environment variables (does not override existing).
func loadEnvFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return // .env 없으면 무시
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		// 기존 환경변수가 없을 때만 설정
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
}
