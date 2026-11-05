package main

// 서비스 중계 (설계서 §7.3).
//
// 런처가 브라우저 보안(Origin·토큰·CORS)을 전담하고(D5), 서비스는 런처가 주입한
// 내부 비밀값만 검증한다. 그래서 중계할 때 브라우저발 신원 헤더는 **제거**하고
// 서비스용 헤더를 **주입**한다 — 브라우저가 서비스 포트를 직접 때려도 통과하지 못한다.

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const (
	defaultProxyTimeout = 120 * time.Second
	maxHopCount         = 3 // 서비스 간 호출 순환 방지(§7.4)
)

// proxyToService — 요청을 서비스로 중계한다. on-demand 면 기동·헬스까지 기다린다.
func (s *Server) proxyToService(w http.ResponseWriter, r *http.Request, e routeEntry) {
	// 순환 호출 방지(§7.4) — 서비스가 런처를 통해 다른 서비스를 부를 때 깊이를 센다.
	hop := 0
	if v := r.Header.Get("X-Svc-Hop"); v != "" {
		hop, _ = strconv.Atoi(v)
	}
	if hop > maxHopCount {
		writeRouteErr(w, http.StatusLoopDetected, "hop_limit",
			fmt.Sprintf("서비스 간 호출이 %d 단계를 넘었습니다", maxHopCount), nil)
		return
	}

	timeout := defaultProxyTimeout
	if e.route.TimeoutSec > 0 {
		timeout = time.Duration(e.route.TimeoutSec) * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	if err := s.sup.ensureRunning(ctx, e.service); err != nil {
		writeRouteErr(w, http.StatusBadGateway, "service_unavailable",
			fmt.Sprintf("서비스 %s 를 기동하지 못했습니다: %v", e.service, err),
			map[string]any{"service": e.service})
		return
	}

	port, secret := s.sup.endpoint(e.service)
	if port == 0 {
		writeRouteErr(w, http.StatusBadGateway, "service_unavailable",
			"서비스 주소를 확인하지 못했습니다", map[string]any{"service": e.service})
		return
	}

	// 본문 크기 제한(§7.3) — 초과는 413.
	body := r.Body
	if e.route.MaxBodyMB > 0 {
		limit := int64(e.route.MaxBodyMB) << 20
		if r.ContentLength > limit {
			writeRouteErr(w, http.StatusRequestEntityTooLarge, "body_too_large",
				fmt.Sprintf("본문이 %dMB 를 넘습니다", e.route.MaxBodyMB), nil)
			return
		}
		body = http.MaxBytesReader(w, r.Body, limit)
	}

	// 경로·쿼리는 그대로 전달한다 — prefix 를 떼지 않는다(§7.3).
	target := fmt.Sprintf("http://127.0.0.1:%d%s", port, r.URL.RequestURI())
	out, err := http.NewRequestWithContext(ctx, r.Method, target, body)
	if err != nil {
		writeRouteErr(w, http.StatusInternalServerError, "proxy_error", err.Error(), nil)
		return
	}
	copyProxyHeaders(out, r)
	out.Header.Set("X-Svc-Secret", secret)
	out.Header.Set("X-Request-Id", newRequestID())
	out.Header.Set("X-Svc-Hop", strconv.Itoa(hop+1))
	if u := s.routeUser(r); u != "" {
		out.Header.Set(launcherUserHeader, u)
	}
	if tok := s.ssoToken(); tok != "" {
		out.Header.Set("X-Gojira-Session", tok)
	}

	resp, err := proxyClient.Do(out)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			writeRouteErr(w, http.StatusGatewayTimeout, "timeout",
				fmt.Sprintf("서비스 응답이 %v 를 넘었습니다", timeout), nil)
			return
		}
		writeRouteErr(w, http.StatusBadGateway, "proxy_error", err.Error(), nil)
		return
	}
	defer resp.Body.Close()

	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	streamBody(w, resp.Body)
}

// proxyClient — 스트리밍·SSE 를 위해 응답 압축/타임아웃을 런처가 직접 통제한다.
var proxyClient = &http.Client{
	Transport: &http.Transport{DisableCompression: true},
	// 전체 타임아웃은 요청별 context 로 건다 — 여기 두면 SSE 가 끊긴다.
}

// copyProxyHeaders — 브라우저발 신원 헤더를 **제거**하고 나머지는 넘긴다(§7.3).
func copyProxyHeaders(dst *http.Request, src *http.Request) {
	drop := map[string]bool{
		"X-Gojira-Launcher-Token": true, // 런처 전용 — 서비스에 흘리지 않는다
		"Cookie":                  true,
		"Origin":                  true,
		"Connection":              true,
		"Keep-Alive":              true,
		"Transfer-Encoding":       true,
		"Upgrade":                 true,
	}
	for k, vs := range src.Header {
		if drop[http.CanonicalHeaderKey(k)] {
			continue
		}
		for _, v := range vs {
			dst.Header.Add(k, v)
		}
	}
}

// streamBody — 메모리에 쌓지 않고 흘려보내며, SSE 처럼 끊겨 오는 응답은 즉시 flush 한다(§7.3).
func streamBody(w http.ResponseWriter, body io.Reader) {
	flusher, canFlush := w.(http.Flusher)
	buf := make([]byte, 32<<10)
	for {
		n, err := body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
			if canFlush {
				flusher.Flush()
			}
		}
		if err != nil {
			return
		}
	}
}

func newRequestID() string {
	s, err := randomSecret()
	if err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return s[:16]
}
