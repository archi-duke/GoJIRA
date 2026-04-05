package github

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"platform-api/config"
)

// repoDir bare repository 경로
func repoDir(owner, repo string) string {
	return filepath.Join(config.RepoBaseDir, owner, repo+".git")
}

// worktreeDir 사용자별 worktree 경로 (bare repo 아래)
func worktreeDir(owner, repo, userID string) string {
	return filepath.Join(repoDir(owner, repo), userID)
}

// repoLocks 레포별 mutex (동시 git 작업 방지)
var (
	repoLocks   = map[string]*sync.Mutex{}
	repoLocksMu sync.Mutex
)

func getRepoLock(owner, repo string) *sync.Mutex {
	key := owner + "/" + repo
	repoLocksMu.Lock()
	defer repoLocksMu.Unlock()
	if repoLocks[key] == nil {
		repoLocks[key] = &sync.Mutex{}
	}
	return repoLocks[key]
}

// git 명령 실행
func gitCmd(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("git %s: %s (%v)", strings.Join(args, " "), string(out), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// cloneURL 인증 정보가 포함된 clone URL 생성
func cloneURL(owner, repo string) string {
	base := strings.TrimRight(config.GitHubURL, "/")
	if config.GitHubToken != "" {
		base = strings.Replace(base, "https://", fmt.Sprintf("https://%s@", config.GitHubToken), 1)
		base = strings.Replace(base, "http://", fmt.Sprintf("http://%s@", config.GitHubToken), 1)
	}
	return fmt.Sprintf("%s/%s/%s.git", base, owner, repo)
}

// EnsureClone bare repository가 없으면 clone, 있으면 fetch
func EnsureClone(owner, repo string) (string, error) {
	mu := getRepoLock(owner, repo)
	mu.Lock()
	defer mu.Unlock()

	dir := repoDir(owner, repo)

	if _, err := os.Stat(filepath.Join(dir, "HEAD")); os.IsNotExist(err) {
		os.MkdirAll(filepath.Dir(dir), 0755)
		url := cloneURL(owner, repo)
		log.Printf("git clone --bare %s/%s → %s", owner, repo, dir)
		if _, err := gitCmd(".", "clone", "--bare", url, dir); err != nil {
			return "", err
		}
		// bare clone은 fetch refspec이 없으므로 설정
		gitCmd(dir, "config", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*")
		gitCmd(dir, "fetch", "origin")
	} else {
		gitCmd(dir, "fetch", "origin")
	}

	return dir, nil
}

// EnsureWorktree 사용자별 worktree 생성/반환
// bare repo에서 사용자 브랜치로 worktree를 만든다.
func EnsureWorktree(owner, repo, userID string) (string, string, error) {
	// 먼저 bare repo 확보
	bareDir, err := EnsureClone(owner, repo)
	if err != nil {
		return "", "", err
	}

	branch := fmt.Sprintf("docs/%s", userID)
	wtDir := worktreeDir(owner, repo, userID)

	mu := getRepoLock(owner, repo)
	mu.Lock()
	defer mu.Unlock()

	// worktree가 이미 있으면 fetch만 하고 반환
	if _, err := os.Stat(filepath.Join(wtDir, ".git")); err == nil {
		gitCmd(wtDir, "fetch", "origin")
		TouchWorkspace(owner, repo, userID)
		return wtDir, branch, nil
	}

	// bare repo에서 fetch
	gitCmd(bareDir, "fetch", "origin")

	// 브랜치 존재 여부 확인
	os.MkdirAll(filepath.Dir(wtDir), 0755)
	_, branchErr := gitCmd(bareDir, "rev-parse", "--verify", branch)
	if branchErr == nil {
		// 브랜치가 이미 존재 → 기존 브랜치로 worktree 생성
		if _, err := gitCmd(bareDir, "worktree", "add", wtDir, branch); err != nil {
			gitCmd(bareDir, "worktree", "remove", wtDir, "--force")
			if _, err := gitCmd(bareDir, "worktree", "add", wtDir, branch); err != nil {
				return "", "", fmt.Errorf("worktree 생성 실패: %v", err)
			}
		}
	} else {
		// 새 브랜치 생성 (origin/main 기준)
		if _, err := gitCmd(bareDir, "worktree", "add", wtDir, "-b", branch, "origin/main"); err != nil {
			gitCmd(bareDir, "worktree", "remove", wtDir, "--force")
			if _, err := gitCmd(bareDir, "worktree", "add", wtDir, "-b", branch, "origin/main"); err != nil {
				return "", "", fmt.Errorf("worktree 생성 실패: %v", err)
			}
		}
	}

	// worktree에 git config 설정
	gitCmd(wtDir, "config", "user.name", userID)
	gitCmd(wtDir, "config", "user.email", userID+"@gojira.local")

	// 리모트에 이미 해당 브랜치가 있으면 pull
	if _, err := gitCmd(wtDir, "pull", "origin", branch); err != nil {
		// 리모트에 없으면 무시 (새 브랜치)
		log.Printf("worktree pull skip (new branch): %s", branch)
	}

	log.Printf("worktree created: %s → %s (branch: %s)", owner+"/"+repo, wtDir, branch)
	TouchWorkspace(owner, repo, userID)
	return wtDir, branch, nil
}

// RebaseResult rebase 결과
type RebaseResult struct {
	Success  bool   `json:"success"`
	Conflict bool   `json:"conflict"`
	Diff     string `json:"diff,omitempty"` // 충돌 시 사용자 변경 diff
	Message  string `json:"message"`
}

// RebaseOnMain 사용자 브랜치를 origin/main 위로 rebase
// 충돌 시: abort → diff 보존 → reset
func RebaseOnMain(owner, repo, userID string) *RebaseResult {
	branch := fmt.Sprintf("docs/%s", userID)
	wtDir := worktreeDir(owner, repo, userID)

	// worktree가 없으면 먼저 생성
	if _, err := os.Stat(filepath.Join(wtDir, ".git")); os.IsNotExist(err) {
		var err2 error
		wtDir, _, err2 = EnsureWorktree(owner, repo, userID)
		if err2 != nil {
			return &RebaseResult{Success: false, Message: "worktree 생성 실패: " + err2.Error()}
		}
	}

	mu := getRepoLock(owner, repo)
	mu.Lock()
	defer mu.Unlock()

	bareDir := repoDir(owner, repo)
	gitCmd(bareDir, "fetch", "origin")
	gitCmd(wtDir, "fetch", "origin")

	// rebase 시도
	_, err := gitCmd(wtDir, "rebase", "origin/main")
	if err == nil {
		return &RebaseResult{
			Success: true,
			Message: fmt.Sprintf("브랜치 %s가 main과 동기화되었습니다.", branch),
		}
	}

	// 충돌 발생 — abort 후 diff 보존
	log.Printf("rebase 충돌 발생: %s/%s user=%s, abort 진행", owner, repo, userID)
	gitCmd(wtDir, "rebase", "--abort")

	// 사용자 변경 내용을 diff로 추출
	diff, _ := gitCmd(wtDir, "diff", "origin/main..HEAD")

	// main 기준으로 reset
	gitCmd(wtDir, "reset", "--hard", "origin/main")

	return &RebaseResult{
		Success:  false,
		Conflict: true,
		Diff:     diff,
		Message:  "충돌이 발생하여 main 기준으로 초기화되었습니다. 변경 내용을 참고하여 재작업해 주세요.",
	}
}

// CheckoutNewBranch 사용자 worktree에서 main 기준으로 브랜치 리셋 (하위호환)
func CheckoutNewBranch(owner, repo, branch string) error {
	userID := branch
	if strings.HasPrefix(branch, "docs/") {
		userID = strings.TrimPrefix(branch, "docs/")
	}
	result := RebaseOnMain(owner, repo, userID)
	if !result.Success && !result.Conflict {
		return fmt.Errorf(result.Message)
	}
	return nil
}

// WriteLocalFile 사용 금지 (bare repo이므로 불가)
// WriteWorktreeFile을 사용하세요.
func WriteLocalFile(owner, repo, filePath, content string) error {
	return fmt.Errorf("WriteLocalFile은 bare repo에서 사용 불가. WriteWorktreeFile을 사용하세요.")
}

// WriteWorktreeFile 사용자 worktree에 파일 쓰기
func WriteWorktreeFile(owner, repo, userID, filePath, content string) error {
	dir := worktreeDir(owner, repo, userID)
	fullPath := filepath.Join(dir, filePath)
	os.MkdirAll(filepath.Dir(fullPath), 0755)
	TouchWorkspace(owner, repo, userID)
	return os.WriteFile(fullPath, []byte(content), 0644)
}

// ReadWorktreeFile 사용자 worktree에서 파일 읽기
func ReadWorktreeFile(owner, repo, userID, filePath string) (string, error) {
	dir := worktreeDir(owner, repo, userID)
	fullPath := filepath.Join(dir, filePath)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return "", err
	}
	TouchWorkspace(owner, repo, userID)
	return string(data), nil
}

// CommitAndPush worktree에서 commit → push
func CommitAndPush(owner, repo, branch, message string) error {
	userID := branch
	if strings.HasPrefix(branch, "docs/") {
		userID = strings.TrimPrefix(branch, "docs/")
	}

	wtDir := worktreeDir(owner, repo, userID)
	// worktree가 없으면 메인 클론에서 (하위호환)
	if _, err := os.Stat(filepath.Join(wtDir, ".git")); os.IsNotExist(err) {
		wtDir = repoDir(owner, repo)
	}

	mu := getRepoLock(owner, repo)
	mu.Lock()
	defer mu.Unlock()

	if _, err := gitCmd(wtDir, "add", "."); err != nil {
		return err
	}
	status, _ := gitCmd(wtDir, "status", "--porcelain")
	if status == "" {
		log.Printf("git: no changes to commit for %s/%s", owner, repo)
		return nil
	}
	if _, err := gitCmd(wtDir, "commit", "-m", message); err != nil {
		return err
	}
	if _, err := gitCmd(wtDir, "push", "--force-with-lease", "origin", branch); err != nil {
		return err
	}
	return nil
}

// ListWorktrees 사용자별 worktree 목록 반환
func ListWorktrees(owner, repo string) []string {
	wtBase := repoDir(owner, repo)
	entries, err := os.ReadDir(wtBase)
	if err != nil {
		return nil
	}
	var users []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") && e.Name() != "objects" && e.Name() != "refs" && e.Name() != "hooks" {
			users = append(users, e.Name())
		}
	}
	return users
}

