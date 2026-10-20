package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// 사용자 PAT 조회 성공 — launcher.json 폴백이 아니라 본인 PAT 를 써야 한다.
// (이 전환의 목적: Gitea 상 pusher 가 공용 관리자 계정이 아닌 실제 작업자로 기록되는 것)
func TestUserGitCredsUsesPerUserToken(t *testing.T) {
	var gotUserHeader, gotAuthHeader, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotUserHeader = r.Header.Get("X-User-Id")
		gotAuthHeader = r.Header.Get("Authorization")
		fmt.Fprint(w, `{"login":"duke.kimm","token":"usertok123"}`)
	}))
	defer srv.Close()

	s := newCredServer(srv.URL, "adminuser", "admintok")
	s.setSSOToken("sess-abc")

	login, token := s.userGitCreds("duke.kimm")
	if login != "duke.kimm" || token != "usertok123" {
		t.Fatalf("사용자 PAT 미사용: login=%q token=%q", login, token)
	}
	if gotPath != "/Account/me/gitCredentials" {
		t.Errorf("엔드포인트 경로 = %q", gotPath)
	}
	if gotUserHeader != "duke.kimm" {
		t.Errorf("X-User-Id = %q", gotUserHeader)
	}
	if gotAuthHeader != "Bearer sess-abc" {
		t.Errorf("Authorization = %q — SSO 세션토큰이 실려야 한다", gotAuthHeader)
	}
}

// 조회 결과는 캐시된다 — git 명령마다 HTTP 가 나가면 사내망에서 지연이 누적된다.
func TestUserGitCredsCaches(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		fmt.Fprint(w, `{"login":"duke.kimm","token":"usertok123"}`)
	}))
	defer srv.Close()

	s := newCredServer(srv.URL, "", "")
	for i := 0; i < 5; i++ {
		if _, token := s.userGitCreds("duke.kimm"); token != "usertok123" {
			t.Fatalf("%d 회차 token=%q", i, token)
		}
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Errorf("백엔드 호출 %d 회 — 1 회여야 한다(캐시 미동작)", n)
	}
}

// 백엔드에 PAT 가 없으면(404) launcher.json 설정으로 폴백한다 — 기존 환경 호환.
// 실패분도 캐시해야 매 git 호출마다 재시도하지 않는다.
func TestUserGitCredsFallsBackAndCachesNegative(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		http.Error(w, "no token", http.StatusNotFound)
	}))
	defer srv.Close()

	s := newCredServer(srv.URL, "fallbackuser", "fallbacktok")
	for i := 0; i < 3; i++ {
		login, token := s.userGitCreds("duke.kimm")
		if login != "fallbackuser" || token != "fallbacktok" {
			t.Fatalf("%d 회차 폴백 실패: login=%q token=%q", i, login, token)
		}
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Errorf("백엔드 호출 %d 회 — 네거티브 캐시가 동작해야 한다", n)
	}
}

// 캐시 만료 후에는 다시 조회한다 — 서버에서 PAT 를 재발급한 경우 반영되어야 한다.
func TestUserGitCredsRefetchesAfterExpiry(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		fmt.Fprintf(w, `{"login":"duke.kimm","token":"tok%d"}`, n)
	}))
	defer srv.Close()

	s := newCredServer(srv.URL, "", "")
	if _, token := s.userGitCreds("duke.kimm"); token != "tok1" {
		t.Fatalf("1 회차 token=%q", token)
	}
	// 캐시를 만료 상태로 바꿔치기 (시간 대기 없이 만료 경로를 태운다)
	s.gitCreds.Store("duke.kimm", gitCred{login: "duke.kimm", token: "tok1", expires: time.Now().Add(-time.Second)})
	if _, token := s.userGitCreds("duke.kimm"); token != "tok2" {
		t.Errorf("만료 후 재조회 실패 — token=%q", token)
	}
}

// 사용자별로 각자의 PAT 를 받아야 한다 — 캐시 키가 user 여야 한다.
func TestUserGitCredsPerUserIsolation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := r.Header.Get("X-User-Id")
		fmt.Fprintf(w, `{"login":%q,"token":"tok-%s"}`, u, u)
	}))
	defer srv.Close()

	s := newCredServer(srv.URL, "", "")
	if _, token := s.userGitCreds("duke.kimm"); token != "tok-duke.kimm" {
		t.Fatalf("duke.kimm token=%q", token)
	}
	if login, token := s.userGitCreds("batman"); token != "tok-batman" || login != "batman" {
		t.Errorf("batman login=%q token=%q — 사용자별로 분리돼야 한다", login, token)
	}
}

// user 가 비면(신원 불명) 백엔드를 호출하지 않고 설정 폴백만 쓴다.
func TestUserGitCredsEmptyUserSkipsBackend(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		fmt.Fprint(w, `{"login":"x","token":"y"}`)
	}))
	defer srv.Close()

	s := newCredServer(srv.URL, "fallbackuser", "fallbacktok")
	login, token := s.userGitCreds("")
	if login != "fallbackuser" || token != "fallbacktok" {
		t.Errorf("빈 user 폴백 실패: login=%q token=%q", login, token)
	}
	if n := atomic.LoadInt32(&calls); n != 0 {
		t.Errorf("빈 user 로 백엔드 호출 %d 회 — 호출하지 않아야 한다", n)
	}
}

// clone URL 에 사용자 PAT 가 임베드되어야 한다(그리고 특수문자는 escape).
func TestCloneURLEmbedsUserToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"login":"duke.kimm","token":"tok/with+special"}`)
	}))
	defer srv.Close()

	s := newCredServer(srv.URL, "", "")
	s.cfg.Git.Base = "http://gitea-host:3300"
	s.cfg.Git.Org = "gojira"

	got := s.cloneURL("number-baseball", "duke.kimm")
	if !strings.HasPrefix(got, "http://duke.kimm:") {
		t.Errorf("사용자 자격 미임베드: %q", got)
	}
	if strings.Contains(got, "tok/with+special") {
		t.Errorf("특수문자가 escape 되지 않았다: %q", got)
	}
	if !strings.HasSuffix(got, "@gitea-host:3300/gojira/number-baseball.git") {
		t.Errorf("URL 꼬리 형태 이상: %q", got)
	}
}

// newCredServer — 테스트용 Server. platformApi 를 stub 으로, git 폴백 자격을 지정.
func newCredServer(platformAPI, fallbackUser, fallbackToken string) *Server {
	return &Server{cfg: &Config{
		Backend: Backend{PlatformAPI: platformAPI},
		Git:     Git{User: fallbackUser, Token: fallbackToken},
	}}
}
