package config

import (
	"bufio"
	"log"
	"os"
	"strings"
)

// 모든 설정은 .env (또는 환경변수) 에서 읽는다. 환경별로 달라야 하는 필수 항목은
// default 가 없어 미설정 시 startup 에서 즉시 실패한다 (fail-fast).
var (
	// 필수 — 환경별로 다른 값
	MongoURI    string
	PlatformAPI string
	JiraMockURL string

	// 선택 — 서비스 내부 컨벤션
	ServerPort  string
	JiraAPIBase string
	MongoDB     string

	// 시크릿 (ENC: 복호화 후 채워짐)
	JiraUser string
	JiraPass string
)

func init() {
	loadEnvFile(".env")

	// 필수 항목 — 미설정 시 log.Fatal
	MongoURI = requireEnv("GOJIRA_MONGO_URI")
	PlatformAPI = requireEnv("GOJIRA_PLATFORM_API")
	JiraMockURL = requireEnv("GOJIRA_JIRA_URL")

	// 선택 항목 — default 허용
	ServerPort = envOrDefault("GOJIRA_SERVER_PORT", ":9001")
	JiraAPIBase = envOrDefault("GOJIRA_JIRA_API_BASE", "/rest/api/2")
	MongoDB = envOrDefault("GOJIRA_MONGO_DB", "gojira")

	// LLM 호출은 Platform-API /LLM/call 게이트웨이로 위임 — GoJIRA-API 는 OpenAI 키 미보유.

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

// requireEnv 는 필수 환경변수를 읽는다. 미설정 시 log.Fatal 로 즉시 종료해
// silent fallback 동작을 차단한다.
func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("[config] 필수 환경변수 미설정: %s — .env 또는 시스템 환경에 명시 필요", key)
	}
	return v
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
		return
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
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
}
