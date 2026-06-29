// gojira-launcher — 웹→로컬 소환 전용 경량 localhost 데몬 (설계: docs/Long-term
// Considerations/local-agent-mvp.md §2). agent-service 부활이 아니라, 웹의 "수정"
// 버튼이 사용자 PC 에서 VS Code + (opencode|claude) 를 띄우도록 중계할 뿐이다.
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	// 서브커맨드(install/uninstall/version/update-finalize) 우선 처리.
	// update-finalize 는 정리 후 데몬으로 계속 진행하므로 shouldExit=false.
	if runSubcommand(os.Args[1:]) {
		return
	}

	// 이전 업데이트 잔여(.old/.new) 정리(베스트에포트).
	cleanupStaleArtifacts()

	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("설정 로드 실패: %v", err)
	}
	srv := &Server{cfg: cfg, sessions: newSessionRegistry()}

	// 임베드 스킬을 추출(첫 기동/버전 변경 시만 실제 쓰기). 실패는 비치명적.
	if err := ExtractSkills(cfg.SkillsSourceDir, version, false); err != nil {
		log.Printf("스킬 추출 실패(비치명적): %v", err)
	}

	// 의존성(jq, python) 상태 1회 로깅(비치명적) — 확보는 install 이 담당.
	logDepsStatus()

	// 자동 업데이트 — 기동 시 1회 + 주기 체크(백그라운드).
	srv.startUpdater()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", srv.secure(false, srv.handleHealth))
	mux.HandleFunc("OPTIONS /health", srv.secure(false, srv.handleHealth))
	mux.HandleFunc("POST /open", srv.secure(true, srv.handleOpen))
	mux.HandleFunc("OPTIONS /open", srv.secure(true, srv.handleOpen))
	mux.HandleFunc("POST /edit", srv.secure(true, srv.handleEdit))
	mux.HandleFunc("OPTIONS /edit", srv.secure(true, srv.handleEdit))
	mux.HandleFunc("POST /chat", srv.secure(true, srv.handleChat))
	mux.HandleFunc("OPTIONS /chat", srv.secure(true, srv.handleChat))
	mux.HandleFunc("POST /prewarm", srv.secure(true, srv.handlePrewarm))
	mux.HandleFunc("OPTIONS /prewarm", srv.secure(true, srv.handlePrewarm))
	mux.HandleFunc("POST /push", srv.secure(true, srv.handlePush))
	mux.HandleFunc("OPTIONS /push", srv.secure(true, srv.handlePush))
	mux.HandleFunc("POST /pull", srv.secure(true, srv.handlePull))
	mux.HandleFunc("OPTIONS /pull", srv.secure(true, srv.handlePull))
	mux.HandleFunc("POST /sync", srv.secure(true, srv.handleSync))
	mux.HandleFunc("OPTIONS /sync", srv.secure(true, srv.handleSync))
	mux.HandleFunc("POST /write", srv.secure(true, srv.handleWrite))
	mux.HandleFunc("OPTIONS /write", srv.secure(true, srv.handleWrite))
	mux.HandleFunc("POST /delete", srv.secure(true, srv.handleDelete))
	mux.HandleFunc("OPTIONS /delete", srv.secure(true, srv.handleDelete))
	mux.HandleFunc("POST /rename", srv.secure(true, srv.handleRename))
	mux.HandleFunc("OPTIONS /rename", srv.secure(true, srv.handleRename))
	mux.HandleFunc("GET /status", srv.secure(true, srv.handleStatus))
	mux.HandleFunc("OPTIONS /status", srv.secure(true, srv.handleStatus))
	mux.HandleFunc("GET /sessions", srv.secure(true, srv.handleSessions))
	mux.HandleFunc("OPTIONS /sessions", srv.secure(true, srv.handleSessions))
	mux.HandleFunc("GET /files", srv.secure(true, srv.handleFiles))
	mux.HandleFunc("OPTIONS /files", srv.secure(true, srv.handleFiles))
	mux.HandleFunc("GET /file", srv.secure(true, srv.handleFile))
	mux.HandleFunc("OPTIONS /file", srv.secure(true, srv.handleFile))

	// 127.0.0.1 전용 bind — 외부 인터페이스 노출 금지 (§2.3).
	addr := fmt.Sprintf("127.0.0.1:%d", cfg.Port)
	log.Printf("gojira-launcher v%s — http://%s", version, addr)
	log.Printf("  defaultAgent=%s repoRoot=%s webOrigin=%s webOrigins=%v", cfg.DefaultAgent, cfg.RepoRoot, cfg.WebOrigin, cfg.WebOrigins)
	log.Printf("  설치 토큰: ~/.gojira/%s (웹은 %s 헤더로 전송)", tokenFileName, tokenHeader)
	log.Printf("  %s", cfg.gitAuthSummary())

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("서버 종료: %v", err)
	}
}