// HasPendingWork 사용자의 미완료 작업이 있는지 확인
func HasPendingWork(owner, repo, userID string) bool {
	wtDir := worktreeDir(owner, repo, userID)
	if _, err := os.Stat(filepath.Join(wtDir, ".git")); os.IsNotExist(err) {
		return false
	}
	// 커밋되지 않은 변경이 있는지 확인
	status, _ := gitCmd(wtDir, "status", "--porcelain")
	if status != "" {
		return true
	}
	// main과 다른 커밋이 있는지 확인
	diff, _ := gitCmd(wtDir, "log", "origin/main..HEAD", "--oneline")
	return diff != ""
}

// PendingFiles 워크트리에서 미커밋 변경 파일 목록 반환
func PendingFiles(owner, repo, userID string) []string {
	wtDir := worktreeDir(owner, repo, userID)
	if _, err := os.Stat(filepath.Join(wtDir, ".git")); os.IsNotExist(err) {
		return nil
	}
	status, _ := gitCmd(wtDir, "status", "--porcelain")
	if status == "" {
		return nil
	}
	var files []string
	for _, line := range strings.Split(status, "\n") {
		line = strings.TrimSpace(line)
		if len(line) > 3 {
			files = append(files, strings.TrimSpace(line[2:]))
		}
	}
	return files
}

