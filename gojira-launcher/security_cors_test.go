package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// 서비스 경로 계약 헤더가 preflight 허용 목록에 없으면 브라우저가 요청 자체를 막는다.
func TestWriteCORSAllowsServiceContractHeaders(t *testing.T) {
	s := &Server{cfg: &Config{}}
	w := httptest.NewRecorder()
	s.writeCORS(w, "http://localhost:3002")

	got := strings.ToLower(w.Header().Get("Access-Control-Allow-Headers"))
	for _, h := range []string{"content-type", "x-gojira-launcher-token", "x-file-name"} {
		if !strings.Contains(got, h) {
			t.Errorf("Access-Control-Allow-Headers 에 %q 없음: %q", h, got)
		}
	}
}
