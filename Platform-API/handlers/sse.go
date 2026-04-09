package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
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

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// GET /Notice/ws — WebSocket 연결 (SSE 대체)
func NoticeWSHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade failed: %v", err)
		return
	}

	ch := make(chan sseMessage, 10)
	sseMu.Lock()
	sseClients[ch] = true
	sseMu.Unlock()

	// connected 메시지 전송
	conn.WriteJSON(map[string]string{"event": "connected", "data": "ok"})

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
