package github

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"platform-api/config"
)

// repoDir 로컬 repo 경로 반환
func repoDir(owner, repo string) string {
	return filepath.Join(config.RepoBaseDir, owner, repo)
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
		// https://token@github.com/owner/repo.git
		base = strings.Replace(base, "https://", fmt.Sprintf("https://%s@", config.GitHubToken), 1)
	}
	return fmt.Sprintf("%s/%s/%s.git", base, owner, repo)
}

// EnsureClone clone이 없으면 clone, 있으면 fetch + reset
func EnsureClone(owner, repo string) (string, error) {
	dir := repoDir(owner, repo)

	if _, err := os.Stat(filepath.Join(dir, ".git")); os.IsNotExist(err) {
		// clone
		os.MkdirAll(filepath.Dir(dir), 0755)
		url := cloneURL(owner, repo)
		log.Printf("git clone %s/%s → %s", owner, repo, dir)
		if _, err := gitCmd(".", "clone", url, dir); err != nil {
			return "", err
		}
		// git config
		gitCmd(dir, "config", "user.name", "GoJIRA Platform")
		gitCmd(dir, "config", "user.email", "platform@gojira.local")
	} else {
		// fetch + reset to origin/main
		gitCmd(dir, "fetch", "origin")
	}

	return dir, nil
}

// Pull 최신 코드 pull
func Pull(owner, repo, branch string) error {
	dir := repoDir(owner, repo)
	if _, err := gitCmd(dir, "checkout", branch); err != nil {
		// 브랜치가 없으면 origin에서 가져오기
		gitCmd(dir, "fetch", "origin", branch)
		gitCmd(dir, "checkout", "-b", branch, "origin/"+branch)
	}
	_, err := gitCmd(dir, "pull", "origin", branch)
	return err
}

// CheckoutNewBranch main에서 새 브랜치 생성
func CheckoutNewBranch(owner, repo, branch string) error {
	dir := repoDir(owner, repo)
	gitCmd(dir, "checkout", "main")
	gitCmd(dir, "pull", "origin", "main")
	_, err := gitCmd(dir, "checkout", "-b", branch)
	return err
}

// WriteFile 파일 쓰기 (디렉토리 자동 생성)
func WriteLocalFile(owner, repo, filePath, content string) error {
	dir := repoDir(owner, repo)
	fullPath := filepath.Join(dir, filePath)
	os.MkdirAll(filepath.Dir(fullPath), 0755)
	return os.WriteFile(fullPath, []byte(content), 0644)
}

// CommitAndPush git add . → commit → push
func CommitAndPush(owner, repo, branch, message string) error {
	dir := repoDir(owner, repo)
	if _, err := gitCmd(dir, "add", "."); err != nil {
		return err
	}
	// 변경사항 없으면 skip
	status, _ := gitCmd(dir, "status", "--porcelain")
	if status == "" {
		log.Printf("git: no changes to commit for %s/%s", owner, repo)
		return nil
	}
	if _, err := gitCmd(dir, "commit", "-m", message); err != nil {
		return err
	}
	if _, err := gitCmd(dir, "push", "origin", branch); err != nil {
		return err
	}
	return nil
}

// GetLocalRepoDir 로컬 repo 경로 반환 (존재 여부 확인 없이)
func GetLocalRepoDir(owner, repo string) string {
	return repoDir(owner, repo)
}
