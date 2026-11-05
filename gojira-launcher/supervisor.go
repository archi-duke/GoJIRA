package main

// Supervisor — 로컬 서비스 프로세스의 생명주기 (설계서 §10.5, §9.2).
//
// 런처 책임 중 "소환" 에 해당한다. 서비스를 기동하고, 헬스를 확인하고, 죽으면 되살리고,
// 쓰지 않으면 내린다. 내장 서비스(builtin)는 런처 안에서 처리되므로 대상이 아니다.
//
// 기동 시점(§10.5):
//   always     런처 기동 시
//   on-demand  첫 요청 시 — 헬스 통과까지 요청을 대기시킨다(§7.3, 최대 30s)
//
// 런타임 계약(§9.2): 서비스는 127.0.0.1:$SVC_PORT 에 바인드하고 X-Svc-Secret 을 검증하며
// GET /health 로 {version, ready, busy} 를 돌려준다. busy=true 면 업데이트를 미룬다.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

type serviceState string

const (
	stateStopped  serviceState = "stopped"
	stateStarting serviceState = "starting"
	stateRunning  serviceState = "running"
	stateFailed   serviceState = "failed"
)

const (
	startupTimeout   = 30 * time.Second // on-demand 요청이 기동을 기다리는 한도(§7.3)
	healthInterval   = 15 * time.Second
	shutdownGrace    = 10 * time.Second // §9.2
	maxRestartTries  = 5                // §10.5 — 초과 시 failed
	restartBaseDelay = time.Second
)

// serviceProc — 관리 중인 서비스 하나.
type serviceProc struct {
	spec     ServiceSpec
	state    serviceState
	port     int
	secret   string
	cmd      *exec.Cmd
	lastUsed time.Time
	tries    int
	lastErr  string
}

type Supervisor struct {
	mu    sync.Mutex
	procs map[string]*serviceProc
	srv   *Server
}

func newSupervisor(s *Server) *Supervisor {
	return &Supervisor{procs: map[string]*serviceProc{}, srv: s}
}

// register — 설치된(또는 내장) 서비스를 관리 대상에 올린다. 기동은 하지 않는다.
func (sv *Supervisor) register(spec ServiceSpec) {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	if _, ok := sv.procs[spec.Name]; ok {
		return
	}
	sv.procs[spec.Name] = &serviceProc{spec: spec, state: stateStopped}
}

// snapshot — `/launcher/services` 용 상태 사본.
func (sv *Supervisor) snapshot() []map[string]any {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	out := make([]map[string]any, 0, len(sv.procs))
	for _, p := range sv.procs {
		ok, missing := checkRequirements(p.spec.Requires)
		keys := make([]string, 0, len(missing))
		for _, m := range missing {
			keys = append(keys, m.Key)
		}
		// 내장 서비스는 런처 프로세스 안에서 처리되므로 따로 기동하지 않는다 —
		// 요건만 충족하면 항상 쓸 수 있으니 running 으로 보고한다(stopped 로 보이면 오해를 산다).
		st := p.state
		if p.spec.builtin {
			st = stateRunning
			if !ok {
				st = stateFailed
			}
		}
		e := map[string]any{
			"name":     p.spec.Name,
			"version":  p.spec.Version,
			"state":    string(st),
			"builtin":  p.spec.builtin,
			"requires": map[string]any{"ok": ok, "missing": keys},
		}
		if p.lastErr != "" {
			e["lastError"] = p.lastErr
		}
		out = append(out, e)
	}
	return out
}

// ensureRunning — on-demand 서비스를 기동하고 헬스 통과까지 기다린다.
// 내장 서비스는 프로세스가 없으므로 즉시 성공.
func (sv *Supervisor) ensureRunning(ctx context.Context, name string) error {
	sv.mu.Lock()
	p, ok := sv.procs[name]
	if !ok {
		sv.mu.Unlock()
		return fmt.Errorf("등록되지 않은 서비스: %s", name)
	}
	if p.spec.builtin {
		sv.mu.Unlock()
		return nil
	}
	p.lastUsed = time.Now()
	switch p.state {
	case stateRunning:
		sv.mu.Unlock()
		return nil
	case stateFailed:
		sv.mu.Unlock()
		return fmt.Errorf("서비스 %s 는 기동 실패 상태입니다: %s", name, p.lastErr)
	}
	sv.mu.Unlock()

	if err := sv.start(name); err != nil {
		return err
	}
	return sv.waitHealthy(ctx, name, startupTimeout)
}

