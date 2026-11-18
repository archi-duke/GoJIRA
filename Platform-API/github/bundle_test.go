// 과제 내보내기/가져오기의 git 조작 단위 테스트.
//
// 실행: PLATFORM_ENV_FILE=testdata/test.env go test ./github/
//
// 여기 묶인 두 케이스는 2026-11-18 에 실제로 터진 것들이다. 둘 다 "코드를 바꿔
// 가져오기" 경로만 시험해서 못 잡았다 — 같은 코드로 제자리 복원하는 경로가
// 미검증이었다.
package github

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// newTestRepo 커밋 1개와 브랜치 2개, 그리고 Gitea 가 만드는 refs/pull/<n>/head 를
// 가진 저장소를 만든다.
func newTestRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git 없음")
	}
	dir := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		out, err := gitCmd(dir, args...)
		if err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
		return out
	}
	run("init", "-q", "-b", "main", ".")
	run("config", "user.name", "T")
	run("config", "user.email", "t@example.com")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# t\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-q", "-m", "init")
	run("branch", "docs/duke.kimm")
	// Gitea 의 PR ref 를 흉내낸다 — push 대상에서 걸러져야 하는 ref.
	head := run("rev-parse", "HEAD")
	run("update-ref", "refs/pull/1/head", head)
	return dir
}

// bundle 은 refs/pull/* 까지 담되(완전한 보관), 복원 대상 브랜치 목록에는
// 그것이 섞이지 않아야 한다. 섞이면 Gitea 서버 훅이
// `hook declined to update refs/pull/1/head` 로 push 전체를 거부한다.
func TestBundleCarriesPullRefsButBranchesExcludeThem(t *testing.T) {
	src := newTestRepo(t)
	out := filepath.Join(t.TempDir(), "t.bundle")
	if _, err := gitCmd(src, "bundle", "create", out, "--all"); err != nil {
		t.Fatalf("bundle create: %v", err)
	}

	refs, err := VerifyBundle(out)
	if err != nil {
		t.Fatalf("VerifyBundle: %v", err)
	}
	var hasPull bool
	for _, r := range refs {
		if r == "refs/pull/1/head" {
			hasPull = true
		}
	}
	if !hasPull {
		t.Errorf("bundle 에 refs/pull/1/head 가 있어야 한다 (보관은 완전해야 함): %v", refs)
	}

	// clone 후 브랜치 목록에는 PR ref 가 없어야 한다.
	work := filepath.Join(t.TempDir(), "work")
	if _, err := gitCmd(t.TempDir(), "clone", "-q", out, work); err != nil {
		t.Fatalf("clone: %v", err)
	}
	brs, err := listRemoteBranches(work)
	if err != nil {
		t.Fatalf("listRemoteBranches: %v", err)
	}
	for _, b := range brs {
		if b == "pull/1/head" || b == "HEAD" || b == "origin" {
			t.Errorf("브랜치 목록에 %q 가 섞였다: %v", b, brs)
		}
	}
	if len(brs) != 2 {
		t.Errorf("브랜치 2개여야 한다 (main, docs/duke.kimm), 실제 %v", brs)
	}
}

// VerifyBundle 은 저장소 바깥에서도 동작해야 한다.
// `git bundle verify` 는 저장소 안에서만 돌아 "need a repository to verify a
// bundle" 로 죽는다 — 빈 bare 저장소를 만들어 검증하는 구현이 그걸 막는다.
func TestVerifyBundleWorksOutsideAnyRepo(t *testing.T) {
	src := newTestRepo(t)
	out := filepath.Join(t.TempDir(), "t.bundle")
	if _, err := gitCmd(src, "bundle", "create", out, "--all"); err != nil {
		t.Fatalf("bundle create: %v", err)
	}
	// 저장소가 아닌 디렉터리에 bundle 만 두고 검증한다.
	plain := t.TempDir()
	moved := filepath.Join(plain, "t.bundle")
	if b, err := os.ReadFile(out); err != nil {
		t.Fatal(err)
	} else if err := os.WriteFile(moved, b, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyBundle(moved); err != nil {
		t.Fatalf("저장소 바깥 검증이 실패했다: %v", err)
	}
}

// hasTags 는 태그 없는 저장소에서 false 여야 한다 — true 로 잘못 보면
// refs/tags/* refspec 이 `src refspec does not match any` 로 push 를 깨뜨린다.
func TestHasTags(t *testing.T) {
	src := newTestRepo(t)
	if hasTags(src) {
		t.Error("태그 없는 저장소인데 hasTags=true")
	}
	if _, err := gitCmd(src, "tag", "v1"); err != nil {
		t.Fatalf("tag: %v", err)
	}
	if !hasTags(src) {
		t.Error("태그를 만들었는데 hasTags=false")
	}
}

// redactURL 은 자격증명을 가려야 한다 — 에러 메시지에 토큰이 실려 나가면 안 된다.
func TestRedactURL(t *testing.T) {
	got := redactURL("http://user:deadbeef@host:44372/gojira/at_bkg.git")
	if got != "http://***@host:44372/gojira/at_bkg.git" {
		t.Errorf("가려지지 않았다: %q", got)
	}
	if plain := redactURL("http://host/x.git"); plain != "http://host/x.git" {
		t.Errorf("자격증명 없는 URL 을 바꿨다: %q", plain)
	}
}
