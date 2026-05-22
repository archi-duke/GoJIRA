package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config 는 ~/.gojira/launcher.json 에서 로드된다 (설계 §2.4).
//
// SSO/CLI 토큰은 여기에 넣지 않는다 — opencode/claude 가 첫 실행 시 자체 로그인하고
// 토큰을 사용자 홈에 저장한다. 여기 Token 은 웹↔런처 핸드셰이크용 설치 토큰이며,
// 설정에 없으면 첫 기동 시 자동 생성되어 token 파일에 영구 저장된다(§2.3).
type Config struct {
	DefaultAgent string  `json:"defaultAgent"` // opencode | claude
	RepoRoot     string  `json:"repoRoot"`     // 로컬 clone 루트
	Backend      Backend `json:"backend"`
	Port         int     `json:"port"`
	WebOrigin    string  `json:"webOrigin"` // 허용할 웹 origin (CORS/Origin 검증)
	Git          Git     `json:"git"`       // clone 대상 Gitea base + org
	Token        string  `json:"token"`     // 설치 토큰(미설정 시 자동 생성)
}

type Backend struct {
	PlatformAPI string `json:"platformApi"`
	GojiraAPI   string `json:"gojiraApi"`
}

// Git 은 clone URL 조립용. <Base>/<Org>/<code>.git
type Git struct {
	Base string `json:"base"` // 예: http://gitea-host:3000
	Org  string `json:"org"`  // 예: gojira
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
