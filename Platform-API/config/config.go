package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	ServerPort  = envOrDefault("PLATFORM_SERVER_PORT", ":9000")
	MongoURI    = envOrDefault("PLATFORM_MONGO_URI", "mongodb://localhost:27017")
	MongoDB     = envOrDefault("PLATFORM_MONGO_DB", "gojira")
	GitHubToken = envOrDefault("PLATFORM_GITHUB_TOKEN", "")
	GitHubOrg   = envOrDefault("PLATFORM_GITHUB_ORG", "")
	GitHubURL   = envOrDefault("GITHUB_URL", "https://github.com/")
	GitHubAPI      string
	RepoBaseDir    = envOrDefault("PLATFORM_REPO_DIR", "./repos")
	WebhookURL     = envOrDefault("PLATFORM_WEBHOOK_URL", "")
	WebhookSecret  = envOrDefault("PLATFORM_WEBHOOK_SECRET", "gojira-webhook-secret")
	GoJiraAPI      = envOrDefault("PLATFORM_GOJIRA_API", "http://localhost:9001/api/v2")

	SMTPHost = envOrDefault("SMTP_HOST", "")
	SMTPPort = envOrDefault("SMTP_PORT", "587")
	SMTPUser = envOrDefault("SMTP_USER", "")
	SMTPPass string
	SMTPFrom = envOrDefault("SMTP_FROM", "")

	// 편집 세션 TTL (기본 30분)
	EditSessionTTL time.Duration
)

func init() {
	loadEnvFile(".env")

	// .env 로드 후 변수 재설정
	ServerPort = envOrDefault("PLATFORM_SERVER_PORT", ":9000")
	MongoURI = envOrDefault("PLATFORM_MONGO_URI", "mongodb://localhost:27017")
	MongoDB = envOrDefault("PLATFORM_MONGO_DB", "gojira")
	GitHubToken = envOrDefault("PLATFORM_GITHUB_TOKEN", "")
	GitHubOrg = envOrDefault("PLATFORM_GITHUB_ORG", "")
	GitHubURL = envOrDefault("GITHUB_URL", "https://github.com/")
	RepoBaseDir = envOrDefault("PLATFORM_REPO_DIR", "./repos")
	WebhookURL = envOrDefault("PLATFORM_WEBHOOK_URL", "")
	WebhookSecret = envOrDefault("PLATFORM_WEBHOOK_SECRET", "gojira-webhook-secret")
	GoJiraAPI = envOrDefault("PLATFORM_GOJIRA_API", "http://localhost:9001/api/v2")
	SMTPHost = envOrDefault("SMTP_HOST", "")
	SMTPPort = envOrDefault("SMTP_PORT", "587")
	SMTPUser = envOrDefault("SMTP_USER", "")
	SMTPFrom = envOrDefault("SMTP_FROM", "")

	// 편집 세션 TTL
	ttlMin, _ := strconv.Atoi(envOrDefault("PLATFORM_EDIT_SESSION_TTL", "30"))
	EditSessionTTL = time.Duration(ttlMin) * time.Minute

	// SMTP 비밀번호 복호화
	raw := envOrDefault("SMTP_PASS", "")
	if val, err := Decrypt(raw); err == nil {
		SMTPPass = val
	} else {
		SMTPPass = raw
	}

	// OPENAI_API_KEY: ENC: prefix 면 복호화 후 환경변수에 평문으로 재설정
	// (translateRepoName 등이 os.Getenv 직접 호출하므로 평문으로 풀어둠)
	rawAPIKey := envOrDefault("OPENAI_API_KEY", "")
	if val, err := Decrypt(rawAPIKey); err == nil {
		os.Setenv("OPENAI_API_KEY", val)
	}

	// GitHubURL → GitHubAPI 변환
	base := strings.TrimRight(GitHubURL, "/")
	if strings.Contains(base, "github.com") && !strings.Contains(base, ".github.com") {
		GitHubAPI = "https://api.github.com"
	} else {
		// Gitea, Forgejo 등 self-hosted Git 플랫폼
		GitHubAPI = base + "/api/v1"
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

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
