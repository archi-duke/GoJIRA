package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

func apiURL() string {
	if v := os.Getenv("GOJIRA_API_URL"); v != "" {
		return v
	}
	return "http://localhost:9001/api/v2"
}

func platformURL() string {
	if v := os.Getenv("GOJIRA_PLATFORM_URL"); v != "" {
		return v
	}
	return "http://localhost:9000/api/v2"
}

func userID() string {
	if v := os.Getenv("GOJIRA_USER"); v != "" {
		return v
	}
	return ""
}

func apiGet(path string, headers map[string]string) (interface{}, error) {
	req, _ := http.NewRequest("GET", apiURL()+path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return doRequest(req)
}

func platformGet(path string) (interface{}, error) {
	req, _ := http.NewRequest("GET", platformURL()+path, nil)
	return doRequest(req)
}

func apiPost(path string, body interface{}) (interface{}, error) {
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", apiURL()+path, strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", userID())
	return doRequest(req)
}

func apiPut(path string, body interface{}) (interface{}, error) {
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("PUT", apiURL()+path, strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", userID())
	return doRequest(req)
}

func apiDelete(path string, body interface{}) (interface{}, error) {
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("DELETE", apiURL()+path, strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", userID())
	return doRequest(req)
}

// apiWriteLocal — 이슈 쓰기(create/update/delete/link) 전용. X-Local-Write 헤더를 붙여
// 서버가 worktree 에 손대지 않고 YAML 본문을 응답(localWrites)으로 돌려주게 한다.
// 응답의 localWrites 는 호출부에서 applyLocalWrites 로 로컬 clone 에 반영한다.
func apiWriteLocal(method, path string, body interface{}) (interface{}, error) {
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(method, apiURL()+path, strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", userID())
	req.Header.Set("X-Local-Write", "true")
	return doRequest(req)
}

// repoRoot — 로컬 clone 의 루트. GOJIRA_REPO_ROOT 우선, 없으면 git toplevel.
func repoRoot() (string, error) {
	if v := os.Getenv("GOJIRA_REPO_ROOT"); v != "" {
		return v, nil
	}
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("repo root 미확인: GOJIRA_REPO_ROOT 설정 또는 clone 안에서 실행 (%v)", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// findIssueYamlByKey — repoRoot()/data/issues 안의 *.yaml(.yml) 파일들을 이름순으로 스캔해
// `key: <KEY>` 라인이 있는 파일을 찾는다(트랙 E Phase 2: PUT/DELETE/link 시 baseYaml 동봉용).
// yaml 라이브러리 의존 없이 라인 정규식 매칭만으로 충분 — 이슈 YAML 은 항상 최상위에
// `key: XXX-1` 형태 라인을 갖는다.
func findIssueYamlByKey(key string) (string, string, error) {
	root, err := repoRoot()
	if err != nil {
		return "", "", err
	}
	issuesDir := filepath.Join(root, "data", "issues")
	entries, err := os.ReadDir(issuesDir)
	if err != nil {
		return "", "", fmt.Errorf("로컬 clone에서 이슈 YAML을 찾지 못함: %s — 과제 clone 안에서 실행했는지 확인 (%v)", key, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		lower := strings.ToLower(e.Name())
		if strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	pattern := fmt.Sprintf(`^key:\s*['"]?%s['"]?\s*$`, regexp.QuoteMeta(key))
	re := regexp.MustCompile(pattern)
	for _, name := range names {
		full := filepath.Join(issuesDir, name)
		data, err := os.ReadFile(full)
		if err != nil {
			continue
		}
		content := string(data)
		for _, line := range strings.Split(content, "\n") {
			if re.MatchString(strings.TrimRight(line, "\r")) {
				rel := filepath.Join("data", "issues", name)
				return rel, content, nil
			}
		}
	}
	return "", "", fmt.Errorf("로컬 clone에서 이슈 YAML을 찾지 못함: %s — 과제 clone 안에서 실행했는지 확인", key)
}

// applyLocalWrites — 서버 응답의 localWrites 배열을 로컬 clone 에 반영한다.
// content 있으면 기록, delete=true 면 삭제. 경로는 repo root 기준 상대경로.
func applyLocalWrites(result interface{}) error {
	m, ok := result.(map[string]interface{})
	if !ok {
		return nil
	}
	raw, ok := m["localWrites"].([]interface{})
	if !ok || len(raw) == 0 {
		return nil
	}
	root, err := repoRoot()
	if err != nil {
		return err
	}
	for _, item := range raw {
		w, _ := item.(map[string]interface{})
		rel, _ := w["path"].(string)
		if rel == "" {
			continue
		}
		full := filepath.Join(root, filepath.FromSlash(rel))
		if del, _ := w["delete"].(bool); del {
			if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("local delete %s: %w", rel, err)
			}
			continue
		}
		content, _ := w["content"].(string)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return fmt.Errorf("local mkdir %s: %w", rel, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			return fmt.Errorf("local write %s: %w", rel, err)
		}
	}
	return nil
}

func platformPost(path string, body interface{}) (interface{}, error) {
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", platformURL()+path, strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", userID())
	return doRequest(req)
}

func doRequest(req *http.Request) (interface{}, error) {
	// SSO 모드(Phase 4 C): 런처가 주입한 사용자 세션토큰을 Authorization: Bearer 로 전송 →
	// 백엔드가 introspect 검증해 신원 추출. dev 모드(토큰 없음)면 미설정 → 기존 X-User-Id 폴백.
	if t := os.Getenv("GOJIRA_SESSION_TOKEN"); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(data))
	}
	var result interface{}
	json.Unmarshal(data, &result)
	return result, nil
}

func outputJSON(v interface{}) {
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
}

func parseFlag(args []string, name string) string {
	for i, a := range args {
		if a == "--"+name && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(a, "--"+name+"=") {
			return a[len("--"+name+"="):]
		}
	}
	return ""
}

func hasFlag(args []string, name string) bool {
	for _, a := range args {
		if a == "--"+name {
			return true
		}
	}
	return false
}

func fatal(msg string) {
	fmt.Fprintln(os.Stderr, "error:", msg)
	os.Exit(1)
}
