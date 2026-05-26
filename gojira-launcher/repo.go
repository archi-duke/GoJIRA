package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
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

func (s *Server) cloneURL(repoName string) string {
	base := strings.TrimRight(s.cfg.Git.Base, "/")
	org := strings.Trim(s.cfg.Git.Org, "/")
	return fmt.Sprintf("%s/%s/%s.git", base, org, repoName)
}

// projCode — 프로젝트 문서에서 code 추출 (jiraCodes.main.jiraCode 우선, 없으면 top-level code).
func projCode(m map[string]any) string {
	if jc, ok := m["jiraCodes"].(map[string]any); ok {
		if main, ok := jc["main"].(map[string]any); ok {
			if c, ok := main["jiraCode"].(string); ok {
				return c
			}
		}
	}
	if c, ok := m["code"].(string); ok {
		return c
	}
	return ""
}

func repoNameOf(m map[string]any) string {
	if docs, ok := m["documents"].(map[string]any); ok {
		if rn, ok := docs["repoName"].(string); ok {
			return rn
		}
	}
	return ""
}

// resolveRepoName — 프로젝트 code 를 실제 Gitea repo 이름으로 해석한다.
// code 와 repo 이름이 다를 수 있으므로(예: NBBALL → number-baseball) GoJIRA-API 의
// 프로젝트 documents.repoName 을 조회한다. 백엔드 미설정/조회 실패 시 code 그대로(하위호환).
func (s *Server) resolveRepoName(code, user string) string {
	base := strings.TrimRight(s.cfg.Backend.GojiraAPI, "/")
	if base == "" {
		return code
	}
	req, _ := http.NewRequest(http.MethodGet, base+"/Project", nil)
	req.Header.Set("XBody", fmt.Sprintf(`{"jiraCode":%q}`, code))
	req.Header.Set("X-User-Id", user)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		log.Printf("resolveRepoName warn (%s): %v — code 사용", code, err)
		return code
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		log.Printf("resolveRepoName warn (%s): HTTP %d — code 사용", code, resp.StatusCode)
		return code
	}
	body, _ := io.ReadAll(resp.Body)

	// 응답은 배열(필터 안 될 수 있음) 또는 단일 객체. code 일치 항목의 repoName 사용.
	var arr []map[string]any
	if json.Unmarshal(body, &arr) == nil && len(arr) > 0 {
		for _, m := range arr {
			if projCode(m) == code {
				if rn := repoNameOf(m); rn != "" {
					return rn
				}
			}
		}
		// 단일 결과면 code 검증 없이도 사용
		if len(arr) == 1 {
			if rn := repoNameOf(arr[0]); rn != "" {
				return rn
			}
		}
	}
	var obj map[string]any
	if json.Unmarshal(body, &obj) == nil {
		if rn := repoNameOf(obj); rn != "" {
			return rn
		}
	}
	log.Printf("resolveRepoName: %s 의 repoName 미발견 — code 사용", code)
	return code
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
		// code ≠ Gitea repo 이름일 수 있으므로 repoName 으로 clone (로컬 디렉터리명은 code).
		repoName := s.resolveRepoName(code, user)
		if _, err := gitCmd(s.cfg.RepoRoot, "clone", s.cloneURL(repoName), code); err != nil {
			return "", err
		}
	} else {
		gitCmd(path, "fetch", "origin")
	}

	if err := s.checkoutUserBranch(path, branch); err != nil {
		return "", err
	}
	s.ensureSkillsSymlink(path)
	s.ensureExcluded(path, ".claude") // /sync 의 git add -A 가 .claude 심볼릭링크를 staging 하지 않도록 local-only ignore.
	s.ensureGitIdentity(path, user)
	return path, nil
}

// ensureExcluded — .git/info/exclude (local-only gitignore) 에 entry 가 없으면 추가.
// .gitignore 와 달리 트래킹되지 않아 사용자/프로젝트 마다 다른 로컬 산물(예: 스킬 심볼릭링크)에 적합.
func (s *Server) ensureExcluded(path, entry string) {
	excludePath := filepath.Join(path, ".git", "info", "exclude")
	data, err := os.ReadFile(excludePath)
	if err != nil && !os.IsNotExist(err) {
		log.Printf("ensureExcluded read 실패 (%s): %v", excludePath, err)
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == entry {
			return
		}
	}
	_ = os.MkdirAll(filepath.Dir(excludePath), 0o755)
	content := string(data)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += entry + "\n"
	if err := os.WriteFile(excludePath, []byte(content), 0o644); err != nil {
		log.Printf("ensureExcluded write 실패 (%s): %v", excludePath, err)
	}
}

