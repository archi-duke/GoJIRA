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

// repoDir 메인 클론 경로 (main 브랜치 용)
func repoDir(owner, repo string) string {
	return filepath.Join(config.RepoBaseDir, owner, repo)
}

// worktreeDir 사용자별 worktree 경로
func worktreeDir(owner, repo, userID string) string {
	return filepath.Join(config.RepoBaseDir, owner, repo+"-wt", userID)
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

// EnsureClone 메인 클론이 없으면 clone, 있으면 fetch
func EnsureClone(owner, repo string) (string, error) {
	mu := getRepoLock(owner, repo)
	mu.Lock()
	defer mu.Unlock()

	dir := repoDir(owner, repo)

	if _, err := os.Stat(filepath.Join(dir, ".git")); os.IsNotExist(err) {
		os.MkdirAll(filepath.Dir(dir), 0755)
		url := cloneURL(owner, repo)
		log.Printf("git clone %s/%s → %s", owner, repo, dir)
		if _, err := gitCmd(".", "clone", url, dir); err != nil {
			return "", err
		}
		gitCmd(dir, "config", "user.name", "GoJIRA Platform")
		gitCmd(dir, "config", "user.email", "platform@gojira.local")
	} else {
		gitCmd(dir, "fetch", "origin")
	}

	return dir, nil
}

// EnsureWorktree 사용자별 worktree 생성/반환
// 메인 클론에서 사용자 브랜치로 worktree를 만든다.
func EnsureWorktree(owner, repo, userID string) (string, string, error) {
	// 먼저 메인 클론 확보
	mainDir, err := EnsureClone(owner, repo)
	if err != nil {
		return "", "", err
	}

	branch := fmt.Sprintf("docs/%s", userID)
	wtDir := worktreeDir(owner, repo, userID)

	mu := getRepoLock(owner, repo)
	mu.Lock()
	defer mu.Unlock()

	// worktree가 이미 있으면 반환
	if _, err := os.Stat(filepath.Join(wtDir, ".git")); err == nil {
		// fetch하고 최신화
		gitCmd(mainDir, "fetch", "origin")
		return wtDir, branch, nil
	}

	// 메인 클론에서 fetch
	gitCmd(mainDir, "fetch", "origin")

	// 리모트에 브랜치가 있으면 로컬에 등록
	gitCmd(mainDir, "branch", "-D", branch)                        // 기존 로컬 브랜치 정리
	gitCmd(mainDir, "branch", branch, "origin/main")               // main 기준으로 생성

	// worktree 추가
	os.MkdirAll(filepath.Dir(wtDir), 0755)
	if _, err := gitCmd(mainDir, "worktree", "add", wtDir, branch); err != nil {
		// 이미 있는 경우 정리 후 재시도
		gitCmd(mainDir, "worktree", "remove", wtDir, "--force")
		gitCmd(mainDir, "branch", "-D", branch)
		gitCmd(mainDir, "branch", branch, "origin/main")
		if _, err := gitCmd(mainDir, "worktree", "add", wtDir, branch); err != nil {
			return "", "", fmt.Errorf("worktree 생성 실패: %v", err)
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
	return wtDir, branch, nil
}

// CheckoutNewBranch 사용자 worktree에서 main 기준으로 브랜치 리셋
func CheckoutNewBranch(owner, repo, branch string) error {
	// branch에서 userID 추출 (docs/{userId} 형식)
	userID := branch
	if strings.HasPrefix(branch, "docs/") {
		userID = strings.TrimPrefix(branch, "docs/")
	}

	wtDir := worktreeDir(owner, repo, userID)

	// worktree가 없으면 먼저 생성
	if _, err := os.Stat(filepath.Join(wtDir, ".git")); os.IsNotExist(err) {
		var err2 error
		wtDir, _, err2 = EnsureWorktree(owner, repo, userID)
		if err2 != nil {
			return err2
		}
	}

	mu := getRepoLock(owner, repo)
	mu.Lock()
	defer mu.Unlock()

	// main 최신화 후 리셋
	mainDir := repoDir(owner, repo)
	gitCmd(mainDir, "fetch", "origin")
	gitCmd(wtDir, "reset", "--hard", "origin/main")

	return nil
}

// WriteLocalFile worktree에 파일 쓰기
func WriteLocalFile(owner, repo, filePath, content string) error {
	// filePath에서 userID 판별은 어려우므로, 현재 작업 중인 worktree를 찾기 위해
	// 먼저 메인 클론 경로에서 시도, 없으면 에러
	// 호출자가 worktree 경로를 직접 지정하는 방식으로 전환
	dir := repoDir(owner, repo)
	fullPath := filepath.Join(dir, filePath)
	os.MkdirAll(filepath.Dir(fullPath), 0755)
	return os.WriteFile(fullPath, []byte(content), 0644)
}

// WriteWorktreeFile 사용자 worktree에 파일 쓰기
func WriteWorktreeFile(owner, repo, userID, filePath, content string) error {
	dir := worktreeDir(owner, repo, userID)
	fullPath := filepath.Join(dir, filePath)
	os.MkdirAll(filepath.Dir(fullPath), 0755)
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
	wtBase := filepath.Join(config.RepoBaseDir, owner, repo+"-wt")
	entries, err := os.ReadDir(wtBase)
	if err != nil {
		return nil
	}
	var users []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
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

// DiscardWorktree 사용자 worktree 삭제 (작업 폐기)
func DiscardWorktree(owner, repo, userID string) error {
	mu := getRepoLock(owner, repo)
	mu.Lock()
	defer mu.Unlock()

	mainDir := repoDir(owner, repo)
	wtDir := worktreeDir(owner, repo, userID)
	branch := fmt.Sprintf("docs/%s", userID)

	gitCmd(mainDir, "worktree", "remove", wtDir, "--force")
	gitCmd(mainDir, "branch", "-D", branch)
	os.RemoveAll(wtDir)

	log.Printf("worktree discarded: %s/%s user=%s", owner, repo, userID)
	return nil
}

// Pull 최신 코드 pull
func Pull(owner, repo, branch string) error {
	dir := repoDir(owner, repo)
	if _, err := gitCmd(dir, "checkout", branch); err != nil {
		gitCmd(dir, "fetch", "origin", branch)
		gitCmd(dir, "checkout", "-b", branch, "origin/"+branch)
	}
	_, err := gitCmd(dir, "pull", "origin", branch)
	return err
}

// GetLocalRepoDir 로컬 repo 경로 반환
func GetLocalRepoDir(owner, repo string) string {
	return repoDir(owner, repo)
}

// ListLocalRepos 로컬 repos 폴더에 있는 {owner}/{repo} 목록 반환
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
			// -wt 디렉토리는 제외
			if strings.HasSuffix(repoEntry.Name(), "-wt") {
				continue
			}
			result = append(result, struct{ Owner, Repo string }{ownerEntry.Name(), repoEntry.Name()})
		}
	}
	return result
}

// RemoveLocalRepo 로컬 clone 및 worktree 폴더 삭제
func RemoveLocalRepo(owner, repo string) error {
	dir := repoDir(owner, repo)
	wtBase := filepath.Join(config.RepoBaseDir, owner, repo+"-wt")

	// worktree 먼저 정리
	if _, err := os.Stat(wtBase); err == nil {
		log.Printf("removing worktrees: %s", wtBase)
		os.RemoveAll(wtBase)
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil
	}
	log.Printf("removing local repo: %s", dir)
	return os.RemoveAll(dir)
}
