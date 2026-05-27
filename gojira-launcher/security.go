package main

import (
	"crypto/subtle"
	"log"
	"net/http"
	"strings"
)

// 보안 모델 (설계 §2.3) — git/프로세스를 실행하는 localhost 데몬이므로 필수.
//
//  1. 127.0.0.1 전용 bind        : main.go 의 listen 주소로 강제 (외부 IF 노출 금지)
//  2. Origin 허용목록            : 브라우저發 요청의 Origin 이 webOrigin 과 일치해야 함
//     (CSRF / DNS-rebinding 차단). Origin 헤더가 없는 로컬 도구(curl)는 통과시키되
//     민감 엔드포인트는 토큰을 요구한다.
//  3. 설치 토큰                  : /open /edit /status 는 Authorization: Bearer <token>
//     (또는 X-Gojira-Launcher-Token) 필요. 임의 웹페이지의 fetch 차단.
//  4. 명령 화이트리스트          : spawn.go (clone/checkout/code/CLI 만 실행)

const tokenHeader = "X-Gojira-Launcher-Token"

// secure 는 모든 핸들러를 감싸 Origin/CORS/토큰을 검증한다.
//   - requireToken=true 면 인증 토큰이 없으면 401.
func (s *Server) secure(requireToken bool, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		// CORS preflight — origin 검증 후 허용 헤더 회신.
		if r.Method == http.MethodOptions {
			if s.originAllowed(origin) {
				s.writeCORS(w, origin)
				w.WriteHeader(http.StatusNoContent)
			} else {
				http.Error(w, "origin not allowed", http.StatusForbidden)
			}
			return
		}

		// 브라우저發(Origin 존재) 요청은 허용목록과 일치해야 한다.
		if origin != "" {
			if !s.originAllowed(origin) {
				log.Printf("거부: origin=%q (허용 webOrigin=%q webOrigins=%v)", origin, s.cfg.WebOrigin, s.cfg.WebOrigins)
				http.Error(w, "origin not allowed", http.StatusForbidden)
				return
			}
			s.writeCORS(w, origin)
		}

		if requireToken && !s.tokenValid(r) {
			http.Error(w, "invalid or missing launcher token", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}

func (s *Server) originAllowed(origin string) bool {
	if origin == "" {
		return false
	}
	got := strings.TrimRight(origin, "/")
	if want := strings.TrimRight(s.cfg.WebOrigin, "/"); want != "" && got == want {
		return true
	}
	for _, w := range s.cfg.WebOrigins {
		if want := strings.TrimRight(w, "/"); want != "" && got == want {
			return true
		}
	}
	// webOrigin/webOrigins 미설정 또는 매치 없음: 안전하게 거부.
	return false
}

func (s *Server) writeCORS(w http.ResponseWriter, origin string) {
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Vary", "Origin")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, "+tokenHeader)
}

func (s *Server) tokenValid(r *http.Request) bool {
	got := r.Header.Get(tokenHeader)
	if got == "" {
		if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			got = strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		}
	}
	if got == "" || s.cfg.Token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(s.cfg.Token)) == 1
}
