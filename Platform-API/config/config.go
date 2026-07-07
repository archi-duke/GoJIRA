package config

import (
	"log"
	"os"
	"strings"

	"gojira.local/shared/envload"
)

// 모든 설정은 .env (또는 환경변수) 에서 읽는다. 환경별로 달라야 하는 필수 항목은
// default 가 없어 미설정 시 startup 에서 즉시 실패한다 (fail-fast). 임의의
// localhost fallback 이 silent 동작하는 것을 방지한다.
var (
	// 필수 — 환경별로 다른 값 필요
	MongoURI    string
	GitHubToken string
	GitHubOrg   string
	GitHubURL   string
	GoJiraAPI   string

	// 선택 — 서비스 내부 컨벤션
	ServerPort    string
	MongoDB       string
	GojiraMongoDB string // DocRepo 의 projects 직접 갱신용 (D8 역방향 결합 — db/mongo.go 참조)
	RepoBaseDir   string
	WebhookURL    string
	WebhookSecret string
	GitHubAPI     string

	// 알림 발송 provider — "smtp"(사외/dev) | "internal"(사내 API) | "none" | ""(자동)
	NotifyProvider string
	// 사내 알림 API (NOTIFY_PROVIDER=internal) — 스펙 확정 전까지 스텁이 참조만 함
	NotifyAPIURL   string
	NotifyAPIToken string

	// SMTP — 메일 기능 미사용 시 비어있어도 무방
	SMTPHost string
	SMTPPort string
	SMTPUser string
	SMTPPass string
	SMTPFrom string
)

func init() {
	// .env 탐색 순서: PLATFORM_ENV_FILE → cwd/.env → <binary dir>/.env → <binary dir>/../.env
	envload.Load("PLATFORM_ENV_FILE")

	// 필수 항목 — 미설정 시 log.Fatal
	MongoURI = requireEnv("MONGO_URI")
	GitHubToken = requireEnv("PLATFORM_GITHUB_TOKEN")
	GitHubOrg = requireEnv("PLATFORM_GITHUB_ORG")
	GitHubURL = requireEnv("GITHUB_URL")
	GoJiraAPI = requireEnv("GOJIRA_API_URL")

	// 선택 항목 — default 허용
	ServerPort = envOrDefault("PLATFORM_SERVER_PORT", ":9000")
	// D8 (2026-07-07): 플랫폼 데이터(users/authGroups/orgUnits/notices 등)는 platform DB 로 분리.
	// 기존 gojira DB 의 해당 컬렉션은 deploy/scripts/migrate-platform-db.js 로 이관.
	MongoDB = envOrDefault("PLATFORM_MONGO_DB", "platform")
	GojiraMongoDB = envOrDefault("PLATFORM_GOJIRA_MONGO_DB", "gojira")
	RepoBaseDir = envOrDefault("PLATFORM_REPO_DIR", "./repos")
	WebhookURL = envOrDefault("PLATFORM_WEBHOOK_URL", "")
	WebhookSecret = envOrDefault("PLATFORM_WEBHOOK_SECRET", "gojira-webhook-secret")
	NotifyProvider = envOrDefault("NOTIFY_PROVIDER", "")
	NotifyAPIURL = envOrDefault("NOTIFY_API_URL", "")
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

	// 사내 알림 API 토큰 복호화 (SMTP_PASS 와 동일 규칙)
	rawTok := envOrDefault("NOTIFY_API_TOKEN", "")
	if val, err := Decrypt(rawTok); err == nil {
		NotifyAPIToken = val
	} else {
		NotifyAPIToken = rawTok
	}

	// LLM 관련 시크릿 복호화: ENC: prefix 가 있으면 복호화 후 환경에 평문으로 재설정.
	// llm.json 의 ${VAR} 치환은 LoadLLMConfig 시점에 os.Getenv 로 읽으므로,
	// 그 전에 평문이 환경에 들어가 있어야 한다.
	for _, key := range []string{"OPENAI_API_KEY", "PLATFORM_LLM_TICKET"} {
		raw := envOrDefault(key, "")
		if raw == "" {
			continue
		}
		if val, err := Decrypt(raw); err == nil && val != raw {
			os.Setenv(key, val)
		}
	}

	// LLM 설정 로드 (llm.json 기반 — translateRepoName 등이 사용)
	LoadLLMConfig()

	// GitHubURL → GitHubAPI 변환
	base := strings.TrimRight(GitHubURL, "/")
	if strings.Contains(base, "github.com") && !strings.Contains(base, ".github.com") {
		GitHubAPI = "https://api.github.com"
	} else {
		// Gitea, Forgejo 등 self-hosted Git 플랫폼
		GitHubAPI = base + "/api/v1"
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

// loadEnvFile 은 gojira-shared/envload 로 이관됨. 본 함수는 제거.
