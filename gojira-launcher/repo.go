package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// repoLocks 레포별 mutex (동시 git 작업 직렬화) — Platform-API/github 의 패턴을 단순화.
var (
	repoLocks   = map[string]*sync.Mutex{}
	repoLocksMu sync.Mutex
)

func getRepoLock(key string) *sync.Mutex {
	repoLocksMu.Lock()
	defer repoLocksMu.Unlock()
	if repoLocks[key] == nil {
		repoLocks[key] = &sync.Mutex{}
	}
	return repoLocks[key]
}

// safeName — code/user 는 경로·URL·브랜치명에 그대로 들어가므로 엄격히 검증
// (path traversal / 명령 인젝션 차단). 영숫자 + . _ - 만 허용, 점 시작 금지.
var safeName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func validateName(kind, v string) error {
	if v == "" {
		return fmt.Errorf("%s 누락", kind)
	}
	if !safeName.MatchString(v) || strings.Contains(v, "..") {
		return fmt.Errorf("%s 형식이 올바르지 않습니다: %q", kind, v)
	}
	return nil
}

// gitCmd 는 git 을 실행한다. 자격 프롬프트는 비활성(GIT_TERMINAL_PROMPT=0).
func gitCmd(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("git %s: %s (%w)", strings.Join(args, " "), strings.TrimSpace(string(out)), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// repoPath — 로컬 clone 경로 (repoRoot/<code>).
func (s *Server) repoPath(code string) string {
	return filepath.Join(s.cfg.RepoRoot, code)
}

func (s *Server) cloneURL(code string) string {
	base := strings.TrimRight(s.cfg.Git.Base, "/")
	org := strings.Trim(s.cfg.Git.Org, "/")
	return fmt.Sprintf("%s/%s/%s.git", base, org, code)
}

// EnsureRepo 는 로컬 clone 을 보장하고 docs/<user> 브랜치를 체크아웃한다.
//   - 서버측 bare+worktree 모델과 달리 사용자 PC 의 일반 작업본 1개 (설계 §4: 브랜치 내부 충돌 0).
//   - 이미 있으면 fetch 후 브랜치 정렬만 수행.
func (s *Server) EnsureRepo(code, user string) (string, error) {
	if err := validateName("code", code); err != nil {
		return "", err
	}
	if err := validateName("user", user); err != nil {
		return "", err
	}
	if s.cfg.Git.Base == "" {
		return "", fmt.Errorf("git.base 미설정 — launcher.json 에 clone 대상 Gitea 주소를 지정하세요")
	}

	path := s.repoPath(code)
	mu := getRepoLock(code)
	mu.Lock()
	defer mu.Unlock()

	branch := "docs/" + user

	if _, err := os.Stat(filepath.Join(path, ".git")); os.IsNotExist(err) {
		if err := os.MkdirAll(s.cfg.RepoRoot, 0o755); err != nil {
			return "", fmt.Errorf("repoRoot 생성 실패: %w", err)
		}
		if _, err := gitCmd(s.cfg.RepoRoot, "clone", s.cloneURL(code), code); err != nil {
			return "", err
		}
	} else {
		gitCmd(path, "fetch", "origin")
	}

	if err := s.checkoutUserBranch(path, branch); err != nil {
		return "", err
	}
	return path, nil
}

// checkoutUserBranch — origin/<branch> 가 있으면 추적 체크아웃, 없으면 origin/main 기반 신규 생성.
func (s *Server) checkoutUserBranch(path, branch string) error {
	// 이미 해당 브랜치면 끝.
	if cur, err := gitCmd(path, "rev-parse", "--abbrev-ref", "HEAD"); err == nil && cur == branch {
		gitCmd(path, "pull", "--ff-only")
		return nil
	}
	// 원격에 존재하면 추적 체크아웃.
	if _, err := gitCmd(path, "rev-parse", "--verify", "refs/remotes/origin/"+branch); err == nil {
		if _, err := gitCmd(path, "checkout", branch); err != nil {
			return err
		}
		gitCmd(path, "pull", "--ff-only")
		return nil
	}
	// 없으면 origin/main 기반 새 브랜치.
	base := "origin/main"
	if _, err := gitCmd(path, "rev-parse", "--verify", "refs/remotes/origin/main"); err != nil {
		base = "" // main 도 없으면 현재 HEAD 기반
	}
	args := []string{"checkout", "-b", branch}
	if base != "" {
		args = append(args, base)
	}
	_, err := gitCmd(path, args...)
	return err
}

// PushResult — /push 응답용.
type PushResult struct {
	Status     string `json:"status"` // "ok"
	Branch     string `json:"branch"`
	Pushed     bool   `json:"pushed"`  // 실제로 push 한 커밋이 있었는지
	Commits    int    `json:"commits"` // push 전 upstream 대비 ahead 수
	Dirty      bool   `json:"dirty"`   // 미커밋 변경 잔존 여부(경고)
	LastCommit string `json:"lastCommit,omitempty"`
	Message    string `json:"message,omitempty"`
}

// Push — 로컬 clone 의 docs/<user> 브랜치를 origin 으로 push 한다.
//
// 로컬 커밋 자체는 사용자/CLI 의 몫(설계 §1: "저장 → 로컬 git commit → push").
// 여기서는 이미 커밋된 것을 올릴 뿐이며, 미커밋(dirty) 변경은 경고만 한다.
// 인증은 git 자격증명 헬퍼/clone URL 에 위임(PoC — §6 잔여).
func (s *Server) Push(code, user string) (*PushResult, error) {
	path, err := s.EnsureRepo(code, user)
	if err != nil {
		return nil, err
	}
	mu := getRepoLock(code)
	mu.Lock()
	defer mu.Unlock()

	branch := "docs/" + user
	res := &PushResult{Status: "ok", Branch: branch}
	if porcelain, err := gitCmd(path, "status", "--porcelain"); err == nil && porcelain != "" {
		res.Dirty = true
	}

	// push 전 원격 변경 반영(rebase). 이슈(data/issues)·문서(business/architecture)는 경로가
	// 분리돼 같은 파일을 건드리지 않으므로 rebase 는 항상 무충돌(설계 §경로 분리).
	// dirty 면 rebase 가 실패하므로 건너뛰고 push 단계에서 경고만 한다(커밋은 사용자 몫).
	if !res.Dirty {
		if _, err := gitCmd(path, "rev-parse", "--verify", "refs/remotes/origin/"+branch); err == nil {
			if _, err := gitCmd(path, "pull", "--rebase", "origin", branch); err != nil {
				return nil, fmt.Errorf("pull --rebase 실패(수동 해결 필요): %w", err)
			}
		}
	}

	if c, err := gitCmd(path, "log", "-1", "--format=%h %s"); err == nil {
		res.LastCommit = c
	}

	// upstream 대비 ahead 수 — 새 커밋이 없으면 push 생략.
	if counts, err := gitCmd(path, "rev-list", "--count", "@{upstream}..HEAD"); err == nil {
		res.Commits, _ = strconv.Atoi(strings.TrimSpace(counts))
	} else {
		// upstream 미설정(원격에 브랜치 없음) → 새 브랜치 push 필요. 커밋 수는 미상.
		res.Commits = -1
	}

	if res.Commits == 0 {
		res.Pushed = false
		res.Message = "push 할 새 커밋이 없습니다"
		if res.Dirty {
			res.Message += " (미커밋 변경 있음 — 먼저 commit 하세요)"
		}
		return res, nil
	}

	// 신규 브랜치면 upstream 설정, 아니면 일반 push.
	if _, err := gitCmd(path, "push", "-u", "origin", branch); err != nil {
		return nil, err
	}
	res.Pushed = true
	if res.Dirty {
		res.Message = "push 완료 (단, 미커밋 변경이 남아있습니다)"
	}
	return res, nil
}

// RepoStatus — /status 응답용 (브랜치/ahead·behind/dirty/마지막 커밋).
type RepoStatus struct {
	State      string `json:"state"` // "clean" | "dirty" | "missing"
	Branch     string `json:"branch,omitempty"`
	Ahead      int    `json:"ahead"`
	Behind     int    `json:"behind"`
	Dirty      bool   `json:"dirty"`
	LastCommit string `json:"lastCommit,omitempty"`
}

func (s *Server) Status(code string) (*RepoStatus, error) {
	if err := validateName("code", code); err != nil {
		return nil, err
	}
	path := s.repoPath(code)
	if _, err := os.Stat(filepath.Join(path, ".git")); os.IsNotExist(err) {
		return &RepoStatus{State: "missing"}, nil
	}

	st := &RepoStatus{State: "clean"}
	if br, err := gitCmd(path, "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		st.Branch = br
	}
	if porcelain, err := gitCmd(path, "status", "--porcelain"); err == nil && porcelain != "" {
		st.Dirty = true
		st.State = "dirty"
	}
	// ahead/behind vs upstream (있을 때만).
	if counts, err := gitCmd(path, "rev-list", "--left-right", "--count", "@{upstream}...HEAD"); err == nil {
		if f := strings.Fields(counts); len(f) == 2 {
			st.Behind, _ = strconv.Atoi(f[0])
			st.Ahead, _ = strconv.Atoi(f[1])
		}
	}
	if c, err := gitCmd(path, "log", "-1", "--format=%h %s"); err == nil {
		st.LastCommit = c
	}
	return st, nil
}
