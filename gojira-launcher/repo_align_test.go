package main

import "testing"

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

// 정렬 게이트 — 같은 (code,user) 재호출은 fast-path, user 전환은 풀 정렬로 되돌아가야 한다.
func TestAlignedGateUserSwitch(t *testing.T) {
	s := &Server{}
	s.alignedRepos.Store("NBBALL", "duke.kimm")

	if u, ok := s.alignedRepos.Load("NBBALL"); !ok || u.(string) != "duke.kimm" {
		t.Fatal("게이트 저장 실패")
	}
	// 다른 user 는 게이트 miss 여야 함 (docs/{user} 브랜치 불일치 방지)
	if u, ok := s.alignedRepos.Load("NBBALL"); ok && u.(string) == "batman" {
		t.Error("user 전환인데 게이트 hit — docs 브랜치가 이전 사용자로 남는다")
	}
}
