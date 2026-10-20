package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// repoNameFromURL — origin URL 에서 repo 이름 파생 (ensureOriginURL 이 매 호출
// resolveRepoName HTTP 를 나가지 않도록 하는 핵심 헬퍼).
func TestRepoNameFromURL(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"http://192.168.219.109:3300/gojira/number-baseball.git", "number-baseball"},
		{"http://duke:tok123@192.168.219.109:3300/gojira/number-baseball.git", "number-baseball"},
		{"https://host/org/repo", "repo"},
		{"https://host/org/repo/", "repo"},
		{"http://host:3300", ""},        // path 없음 — HTTP 폴백 대상
		{"", ""},                        // 빈 origin — HTTP 폴백 대상
		{"   ", ""},                     // 공백만
		{"http://host/repo.git", "repo"}, // org 없는 flat 경로
	}
	for _, c := range cases {
		if got := repoNameFromURL(c.in); got != c.want {
			t.Errorf("repoNameFromURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// 사용자별 clone 분리 — 같은 과제라도 user 가 다르면 경로·정렬 게이트 키가 달라야 한다.
func TestRepoPathPerUser(t *testing.T) {
	root := t.TempDir()
	s := &Server{cfg: &Config{RepoRoot: root}}

	a, b := s.repoPath("NBBALL", "duke.kimm"), s.repoPath("NBBALL", "batman")
	if a == b {
		t.Fatalf("사용자별 경로가 같음: %s — 전환 시 작업본이 섞인다", a)
	}
	if want := filepath.Join(root, "duke.kimm", "NBBALL"); a != want {
		t.Errorf("repoPath = %s, want %s", a, want)
	}

	s.alignedRepos.Store(repoKey("NBBALL", "duke.kimm"), true)
	if _, ok := s.alignedRepos.Load(repoKey("NBBALL", "batman")); ok {
		t.Error("user 전환인데 게이트 hit — 다른 사용자 clone 정렬을 건너뛴다")
	}
}

// 공유 clone 이관 — HEAD 가 docs/<user> 인 사용자만 가져가고, 다른 사용자는 건드리지 않는다.
func TestMigrateLegacyClone(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git 없음")
	}
	root := t.TempDir()
	s := &Server{cfg: &Config{RepoRoot: root}}
	legacy := s.legacyRepoPath("NBBALL")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "docs/duke.kimm"},
		{"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "init"},
	} {
		if out, err := gitCmd(legacy, args...); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}

	// 다른 사용자 — 이관 안 됨.
	s.migrateLegacyClone("NBBALL", "batman", s.repoPath("NBBALL", "batman"))
	if _, err := os.Stat(filepath.Join(legacy, ".git")); err != nil {
		t.Fatal("다른 사용자 접근으로 공유 clone 이 옮겨짐")
	}

	// 브랜치 주인 — 사용자별 경로로 이관.
	dst := s.repoPath("NBBALL", "duke.kimm")
	s.migrateLegacyClone("NBBALL", "duke.kimm", dst)
	if _, err := os.Stat(filepath.Join(dst, ".git")); err != nil {
		t.Fatalf("이관 실패: %v", err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Error("이관 후 공유 clone 이 남아있음")
	}
}

// /remove-clone 대상 — 모든 사용자 clone + 공유 clone, .git 없는 폴더는 제외.
func TestClonePathsOf(t *testing.T) {
	root := t.TempDir()
	s := &Server{cfg: &Config{RepoRoot: root}}
	want := map[string]bool{
		s.legacyRepoPath("NBBALL"):        true,
		s.repoPath("NBBALL", "duke.kimm"): true,
		s.repoPath("NBBALL", "batman"):    true,
	}
	for p := range want {
		if err := os.MkdirAll(filepath.Join(p, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// 다른 과제 / .git 없는 동명 폴더 — 대상 아님.
	_ = os.MkdirAll(filepath.Join(s.repoPath("OTHER", "duke.kimm"), ".git"), 0o755)
	_ = os.MkdirAll(s.repoPath("NBBALL", "nogit"), 0o755)

	got := s.clonePathsOf("NBBALL")
	if len(got) != len(want) {
		t.Fatalf("clonePathsOf = %v, want %d개", got, len(want))
	}
	for _, p := range got {
		if !want[p] {
			t.Errorf("예기치 않은 경로: %s", p)
		}
	}
}
