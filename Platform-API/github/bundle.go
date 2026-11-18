package github

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// 과제 내보내기/가져오기용 git bundle 헬퍼.
//
// 왜 bundle 인가 — 과제 전달의 저장 단위는 "파일 하나"여야 한다. 예전 archive.go 는
// config.RepoBaseDir 의 bare 디렉터리를 그대로 zip 에 담았는데, (1) Gitea 로 저장소가
// 넘어간 뒤 그 디렉터리가 비어 있어 문서가 조용히 누락됐고, (2) bare 트리에는 hooks/
// 의 실행 스크립트가 함께 들어가 다른 시스템에서 받은 zip 을 푸는 경로에서 바람직하지
// 않다. bundle 은 ref + object 만 담는 단일 파일이고 전체 이력·전 브랜치가 보존된다.

// CloneURL 인증 정보가 포함된 clone URL. 외부 패키지(handlers)에서 쓰기 위한 export.
// 주의: 토큰이 들어 있으므로 로그·응답에 그대로 싣지 말 것.
func CloneURL(owner, repo string) string {
	return cloneURL(owner, repo)
}

// redactURL 로그용 — URL 에서 자격증명 부분을 지운다.
func redactURL(u string) string {
	if i := strings.Index(u, "@"); i > 0 {
		if j := strings.Index(u, "://"); j >= 0 && j+3 < i {
			return u[:j+3] + "***@" + u[i+1:]
		}
	}
	return u
}

// BundleStats bundle 에 담긴 내용 요약 — manifest 에 실어 가져오기 쪽에서 미리 보여준다.
type BundleStats struct {
	Refs       []string `json:"refs"`
	Branches   []string `json:"branches"`
	FileCount  int      `json:"fileCount"`  // 기본 브랜치의 추적 파일 수
	CommitHead string   `json:"commitHead"` // 기본 브랜치 HEAD
}

// BundleRepo Gitea 저장소를 bundle 파일 하나로 뜨고 내용 요약을 돌려준다.
//
// --mirror clone 으로 모든 ref 를 받고 `git bundle create --all` 로 묶는다.
// 빈 저장소(ref 0개)는 bundle 생성 자체가 실패하므로 명시적으로 구분해 알린다.
func BundleRepo(owner, repo, outPath, defaultBranch string) (*BundleStats, error) {
	tmp, err := os.MkdirTemp("", "gojira-bundle-")
	if err != nil {
		return nil, fmt.Errorf("임시 디렉터리 생성 실패: %w", err)
	}
	defer os.RemoveAll(tmp)

	mirror := filepath.Join(tmp, repo+".git")
	url := CloneURL(owner, repo)
	if out, err := gitCmd(tmp, "clone", "--mirror", url, mirror); err != nil {
		return nil, fmt.Errorf("mirror clone 실패 (%s): %s", redactURL(url), firstLines(out, 3))
	}

	refs, err := listRefs(mirror)
	if err != nil {
		return nil, err
	}
	if len(refs) == 0 {
		return nil, fmt.Errorf("저장소 %s/%s 에 ref 가 없다 (빈 저장소) — bundle 을 만들 수 없다", owner, repo)
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return nil, fmt.Errorf("bundle 경로 생성 실패: %w", err)
	}
	if out, err := gitCmd(mirror, "bundle", "create", outPath, "--all"); err != nil {
		return nil, fmt.Errorf("bundle create 실패: %s", firstLines(out, 3))
	}

	st := &BundleStats{Refs: refs}
	for _, r := range refs {
		if strings.HasPrefix(r, "refs/heads/") {
			st.Branches = append(st.Branches, strings.TrimPrefix(r, "refs/heads/"))
		}
	}
	// 요약은 부가정보다 — 실패해도 bundle 자체는 유효하므로 에러로 올리지 않는다.
	if defaultBranch == "" {
		defaultBranch = "main"
	}
	if sha, err := gitCmd(mirror, "rev-parse", defaultBranch); err == nil {
		st.CommitHead = sha
		if out, err := gitCmd(mirror, "ls-tree", "-r", "--name-only", defaultBranch); err == nil {
			for _, l := range strings.Split(out, "\n") {
				if strings.TrimSpace(l) != "" {
					st.FileCount++
				}
			}
		}
	}
	return st, nil
}

