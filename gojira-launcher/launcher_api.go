package main

// 런처 자체 API — `/launcher/*` (설계서 §6).
//
// 이 이름공간은 런처가 소유하며 서비스가 가져갈 수 없다(§5). 웹은 여기서
// "무엇을 호출할 수 있는지"(capabilities)와 "서비스가 어떤 상태인지"(services)를 읽고,
// 필요하면 설치·재기동을 요청한다.

import (
	"net/http"
	"runtime"
)

// capabilityRoute — /launcher/capabilities 의 한 줄.
type capabilityRoute struct {
	Prefix  string   `json:"prefix"`
	Methods []string `json:"methods"`
	Service string   `json:"service"`
	State   string   `json:"state"` // available | installable | unsupported
	Missing []string `json:"missing,omitempty"`
}

// handleCapabilities — 호출 가능한 경로 목록(§6).
// 웹은 서비스 이름이 아니라 경로로 판단하므로(D3), 여기서 경로별 가용 상태를 알려준다.
func (s *Server) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	var out []capabilityRoute
	for _, e := range s.routeTable() {
		spec := s.specOf(e.service)
		state, missing := routeState(spec)
		methods := e.route.Methods
		if len(methods) == 0 {
			methods = []string{"*"}
		}
		out = append(out, capabilityRoute{
			Prefix: e.route.Prefix, Methods: methods, Service: e.service,
			State: state, Missing: missing,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"routes": out})
}

// routeState — 경로의 가용 상태(§6).
//
//	available    설치(또는 내장)돼 있고 요건 충족
//	unsupported  OS 또는 요건 불충족 — 호출하면 424
//	installable  카탈로그에 있으나 미설치 — 호출하면 409 (Installer 묶음에서 채운다)
func routeState(spec *ServiceSpec) (string, []string) {
	if spec == nil {
		return "installable", nil
	}
	if len(spec.OS) > 0 {
		supported := false
		for _, o := range spec.OS {
			if o == runtime.GOOS {
				supported = true
				break
			}
		}
		if !supported {
			return "unsupported", []string{"os:" + runtime.GOOS}
		}
	}
	ok, missing := checkRequirements(spec.Requires)
	if !ok {
		keys := make([]string, 0, len(missing))
		for _, m := range missing {
			keys = append(keys, m.Key)
		}
		return "unsupported", keys
	}
	return "available", nil
}

// specOf — 등록된 서비스의 선언을 찾는다.
func (s *Server) specOf(name string) *ServiceSpec {
	for _, svc := range builtinServices() {
		if svc.Name == name {
			sp := svc
			return &sp
		}
	}
	// TODO(Installer 묶음): 설치된 외부 서비스의 service.json 도 여기서 찾는다.
	return nil
}

// handleServices — 설치·실행 상태(§6).
func (s *Server) handleServices(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"services": s.sup.snapshot()})
}

// handleServiceInstall — optional 서비스 설치(§6).
// Installer 는 다음 묶음 — 지금은 "아직 구현 전" 을 기계 판독 가능하게 알린다.
func (s *Server) handleServiceInstall(w http.ResponseWriter, r *http.Request) {
	writeRouteErr(w, http.StatusNotImplemented, "not_implemented",
		"서비스 설치는 아직 구현 전입니다 (카탈로그·Installer 묶음)",
		map[string]any{"service": r.PathValue("name")})
}

// handleServiceRestart — 재기동(§6).
func (s *Server) handleServiceRestart(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if sp := s.specOf(name); sp != nil && sp.builtin {
		writeRouteErr(w, http.StatusConflict, "builtin_service",
			"내장 서비스는 재기동 대상이 아닙니다 — 런처를 재시작하세요",
			map[string]any{"service": name})
		return
	}
	if err := s.sup.restart(name); err != nil {
		writeRouteErr(w, http.StatusBadGateway, "restart_failed", err.Error(),
			map[string]any{"service": name})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "service": name})
}
