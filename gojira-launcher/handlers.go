package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const version = "0.1.34"

// Server — 핸들러가 공유하는 설정 보관.
type Server struct {
	cfg      *Config
	sessions *sessionRegistry // 과제별 에이전트 세션 추적(Design B). session.go.

	// SSO 세션토큰(Phase 4 C) — 웹이 X-Gojira-Session 헤더로 매 요청 보낸다(secure 가 캡처).
	// 런처의 백엔드 호출(ingest/resolveRepoName)에 Bearer 로 쓰고 에이전트 env 로 주입한다.
	// 단일 사용자 로컬 데몬이라 "마지막 값" 보관으로 충분(함수 시그니처 threading 회피).
	ssoMu  sync.Mutex
	ssoTok string

	// alignedRepos — repoKey(code,user) → 정렬(origin URL·branch checkout·identity·assets)
	// 완료 표시. no-fetch 조회 경로(/files 등)는 이미 정렬된 (code,user) 면 정렬을 통째로 건너뛴다.
	// Windows 사내망에서 git 프로세스 기동이 spawn 당 0.5s+ 로 느려, 매 /files 마다 반복되던
	// 정렬 서브프로세스들이 체감 지연의 주범이었다. clone 이 사용자별 경로라 키도 (code,user) 단위.
	alignedRepos sync.Map

	// gitCreds — user(knoxId) → gitCred. Gitea 자격을 사용자별로 캐시한다(gitcreds.go).
	// 이전처럼 launcher.json 의 단일 PAT 를 쓰지 않고 요청 사용자 본인의 PAT 로
	// clone/fetch/push 하기 위한 것 — Gitea 상 pusher 가 실제 작업자로 기록된다.
	gitCreds sync.Map
}

// setSSOToken/ssoToken — 웹이 보낸 SSO 세션토큰 캡처/조회.
func (s *Server) setSSOToken(t string) {
	if t == "" {
		return
	}
	s.ssoMu.Lock()
	s.ssoTok = t
	s.ssoMu.Unlock()
}

func (s *Server) ssoToken() string {
	s.ssoMu.Lock()
	defer s.ssoMu.Unlock()
	return s.ssoTok
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"status": "error", "error": msg})
}

// GET /health — 웹이 런처 설치/기동을 감지해 "수정" 버튼을 활성화 (§2.1).
//
// 토큰 핸드셰이크(설계 §6): Origin 이 webOrigin 과 일치하는 요청에만 응답에 token 을
// 포함한다. 브라우저는 Origin 을 위조할 수 없으므로 GoJIRA 웹만 토큰을 읽어 이후
// /edit·/open 호출에 사용한다. (DNS-rebinding 은 잔여 리스크 — §6)
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	agents := availableAgents()
	resp := map[string]any{
		"version": version,
		"agents":  agents,
		"ready":   hasGit() && hasEditor() && len(agents) > 0,
	}
	if s.originAllowed(r.Header.Get("Origin")) {
		resp["token"] = s.cfg.Token
	}
	writeJSON(w, http.StatusOK, resp)
}

// POST /shutdown — 실행 중인 데몬을 정상 종료시킨다. `install`(doInstall) 이 launcher.json
// 을 새로 쓴 뒤, 이미 떠 있는 구 데몬을 내리고 새 프로세스가 그 포트를 이어받게 하는 용도
// (설정은 기동 시 1회만 읽으므로, 재시작 없이는 손편집/재설치 값이 절대 반영되지 않았던
// 문제의 근본 수정). 토큰 필요 — 외부에서 임의로 데몬을 죽이지 못하게.
func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "shutting down"})
	go func() {
		time.Sleep(200 * time.Millisecond) // 응답 flush 시간 확보
		os.Exit(0)
	}()
}

type openReq struct {
	Code string `json:"code"`
	User string `json:"user"`
}

// POST /open — clone 보장 + docs/{user} 체크아웃 + VS Code 오픈 (§2.1).
func (s *Server) handleOpen(w http.ResponseWriter, r *http.Request) {
	var req openReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "잘못된 JSON 본문")
		return
	}
	repoPath, err := s.EnsureRepo(req.Code, req.User)
	if err != nil {
		log.Printf("/open 실패 code=%s user=%s: %v", req.Code, req.User, err)
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := OpenInEditor(repoPath, ""); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "repoPath": repoPath})
}

type editReq struct {
	Code string `json:"code"`
	User string `json:"user"`
	File string `json:"file"`
}