// VerifyBundle bundle 파일이 git 이 읽을 수 있는 온전한 묶음인지 확인하고 ref 목록을 돌려준다.
// 신뢰할 수 없는 입력(다른 시스템에서 받은 zip)을 풀기 전 관문으로 쓴다.
//
// `git bundle verify` 는 **저장소 안에서만** 동작한다("need a repository to verify a
// bundle") — prerequisite 커밋을 현재 저장소와 맞춰보기 때문이다. 그래서 빈 bare
// 저장소를 하나 만들어 그 안에서 돌린다. 내보내기가 만드는 bundle 은 --all 전체
// 묶음이라 prerequisite 가 없고, 빈 저장소에서도 통과해야 정상이다.
func VerifyBundle(bundlePath string) ([]string, error) {
	scratch, err := os.MkdirTemp("", "gojira-verify-")
	if err != nil {
		return nil, fmt.Errorf("검증용 임시 디렉터리 생성 실패: %w", err)
	}
	defer os.RemoveAll(scratch)
	repo := filepath.Join(scratch, "scratch.git")
	if out, err := gitCmd(scratch, "init", "--bare", "-q", repo); err != nil {
		return nil, fmt.Errorf("검증용 저장소 생성 실패: %s", firstLines(out, 3))
	}
	if out, err := gitCmd(repo, "bundle", "verify", bundlePath); err != nil {
		return nil, fmt.Errorf("bundle 검증 실패: %s", firstLines(out, 5))
	}
	out, err := gitCmd(repo, "bundle", "list-heads", bundlePath)
	if err != nil {
		return nil, fmt.Errorf("bundle list-heads 실패: %s", firstLines(out, 3))
	}
	var refs []string
	for _, line := range strings.Split(out, "\n") {
		parts := strings.Fields(strings.TrimSpace(line))
		if len(parts) == 2 {
			refs = append(refs, parts[1])
		}
	}
	return refs, nil
}

// RewriteFunc 체크아웃된 작업 트리 하나를 받아 파일 내용을 고친다.
// 고친 게 있으면 true 를 돌려준다(커밋 여부 판단용).
type RewriteFunc func(worktree string) (changed bool, err error)

