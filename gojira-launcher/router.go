package main

// 경로 라우터 (설계서 §7).
//
// 웹은 서비스 이름이 아니라 **기능 경로**로 호출하고(D3), 런처가 최장 prefix 일치로
// 담당 서비스에 넘긴다. 1단계에서는 내장 서비스(기존 핸들러)만 있고, 외부 서비스는
// 다음 묶음(Installer·Supervisor)에서 붙는다 — 그때 routeTable 에 항목이 늘어날 뿐
// 매칭 규칙은 그대로다.
//
// 신규 `/v1/*` 경로는 기존 핸들러를 재사용한다. 기존 핸들러는 code/user 를
// POST 는 바디 JSON, GET 은 쿼리에서 읽으므로 어댑터가 경로 변수를 그 자리에 넣어준다.
// 기존 경로(`/chat`, `/sync` …)는 등록을 그대로 두어 **동작이 변하지 않는다**(D7 호환 별칭).

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
)

// routeEntry — 라우팅 표의 한 줄.
type routeEntry struct {
	route   Route
	service string
	builtin bool
}

// routeTable — 설치된 서비스들의 경로 선언을 모은 표. 최장 prefix 우선으로 정렬해 둔다.
func (s *Server) routeTable() []routeEntry {
	var out []routeEntry
	for _, svc := range builtinServices() {
		for _, rt := range svc.Routes {
			out = append(out, routeEntry{route: rt, service: svc.Name, builtin: svc.builtin})
		}
	}
	// TODO(1단계 3묶음): 설치된 외부 서비스의 service.json 선언을 여기에 더한다.
	sort.SliceStable(out, func(i, j int) bool {
		return len(out[i].route.Prefix) > len(out[j].route.Prefix)
	})
	return out
}

// handleDynamicRoute — mux 에 직접 등록되지 않은 경로의 처리(fallback).
//
// 내장 경로는 main.go 가 mux 에 명시 등록하므로 여기 오지 않는다. 여기 오는 것은
// ① 카탈로그에 있으나 미설치인 서비스의 경로, ② 어디에도 없는 경로 둘 중 하나다.
func (s *Server) handleDynamicRoute(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// `/launcher/*` 는 런처 예약 이름공간(§5) — 서비스가 가져갈 수 없다.
	if strings.HasPrefix(path, "/launcher/") {
		writeRouteErr(w, http.StatusNotFound, "no_route", "런처 API 에 없는 경로입니다: "+path, nil)
		return
	}

	var methodMismatch bool
	for _, e := range s.routeTable() {
		if !e.route.matches(path) {
			continue
		}
		if !e.route.allowsMethod(r.Method) {
			methodMismatch = true
			continue
		}
		// 내장 서비스인데 여기까지 왔다 = mux 에 등록되지 않은 하위 경로.
		if e.builtin {
			writeRouteErr(w, http.StatusNotFound, "no_route",
				"내장 서비스 "+e.service+" 가 처리하지 않는 하위 경로입니다: "+path, nil)
			return
		}
		// TODO(1단계 3묶음): 설치됐으면 프록시, 미설치면 409 service_not_installed,
		// 요건 불충족이면 424 requirement_missing (§7.2).
		writeRouteErr(w, http.StatusServiceUnavailable, "service_not_running",
			"서비스 "+e.service+" 중계는 아직 구현 전입니다", map[string]any{"service": e.service})
		return
	}

	if methodMismatch {
		writeRouteErr(w, http.StatusMethodNotAllowed, "method_not_allowed",
			r.Method+" 은 이 경로에서 허용되지 않습니다", nil)
		return
	}
	writeRouteErr(w, http.StatusNotFound, "no_route", "경로를 처리할 서비스가 없습니다: "+path, nil)
}

// writeRouteErr — 라우팅 오류 응답(§7.2). 웹이 code 로 분기할 수 있게 기계 판독 가능한 형태.
func writeRouteErr(w http.ResponseWriter, status int, code, msg string, extra map[string]any) {
	body := map[string]any{"code": code, "error": msg}
	for k, v := range extra {
		body[k] = v
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// ── 신규 경로 → 기존 핸들러 어댑터 ──────────────────────────────────────────
//
// 설계 §5: 신규 경로는 code 를 경로 변수로, 사용자 식별은 런처가 주입하는 헤더로 받는다
// (현행 {code,user} 바디는 호환 별칭에서만 허용). 기존 핸들러를 그대로 쓰기 위해
// 경로·헤더의 값을 핸들러가 읽는 자리(POST=바디, GET=쿼리)에 넣어준다.

// launcherUserHeader — 런처가 주입하는 사용자 식별 헤더(§7.3).
const launcherUserHeader = "X-Launcher-User"

// withBodyCode — POST 계열: 경로 {code} 와 사용자 식별을 바디 JSON 에 주입한다.
// 바디에 이미 값이 있으면 덮어쓰지 않는다(호환 별칭 경유 호출 보호).
func (s *Server) withBodyCode(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		code := r.PathValue("code")
		user := s.routeUser(r)

		raw, err := io.ReadAll(io.LimitReader(r.Body, 32<<20))
		if err != nil {
			writeErr(w, http.StatusBadRequest, "본문을 읽지 못했습니다")
			return
		}
		_ = r.Body.Close()

		obj := map[string]any{}
		if len(bytes.TrimSpace(raw)) > 0 {
			if err := json.Unmarshal(raw, &obj); err != nil {
				writeErr(w, http.StatusBadRequest, "잘못된 JSON 본문")
				return
			}
		}
		if code != "" {
			if v, ok := obj["code"].(string); !ok || v == "" {
				obj["code"] = code
			}
		}
		if user != "" {
			if v, ok := obj["user"].(string); !ok || v == "" {
				obj["user"] = user
			}
		}
		merged, err := json.Marshal(obj)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "본문 재구성 실패")
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(merged))
		r.ContentLength = int64(len(merged))
		h(w, r)
	}
}

// withQueryCode — GET/DELETE 계열: 경로 {code} 와 사용자 식별을 쿼리에 주입한다.
func (s *Server) withQueryCode(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if code := r.PathValue("code"); code != "" && q.Get("code") == "" {
			q.Set("code", code)
		}
		if user := s.routeUser(r); user != "" && q.Get("user") == "" {
			q.Set("user", user)
		}
		r.URL.RawQuery = q.Encode()
		h(w, r)
	}
}

// routeUser — 사용자 식별(§7.3). 현재 런처 설정에는 사용자 개념이 없고(요청마다 받는다)
// Git.User 는 Gitea 계정이라 용도가 다르다. 그래서 헤더만 보고, 없으면 빈 문자열을 돌려
// 호출자가 바디·쿼리에 담아 보낸 값을 그대로 쓰게 둔다.
func (s *Server) routeUser(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get(launcherUserHeader))
}