// ensureGitIdentity — clone 의 user.name/user.email 이 비어있으면 주입한다 (commit author 식별).
// 우선순위: launcher.json git.authorName/Email → 요청 user(knoxId) 기반 fallback.
// 인증과는 무관 (인증은 clone URL embedded PAT). 이미 설정돼 있으면 손대지 않는다.
func (s *Server) ensureGitIdentity(path, user string) {
	name := s.cfg.Git.AuthorName
	if name == "" {
		name = user
	}
	email := s.cfg.Git.AuthorEmail
	if email == "" {
		email = user + "@gojira.local"
	}
	if cur, _ := gitCmd(path, "config", "--get", "user.name"); cur == "" {
		if _, err := gitCmd(path, "config", "user.name", name); err != nil {
			log.Printf("ensureGitIdentity user.name 실패 (%s): %v", path, err)
		}
	}
	if cur, _ := gitCmd(path, "config", "--get", "user.email"); cur == "" {
		if _, err := gitCmd(path, "config", "user.email", email); err != nil {
			log.Printf("ensureGitIdentity user.email 실패 (%s): %v", path, err)
		}
	}
}

// ensureSkillsSymlink — skillsSourceDir 가 설정돼 있고 그 안에 .claude 가 있으면,
// target/.claude 가 마스터 레포의 .claude 를 참조하도록 만든다 (이미 있으면 노옵).
// 실패는 비치명적 — 로그만 남긴다. 목적: 런처가 spawn 하는 opencode/claude 가
// 마스터 레포의 GoJIRA 스킬을 그대로 호출하도록.
//
// OS별 전략 (사용자가 추가한 .claude 가 있으면 어느 경우든 손대지 않음):
//   - Linux/macOS: os.Symlink (권한 불요)
//   - Windows: 1) symlink (Dev Mode 필요), 2) mklink /J 디렉토리 junction (권한 불요/동일 볼륨),
//              3) .claude 트리 복사 (cross-volume 마지막 수단 — 스킬 수정 시 stale)
func (s *Server) ensureSkillsSymlink(target string) {
	src := s.cfg.SkillsSourceDir
	if src == "" {
		return
	}
	srcClaude := filepath.Join(src, ".claude")
	if _, err := os.Stat(srcClaude); err != nil {
		return
	}
	dst := filepath.Join(target, ".claude")
	if _, err := os.Lstat(dst); err == nil {
		return // 이미 있음 — 사용자 커밋이든 이전 symlink 든 건드리지 않는다.
	}

	if runtime.GOOS != "windows" {
		if err := os.Symlink(srcClaude, dst); err != nil {
			log.Printf("ensureSkillsSymlink symlink 실패 (%s → %s): %v", srcClaude, dst, err)
		}
		return
	}

	// Windows: 단계적 폴백.
	if err := os.Symlink(srcClaude, dst); err == nil {
		return
	}
	if err := exec.Command("cmd", "/c", "mklink", "/J", dst, srcClaude).Run(); err == nil {
		return
	}
	if err := copyTree(srcClaude, dst); err != nil {
		log.Printf("ensureSkillsSymlink Windows fallback (copy) 실패 (%s → %s): %v — Dev Mode 활성화를 권장합니다", srcClaude, dst, err)
	}
}

// copyTree — src 트리를 dst 로 재귀 복사 (Windows symlink/junction 폴백 전용).
// 심볼릭 링크 자체는 따라가지 않고 일반 파일로 복사 — .claude/skills 내부 일반 파일/디렉토리만 다룬다.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}