// POST /edit — clone 보장 + ingest + VS Code(특정 file) 오픈. agent spawn 은 /chat 으로 분리.
func (s *Server) handleEdit(w http.ResponseWriter, r *http.Request) {
	var req editReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "잘못된 JSON 본문")
		return
	}
	repoPath, err := s.EnsureRepo(req.Code, req.User)
	if err != nil {
		log.Printf("/edit 실패 code=%s user=%s: %v", req.Code, req.User, err)
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	fullFile, err := safeFilePath(repoPath, req.File)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := OpenInEditor(repoPath, fullFile); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"repoPath": repoPath,
	})
}

type chatReq struct {
	Code    string `json:"code"`
	User    string `json:"user"`
	Agent   string `json:"agent"`
	Replace bool   `json:"replace"` // true 면 다른 과제의 살아있는 에이전트를 종료 후 spawn(과제전환).
}

type prewarmReq struct {
	Code string `json:"code"`
	User string `json:"user"`
}

// POST /prewarm — 과제 생성 직후 또는 메인 화면 진입 시 호출되어, EnsureRepo 를
// 백그라운드로 미리 수행한다(clone + docs/<user> 체크아웃). 이후 사용자가 로고를
// 클릭해 /chat 호출하는 시점에는 EnsureRepo 가 이미 끝나 있어 SpawnAgent 까지
// 체감 지연이 거의 없다. 비동기 — 즉시 200 응답 후 goroutine 에서 작업.
func (s *Server) handlePrewarm(w http.ResponseWriter, r *http.Request) {
	var req prewarmReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "잘못된 JSON 본문")
		return
	}
	if req.Code == "" || req.User == "" {
		writeErr(w, http.StatusBadRequest, "code, user 필수")
		return
	}
	log.Printf("[TEMP-DEBUG] handlePrewarm scheduling code=%q user=%q", req.Code, req.User)
	// fire-and-forget — 클라이언트는 응답을 기다리지 않고 즉시 반환.
	// 이미 락이 잡혀있으면 (다른 prewarm/chat 동시 호출) goroutine 안에서 대기 후 처리.
	go func() {
		if _, err := s.EnsureRepo(req.Code, req.User); err != nil {
			log.Printf("[TEMP-DEBUG] handlePrewarm FAIL code=%s user=%s: %v", req.Code, req.User, err)
		} else {
			log.Printf("[TEMP-DEBUG] handlePrewarm OK code=%s user=%s", req.Code, req.User)
		}
	}()
	writeJSON(w, http.StatusOK, map[string]any{"status": "started"})
}

