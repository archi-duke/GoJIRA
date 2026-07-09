package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCaseInsensitiveRouting(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"lowercase resource", "/api/v2/issue", "/api/v2/Issue"},
		{"already canonical", "/api/v2/Issue", "/api/v2/Issue"},
		{"uppercase sub-segment", "/api/v2/Issue/LINK", "/api/v2/Issue/link"},
		{"mixed-case sub-segment", "/api/v2/issue/Ingest", "/api/v2/Issue/ingest"},
		{"unknown resource untouched", "/api/v2/whatever", "/api/v2/whatever"},
		{
			// Project 의 다음 세그먼트는 {jiraCode} 실값이다 — 우연히 sub-segment 키워드와
			// 같은 프로젝트 코드가 와도 손상되면 안 된다.
			"project dynamic segment not corrupted",
			"/api/v2/project/link/dump",
			"/api/v2/Project/link/dump",
		},
		{"project resource canonicalized, code preserved", "/api/v2/project/DKDOC/dump", "/api/v2/Project/DKDOC/dump"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got string
			h := caseInsensitiveRouting(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.URL.Path
			}))
			req := httptest.NewRequest(http.MethodGet, c.in, nil)
			h.ServeHTTP(httptest.NewRecorder(), req)
			if got != c.want {
				t.Errorf("caseInsensitiveRouting(%q) path=%q, want %q", c.in, got, c.want)
			}
		})
	}
}
