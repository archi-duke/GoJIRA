package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// 비 ASCII(한글) 파일명이 headTree 와 tree 에서 같은 경로 문자열로 나와야 한다.
// git 기본값(core.quotepath=true)에서 `git ls-tree` 는 비 ASCII 경로를
// "\353\246\254..." 형태로 8진수 이스케이프+따옴표 처리하는데, 웹(DocEditor)은
// tree 와 headTree 를 경로 문자열로 매칭하므로 커밋된 한글 파일이 전부 "미커밋" 으로
// 판정돼 발행 버튼 대신 등록 버튼이 계속 뜨던 회귀를 막는다.
func TestListHeadTreeNonASCIIPathMatchesLocalTree(t *testing.T) {
	resetTreeHashCache()
	dir := setupTestRepo(t)
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	// 사용자 전역 설정과 무관하게 git 기본 동작(quotepath=true)을 재현.
	run("config", "core.quotepath", "true")

	rel := "architecture/design-decisions/DP-01-리소스-접근.md"
	if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(rel)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, rel), []byte("# DP-01\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("readme\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-q", "-m", "init")

	head, err := listHeadTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	local, err := listLocalTree(dir)
	if err != nil {
		t.Fatal(err)
	}

	headSha := map[string]string{}
	for _, e := range head {
		headSha[e["path"].(string)] = e["sha"].(string)
	}
	if len(headSha) != 2 {
		t.Fatalf("want 2 head entries, got %d: %v", len(headSha), headSha)
	}
	for _, e := range local {
		p := e["path"].(string)
		hs, ok := headSha[p]
		if !ok {
			t.Fatalf("local path %q not found in headTree paths %v", p, keys(headSha))
		}
		if hs != e["sha"].(string) {
			t.Fatalf("sha mismatch for %q: head=%s local=%s", p, hs, e["sha"])
		}
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
