package main

import "sync"

// agentSession — 과제(code)별로 spawn 된 에이전트 CLI 추적 (설계: local-agent-mvp.md
// "Design B" — 과제별 다중·라벨링). 하나의 에이전트는 과제를 갈아타지 않으므로(claude/
// opencode 는 cwd 스코프), 과제별로 따로 띄우되 살아있는 세션을 추적해 같은 과제의
// 중복 spawn 을 막고, 다른 과제는 공존시킨다.
type agentSession struct {
	Code  string `json:"code"`
	Agent string `json:"agent"`
	PID   int    `json:"pid"`
	// Dir — 에이전트 cwd(사용자별 clone 경로). 같은 과제라도 Dir 이 다르면 다른 사용자의
	// 세션이므로 재사용하면 안 된다(남의 작업본에 쓰게 됨) — handleChat 이 비교한다.
	Dir   string `json:"dir"`
	alive bool   // cmd.Wait 회수 시 false 로 전환(registry.markDead).
}

// sessionRegistry — code → 현재 살아있는 에이전트 세션. 동시 /chat 호출 대비 mutex 보호.
type sessionRegistry struct {
	mu     sync.Mutex
	byCode map[string]*agentSession
}

func newSessionRegistry() *sessionRegistry {
	return &sessionRegistry{byCode: map[string]*agentSession{}}
}

// live — code 에 살아있는 세션이 있으면 복사본을 반환, 없으면 nil.
// code=="" (프로젝트 없는 일반 채팅)은 추적하지 않으므로 항상 nil.
func (r *sessionRegistry) live(code string) *agentSession {
	if code == "" {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.byCode[code]
	if s == nil || !s.alive {
		return nil
	}
	cp := *s
	return &cp
}

// add — 새 세션 등록(같은 code 의 기존 항목은 덮어씀). code=="" 는 추적 제외.
func (r *sessionRegistry) add(code, agent, dir string, pid int) {
	if code == "" {
		return
	}
	r.mu.Lock()
	r.byCode[code] = &agentSession{Code: code, Agent: agent, Dir: dir, PID: pid, alive: true}
	r.mu.Unlock()
}

// markDead — pid 종료 시 alive=false. 단, 그 사이 같은 code 가 새 pid 로 재spawn 됐다면
// (PID 불일치) 건드리지 않아 최신 세션을 살아있는 채로 유지한다.
func (r *sessionRegistry) markDead(code string, pid int) {
	if code == "" {
		return
	}
	r.mu.Lock()
	if s := r.byCode[code]; s != nil && s.PID == pid {
		s.alive = false
	}
	r.mu.Unlock()
}

// liveExcept — code 와 다른, 살아있는 세션들(복사본). 과제전환 시 종료 대상 목록.
func (r *sessionRegistry) liveExcept(code string) []agentSession {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []agentSession
	for c, s := range r.byCode {
		if c != code && s != nil && s.alive {
			out = append(out, *s)
		}
	}
	return out
}

// snapshot — 현재 살아있는 세션 목록(복사본). /status 등 진단용.
func (r *sessionRegistry) snapshot() []agentSession {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]agentSession, 0, len(r.byCode))
	for _, s := range r.byCode {
		if s != nil && s.alive {
			out = append(out, *s)
		}
	}
	return out
}