// DiscardWorktree 사용자 worktree 삭제 (작업 폐기)
func DiscardWorktree(owner, repo, userID string) error {
	mu := getRepoLock(owner, repo)
	mu.Lock()
	defer mu.Unlock()

	bareDir := repoDir(owner, repo)
	wtDir := worktreeDir(owner, repo, userID)
	branch := fmt.Sprintf("docs/%s", userID)

	gitCmd(bareDir, "worktree", "remove", wtDir, "--force")
	gitCmd(bareDir, "branch", "-D", branch)
	os.RemoveAll(wtDir)

	log.Printf("worktree discarded: %s/%s user=%s", owner, repo, userID)
	return nil
}

// RemoveWorktreeOnly worktree만 삭제 (브랜치는 유지)
func RemoveWorktreeOnly(owner, repo, userID string) error {
	mu := getRepoLock(owner, repo)
	mu.Lock()
	defer mu.Unlock()

	bareDir := repoDir(owner, repo)
	wtDir := worktreeDir(owner, repo, userID)

	gitCmd(bareDir, "worktree", "remove", wtDir, "--force")
	os.RemoveAll(wtDir)
	removeWorkspaceAccess(owner, repo, userID)

	log.Printf("worktree removed (branch kept): %s/%s user=%s", owner, repo, userID)
	return nil
}

// Pull 최신 코드 pull (bare repo에서는 사용 불가, 호환성 유지용)
func Pull(owner, repo, branch string) error {
	bareDir := repoDir(owner, repo)
	// bare repo에서는 단순히 fetch만 수행
	_, err := gitCmd(bareDir, "fetch", "origin", branch)
	return err
}

// GetLocalRepoDir 로컬 repo 경로 반환
func GetLocalRepoDir(owner, repo string) string {
	return repoDir(owner, repo)
}

// ListLocalRepos 로컬 bare repos 폴더에 있는 {owner}/{repo} 목록 반환
func ListLocalRepos() []struct{ Owner, Repo string } {
	var result []struct{ Owner, Repo string }
	owners, err := os.ReadDir(config.RepoBaseDir)
	if err != nil {
		return result
	}
	for _, ownerEntry := range owners {
		if !ownerEntry.IsDir() || strings.HasPrefix(ownerEntry.Name(), ".") {
			continue
		}
		repos, err := os.ReadDir(filepath.Join(config.RepoBaseDir, ownerEntry.Name()))
		if err != nil {
			continue
		}
		for _, repoEntry := range repos {
			if !repoEntry.IsDir() || strings.HasPrefix(repoEntry.Name(), ".") {
				continue
			}
			// bare repo만 (*.git 파일이 있는 디렉토리)
			if !strings.HasSuffix(repoEntry.Name(), ".git") {
				continue
			}
			// {repo}.git → {repo}로 변환
			repoName := strings.TrimSuffix(repoEntry.Name(), ".git")
			result = append(result, struct{ Owner, Repo string }{ownerEntry.Name(), repoName})
		}
	}
	return result
}

// RemoveLocalRepo bare repo 및 모든 worktree 폴더 삭제
func RemoveLocalRepo(owner, repo string) error {
	dir := repoDir(owner, repo)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil
	}
	log.Printf("removing bare repo and all worktrees: %s", dir)
	return os.RemoveAll(dir)
}
