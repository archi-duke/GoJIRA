package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// WebSocket 인증 — 헤더를 실을 수 없는 연결이 첫 메시지로 인증한다.
//
//	실행: MONGO_URI=x PLATFORM_GITHUB_TOKEN=x PLATFORM_GITHUB_ORG=x \
//	      GITHUB_URL=http://x/ GOJIRA_API_URL=x go test ./handlers -run TestNoticeWSAuth
func TestNoticeWSAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(NoticeWSHandler))
	defer srv.Close()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

	dial := func(t *testing.T) *websocket.Conn {
		t.Helper()
		c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("dial 실패: %v", err)
		}
		return c
	}

	t.Run("핸드셰이크는 헤더 없이도 통과한다", func(t *testing.T) {
		c := dial(t) // 여기서 실패하면 미들웨어 예외가 빠진 것
		c.Close()
	})

	t.Run("auth 메시지를 보내면 connected 를 받는다", func(t *testing.T) {
		t.Setenv("USE_SSO", "false") // dev 분기 — introspect 없이 통과
		c := dial(t)
		defer c.Close()
		if err := c.WriteJSON(map[string]string{"type": "auth", "token": "", "user": "tester"}); err != nil {
			t.Fatal(err)
		}
		c.SetReadDeadline(time.Now().Add(5 * time.Second))
		var got map[string]string
		if err := c.ReadJSON(&got); err != nil {
			t.Fatalf("connected 수신 실패: %v", err)
		}
		if got["event"] != "connected" {
			t.Fatalf("event=%q, want connected (got=%v)", got["event"], got)
		}
		if got["user"] != "tester" {
			t.Errorf("user=%q, want tester — dev 폴백 사용자가 반영되지 않았다", got["user"])
		}
	})

	t.Run("type 이 auth 가 아니면 거부한다", func(t *testing.T) {
		c := dial(t)
		defer c.Close()
		if err := c.WriteJSON(map[string]string{"type": "subscribe"}); err != nil {
			t.Fatal(err)
		}
		c.SetReadDeadline(time.Now().Add(5 * time.Second))
		var got map[string]string
		if err := c.ReadJSON(&got); err != nil {
			t.Fatalf("error 수신 실패: %v", err)
		}
		if got["event"] != "error" {
			t.Fatalf("event=%q, want error", got["event"])
		}
	})

	t.Run("SSO 활성 + 토큰 없음 = 거부", func(t *testing.T) {
		t.Setenv("USE_SSO", "true") // introspect 로 가기 전에 빈 토큰에서 걸러야 한다
		c := dial(t)
		defer c.Close()
		if err := c.WriteJSON(map[string]string{"type": "auth", "token": ""}); err != nil {
			t.Fatal(err)
		}
		c.SetReadDeadline(time.Now().Add(5 * time.Second))
		var got map[string]string
		if err := c.ReadJSON(&got); err != nil {
			t.Fatalf("error 수신 실패: %v", err)
		}
		if got["event"] != "error" {
			t.Fatalf("event=%q, want error — 인증 없이 연결이 열렸다", got["event"])
		}
	})
}