// POST /chat — 등록된 에이전트 CLI(opencode|claude) 만 새 터미널로 spawn.
// /edit 과 분리된 동작 — 사용자는 VS Code 만 열거나, 그 위에 채팅을 추가로 띄울 수 있다.
// code 가 비어있으면 프로젝트 없는 일반 채팅 — repoRoot 에서 그대로 spawn (clone 안 함).
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	log.Printf("[TEMP-DEBUG] handleChat ENTRY")
	var req chatReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[TEMP-DEBUG] handleChat decode err=%v", err)
		writeErr(w, http.StatusBadRequest, "잘못된 JSON 본문")
		return
	}
	log.Printf("[TEMP-DEBUG] handleChat req code=%q user=%q agent=%q", req.Code, req.User, req.Agent)
	agent, err := s.resolveAgent(req.Agent)
	if err != nil {
		log.Printf("[TEMP-DEBUG] handleChat resolveAgent err=%v", err)
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	log.Printf("[TEMP-DEBUG] handleChat resolved agent=%q", agent)
	spawnDir := s.cfg.RepoRoot
	if req.Code != "" {
		// EnsureRepoLocal (no fetch) — 신규 과제는 게이트가 prewarm 으로 미리 clone+체크아웃
		// 시켜놓으므로 이 시점에는 fetch 가 불필요. fetch 를 skip 해 폐쇄망 RTT 만큼의
		// spawn 지연을 제거한다. 만약 polled clone 이 아직 없으면 (드물게) 자동 clone.
		// 사용자가 origin 의 최신 변경을 가져와야 한다면 별도 /pull 핸들러 사용.
		log.Printf("[TEMP-DEBUG] handleChat EnsureRepoLocal code=%q user=%q", req.Code, req.User)
		repoPath, err := s.EnsureRepoLocal(req.Code, req.User)
		if err != nil {
			log.Printf("[TEMP-DEBUG] handleChat EnsureRepoLocal FAIL code=%s user=%s: %v", req.Code, req.User, err)
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		log.Printf("[TEMP-DEBUG] handleChat EnsureRepoLocal OK repoPath=%q", repoPath)
		spawnDir = repoPath
	}
	// Design B 과제전환(replace): 다른 과제의 살아있는 에이전트를 먼저 트리종료한다.
	// 웹이 "과제 전환할까요?" 확인을 받은 뒤 replace=true 로 호출 → 옛 에이전트 닫고 새로 연다.
	if req.Replace {
		for _, other := range s.sessions.liveExcept(req.Code) {
			log.Printf("[TEMP-DEBUG] handleChat replace: closing code=%s pid=%d", other.Code, other.PID)
			if err := killProcessTree(other.PID); err != nil {
				log.Printf("[TEMP-DEBUG] handleChat replace kill 실패 code=%s pid=%d: %v", other.Code, other.PID, err)
			}
			s.sessions.markDead(other.Code, other.PID)
		}
	}
	// 같은 과제라도 다른 사용자 clone 에서 떠 있는 에이전트(사용자 전환)는 재사용하면 남의
	// 작업본에 쓰게 되므로 종료하고 이 사용자 clone 에서 새로 띄운다.
	if existing := s.sessions.live(req.Code); existing != nil && existing.Dir != spawnDir {
		log.Printf("handleChat 사용자 전환: code=%s 기존 에이전트(pid=%d, dir=%s) 종료", req.Code, existing.PID, existing.Dir)
		if err := killProcessTree(existing.PID); err != nil {
			log.Printf("handleChat 사용자 전환 kill 실패 code=%s pid=%d: %v", req.Code, existing.PID, err)
		}
		s.sessions.markDead(req.Code, existing.PID)
	}
	// Design B: 같은 과제에 이미 살아있는 에이전트가 있으면 중복 spawn 하지 않고 기존
	// 세션을 알려준다(과제별 1 에이전트). 사용자는 창 제목 "GoJIRA · CODE" 로 찾는다.
	// 다른 과제는 별개 세션이라 이 검사를 통과해 새로 뜬다(과제별 공존).
	if existing := s.sessions.live(req.Code); existing != nil {
		log.Printf("[TEMP-DEBUG] handleChat reuse code=%q pid=%d agent=%q", req.Code, existing.PID, existing.Agent)
		writeJSON(w, http.StatusOK, map[string]any{
			"status":    "already-running",
			"reused":    true,
			"repoPath":  spawnDir,
			"agent":     existing.Agent,
			"sessionId": fmt.Sprintf("%d", existing.PID),
			"label":     chatLabel(req.Code),
		})
		return
	}
	env := s.agentEnv(spawnDir, req.User, req.Code)
	label := chatLabel(req.Code)
	log.Printf("[TEMP-DEBUG] handleChat about to spawn agent=%q spawnDir=%q label=%q envCount=%d", agent, spawnDir, label, len(env))
	sessionID, err := s.SpawnAgent(agent, spawnDir, label, req.Code, env)
	if err != nil {
		log.Printf("[TEMP-DEBUG] handleChat SpawnAgent FAIL: %v", err)
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	log.Printf("[TEMP-DEBUG] handleChat SUCCESS sessionID=%s", sessionID)
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"reused":    false,
		"repoPath":  spawnDir,
		"agent":     agent,
		"sessionId": sessionID,
		"label":     label,
	})
}

// chatLabel — 에이전트 터미널 창 제목(Design B 과제별 구분).
func chatLabel(code string) string {
	if code == "" {
		return "GoJIRA · 일반 채팅"
	}
	return "GoJIRA · " + code
}

// GET /sessions — 현재 살아있는 에이전트 세션 목록(과제별). 웹이 과제전환 시 "다른 과제
// 에이전트가 떠 있는가" 를 판정해 전환 확인 다이얼로그를 띄울지 결정하는 데 쓴다.
func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"sessions": s.sessions.snapshot()})
}

type syncReq struct {
	Code    string `json:"code"`
	User    string `json:"user"`
	Message string `json:"message"` // 옵션 — 미지정시 auto-message.
}

// POST /sync — "등록" 액션: working tree dirty 면 stage+commit 한 뒤 origin 으로 push.
// 웹에서 미커밋(-) 또는 unpushed 가 하나라도 있으면 이 엔드포인트로 한 번에 해소한다.
// Push() 자체에는 손대지 않음 — 여기서 commit 만 보장하고 그 다음 기존 Push 흐름 재사용.
func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	var req syncReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "잘못된 JSON 본문")
		return
	}
	path, err := s.EnsureRepo(req.Code, req.User)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	committed := false
	if porcelain, err := gitCmd(path, "status", "--porcelain"); err == nil && porcelain != "" {
		if _, err := gitCmd(path, "add", "-A"); err != nil {
			writeErr(w, http.StatusInternalServerError, "git add 실패: "+err.Error())
			return
		}
		msg := strings.TrimSpace(req.Message)
		if msg == "" {
			msg = "[GoJIRA-web] sync local changes"
		}
		if _, err := gitCmd(path, "commit", "-m", msg); err != nil {
			writeErr(w, http.StatusInternalServerError, "git commit 실패: "+err.Error())
			return
		}
		committed = true
	}
	res, err := s.Push(req.Code, req.User)
	if err != nil {
		log.Printf("/sync push 실패 code=%s user=%s: %v", req.Code, req.User, err)
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"committed": committed,
		"push":      res,
	})
}

