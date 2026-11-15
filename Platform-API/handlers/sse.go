package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"platform-api/auth"
)

type sseMessage struct {
	Event string
	Data  string
}

// SSE 브로드캐스터
var (
	sseClients = make(map[chan sseMessage]bool)
	sseMu      sync.Mutex
)

// GET /Notice/stream — SSE 연결 (공지 + 문서 이벤트 공용)
func NoticeStreamHandler(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := make(chan sseMessage, 10)
	sseMu.Lock()
	sseClients[ch] = true
	sseMu.Unlock()

	defer func() {
		sseMu.Lock()
		delete(sseClients, ch)
		sseMu.Unlock()
		close(ch)
	}()

	fmt.Fprintf(w, "event: connected\ndata: ok\n\n")
	flusher.Flush()

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", msg.Event, msg.Data)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func broadcast(event string, data interface{}) {
	b, _ := json.Marshal(data)
	msg := sseMessage{Event: event, Data: string(b)}

	sseMu.Lock()
	defer sseMu.Unlock()
	sent := 0
	for ch := range sseClients {
		select {
		case ch <- msg:
			sent++
		default:
		}
	}
	log.Printf("SSE broadcast: event=%s clients=%d sent=%d", event, len(sseClients), sent)
}

// --- WebSocket 지원 ---

// writeWSClose — 사유를 한 번 보내고 닫는다. 클라이언트가 재연결 전에 원인을 알 수 있게.
func writeWSClose(conn *websocket.Conn, reason string) {
	_ = conn.WriteJSON(map[string]string{"event": "error", "data": reason})
	_ = conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.ClosePolicyViolation, reason),
		time.Now().Add(time.Second),
	)
	conn.Close()
}

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// wsAuthTimeout — 연결 후 이 시간 안에 auth 메시지가 오지 않으면 닫는다.
// 인증되지 않은 연결이 쌓이는 것을 막는다.
const wsAuthTimeout = 10 * time.Second

// wsAuthMessage — 클라이언트가 연결 직후 보내는 첫 메시지.
type wsAuthMessage struct {
	Type  string `json:"type"`  // "auth"
	Token string `json:"token"` // 세션 토큰 (dev 에서는 빈 값 허용)
	User  string `json:"user"`  // dev 폴백 사용자 (USE_SSO!=true 에서만 쓰인다)
}

// GET /Notice/ws — WebSocket 연결 (SSE 대체)
//
// 인증: 핸드셰이크는 미들웨어를 통과하고(브라우저 WS 는 Authorization 헤더를 실을 수
// 없다), **연결 직후 첫 메시지로 받은 토큰을 여기서 검증**한다. 인증 전에는
// 브로드캐스트 대상에 등록하지 않으므로 이벤트가 새지 않는다.
// 계약 05 §WebSocket 인증 · 2026-07-07 D2 (쿼리파라미터는 액세스 로그 노출로 기각).
func NoticeWSHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade failed: %v", err)
		return
	}

	// ── 첫 메시지 인증. 통과할 때까지 아무것도 등록·전송하지 않는다.
	if err := conn.SetReadDeadline(time.Now().Add(wsAuthTimeout)); err != nil {
		conn.Close()
		return
	}
	var am wsAuthMessage
	if err := conn.ReadJSON(&am); err != nil {
		log.Printf("ws auth: 첫 메시지 수신 실패: %v", err)
		writeWSClose(conn, "auth required")
		return
	}
	if am.Type != "auth" {
		log.Printf("ws auth: 첫 메시지 type=%q (auth 여야 함)", am.Type)
		writeWSClose(conn, "first message must be {type:\"auth\"}")
		return
	}
	user, err := auth.AuthenticateToken(am.Token, am.User)
	if err != nil {
		log.Printf("ws auth 실패: %v", err)
		writeWSClose(conn, "unauthorized")
		return
	}
	// 읽기 기한 해제 — 이후는 ping/pong 과 종료 감지에만 쓴다.
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		conn.Close()
		return
	}

	ch := make(chan sseMessage, 10)
	sseMu.Lock()
	sseClients[ch] = true
	sseMu.Unlock()

	// connected 메시지 전송 — 인증된 사용자를 함께 알려 클라이언트가 확인할 수 있게 한다.
	conn.WriteJSON(map[string]string{"event": "connected", "data": "ok", "user": user})

	// 클라이언트 → 서버 메시지 읽기 (ping/pong 유지용)
	go func() {
		defer func() {
			sseMu.Lock()
			delete(sseClients, ch)
			sseMu.Unlock()
			close(ch)
			conn.Close()
		}()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	// 서버 → 클라이언트 브로드캐스트 전달
	for msg := range ch {
		payload := map[string]string{"event": msg.Event, "data": msg.Data}
		if err := conn.WriteJSON(payload); err != nil {
			return
		}
	}
}

// BroadcastEmergencyNotice 비상 공지 SSE
func BroadcastEmergencyNotice(notice map[string]interface{}) {
	broadcast("emergency", notice)
}

// BroadcastDocEvent 문서 관련 SSE (PR 상태 변경, push 등)
func BroadcastDocEvent(event map[string]interface{}) {
	broadcast("doc_update", event)
}