// PushBundleToRepo bundle 의 내용을 대상 Gitea 저장소로 올린다.
//
// rewrite == nil 이면 --mirror push 로 ref 를 그대로 복제한다(원본과 바이트 동일).
// rewrite != nil 이면 브랜치별로 작업 트리를 만들어 내용을 고치고 커밋한 뒤 올린다
// (과제 코드가 바뀌어 이슈 키를 다시 써야 하는 경우).
//
// 호출 시점 주의: 브랜치 보호를 걸기 **전에** 호출해야 한다. auto_init 으로 만들어진
// main 커밋을 밀어내는 force 성 push 이기 때문이다.
func PushBundleToRepo(bundlePath, owner, repo, commitMsg string, rewrite RewriteFunc) ([]string, error) {
	if _, err := VerifyBundle(bundlePath); err != nil {
		return nil, err
	}
	tmp, err := os.MkdirTemp("", "gojira-restore-")
	if err != nil {
		return nil, fmt.Errorf("임시 디렉터리 생성 실패: %w", err)
	}
	defer os.RemoveAll(tmp)

	target := CloneURL(owner, repo)

	if rewrite == nil {
		mirror := filepath.Join(tmp, "mirror.git")
		if out, err := gitCmd(tmp, "clone", "--mirror", bundlePath, mirror); err != nil {
			return nil, fmt.Errorf("bundle mirror clone 실패: %s", firstLines(out, 3))
		}
		refs, err := listRefs(mirror)
		if err != nil {
			return nil, err
		}
		// `push --mirror` 를 쓰면 안 된다 — bundle 은 --all 로 떠서 Gitea 가 만드는
		// refs/pull/<n>/head 까지 들고 있고, 그걸 올리려 하면 서버 훅이
		// `hook declined to update refs/pull/1/head` 로 push 전체를 거부한다.
		// 복원에 필요한 것은 브랜치와 태그뿐이므로 refspec 을 명시한다.
		// (--force: auto_init 으로 생긴 main 커밋과 이력이 겹치지 않는다.)
		args := []string{"push", "--force", target, "refs/heads/*:refs/heads/*"}
		if hasTags(mirror) {
			args = append(args, "refs/tags/*:refs/tags/*")
		}
		if out, err := gitCmd(mirror, args...); err != nil {
			return nil, fmt.Errorf("저장소 push 실패 (%s): %s", redactURL(target), firstLines(out, 5))
		}
		var heads []string
		for _, r := range refs {
			if strings.HasPrefix(r, "refs/heads/") {
				heads = append(heads, strings.TrimPrefix(r, "refs/heads/"))
			}
		}
		return heads, nil
	}

	work := filepath.Join(tmp, "work")
	if out, err := gitCmd(tmp, "clone", bundlePath, work); err != nil {
		return nil, fmt.Errorf("bundle clone 실패: %s", firstLines(out, 3))
	}
	// 커밋 주체 — 컨테이너에 전역 git identity 가 없어 명시해야 commit 이 거부되지 않는다.
	_, _ = gitCmd(work, "config", "user.name", "GoJIRA Platform")
	_, _ = gitCmd(work, "config", "user.email", "platform@gojira.local")

	branches, err := listRemoteBranches(work)
	if err != nil {
		return nil, err
	}
	if len(branches) == 0 {
		return nil, fmt.Errorf("bundle 에 브랜치가 없다")
	}

	var pushed []string
	for _, br := range branches {
		if out, err := gitCmd(work, "checkout", "-B", br, "origin/"+br); err != nil {
			return nil, fmt.Errorf("checkout %s 실패: %s", br, firstLines(out, 3))
		}
		changed, err := rewrite(work)
		if err != nil {
			return nil, fmt.Errorf("브랜치 %s 재작성 실패: %w", br, err)
		}
		if changed {
			if out, err := gitCmd(work, "add", "-A"); err != nil {
				return nil, fmt.Errorf("git add (%s) 실패: %s", br, firstLines(out, 3))
			}
			if out, err := gitCmd(work, "commit", "-m", commitMsg); err != nil {
				return nil, fmt.Errorf("commit (%s) 실패: %s", br, firstLines(out, 3))
			}
		}
		pushed = append(pushed, br)
	}

	// --force: auto_init 으로 생긴 main 커밋과 이력이 겹치지 않는다.
	if out, err := gitCmd(work, "push", "--force", target, "--all"); err != nil {
		return nil, fmt.Errorf("push --all 실패 (%s): %s", redactURL(target), firstLines(out, 5))
	}
	// 태그는 없을 수 있다 — 있을 때만 보낸다(없이 --tags 를 보내면 실패한다).
	if hasTags(work) {
		if out, err := gitCmd(work, "push", "--force", target, "--tags"); err != nil {
			return nil, fmt.Errorf("태그 push 실패 (%s): %s", redactURL(target), firstLines(out, 3))
		}
	}
	return pushed, nil
}

// hasTags 태그가 하나라도 있는지 — 없을 때 refs/tags/* refspec 을 보내면
// `src refspec does not match any` 로 push 가 실패한다.
func hasTags(gitDir string) bool {
	out, err := gitCmd(gitDir, "for-each-ref", "--count=1", "--format=%(refname)", "refs/tags")
	return err == nil && strings.TrimSpace(out) != ""
}

// listRefs mirror/bare 저장소의 ref 전체 이름.
func listRefs(gitDir string) ([]string, error) {
	out, err := gitCmd(gitDir, "for-each-ref", "--format=%(refname)")
	if err != nil {
		return nil, fmt.Errorf("for-each-ref 실패: %s", firstLines(out, 3))
	}
	var refs []string
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			refs = append(refs, l)
		}
	}
	return refs, nil
}

// listRemoteBranches 일반 clone 의 origin/* 브랜치 이름 (HEAD 별칭 제외).
//
// %(refname:short) 를 쓰면 안 된다 — refs/remotes/origin/HEAD 가 "origin/HEAD" 가
// 아니라 **"origin"** 으로 줄어들어, 그대로 쓰면 `checkout -B origin origin/origin`
// 이 되어 죽는다. 전체 refname 에서 접두사를 직접 떼고 HEAD 를 걸러낸다.
func listRemoteBranches(workdir string) ([]string, error) {
	const prefix = "refs/remotes/origin/"
	out, err := gitCmd(workdir, "for-each-ref", "--format=%(refname)", "refs/remotes/origin")
	if err != nil {
		return nil, fmt.Errorf("브랜치 목록 조회 실패: %s", firstLines(out, 3))
	}
	var brs []string
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimSpace(l)
		if !strings.HasPrefix(l, prefix) {
			continue
		}
		br := strings.TrimPrefix(l, prefix)
		if br == "" || br == "HEAD" {
			continue
		}
		brs = append(brs, br)
	}
	return brs, nil
}

func firstLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, " | ")
}