// POST /push — 로컬 docs/{user} 브랜치를 origin 으로 push (§5-3: push→PR 흐름).
func (s *Server) handlePush(w http.ResponseWriter, r *http.Request) {
	var req openReq // {code, user}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "잘못된 JSON 본문")
		return
	}
	res, err := s.Push(req.Code, req.User)
	if err != nil {
		log.Printf("/push 실패 code=%s user=%s: %v", req.Code, req.User, err)
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// POST /pull — origin/main 의 새 내용을 로컬 docs/{user} 로 끌어온다.
// FF 가능하면 fast-forward, 사용자 커밋이 있으면 rebase. dirty 면 거부.
func (s *Server) handlePull(w http.ResponseWriter, r *http.Request) {
	var req openReq // {code, user}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "잘못된 JSON 본문")
		return
	}
	res, err := s.Pull(req.Code, req.User)
	if err != nil {
		log.Printf("/pull 실패 code=%s user=%s: %v", req.Code, req.User, err)
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type writeReq struct {
	Code    string `json:"code"`
	User    string `json:"user"`
	Path    string `json:"path"`
	Content string `json:"content"` // raw text (인라인 에디터 산출물). base64 인코딩 안 함.
}

// POST /write — 인라인 편집 저장. 로컬 working tree 의 단일 파일에 raw text 를 쓴다.
// commit/push 는 하지 않음 — Local 탭의 "등록" 흐름과 책임 분리(VS Code 저장과 동일 의미).
// no-fetch: 저장은 디스크 변형일 뿐이라 origin fetch 가 필요 없다 — 신선한 main 동기화는
// Pull 버튼(/pull)의 책임. 느린 Gitea/사내망에서 저장마다 네트워크 대기하던 것 제거.
// safeFilePath 로 경로 컨테인먼트, 부모 디렉터리 자동 생성, mode 0644.
func (s *Server) handleWrite(w http.ResponseWriter, r *http.Request) {
	var req writeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "잘못된 JSON 본문")
		return
	}
	if req.Path == "" {
		writeErr(w, http.StatusBadRequest, "path 누락")
		return
	}
	repoPath, err := s.EnsureRepoLocal(req.Code, req.User)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	full, err := safeFilePath(repoPath, req.Path)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := os.WriteFile(full, []byte(req.Content), 0o644); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"path":   req.Path,
		"size":   len(req.Content),
	})
}

type deleteReq struct {
	Code string `json:"code"`
	User string `json:"user"`
	Path string `json:"path"`
}

// POST /delete — 로컬 working tree 에서 단일 파일 삭제. commit/push 는 하지 않음 —
// /sync 의 `git add -A` 가 삭제를 포착해 커밋한다(/write 와 동일한 책임 분리). 디렉터리는 거부.
// no-fetch: /write 와 같은 디스크 전용 변형 — origin fetch 불필요.
func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	var req deleteReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "잘못된 JSON 본문")
		return
	}
	if req.Path == "" {
		writeErr(w, http.StatusBadRequest, "path 누락")
		return
	}
	repoPath, err := s.EnsureRepoLocal(req.Code, req.User)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	full, err := safeFilePath(repoPath, req.Path)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	info, err := os.Stat(full)
	if err != nil {
		if os.IsNotExist(err) {
			writeErr(w, http.StatusNotFound, "file 없음: "+req.Path)
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if info.IsDir() {
		writeErr(w, http.StatusBadRequest, "디렉터리는 삭제할 수 없습니다: "+req.Path)
		return
	}
	if err := os.Remove(full); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "path": req.Path, "deleted": true})
}

type removeCloneReq struct {
	Code string `json:"code"`
	User string `json:"user"`
}