// checkoutUserBranch — origin/<branch> 가 있으면 추적 체크아웃, 없으면 origin/main 기반 신규 생성.
// 추가로: docs/{user} 가 origin/main 의 후행이고 미커밋 변경이 없으면 silently FF merge.
// (신규 프로젝트에서 docs/{user} 가 빈 채 만들어진 케이스 + 그 외 단순 후행 케이스 자동 복구.)
func (s *Server) checkoutUserBranch(path, branch string) error {
	// 이미 해당 브랜치면 끝.
	if cur, err := gitCmd(path, "rev-parse", "--abbrev-ref", "HEAD"); err == nil && cur == branch {
		gitCmd(path, "pull", "--ff-only")
		s.tryFFFromMain(path)
		return nil
	}
	// 원격에 존재하면 추적 체크아웃.
	if _, err := gitCmd(path, "rev-parse", "--verify", "refs/remotes/origin/"+branch); err == nil {
		if _, err := gitCmd(path, "checkout", branch); err != nil {
			return err
		}
		gitCmd(path, "pull", "--ff-only")
		s.tryFFFromMain(path)
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

// tryFFFromMain — docs/{user} 가 origin/main 의 후행이면 silently FF merge.
// 미커밋 dirty 거나, diverged, 또는 ahead 면 그대로 둔다 (사용자가 Pull 버튼으로 명시적 해결).
func (s *Server) tryFFFromMain(path string) {
	if _, err := gitCmd(path, "rev-parse", "--verify", "refs/remotes/origin/main"); err != nil {
		return // main 없음
	}
	if porcelain, err := gitCmd(path, "status", "--porcelain"); err == nil && porcelain != "" {
		return // dirty
	}
	if _, err := gitCmd(path, "merge", "--ff-only", "origin/main"); err != nil {
		// FF 불가 (diverged or ahead) — 정상 케이스. 로그만 남김.
		log.Printf("tryFFFromMain skipped (FF 불가) at %s: %v", path, err)
	}
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

// PullResult — /pull 응답용.
type PullResult struct {
	Status     string `json:"status"`              // "ok" | "noop"
	Branch     string `json:"branch"`              // 사용자 브랜치 (docs/<user>)
	Strategy   string `json:"strategy"`            // "ff" | "rebase"
	Merged     int    `json:"merged"`              // FF 로 끌어온 main 커밋 수
	Ahead      int    `json:"ahead"`               // pull 후 docs/<user> 가 main 보다 ahead 인 수
	LastCommit string `json:"lastCommit,omitempty"`
	Message    string `json:"message,omitempty"`
}

// Pull — docs/<user> 에 origin/main 을 끌어온다.
//   - 미커밋 변경(dirty) 이면 거부 — 사용자가 먼저 등록/스태시해야 안전.
//   - FF 가능: merge --ff-only (linear, 깔끔).
//   - 비-FF (사용자 커밋이 main 위에 있음): pull --rebase origin main 으로 사용자 커밋을 main 위로 옮김.
//   - 충돌 시: 명확한 에러 + 호스트 git 으로 수동 해결 안내.
func (s *Server) Pull(code, user string) (*PullResult, error) {
	path, err := s.EnsureRepo(code, user)
	if err != nil {
		return nil, err
	}
	mu := getRepoLock(code)
	mu.Lock()
	defer mu.Unlock()

	branch := "docs/" + user
	res := &PullResult{Status: "ok", Branch: branch}

	if porcelain, err := gitCmd(path, "status", "--porcelain"); err == nil && porcelain != "" {
		return nil, fmt.Errorf("미커밋 변경이 있어 Pull 거부 — 먼저 '등록'(commit+push) 또는 stash 후 다시 시도하세요")
	}

	if _, err := gitCmd(path, "fetch", "origin"); err != nil {
		return nil, fmt.Errorf("fetch 실패: %w", err)
	}
	if _, err := gitCmd(path, "rev-parse", "--verify", "refs/remotes/origin/main"); err != nil {
		return nil, fmt.Errorf("origin/main 이 없습니다 (clone 직후 상태?) — Gitea 의 main 브랜치를 먼저 확인하세요")
	}

	// origin/main 대비 ahead/behind 측정 (rev-list --left-right --count)
	// 결과: "<behind>\t<ahead>" — main..HEAD ahead, HEAD..main behind.
	behindMain, aheadMain := 0, 0
	if counts, err := gitCmd(path, "rev-list", "--left-right", "--count", "origin/main...HEAD"); err == nil {
		if f := strings.Fields(counts); len(f) == 2 {
			behindMain, _ = strconv.Atoi(f[0])
			aheadMain, _ = strconv.Atoi(f[1])
		}
	}

	if behindMain == 0 {
		res.Status = "noop"
		res.Strategy = "ff"
		res.Ahead = aheadMain
		res.Message = "이미 main 의 최신 내용을 포함합니다"
		if c, err := gitCmd(path, "log", "-1", "--format=%h %s"); err == nil {
			res.LastCommit = c
		}
		return res, nil
	}

	if aheadMain == 0 {
		// FF 가능 — main 의 새 커밋만 끌어온다.
		if _, err := gitCmd(path, "merge", "--ff-only", "origin/main"); err != nil {
			return nil, fmt.Errorf("FF merge 실패: %w", err)
		}
		res.Strategy = "ff"
		res.Merged = behindMain
	} else {
		// 비-FF: 사용자 커밋을 main 위로 옮긴다.
		if _, err := gitCmd(path, "pull", "--rebase", "origin", "main"); err != nil {
			// 충돌 시 rebase 중단 후 에러 (이미 rebase 진행 중 상태일 수 있어 abort 시도).
			gitCmd(path, "rebase", "--abort")
			return nil, fmt.Errorf("rebase 충돌 — 호스트 git 에서 수동 해결 필요 (cd %s && git rebase origin/main): %w", path, err)
		}
		res.Strategy = "rebase"
		res.Merged = behindMain
	}

	if counts, err := gitCmd(path, "rev-list", "--count", "origin/main..HEAD"); err == nil {
		res.Ahead, _ = strconv.Atoi(strings.TrimSpace(counts))
	}
	if c, err := gitCmd(path, "log", "-1", "--format=%h %s"); err == nil {
		res.LastCommit = c
	}
	res.Message = fmt.Sprintf("main 의 %d 커밋을 %s 로 적용", res.Merged, res.Strategy)
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
