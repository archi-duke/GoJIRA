package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config 는 ~/.gojira/launcher.json 에서 로드된다 (설계 §2.4).
//
// SSO/CLI 토큰은 여기에 넣지 않는다 — opencode/claude 가 첫 실행 시 자체 로그인하고
// 토큰을 사용자 홈에 저장한다. 여기 Token 은 웹↔런처 핸드셰이크용 설치 토큰이며,
// 설정에 없으면 첫 기동 시 자동 생성되어 token 파일에 영구 저장된다(§2.3).
type Config struct {
	DefaultAgent    string   `json:"defaultAgent"`    // opencode | claude
	RepoRoot        string   `json:"repoRoot"`        // 로컬 clone 루트
	Backend         Backend  `json:"backend"`
	Port            int      `json:"port"`
	WebOrigin       string   `json:"webOrigin"`       // (legacy) 허용할 웹 origin 단일값
	WebOrigins      []string `json:"webOrigins"`      // 허용할 웹 origin 목록 (Module Federation host + remote 등 다중 origin)
	Git             Git      `json:"git"`             // clone 대상 Gitea base + org
	Token           string   `json:"token"`           // 설치 토큰(미설정 시 자동 생성)
	SkillsSourceDir string   `json:"skillsSourceDir"` // <dir>/.claude 를 각 clone 에 symlink (있으면). agent CLI 가 GoJIRA 스킬 호출 가능하도록.
}

type Backend struct {
	PlatformAPI string `json:"platformApi"`
	GojiraAPI   string `json:"gojiraApi"`
}

// Git 은 clone URL 조립용. <Base>/<Org>/<code>.git
type Git struct {
	Base        string `json:"base"`        // 예: http://gitea-host:3000
	Org         string `json:"org"`         // 예: gojira
	AuthorName  string `json:"authorName"`  // git config user.name (생략 시 요청 user 사용)
	AuthorEmail string `json:"authorEmail"` // git config user.email (생략 시 <user>@gojira.local)
}

const (
	defaultPort         = 5599
	defaultAgent        = "opencode"
	defaultConfigSubdir = ".gojira"
	configFileName      = "launcher.json"
	tokenFileName       = "launcher-token"
)

// gojiraDir 는 ~/.gojira 경로를 돌려준다.
func gojiraDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("홈 디렉터리 확인 실패: %w", err)
	}
	return filepath.Join(home, defaultConfigSubdir), nil
}

// LoadConfig 는 launcher.json 을 읽어 기본값을 채운 Config 를 돌려준다.
// 파일이 없어도 기본값으로 동작한다(PoC 편의).
func LoadConfig() (*Config, error) {
	dir, err := gojiraDir()
	if err != nil {
		return nil, err
	}

	cfg := &Config{}
	path := filepath.Join(dir, configFileName)
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("%s 파싱 실패: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("%s 읽기 실패: %w", path, err)
	}

	applyDefaults(cfg)

	// 설치 토큰 확보(설정에 없으면 token 파일에서 로드/생성).
	if cfg.Token == "" {
		tok, err := ensureToken(dir)
		if err != nil {
			return nil, err
		}
		cfg.Token = tok
	}
	return cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.Port == 0 {
		cfg.Port = defaultPort
	}
	if cfg.DefaultAgent == "" {
		cfg.DefaultAgent = defaultAgent
	}
	if cfg.RepoRoot == "" {
		if home, err := os.UserHomeDir(); err == nil {
			cfg.RepoRoot = filepath.Join(home, "GoJIRA-projects")
		}
	}
	cfg.RepoRoot = expandHome(cfg.RepoRoot)
	cfg.SkillsSourceDir = expandHome(cfg.SkillsSourceDir)
}

// expandHome — 경로 선두의 `~` 를 사용자 home 으로 치환한다. Go 의 filepath.Join 은
// `~` 를 literal 디렉터리로 취급하므로 명시적 확장이 없으면 launcher 가
// `<cwd>/~/GoJIRA-projects/...` 같은 엉뚱한 곳에 clone 한다.
//
// 처리 규칙:
//   - `~` 단독         → home
//   - `~/...` / `~\...` → home + 나머지
//   - 그 외(`~user` 등) → 미지원, 원본 그대로 (확장 안전 보장)
//   - 빈 문자열        → 빈 문자열
//   - home 조회 실패   → 원본 그대로 (호환성)
func expandHome(p string) string {
	if p == "" || p[0] != '~' {
		return p
	}
	if p != "~" && !strings.HasPrefix(p, "~/") && !strings.HasPrefix(p, `~\`) {
		return p // ~user 형식 등은 미지원
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if p == "~" {
		return home
	}
	return filepath.Join(home, p[2:])
}

// ensureToken 은 ~/.gojira/launcher-token 에서 토큰을 읽고, 없으면 생성한다.
// 웹은 이 토큰을 Authorization 헤더로 보내야 한다(§2.3 핸드셰이크 PoC).
func ensureToken(dir string) (string, error) {
	path := filepath.Join(dir, tokenFileName)
	if data, err := os.ReadFile(path); err == nil {
		if tok := trimToken(string(data)); tok != "" {
			return tok, nil
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("%s 읽기 실패: %w", path, err)
	}

	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("토큰 생성 실패: %w", err)
	}
	tok := hex.EncodeToString(buf)

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("%s 생성 실패: %w", dir, err)
	}
	if err := os.WriteFile(path, []byte(tok+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("%s 쓰기 실패: %w", path, err)
	}
	return tok, nil
}

func trimToken(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
			continue
		}
		out = append(out, r)
	}
	return string(out)
}