// POST /remove-clone — 프로젝트 아카이브 후처리: 이 PC 의 해당 프로젝트 로컬 clone 전체 삭제.
// 서버측 archive(Gitea repo + Mongo)는 Platform-API 가 처리하고, 사용자 PC 의 clone 은
// 서버가 닿을 수 없어 이 핸들러가 담당한다(웹 삭제 흐름이 archive 성공 후 호출).
// 프로젝트 자체가 없어지므로 요청 user 뿐 아니라 모든 사용자 clone(RepoRoot/*/{code})과
// 분리 이전 공유 clone(RepoRoot/{code})까지 지운다.
// 멱등: 이미 없으면 성공(removed:false) — 수동 삭제된 경우도 정상 처리(끝까지 정리 원칙).
func (s *Server) handleRemoveClone(w http.ResponseWriter, r *http.Request) {
	var req removeCloneReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "잘못된 JSON 본문")
		return
	}
	// code 검증 — 경로 이탈(../, 절대경로, 구분자) 차단. RepoRoot 하위 단일 세그먼트만 허용.
	if req.Code == "" || strings.ContainsAny(req.Code, `/\`) || strings.Contains(req.Code, "..") {
		writeErr(w, http.StatusBadRequest, "잘못된 code")
		return
	}
	paths := s.clonePathsOf(req.Code)
	if len(paths) == 0 {
		// 이미 없음 — 멱등 성공.
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "removed": false, "reason": "not found"})
		return
	}
	for _, path := range paths {
		if err := os.RemoveAll(path); err != nil {
			writeErr(w, http.StatusInternalServerError, "clone 삭제 실패: "+err.Error())
			return
		}
		log.Printf("/remove-clone code=%s → %s 삭제", req.Code, path)
	}
	// 정렬 캐시에서도 제거 — 키는 repoKey(code,user) = "<user>/<code>".
	s.alignedRepos.Range(func(k, _ any) bool {
		if strings.HasSuffix(k.(string), "/"+req.Code) {
			s.alignedRepos.Delete(k)
		}
		return true
	})
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "removed": true, "repoPaths": paths})
}

// clonePathsOf — code 의 로컬 clone 경로 전부: 사용자별 RepoRoot/*/{code} + 공유 RepoRoot/{code}.
// .git 이 있는 디렉터리만 clone 으로 본다(우연히 같은 이름의 일반 폴더는 건드리지 않음).
func (s *Server) clonePathsOf(code string) []string {
	var out []string
	isClone := func(p string) bool {
		_, err := os.Stat(filepath.Join(p, ".git"))
		return err == nil
	}
	if legacy := s.legacyRepoPath(code); isClone(legacy) {
		out = append(out, legacy)
	}
	entries, _ := os.ReadDir(s.cfg.RepoRoot)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if p := filepath.Join(s.cfg.RepoRoot, e.Name(), code); isClone(p) {
			out = append(out, p)
		}
	}
	return out
}

type renameReq struct {
	Code string `json:"code"`
	User string `json:"user"`
	From string `json:"from"`
	To   string `json:"to"`
}

// POST /rename — 로컬 working tree 의 파일 이동/이름변경. commit/push 는 하지 않음(/sync 포착).
// from/to 모두 safeFilePath 컨테인먼트. to 부모 디렉터리는 자동 생성. to 가 이미 있으면 거부.
// no-fetch: /write 와 같은 디스크 전용 변형 — origin fetch 불필요.
func (s *Server) handleRename(w http.ResponseWriter, r *http.Request) {
	var req renameReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "잘못된 JSON 본문")
		return
	}
	if req.From == "" || req.To == "" {
		writeErr(w, http.StatusBadRequest, "from, to 필수")
		return
	}
	repoPath, err := s.EnsureRepoLocal(req.Code, req.User)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	fromFull, err := safeFilePath(repoPath, req.From)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "from: "+err.Error())
		return
	}
	toFull, err := safeFilePath(repoPath, req.To)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "to: "+err.Error())
		return
	}
	if fi, err := os.Stat(fromFull); err != nil {
		if os.IsNotExist(err) {
			writeErr(w, http.StatusNotFound, "from 없음: "+req.From)
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	} else if fi.IsDir() {
		writeErr(w, http.StatusBadRequest, "디렉터리는 이동할 수 없습니다: "+req.From)
		return
	}
	if _, err := os.Stat(toFull); err == nil {
		writeErr(w, http.StatusConflict, "대상이 이미 존재합니다: "+req.To)
		return
	}
	if err := os.MkdirAll(filepath.Dir(toFull), 0o755); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := os.Rename(fromFull, toFull); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "from": req.From, "to": req.To, "renamed": true})
}

// GET /files?code=&user= — 로컬 clone 의 working tree 파일 목록 + HEAD 트리.
// 응답 shape: {tree, headTree, total_count, source}.
//   - tree     : working tree (디스크에 있는 현재 내용 기준 blob sha) — 서버 /tree 와 호환.
//   - headTree : 로컬 docs/{user} HEAD 의 `git ls-tree -r HEAD` 결과 (커밋된 상태).
//
// 웹은 두 트리를 사용해 path 별로 "main vs local 차이" 외에 "이 차이가 커밋됐는가" 까지 판정해
// 배지에 +(committed) / -(uncommitted) 를 prefix 로 붙인다.
//
// sha 는 모두 git blob 해시라 비교 그대로 가능. .git / .claude(스킬 symlink) 는 노출 제외.
func (s *Server) handleFiles(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	user := r.URL.Query().Get("user")
	// /files 는 read-only — 네트워크 fetch 생략 (느린 Gitea 환경에서 hang 방지).
	// 신선한 origin/main 비교는 /pull 버튼이 책임짐.
	repoPath, err := s.EnsureRepoLocal(code, user)
	if err != nil {
		log.Printf("/files 실패 code=%s user=%s: %v", code, user, err)
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	entries, err := listLocalTree(repoPath)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	headEntries, _ := listHeadTree(repoPath) // HEAD 가 없거나 ls-tree 실패면 빈 슬라이스로 진행(전부 uncommitted 로 표시됨).
	ahead := computeAhead(repoPath)          // upstream 대비 unpushed commit 수. -1 = upstream 미설정(=신규 브랜치, push 필요).
	writeJSON(w, http.StatusOK, map[string]any{
		"tree":        entries,
		"headTree":    headEntries,
		"ahead":       ahead,
		"total_count": len(entries),
		"source":      "local-launcher",
		"repoPath":    repoPath,
	})
}

// computeAhead — `@{upstream}..HEAD` commit 수. upstream 미설정이면 -1 (신규 브랜치 push 필요).
func computeAhead(repoPath string) int {
	out, err := gitCmd(repoPath, "rev-list", "--count", "@{upstream}..HEAD")
	if err != nil {
		return -1
	}
	n, _ := strconv.Atoi(strings.TrimSpace(out))
	return n
}

// GET /file?code=&user=&path= — 로컬 working tree 의 단일 파일 내용.
// 응답 shape 은 서버 /DocRepo/{code}/file 과 호환 — {path, content(base64), encoding:"base64", size, source}.
func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	user := r.URL.Query().Get("user")
	rel := r.URL.Query().Get("path")
	if rel == "" {
		writeErr(w, http.StatusBadRequest, "path 누락")
		return
	}
	// /file 은 순수 읽기 — 프로비저닝/락 없이 경로만 해석 (clone 부재 시에만 폴백 clone).
	repoPath, err := s.resolveRepoLocal(code, user)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	full, err := safeFilePath(repoPath, rel)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	data, err := os.ReadFile(full)
	if err != nil {
		if os.IsNotExist(err) {
			writeErr(w, http.StatusNotFound, "file 없음: "+rel)
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"path":     rel,
		"content":  base64.StdEncoding.EncodeToString(data),
		"encoding": "base64",
		"size":     len(data),
		"source":   "local-launcher",
	})
}

// treeHashCache — repoPath 별 파일 blob SHA 캐시. mtime+size 가 그대로면 재해시(디스크
// 재읽기 + git hash-object 프로세스)를 스킵한다. 파일이 많은 프로젝트에서 /files 호출마다
// 전체 내용을 다시 읽어 해시하던 게 체감 지연의 주 원인이었다 — 외부에서 파일을 직접
// 고쳐도 mtime 이 바뀌므로 정확하게 재계산된다(수동 무효화 로직 불필요).
type cachedFileEntry struct {
	mtime int64
	size  int64
	sha   string
}

var (
	treeHashCacheMu sync.Mutex
	treeHashCache   = map[string]map[string]cachedFileEntry{} // repoPath -> rel path -> entry
)

// listLocalTree — repoPath 하위 모든 파일을 git blob 해시와 함께 평면 리스트로 반환.
// .git / .claude(skills symlink) / 디렉터리 본체는 제외 (blob 만). 정렬은 path 사전순.
//
// SHA 는 `git hash-object --stdin-paths` 로 1회 batch 계산한다. 디스크 raw bytes 가
// 아닌 git attribute 필터(autocrlf 등) 통과 후 해시이므로 `git ls-tree HEAD` 의 blob
// SHA 와 직접 비교 가능 — Windows CRLF 환경에서 모든 파일을 "수정됨" 으로 오인식하던
// false positive 가 사라진다.
func listLocalTree(repoPath string) ([]map[string]any, error) {
	type fileInfo struct {
		rel   string
		size  int64
		mtime int64
	}
	var files []fileInfo
	err := filepath.WalkDir(repoPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(repoPath, path)
		if rel == "." {
			return nil
		}
		// 최상위 .git / 에이전트 자산(.claude/.opencode/opencode.json) 컷오프 — 하위 traversal 도 스킵.
		top := strings.SplitN(filepath.ToSlash(rel), "/", 2)[0]
		if top == ".git" || isAgentAssetTop(top) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil // 한 파일 stat 실패는 무시 — 트리는 계속.
		}
		files = append(files, fileInfo{rel: filepath.ToSlash(rel), size: info.Size(), mtime: info.ModTime().UnixNano()})
		return nil
	})
	if err != nil {
		return nil, err
	}

	treeHashCacheMu.Lock()
	cache := treeHashCache[repoPath]
	if cache == nil {
		cache = map[string]cachedFileEntry{}
		treeHashCache[repoPath] = cache
	}
	// 캐시 스냅샷(락 밖에서 읽기 전용으로 사용) — git hash-object 는 시간이 걸리므로 락을 오래
	// 쥐지 않는다.
	snapshot := make(map[string]cachedFileEntry, len(cache))
	for k, v := range cache {
		snapshot[k] = v
	}
	treeHashCacheMu.Unlock()

	shaOf := make(map[string]string, len(files))
	var missPaths []string
	for _, f := range files {
		if c, ok := snapshot[f.rel]; ok && c.mtime == f.mtime && c.size == f.size {
			shaOf[f.rel] = c.sha
			continue
		}
		missPaths = append(missPaths, f.rel)
	}
	if len(missPaths) > 0 {
		shas, _ := hashObjectsBatch(repoPath, missPaths) // 실패 시 빈 map — 아래에서 fallback.
		for _, rel := range missPaths {
			shaOf[rel] = shas[rel]
		}
	}

	entries := make([]map[string]any, 0, len(files))
	newCache := make(map[string]cachedFileEntry, len(files))
	for _, f := range files {
		sha := shaOf[f.rel]
		if sha == "" {
			// fallback: batch 실패 시 디스크 raw 해시(이전 동작). UI 가 가짜 dirty 로 보일 수
			// 있지만 응답 자체는 유지. 캐시에는 저장하지 않아 다음 호출에서 재시도.
			if s, err := blobShaRaw(filepath.Join(repoPath, filepath.FromSlash(f.rel)), f.size); err == nil {
				sha = s
			} else {
				continue
			}
		} else {
			newCache[f.rel] = cachedFileEntry{mtime: f.mtime, size: f.size, sha: sha}
		}
		entries = append(entries, map[string]any{
			"path": f.rel,
			"type": "blob",
			"sha":  sha,
			"size": f.size,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i]["path"].(string) < entries[j]["path"].(string)
	})

	treeHashCacheMu.Lock()
	treeHashCache[repoPath] = newCache // 삭제/rename 된 파일은 자연스럽게 빠짐(누적 방지).
	treeHashCacheMu.Unlock()

	return entries, nil
}

// hashObjectsBatch — `git hash-object --stdin-paths` 로 여러 파일의 blob SHA 를 한 번에
// 계산한다. paths 는 repoPath 기준 forward-slash 상대경로. git 의 모든 attribute
// (text/eol/autocrlf 등)을 거치므로 `git ls-tree HEAD` 결과와 직접 비교 가능.
//
// 결과 map[path]sha. paths 가 비어있으면 빈 map. git 실행 자체가 실패하면 error
// (호출 측이 raw fallback 으로 진행).
func hashObjectsBatch(repoPath string, paths []string) (map[string]string, error) {
	result := make(map[string]string, len(paths))
	if len(paths) == 0 {
		return result, nil
	}
	cmd := exec.Command("git", "hash-object", "--stdin-paths")
	cmd.Dir = repoPath
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	hideConsole(cmd) // Windows 콘솔 창 번쩍임 방지 (비 Windows no-op)
	var stdinBuf bytes.Buffer
	for _, p := range paths {
		// stdin-paths 는 줄 단위 — 경로에 newline 이 들어가는 케이스는 거부.
		if strings.ContainsAny(p, "\n\r") {
			return nil, fmt.Errorf("path 에 newline 포함 — 거부: %q", p)
		}
		stdinBuf.WriteString(p)
		stdinBuf.WriteByte('\n')
	}
	cmd.Stdin = &stdinBuf
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git hash-object --stdin-paths 실패: %w", err)
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) != len(paths) {
		return nil, fmt.Errorf("hash-object 출력 행수(%d) ≠ 입력 path(%d)", len(lines), len(paths))
	}
	for i, p := range paths {
		result[p] = strings.TrimSpace(lines[i])
	}
	return result, nil
}

// listHeadTree — `git ls-tree -r HEAD` 결과를 파싱해 평면 리스트로 반환.
// HEAD 가 없거나(빈 레포) 실패면 빈 리스트 + err (호출 측이 비치명적으로 처리).
// 포맷: <mode> <type> <sha>\t<path>
func listHeadTree(repoPath string) ([]map[string]any, error) {
	out, err := gitCmd(repoPath, "ls-tree", "-r", "HEAD")
	if err != nil {
		return []map[string]any{}, err
	}
	var entries []map[string]any
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		meta := strings.Fields(parts[0])
		if len(meta) < 3 {
			continue
		}
		entries = append(entries, map[string]any{
			"path": parts[1],
			"type": meta[1],
			"sha":  meta[2],
		})
	}
	return entries, nil
}

// blobShaRaw — 파일의 raw 디스크 내용으로 git blob sha1 ("blob <size>\0<content>") 계산.
// `hashObjectsBatch` 실패 시 fallback 으로만 사용. autocrlf 필터 미적용이라 Windows
// CRLF 체크아웃 파일은 `git ls-tree HEAD` 의 SHA 와 다를 수 있음(가짜 dirty).
func blobShaRaw(path string, size int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha1.New()
	fmt.Fprintf(h, "blob %d\x00", size)
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// GET /issues?code=&user= — 로컬 clone 의 data/issues/*.yaml(.yml) 을 raw 로 일괄 반환
// (Phase 1 — 웹 이슈 뷰 이원화 §3 "내 작업본"). 응답 shape:
//
//	{ issues: [{path:"data/issues/VOS_001.yaml", content:"<raw yaml>"}], count, source:"local" }
//
// 읽기 전용 — EnsureRepo 를 호출하지 않는다(clone 자동 생성 금지). clone 이 없거나
// data/issues 가 없으면 404 + {"error":...} — 웹은 이를 보고 공식본(Mongo)으로 폴백한다.
// user 는 /files 와 동일하게 받되(파라미터 검증 일관성), 경로 해석에는 code 만 쓰인다 —
// 로컬 clone 은 code 당 1개이고 현재 체크아웃된 브랜치 그대로 반환한다(사용자 전환/
// 브랜치 정렬은 /files 등 다른 조회 경로가 이미 책임짐).
func (s *Server) handleIssues(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	user := r.URL.Query().Get("user")
	if err := validateName("code", code); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateName("user", user); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	repoPath := s.repoPath(code, user)
	if _, err := os.Stat(filepath.Join(repoPath, ".git")); err != nil {
		writeErr(w, http.StatusNotFound, "로컬 clone 이 없습니다: "+code)
		return
	}
	issuesDir := filepath.Join(repoPath, "data", "issues")
	files, err := readIssueFiles(issuesDir)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if files == nil {
		writeErr(w, http.StatusNotFound, "data/issues 디렉터리가 없습니다: "+code)
		return
	}
	issues := make([]map[string]any, 0, len(files))
	for _, f := range files {
		issues = append(issues, map[string]any{
			"path":    "data/issues/" + f.Name,
			"content": f.Content,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"issues": issues,
		"count":  len(issues),
		"source": "local",
	})
}

type issueJournalReq struct {
	Code  string            `json:"code"`
	User  string            `json:"user"`
	Entry JournalEntryInput `json:"entry"`
}

// POST /issue-journal — 웹 로컬뷰("내 작업본")에서 발생한 이슈 변경(생성/수정/삭제/링크)을
// ~/.gojira/journal/<code>.yaml 에 append 한다 (D10 Phase 3 §옵션(a), web-local-view-model.md).
// 클론 안이 아니라 런처 영역에 두는 이유: 프로젝트 레포에 .gitignore 가 없어 append 결과가
// 그대로 PR 에 새어 들어가기 때문. 에이전트 세션은 spawn.go agentEnv 가 주입하는
// GOJIRA_ISSUE_JOURNAL env 로 이 파일을 찾아 설계 영향(SATAM/유스케이스/설계문서)을 검토한다.
func (s *Server) handleIssueJournal(w http.ResponseWriter, r *http.Request) {
	var req issueJournalReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "잘못된 JSON 본문")
		return
	}
	if err := validateName("code", req.Code); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateName("user", req.User); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if !validJournalOps[req.Entry.Op] {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("entry.op 이 올바르지 않습니다: %q (create|update|delete|link|unlink)", req.Entry.Op))
		return
	}
	if len(req.Entry.Keys) == 0 {
		writeErr(w, http.StatusBadRequest, "entry.keys 누락")
		return
	}
	if strings.TrimSpace(req.Entry.Summary) == "" {
		writeErr(w, http.StatusBadRequest, "entry.summary 누락")
		return
	}
	path, err := s.appendJournalEntry(req.Code, req.User, req.Entry)
	if err != nil {
		log.Printf("/issue-journal 실패 code=%s user=%s: %v", req.Code, req.User, err)
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "path": path})
}

// GET /status?code=&user= — 로컬 작업/브랜치 진행 상태 (§2.1).
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	user := r.URL.Query().Get("user")
	st, err := s.Status(code, user)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, st)
}
