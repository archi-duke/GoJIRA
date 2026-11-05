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
	mux.HandleFunc("POST /remove-clone", srv.secure(true, srv.handleRemoveClone))
	mux.HandleFunc("OPTIONS /remove-clone", srv.secure(true, srv.handleRemoveClone))
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
	mux.HandleFunc("GET /issues", srv.secure(true, srv.handleIssues))
	mux.HandleFunc("OPTIONS /issues", srv.secure(true, srv.handleIssues))
	mux.HandleFunc("POST /issue-journal", srv.secure(true, srv.handleIssueJournal))
	mux.HandleFunc("OPTIONS /issue-journal", srv.secure(true, srv.handleIssueJournal))
	mux.HandleFunc("POST /shutdown", srv.secure(true, srv.handleShutdown))

	// ── 신규 기능 경로 (설계 §5, §11) ──────────────────────────────────────
	// 기존 경로(위)는 **호환 별칭**으로 그대로 둔다(D7) — 웹을 일괄 수정하지 않아도 되고
	// 동작이 변하지 않는다. 아래 신규 경로는 같은 핸들러를 재사용하며, 경로 변수 {code}
	// 와 X-Launcher-User 헤더를 핸들러가 읽는 자리(POST=바디, GET=쿼리)에 넣어준다.
	//
	// 파라미터 위치는 기존 핸들러에 맞췄다 — 설계 §5 는 file 삭제를 `?path=` 쿼리로
	// 적었으나 현행 handleDelete 는 바디를 읽는다. 3단계에서 gojira-workspace 로
	// 떼어낼 때 정리한다.
	for _, m := range []string{"GET", "OPTIONS"} {
		mux.HandleFunc(m+" /launcher/health", srv.secure(false, srv.handleHealth))
	}
	mux.HandleFunc("POST /launcher/shutdown", srv.secure(true, srv.handleShutdown))

	// /v1/workspace/* — Git 작업본 (②)
	mux.HandleFunc("GET /v1/workspace/{code}/status", srv.secure(true, srv.withQueryCode(srv.handleStatus)))
	mux.HandleFunc("GET /v1/workspace/{code}/files", srv.secure(true, srv.withQueryCode(srv.handleFiles)))
	mux.HandleFunc("GET /v1/workspace/{code}/file", srv.secure(true, srv.withQueryCode(srv.handleFile)))
	mux.HandleFunc("PUT /v1/workspace/{code}/file", srv.secure(true, srv.withBodyCode(srv.handleWrite)))
	mux.HandleFunc("DELETE /v1/workspace/{code}/file", srv.secure(true, srv.withBodyCode(srv.handleDelete)))
	mux.HandleFunc("POST /v1/workspace/{code}/rename", srv.secure(true, srv.withBodyCode(srv.handleRename)))
	mux.HandleFunc("POST /v1/workspace/{code}/sync", srv.secure(true, srv.withBodyCode(srv.handleSync)))
	mux.HandleFunc("POST /v1/workspace/{code}/push", srv.secure(true, srv.withBodyCode(srv.handlePush)))
	mux.HandleFunc("POST /v1/workspace/{code}/pull", srv.secure(true, srv.withBodyCode(srv.handlePull)))
	mux.HandleFunc("POST /v1/workspace/{code}/open", srv.secure(true, srv.withBodyCode(srv.handleOpen)))
	mux.HandleFunc("POST /v1/workspace/{code}/prewarm", srv.secure(true, srv.withBodyCode(srv.handlePrewarm)))
	mux.HandleFunc("DELETE /v1/workspace/{code}", srv.secure(true, srv.withBodyCode(srv.handleRemoveClone)))

	// /v1/agents/* — 에이전트 소환 (①)
	mux.HandleFunc("POST /v1/agents/chat", srv.secure(true, srv.withBodyCode(srv.handleChat)))
	mux.HandleFunc("POST /v1/agents/edit", srv.secure(true, srv.withBodyCode(srv.handleEdit)))
	mux.HandleFunc("GET /v1/agents/sessions", srv.secure(true, srv.withQueryCode(srv.handleSessions)))

	// /v1/issues/* — 이슈 (④)
	mux.HandleFunc("GET /v1/issues/{code}", srv.secure(true, srv.withQueryCode(srv.handleIssues)))
	mux.HandleFunc("POST /v1/issues/{code}/journal", srv.secure(true, srv.withBodyCode(srv.handleIssueJournal)))

	// CORS preflight — 신규 경로도 기존과 같은 처리를 받아야 한다.
	for _, p := range []string{
		"/v1/workspace/{code}/status", "/v1/workspace/{code}/files", "/v1/workspace/{code}/file",
		"/v1/workspace/{code}/rename", "/v1/workspace/{code}/sync", "/v1/workspace/{code}/push",
		"/v1/workspace/{code}/pull", "/v1/workspace/{code}/open", "/v1/workspace/{code}/prewarm",
		"/v1/workspace/{code}", "/v1/agents/chat", "/v1/agents/edit", "/v1/agents/sessions",
		"/v1/issues/{code}", "/v1/issues/{code}/journal",
	} {
		mux.HandleFunc("OPTIONS "+p, srv.secure(true, func(w http.ResponseWriter, r *http.Request) {}))
	}

	// 등록되지 않은 경로 — 외부 서비스 라우팅 대상이거나 오타다(§7.2).
	mux.HandleFunc("/", srv.secure(true, srv.handleDynamicRoute))

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