// start — 프로세스를 띄운다. 포트·비밀값을 새로 할당하고 §9.2 환경변수를 주입한다.
func (sv *Supervisor) start(name string) error {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	p, ok := sv.procs[name]
	if !ok {
		return fmt.Errorf("등록되지 않은 서비스: %s", name)
	}
	if p.spec.builtin || p.state == stateRunning || p.state == stateStarting {
		return nil
	}
	if ok, missing := checkRequirements(p.spec.Requires); !ok {
		p.state = stateFailed
		p.lastErr = fmt.Sprintf("요건 불충족: %v", missing)
		return fmt.Errorf("%s", p.lastErr)
	}

	port, err := freePort()
	if err != nil {
		return fmt.Errorf("포트 할당 실패: %w", err)
	}
	secret, err := randomSecret()
	if err != nil {
		return fmt.Errorf("비밀값 생성 실패: %w", err)
	}
	entry, err := sv.entryPath(p.spec)
	if err != nil {
		p.state = stateFailed
		p.lastErr = err.Error()
		return err
	}
	dataDir, logDir := sv.serviceDirs(p.spec.Name)
	_ = os.MkdirAll(dataDir, 0o755)
	_ = os.MkdirAll(logDir, 0o755)

	logFile, err := os.OpenFile(filepath.Join(logDir, "service.log"),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("로그 파일 열기 실패: %w", err)
	}

	cmd := exec.Command(entry)
	cmd.Dir = filepath.Dir(entry)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("SVC_PORT=%d", port),
		"SVC_SECRET="+secret,
		"SVC_DATA_DIR="+dataDir,
		"SVC_LOG_DIR="+logDir,
		"LAUNCHER_VERSION="+version,
	)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		p.state = stateFailed
		p.lastErr = err.Error()
		return fmt.Errorf("기동 실패: %w", err)
	}

	p.cmd, p.port, p.secret, p.state, p.lastUsed = cmd, port, secret, stateStarting, time.Now()
	log.Printf("서비스 기동: %s v%s pid=%d port=%d", p.spec.Name, p.spec.Version, cmd.Process.Pid, port)

	// 프로세스 종료 감시 — 예기치 않게 죽으면 백오프 재시작(§10.5).
	go sv.reap(name, cmd, logFile)
	return nil
}

// reap — 프로세스 종료를 기다렸다가 상태를 정리하고, 필요하면 재시작한다.
func (sv *Supervisor) reap(name string, cmd *exec.Cmd, logFile *os.File) {
	err := cmd.Wait()
	_ = logFile.Close()

	sv.mu.Lock()
	p, ok := sv.procs[name]
	if !ok || p.cmd != cmd {
		sv.mu.Unlock()
		return // 이미 교체됨(restart 등)
	}
	wasRunning := p.state == stateRunning || p.state == stateStarting
	p.cmd, p.state = nil, stateStopped
	if err != nil {
		p.lastErr = err.Error()
	}
	tries := p.tries
	sv.mu.Unlock()

	if !wasRunning {
		return // 의도적 정지
	}
	if tries >= maxRestartTries {
		sv.mu.Lock()
		p.state, p.lastErr = stateFailed, fmt.Sprintf("재시작 %d회 실패: %s", tries, p.lastErr)
		sv.mu.Unlock()
		log.Printf("서비스 %s 재시작 포기(%d회) — failed", name, tries)
		return
	}
	delay := restartBaseDelay << tries // 1s, 2s, 4s, 8s, 16s
	log.Printf("서비스 %s 종료 감지(%v) — %v 후 재시작(%d/%d)", name, err, delay, tries+1, maxRestartTries)
	time.Sleep(delay)

	sv.mu.Lock()
	p.tries++
	sv.mu.Unlock()
	if err := sv.start(name); err != nil {
		log.Printf("서비스 %s 재시작 실패: %v", name, err)
	}
}

