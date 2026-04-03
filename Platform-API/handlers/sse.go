package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
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
	for ch := range sseClients {
		select {
		case ch <- msg:
		default:
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
