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
// GitCmd — 외부 패키지에서도 호출 가능한 git 명령 실행.
func GitCmd(dir string, args ...string) (string, error) {
	return gitCmd(dir, args...)
}

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
	wtBase := filepath.Join(config.RepoBaseDir, owner, repo+".worktrees")

	if _, err := os.Stat(dir); err == nil {
		log.Printf("removing bare repo: %s", dir)
		os.RemoveAll(dir)
	}
	if _, err := os.Stat(wtBase); err == nil {
		log.Printf("removing worktrees: %s", wtBase)
		os.RemoveAll(wtBase)
	}
	return nil
}