// waitHealthy — /health 가 ready 를 돌려줄 때까지 기다린다.
func (sv *Supervisor) waitHealthy(ctx context.Context, name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if ready, _ := sv.health(name); ready {
			sv.mu.Lock()
			if p, ok := sv.procs[name]; ok {
				p.state, p.tries, p.lastErr = stateRunning, 0, ""
			}
			sv.mu.Unlock()
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("서비스 %s 가 %v 안에 준비되지 않았습니다", name, timeout)
}

// health — 서비스의 GET /health 를 호출해 (ready, busy) 를 읽는다(§9.2).
func (sv *Supervisor) health(name string) (ready, busy bool) {
	sv.mu.Lock()
	p, ok := sv.procs[name]
	if !ok || p.port == 0 {
		sv.mu.Unlock()
		return false, false
	}
	port, secret, path := p.port, p.secret, p.spec.Health
	sv.mu.Unlock()

	if path == "" {
		path = "/health"
	}
	req, err := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d%s", port, path), nil)
	if err != nil {
		return false, false
	}
	req.Header.Set("X-Svc-Secret", secret)
	resp, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
	if err != nil {
		return false, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, false
	}
	var h struct {
		Ready bool `json:"ready"`
		Busy  bool `json:"busy"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&h); err != nil {
		return false, false
	}
	return h.Ready, h.Busy
}

// endpoint — 중계에 쓸 (포트, 비밀값). 미기동이면 (0, "").
func (sv *Supervisor) endpoint(name string) (int, string) {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	p, ok := sv.procs[name]
	if !ok {
		return 0, ""
	}
	p.lastUsed = time.Now() // idle 판정 갱신
	return p.port, p.secret
}

// stop — 정상 종료를 요청하고, 유예 시간이 지나면 강제 종료한다(§9.2).
func (sv *Supervisor) stop(name string) {
	sv.mu.Lock()
	p, ok := sv.procs[name]
	if !ok || p.cmd == nil || p.cmd.Process == nil {
		sv.mu.Unlock()
		return
	}
	cmd, port, secret := p.cmd, p.port, p.secret
	p.state = stateStopped // reap 이 재시작하지 않도록 먼저 표시
	sv.mu.Unlock()

	// POST /shutdown 먼저 — 진행 중 요청을 마무리할 기회를 준다.
	req, _ := http.NewRequest("POST", fmt.Sprintf("http://127.0.0.1:%d/shutdown", port), nil)
	if req != nil {
		req.Header.Set("X-Svc-Secret", secret)
		if resp, err := (&http.Client{Timeout: 2 * time.Second}).Do(req); err == nil {
			_ = resp.Body.Close()
		}
	}

	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(shutdownGrace):
		log.Printf("서비스 %s 가 %v 안에 종료되지 않아 강제 종료합니다", name, shutdownGrace)
		_ = cmd.Process.Kill()
	}
}

// restart — 정지 후 재기동.
func (sv *Supervisor) restart(name string) error {
	sv.stop(name)
	sv.mu.Lock()
	if p, ok := sv.procs[name]; ok {
		p.tries = 0
	}
	sv.mu.Unlock()
	return sv.start(name)
}

// startIdleWatch — on-demand 서비스가 idleTimeoutMin 동안 쓰이지 않으면 내린다(§10.5).
func (sv *Supervisor) startIdleWatch() {
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for range t.C {
			now := time.Now()
			var toStop []string
			sv.mu.Lock()
			for name, p := range sv.procs {
				if p.spec.builtin || p.state != stateRunning || p.spec.Run.Mode != "on-demand" {
					continue
				}
				idle := p.spec.Run.IdleTimeoutMin
				if idle <= 0 {
					continue
				}
				if now.Sub(p.lastUsed) > time.Duration(idle)*time.Minute {
					toStop = append(toStop, name)
				}
			}
			sv.mu.Unlock()
			for _, name := range toStop {
				if ready, busy := sv.health(name); ready && busy {
					continue // 작업 중이면 다음 주기에
				}
				log.Printf("서비스 %s idle 종료", name)
				sv.stop(name)
			}
		}
	}()
}

// ── 경로·자원 ───────────────────────────────────────────────────────────────

// entryPath — 설치된 서비스의 실행 파일 경로 (§10.4 배치).
func (sv *Supervisor) entryPath(spec ServiceSpec) (string, error) {
	dir, err := gojiraDir()
	if err != nil {
		return "", err
	}
	if spec.Entry == "" {
		return "", fmt.Errorf("service.json 에 entry 없음: %s", spec.Name)
	}
	base := filepath.Join(dir, "services", spec.Name, spec.Version)
	p := filepath.Join(base, spec.Entry)
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("실행 파일 없음: %s", p)
	}
	return p, nil
}

func (sv *Supervisor) serviceDirs(name string) (dataDir, logDir string) {
	dir, err := gojiraDir()
	if err != nil {
		return "", ""
	}
	return filepath.Join(dir, "data", name), filepath.Join(dir, "logs", name)
}

// freePort — OS 가 비어 있는 포트를 고르게 한다.
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func randomSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
