package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

// SSE 브로드캐스터
var (
	sseClients   = make(map[chan string]bool)
	sseMu        sync.Mutex
)

// GET /Notice/stream — SSE 연결
func NoticeStreamHandler(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := make(chan string, 10)
	sseMu.Lock()
	sseClients[ch] = true
	sseMu.Unlock()

	defer func() {
		sseMu.Lock()
		delete(sseClients, ch)
		sseMu.Unlock()
		close(ch)
	}()

	// 연결 확인 이벤트
	fmt.Fprintf(w, "event: connected\ndata: ok\n\n")
	flusher.Flush()

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "event: emergency\ndata: %s\n\n", msg)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// BroadcastEmergencyNotice 모든 SSE 클라이언트에 비상 공지 전송
func BroadcastEmergencyNotice(notice map[string]interface{}) {
	data, _ := json.Marshal(notice)
	msg := string(data)

	sseMu.Lock()
	defer sseMu.Unlock()
	for ch := range sseClients {
		select {
		case ch <- msg:
		default: // 버퍼 초과 시 skip
		}
	}
}
