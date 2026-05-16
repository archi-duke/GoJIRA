package config

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"

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
	RepoBaseDir   string
	WebhookURL    string
	WebhookSecret string
	GitHubAPI     string

	// SMTP — 메일 기능 미사용 시 비어있어도 무방
	SMTPHost string
	SMTPPort string
	SMTPUser string
	SMTPPass string
	SMTPFrom string

	// 편집 세션 TTL (기본 30분)
	EditSessionTTL time.Duration
	// Worktree TTL — fs cleanup 전용 (기본 4시간). EditSession TTL 과 분리.
	// idle 사용자의 worktree 디스크 회수 주기. cleanup 은 branch 보존 (RemoveWorktreeOnly).
	WorkspaceTTL time.Duration
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
	MongoDB = envOrDefault("PLATFORM_MONGO_DB", "gojira")
	RepoBaseDir = envOrDefault("PLATFORM_REPO_DIR", "./repos")
	WebhookURL = envOrDefault("PLATFORM_WEBHOOK_URL", "")
	WebhookSecret = envOrDefault("PLATFORM_WEBHOOK_SECRET", "gojira-webhook-secret")
	SMTPHost = envOrDefault("SMTP_HOST", "")
	SMTPPort = envOrDefault("SMTP_PORT", "587")
	SMTPUser = envOrDefault("SMTP_USER", "")
	SMTPFrom = envOrDefault("SMTP_FROM", "")

	// 편집 세션 TTL
	ttlMin, _ := strconv.Atoi(envOrDefault("PLATFORM_EDIT_SESSION_TTL", "30"))
	EditSessionTTL = time.Duration(ttlMin) * time.Minute

	// Worktree TTL — 분 단위 입력, 기본 240분 (=4시간).
	// EditSessionTTL 과 분리해서 운영자가 별도로 늘릴 수 있음.
	wtTTLMin, _ := strconv.Atoi(envOrDefault("PLATFORM_WORKSPACE_TTL", "240"))
	WorkspaceTTL = time.Duration(wtTTLMin) * time.Minute

	// SMTP 비밀번호 복호화
	raw := envOrDefault("SMTP_PASS", "")
	if val, err := Decrypt(raw); err == nil {
		SMTPPass = val
	} else {
		SMTPPass = raw
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
