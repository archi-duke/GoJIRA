// 과제 가져오기의 순수 함수 단위 테스트.
//
// 실행: PLATFORM_ENV_FILE=$PWD/github/testdata/test.env go test ./handlers/
// (config.init() 이 필수 환경변수를 요구하므로 절대 경로로 넘긴다 — 상대 경로는
//
//	패키지 디렉터리 기준으로 풀려 찾지 못한다.)
package handlers

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"platform-api/github"
)

// 가져오기는 원본 저장소 이름을 그대로 살려야 한다.
// slugifyRepoName 은 한글/공백 과제명을 slug 로 만드는 함수라 `_` 를 `-` 로 바꾼다.
// 그걸 가져오기에 쓰면 at_bkg 로 내보낸 과제가 at-bkg 로 복원돼 이름이 달라진다
// (2026-11-18 복원 중 실제로 발생).
func TestImportRepoNamePreservesUnderscore(t *testing.T) {
	cases := []struct{ in, want string }{
		{"at_bkg", "at_bkg"},
		{"at-bkg", "at-bkg"},
		{"AT_Bkg", "AT_Bkg"}, // Gitea 는 대문자도 받는다 — 임의로 낮추지 않는다
		{"my.repo_1-x", "my.repo_1-x"},
		{" at_bkg ", "at_bkg"}, // 앞뒤 공백만 다듬는다
		// 유효하지 않으면 slug 로 떨어뜨린다
		{"한글 이름", ""},
		{"", ""},
		{".", ""},
		{"..", ""},
		{"-leading", "leading"}, // 선행 하이픈은 유효하지 않아 slug 경로
	}
	for _, c := range cases {
		if got := importRepoName(c.in); got != c.want {
			t.Errorf("importRepoName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// 과제 코드는 대문자/숫자/밑줄로 2~20자. 소문자나 하이픈은 키 접두로 쓸 수 없다.
func TestCodeRe(t *testing.T) {
	ok := []string{"ATBKG", "AT_BKG", "A1", "GOJIRA3", "A_1234567890123456789"[:20]}
	bad := []string{"A", "atbkg", "AT-BKG", "1ABC", "_ABC", "", "A_12345678901234567890"}
	for _, s := range ok {
		if !codeRe.MatchString(s) {
			t.Errorf("%q 는 유효해야 한다", s)
		}
	}
	for _, s := range bad {
		if codeRe.MatchString(s) {
			t.Errorf("%q 는 거부돼야 한다", s)
		}
	}
}

// 키 재작성은 숫자가 뒤따르는 형태만 바꾼다 — 산문에 쓰인 제품명은 건드리지 않는다.
// issueLinks.id 의 `tmp_<ts>_OLD-20` 처럼 문장 가운데 박힌 키도 바뀌어야 하고,
// 바이너리 파일과 추적되지 않는 파일은 손대지 않아야 한다.
func TestMakeKeyRewriter(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git 없음")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		if out, err := github.GitCmd(dir, args...); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
	}
	write := func(name, body string) {
		t.Helper()
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "-q", "-b", "main", ".")

	write("data/issues/VOS_001.yaml",
		"key: ATBKG-1\nsubkey: VOS_001\nissueLinks:\n- id: tmp_1779_ATBKG-20\n  outwardIssue:\n    key: ATBKG-20\n")
	// 산문 — 제품명으로 쓰인 ATBKG 는 그대로 남아야 한다(키가 아니다).
	write("docs/prose.md", "ATBKG 는 로컬 에이전트를 다룬다. 관련 이슈는 ATBKG-7 이다.\n")
	// 바이너리 — NUL 이 있으면 건드리지 않는다.
	write("assets/blob.bin", "ATBKG-9\x00binary")
	run("add", "-A")

	// 추적되지 않는 파일은 대상이 아니다.
	write("untracked.md", "ATBKG-5\n")

	rw := makeKeyRewriter("ATBKG", "IMPTEST")
	changed, err := rw(dir)
	if err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	if !changed {
		t.Fatal("바뀐 게 없다고 보고했다")
	}

	read := func(name string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}

	y := read("data/issues/VOS_001.yaml")
	for _, want := range []string{"key: IMPTEST-1", "id: tmp_1779_IMPTEST-20", "key: IMPTEST-20"} {
		if !strings.Contains(y, want) {
			t.Errorf("YAML 에 %q 가 없다:\n%s", want, y)
		}
	}
	if strings.Contains(y, "ATBKG-") {
		t.Errorf("YAML 에 옛 키가 남았다:\n%s", y)
	}

	prose := read("docs/prose.md")
	if !strings.Contains(prose, "ATBKG 는") {
		t.Errorf("산문의 제품명을 바꿨다: %s", prose)
	}
	if !strings.Contains(prose, "IMPTEST-7") {
		t.Errorf("산문 속 키가 안 바뀌었다: %s", prose)
	}

	if !strings.Contains(read("assets/blob.bin"), "ATBKG-9") {
		t.Error("바이너리 파일을 건드렸다")
	}
	if !strings.Contains(read("untracked.md"), "ATBKG-5") {
		t.Error("추적되지 않는 파일을 건드렸다")
	}
}

// isBinary 는 NUL 이 있는 내용만 바이너리로 봐야 한다.
func TestIsBinary(t *testing.T) {
	if isBinary([]byte("key: ATBKG-1\n한글도 포함")) {
		t.Error("텍스트를 바이너리로 판정했다")
	}
	if !isBinary([]byte{0x41, 0x00, 0x42}) {
		t.Error("NUL 포함인데 바이너리가 아니라고 했다")
	}
}

// sanitizeImportedProject 는 이 시스템이 새로 정해야 하는 값을 넘기지 않아야 한다.
// documents(저장소 URL)·_id 가 섞여 들어가면 가져온 과제가 원본 시스템의 저장소를
// 가리키게 된다.
func TestSanitizeImportedProjectDropsSystemFields(t *testing.T) {
	src := map[string]interface{}{
		"name":          "AT_Bkg",
		"status":        "In Progress",
		"feature":       "ATBKG-1",
		"members":       []interface{}{"a"},
		"_id":           "6ae978ffccf7a9e26a0a3fb0",
		"documents":     map[string]interface{}{"repoUrl": "http://other-system/gojira/at_bkg"},
		"jiraCodes":     map[string]interface{}{"main": map[string]interface{}{"jiraCode": "ATBKG"}},
		"gitLastCommit": "deadbeef",
	}
	got := sanitizeImportedProject(src)
	for _, k := range []string{"_id", "documents", "jiraCodes", "gitLastCommit"} {
		if _, ok := got[k]; ok {
			t.Errorf("%q 는 넘어가면 안 된다", k)
		}
	}
	for _, k := range []string{"name", "status", "feature", "members"} {
		if _, ok := got[k]; !ok {
			t.Errorf("%q 는 넘어가야 한다", k)
		}
	}
}
