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
	SkillsSourceDir string   `json:"skillsSourceDir"` // <dir>/.claude 를 각 clone 에 symlink. 미설정 시 ~/.gojira/skills 기본값(임베드 스킬 추출 대상).
	Update          Update   `json:"update"`          // 자동 업데이트 설정(ClickOnce 식 — 기동 시 + 주기 체크).
}

// Update — 자동 업데이트(자기 교체) 설정.
//
// ManifestURL 이 비어있으면 backend.platformApi 호스트에서 /launcher/version.json 을
// 자동 유추한다(폐쇄망에서도 사내 백엔드가 서빙). CheckOnStart=false 또는 IntervalMinutes=0
// 이면 해당 트리거를 끈다.
type Update struct {
	ManifestURL     string `json:"manifestUrl"`     // 예: http://<gojira-host>:9000/launcher/version.json (생략 시 backend 에서 유추)
	CheckOnStart    *bool  `json:"checkOnStart"`    // 기동 시 1회 체크 (기본 true)
	IntervalMinutes int    `json:"intervalMinutes"` // 주기 체크 간격(분). 0 이면 기본 60, 음수면 주기 체크 끔.
}

type Backend struct {
	PlatformAPI string `json:"platformApi"`
	GojiraAPI   string `json:"gojiraApi"`
}

// Git 은 clone URL 조립용. <Base>/<Org>/<code>.git
//
// User/Token 이 설정되면 clone URL 에 자동 임베드 — http://<user>:<token>@<host>/<org>/<repo>.git
// → 사용자 PC 의 git credential helper / .netrc 없이도 인증 자동.
// 폐쇄망 Gitea (익명 차단 + GCM OAuth 차단 환경) 에서 매번 인증 prompt 가 뜨는 문제 해결.
// 보안: launcher.json 은 사용자 홈에 있고 파일 권한이 OS 제어이므로 본인만 접근 가능.
type Git struct {
	Base        string `json:"base"`        // 예: http://gitea-host:3000
	Org         string `json:"org"`         // 예: gojira
	User        string `json:"user"`        // Gitea 사용자명 (생략 시 익명 — 폐쇄망 권장: PAT 발급 후 채움)
	Token       string `json:"token"`       // Gitea PAT (생략 시 익명 — 채워두면 clone/fetch/push 자동 인증)
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
// 파일이 없으면 placeholder 를 생성한 뒤(채워야 할 필드 노출용) 기본값으로 동작한다.
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
	} else if os.IsNotExist(err) {
		// 첫 기동 — placeholder 생성해서 사용자에게 채울 필드를 노출.
		// 실패는 fatal 아님 — 디폴트로 계속 동작.
		if werr := writePlaceholderConfig(dir, path); werr != nil {
			fmt.Fprintf(os.Stderr, "launcher.json placeholder 생성 실패 (%s): %v\n", path, werr)
		} else {
			fmt.Fprintf(os.Stderr, "launcher.json placeholder 생성: %s — git.user/git.token 채우고 launcher 재시작 필요\n", path)
		}
	} else {
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

// writePlaceholderConfig — launcher.json 이 없을 때 사용자에게 채울 필드를 노출하는
// placeholder 를 생성한다. user/token 은 빈 문자열 → 폐쇄망에서 인증 자동화하려면
// 사용자가 직접 채워야 함을 명시.
func writePlaceholderConfig(dir, path string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	content := `{
  "defaultAgent": "opencode",
  "repoRoot": "~/GoJIRA-projects",
  "backend": {
    "platformApi": "http://<gojira-host>:9000/api/v2",
    "gojiraApi": "http://<gojira-host>:9001/api/v2"
  },
  "port": 5599,
  "webOrigin": "http://<gojira-app-host>:3001",
  "git": {
    "base": "http://<gitea-host>:3300",
    "org": "gojira",
    "user": "",
    "token": "",
    "authorName": "",
    "authorEmail": ""
  },
  "skillsSourceDir": "",
  "update": {
    "manifestUrl": "",
    "checkOnStart": true,
    "intervalMinutes": 60
  }
}
`
	return os.WriteFile(path, []byte(content), 0o600)
}

// gitAuthSummary — 시작 로그용. user/token 미설정 시 명시 경고.
func (c *Config) gitAuthSummary() string {
	hasAuth := c.Git.User != "" && c.Git.Token != ""
	if !hasAuth {
		return "git auth=anonymous (WARN: git.user/git.token 미설정 — 폐쇄망 Gitea 에서 매 호출마다 인증 prompt 가 뜰 수 있습니다. " +
			"launcher.json 의 git.user/git.token 을 채우고 재시작하세요.)"
	}
	masked := c.Git.Token
	if len(masked) > 6 {
		masked = masked[:6] + "..."
	}
	return fmt.Sprintf("git auth=user=%q token=%s", c.Git.User, masked)
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
	if cfg.SkillsSourceDir == "" {
		// 임베드 스킬 추출 기본 위치 — ~/.gojira/skills/.claude 로 풀린다.
		if dir, err := gojiraDir(); err == nil {
			cfg.SkillsSourceDir = filepath.Join(dir, "skills")
		}
	}
	cfg.RepoRoot = expandHome(cfg.RepoRoot)
	cfg.SkillsSourceDir = expandHome(cfg.SkillsSourceDir)

	if cfg.Update.CheckOnStart == nil {
		def := true
		cfg.Update.CheckOnStart = &def
	}
	if cfg.Update.IntervalMinutes == 0 {
		cfg.Update.IntervalMinutes = 60
	}
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
