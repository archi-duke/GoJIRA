package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 세그먼트 경계 매칭 — /v1/docs/pptx 는 /v1/docs/pptxx 를 먹지 않아야 한다(§7.2).
func TestRouteMatches(t *testing.T) {
	rt := Route{Prefix: "/v1/docs/pptx"}
	cases := map[string]bool{
		"/v1/docs/pptx":        true,
		"/v1/docs/pptx/text":   true,
		"/v1/docs/pptx/a/b":    true,
		"/v1/docs/pptxx":       false,
		"/v1/docs/pptxx/text":  false,
		"/v1/docs":             false,
		"/v1/docs/docx/text":   false,
	}
	for path, want := range cases {
		if got := rt.matches(path); got != want {
			t.Errorf("matches(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestRouteAllowsMethod(t *testing.T) {
	if !(Route{}).allowsMethod("DELETE") {
		t.Error("Methods 가 비면 전체 허용이어야 한다")
	}
	rt := Route{Methods: []string{"POST"}}
	if !rt.allowsMethod("post") {
		t.Error("메서드 비교는 대소문자 무시여야 한다")
	}
	if rt.allowsMethod("GET") {
		t.Error("선언하지 않은 메서드를 허용했다")
	}
}

// 라우팅 표는 최장 prefix 우선이어야 한다 — 중첩 시 더 구체적인 쪽이 먼저 잡힌다.
func TestRouteTableLongestFirst(t *testing.T) {
	s := &Server{}
	tbl := s.routeTable()
	if len(tbl) < 2 {
		t.Skip("내장 라우트가 2개 미만")
	}
	for i := 1; i < len(tbl); i++ {
		if len(tbl[i-1].route.Prefix) < len(tbl[i].route.Prefix) {
			t.Fatalf("정렬 위반: %q 뒤에 더 긴 %q", tbl[i-1].route.Prefix, tbl[i].route.Prefix)
		}
	}
}

func TestHandleDynamicRoute(t *testing.T) {
	s := &Server{}
	call := func(method, path string) (int, map[string]any) {
		rec := httptest.NewRecorder()
		s.handleDynamicRoute(rec, httptest.NewRequest(method, path, nil))
		var body map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		return rec.Code, body
	}

	t.Run("미등록 경로는 404 no_route", func(t *testing.T) {
		st, b := call("GET", "/v1/docs/pptx/text")
		if st != http.StatusNotFound || b["code"] != "no_route" {
			t.Fatalf("status=%d body=%v", st, b)
		}
	})

	t.Run("런처 예약 이름공간은 서비스로 넘기지 않는다", func(t *testing.T) {
		st, b := call("GET", "/launcher/nope")
		if st != http.StatusNotFound || b["code"] != "no_route" {
			t.Fatalf("status=%d body=%v", st, b)
		}
	})

	t.Run("내장 서비스의 미등록 하위 경로도 404", func(t *testing.T) {
		st, b := call("GET", "/v1/workspace/ABC/nonexistent")
		if st != http.StatusNotFound || b["code"] != "no_route" {
			t.Fatalf("status=%d body=%v", st, b)
		}
	})

	t.Run("선언한 메서드가 아니면 405", func(t *testing.T) {
		st, b := call("PATCH", "/v1/agents/chat")
		if st != http.StatusMethodNotAllowed || b["code"] != "method_not_allowed" {
			t.Fatalf("status=%d body=%v", st, b)
		}
	})
}

// 어댑터 — 경로 {code}/헤더를 핸들러가 읽는 자리에 넣되, 이미 있는 값은 덮지 않는다.
func TestWithBodyCode(t *testing.T) {
	s := &Server{}
	var got map[string]any
	inner := func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got)
	}

	run := func(path, body, userHeader string) {
		got = nil
		mux := http.NewServeMux()
		mux.HandleFunc("POST /v1/workspace/{code}/sync", s.withBodyCode(inner))
		req := httptest.NewRequest("POST", path, strings.NewReader(body))
		if userHeader != "" {
			req.Header.Set(launcherUserHeader, userHeader)
		}
		mux.ServeHTTP(httptest.NewRecorder(), req)
	}

	t.Run("경로 code 와 헤더 user 주입", func(t *testing.T) {
		run("/v1/workspace/NBBALL/sync", `{"message":"hi"}`, "duke.kimm")
		if got["code"] != "NBBALL" || got["user"] != "duke.kimm" || got["message"] != "hi" {
			t.Fatalf("주입 실패: %v", got)
		}
	})

	t.Run("빈 바디도 처리", func(t *testing.T) {
		run("/v1/workspace/ABC/sync", "", "u1")
		if got["code"] != "ABC" || got["user"] != "u1" {
			t.Fatalf("빈 바디 주입 실패: %v", got)
		}
	})

	t.Run("바디에 이미 있으면 덮어쓰지 않는다", func(t *testing.T) {
		run("/v1/workspace/PATHCODE/sync", `{"code":"BODYCODE","user":"bodyuser"}`, "headeruser")
		if got["code"] != "BODYCODE" || got["user"] != "bodyuser" {
			t.Fatalf("기존 값을 덮었다: %v", got)
		}
	})
}

func TestWithQueryCode(t *testing.T) {
	s := &Server{}
	var gotCode, gotUser, gotPath string
	inner := func(w http.ResponseWriter, r *http.Request) {
		gotCode = r.URL.Query().Get("code")
		gotUser = r.URL.Query().Get("user")
		gotPath = r.URL.Query().Get("path")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/workspace/{code}/file", s.withQueryCode(inner))

	req := httptest.NewRequest("GET", "/v1/workspace/NBBALL/file?path=a/b.md", nil)
	req.Header.Set(launcherUserHeader, "duke.kimm")
	mux.ServeHTTP(httptest.NewRecorder(), req)

	if gotCode != "NBBALL" || gotUser != "duke.kimm" || gotPath != "a/b.md" {
		t.Fatalf("code=%q user=%q path=%q", gotCode, gotUser, gotPath)
	}
}
