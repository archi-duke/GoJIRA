package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// newLegacyClone — docs/<user> 를 체크아웃한 공유 clone(repoRoot/<code>)을 만든다.
func newLegacyClone(t *testing.T, s *Server, code, user string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git 없음")
	}
	legacy := s.legacyRepoPath(code)
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "docs/" + user},
		{"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "init"},
	} {
		if out, err := gitCmd(legacy, args...); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	return legacy
}

// 공유 clone 이관 — HEAD 가 docs/<user> 인 사용자만 가져가고, 다른 사용자는 건드리지 않는다.
func TestMigrateLegacyClone(t *testing.T) {
	s := &Server{cfg: &Config{RepoRoot: t.TempDir()}}
	legacy := newLegacyClone(t, s, "NBBALL", "duke.kimm")

	// 다른 사용자 — 이관 안 됨(오류도 아님 — 새로 clone 하면 된다).
	if err := s.migrateLegacyClone("NBBALL", "batman", s.repoPath("NBBALL", "batman")); err != nil {
		t.Fatalf("다른 사용자인데 오류: %v", err)
	}
	if _, err := os.Stat(filepath.Join(legacy, ".git")); err != nil {
		t.Fatal("다른 사용자 접근으로 공유 clone 이 옮겨짐")
	}

	// 브랜치 주인 — 사용자별 경로로 이관.
	dst := s.repoPath("NBBALL", "duke.kimm")
	if err := s.migrateLegacyClone("NBBALL", "duke.kimm", dst); err != nil {
		t.Fatalf("이관 오류: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, ".git")); err != nil {
		t.Fatalf("이관 실패: %v", err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Error("이관 후 공유 clone 이 남아있음")
	}
}

// 이관 실패 — 재시도 후 오류를 반환하고 공유 clone 은 그대로 둔다(새로 clone 으로 넘어가면
// 미커밋 작업이 사라진 것처럼 보인다). 목적지에 비어있지 않은 폴더를 두어 rename 을 실패시킨다.
func TestMigrateLegacyCloneFailure(t *testing.T) {
	oldDelay, oldRetries := migrateRetryDelay, migrateRetries
	migrateRetryDelay, migrateRetries = 0, 3
	defer func() { migrateRetryDelay, migrateRetries = oldDelay, oldRetries }()

	s := &Server{cfg: &Config{RepoRoot: t.TempDir()}}
	s.cfg.Git.Base = "http://127.0.0.1:1" // 이관 단계까지 진행되도록(clone 으로 넘어가면 접속 실패로 다른 오류)
	legacy := newLegacyClone(t, s, "NBBALL", "duke.kimm")
	dst := s.repoPath("NBBALL", "duke.kimm")
	if err := os.MkdirAll(filepath.Join(dst, "leftover"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := s.migrateLegacyClone("NBBALL", "duke.kimm", dst); err == nil {
		t.Fatal("이관 실패인데 오류 없음")
	}
	if _, err := os.Stat(filepath.Join(legacy, ".git")); err != nil {
		t.Fatal("이관 실패 후 공유 clone 이 사라짐")
	}
	if _, err := s.EnsureRepoLocal("NBBALL", "duke.kimm"); err == nil || !strings.Contains(err.Error(), "옮기지 못했습니다") {
		t.Fatalf("이관 실패 안내 오류가 아님(새 clone 으로 진행됐을 수 있음): %v", err)
	}
}

// /issues 가 /files 보다 먼저 호출돼도 공유 clone 을 이관한 뒤 조회한다.
func TestHandleIssuesMigratesLegacy(t *testing.T) {
	s := &Server{cfg: &Config{RepoRoot: t.TempDir()}}
	legacy := newLegacyClone(t, s, "NBBALL", "duke.kimm")
	issuesDir := filepath.Join(legacy, "data", "issues")
	if err := os.MkdirAll(issuesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(issuesDir, "VOS_001.yaml"), []byte("key: T-1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	s.handleIssues(w, httptest.NewRequest(http.MethodGet, "/issues?code=NBBALL&user=duke.kimm", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(s.repoPath("NBBALL", "duke.kimm"), ".git")); err != nil {
		t.Fatal("/issues 호출 후 사용자별 경로로 이관되지 않음")
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
