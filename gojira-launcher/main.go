// gojira-launcher — 웹→로컬 소환 전용 경량 localhost 데몬 (설계: docs/Long-term
// Considerations/local-agent-mvp.md §2). agent-service 부활이 아니라, 웹의 "수정"
// 버튼이 사용자 PC 에서 VS Code + (opencode|claude) 를 띄우도록 중계할 뿐이다.
package main

import (
	"fmt"
	"log"
	"net/http"
)

func main() {
	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("설정 로드 실패: %v", err)
	}
	srv := &Server{cfg: cfg}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", srv.secure(false, srv.handleHealth))
	mux.HandleFunc("OPTIONS /health", srv.secure(false, srv.handleHealth))
	mux.HandleFunc("POST /open", srv.secure(true, srv.handleOpen))
	mux.HandleFunc("OPTIONS /open", srv.secure(true, srv.handleOpen))
	mux.HandleFunc("POST /edit", srv.secure(true, srv.handleEdit))
	mux.HandleFunc("OPTIONS /edit", srv.secure(true, srv.handleEdit))
	mux.HandleFunc("GET /status", srv.secure(true, srv.handleStatus))
	mux.HandleFunc("OPTIONS /status", srv.secure(true, srv.handleStatus))

	// 127.0.0.1 전용 bind — 외부 인터페이스 노출 금지 (§2.3).
	addr := fmt.Sprintf("127.0.0.1:%d", cfg.Port)
	log.Printf("gojira-launcher v%s — http://%s", version, addr)
	log.Printf("  defaultAgent=%s repoRoot=%s webOrigin=%s", cfg.DefaultAgent, cfg.RepoRoot, cfg.WebOrigin)
	log.Printf("  설치 토큰: ~/.gojira/%s (웹은 %s 헤더로 전송)", tokenFileName, tokenHeader)

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("서버 종료: %v", err)
	}
}
