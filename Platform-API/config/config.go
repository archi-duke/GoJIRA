package config

import (
	"bufio"
	"os"
	"strings"
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

	SMTPHost = envOrDefault("SMTP_HOST", "")
	SMTPPort = envOrDefault("SMTP_PORT", "587")
	SMTPUser = envOrDefault("SMTP_USER", "")
	SMTPPass string
	SMTPFrom = envOrDefault("SMTP_FROM", "")
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
	SMTPHost = envOrDefault("SMTP_HOST", "")
	SMTPPort = envOrDefault("SMTP_PORT", "587")
	SMTPUser = envOrDefault("SMTP_USER", "")
	SMTPFrom = envOrDefault("SMTP_FROM", "")

	// SMTP 비밀번호 복호화
	raw := envOrDefault("SMTP_PASS", "")
	if val, err := Decrypt(raw); err == nil {
		SMTPPass = val
	} else {
		SMTPPass = raw
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
