// Package auth — SSO sessionToken 검증 미들웨어 (Platform-API/auth 와 동일 로직).
//
// 보안 모델: 프론트는 MobilAve sessionToken 만 보관. 백엔드가 Authorization: Bearer
// <sessionToken> 을 introspect 검증해 신원 추출 → context 주입. 신뢰 컴포넌트(런처/CLI/
// 스킬/서비스간)는 X-Service-Token + X-User-Id 로 사용자 대신 행위. 핸들러는 client 의
// knoxId/X-User-Id 를 직접 신뢰하지 않고 auth.UserID(ctx) 만 쓴다 → 사칭 차단.
package auth

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type ctxKey string

const userIDKey ctxKey = "authUser"

// 비인증 허용 경로 — 런처 데몬·Gitea 웹훅은 SSO 세션 없이 호출한다.
var publicSubstrings = []string{"/launcher/", "/webhook/"}

func isPublic(path string) bool {
	for _, s := range publicSubstrings {
		if strings.Contains(path, s) {
			return true
		}
	}
	return false
}

// Middleware — USE_SSO=true 면 Bearer introspect 또는 X-Service-Token+X-User-Id 검증,
// 실패 시 401. USE_SSO!=true(dev)면 X-User-Id/X-Dev-User 또는 DEV_USER 폴백.
// 비인증 경로와 OPTIONS preflight 는 통과.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions || isPublic(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		if os.Getenv("USE_SSO") != "true" {
			u := r.Header.Get("X-User-Id")
			if u == "" {
				u = r.Header.Get("X-Dev-User")
			}
			if u == "" {
				u = devUser()
			}
			next.ServeHTTP(w, withUser(r, u))
			return
		}

		// 신뢰 컴포넌트 — X-Service-Token + X-User-Id(행위 사용자).
		if st := r.Header.Get("X-Service-Token"); st != "" {
			if !validServiceToken(st) {
				http.Error(w, "unauthorized (service token)", http.StatusUnauthorized)
				return
			}
			// 행위 사용자(act-as): 있으면 그 사용자로, 없으면 서비스 자체(빈 신원).
			// 사용자가 꼭 필요한 핸들러는 auth.UserID(ctx)=="" 를 보고 거부한다.
			next.ServeHTTP(w, withUser(r, r.Header.Get("X-User-Id")))
			return
		}

		// 브라우저 — Bearer sessionToken → introspect.
		token := bearerToken(r)
		if token == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		loginID, err := introspect(token)
		if err != nil || loginID == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, withUser(r, loginID))
	})
}

// UserID — 검증된 요청자 loginid (없으면 "").
func UserID(ctx context.Context) string {
	v, _ := ctx.Value(userIDKey).(string)
	return v
}

func withUser(r *http.Request, user string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), userIDKey, user))
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(h[len("Bearer "):])
	}
	return ""
}

func validServiceToken(got string) bool {
	want := os.Getenv("SERVICE_TOKEN")
	if want == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func devUser() string {
	if u := os.Getenv("DEV_USER"); u != "" {
		return u
	}
	return "duke.kimm"
}

var introspectClient = &http.Client{
	Timeout: 3 * time.Second,
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: os.Getenv("SSO_INSECURE_TLS") == "true"},
	},
}

func introspect(token string) (string, error) {
	base := os.Getenv("SSO_INTROSPECT_URL")
	if base == "" {
		return "", fmt.Errorf("SSO_INTROSPECT_URL 미설정")
	}
	u := fmt.Sprintf("%s/Account/introspect?token=%s", strings.TrimRight(base, "/"), url.QueryEscape(token))
	resp, err := introspectClient.Get(u)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("introspect status %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	var data struct {
		LoginID string `json:"loginid"`
		Active  string `json:"active"`
	}
	if err := json.Unmarshal(body, &data); err != nil || data.LoginID == "" || data.Active == "false" {
		return "", fmt.Errorf("invalid introspect response")
	}
	return data.LoginID, nil
}
