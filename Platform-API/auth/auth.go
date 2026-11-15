// Package auth — SSO sessionToken 검증 미들웨어.
//
// 보안 모델: 프론트는 MobilAve 가 발급한 sessionToken 만 보관(loginid 자가발급 금지).
// 백엔드가 매 요청의 Authorization: Bearer <sessionToken> 을 MobilAve introspect 로
// 검증해 신원을 추출하고 context 에 주입한다. 핸들러는 client 가 보낸 knoxId/X-User-Id
// 를 더 이상 신뢰하지 않고 auth.UserID(ctx) 만 쓴다 → 사칭 차단.
package auth

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"encoding/json"
	"errors"
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

// 비인증 허용 경로 — 런처 데몬(자동업데이트/다운로드)·Gitea 웹훅은 SSO 세션 없이 호출한다.
//
// `/Notice/ws` 는 **핸드셰이크만** 통과시킨다. 브라우저 WebSocket 은 커스텀 헤더를
// 실을 수 없어 Authorization 으로 인증할 수 없기 때문이다. 연결 직후 클라이언트가
// 보내는 첫 메시지로 NoticeWSHandler 가 직접 인증하고, 실패하면 닫는다
// (계약 05 §WebSocket 인증, 2026-07-07 D2). **핸들러의 인증이 유일한 관문이므로
// 그 코드를 지우면 인증 없는 브로드캐스트가 열린다.**
var publicSubstrings = []string{"/launcher/", "/webhook/", "/Notice/ws"}

func isPublic(path string) bool {
	for _, s := range publicSubstrings {
		if strings.Contains(path, s) {
			return true
		}
	}
	return false
}

// Middleware — USE_SSO=true 면 Bearer 토큰을 introspect 검증 후 loginid 를 context 주입,
// 실패 시 401. USE_SSO!=true(dev)면 X-User-Id/X-Dev-User 헤더 또는 DEV_USER 로 폴백.
// 비인증 경로와 OPTIONS preflight 는 통과.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions || isPublic(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		if os.Getenv("USE_SSO") != "true" {
			// dev: 보안 비활성 — 클라이언트가 보낸 사용자 또는 DEV_USER.
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

		// 신뢰 컴포넌트(런처/CLI/스킬/서비스간) — X-Service-Token 검증 시 X-User-Id(행위 사용자) 신뢰.
		// 브라우저 세션이 없는 트러스티드 호출자가 사용자 대신 행위하는 표준 패턴.
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

		// 브라우저 — Authorization: Bearer <sessionToken> → MobilAve introspect.
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

// AuthenticateToken — WebSocket 등 **헤더를 실을 수 없는 연결**이 첫 메시지로 보낸
// 토큰을 검증한다 (계약 05 §WebSocket 인증). 성공 시 loginid 를 돌려준다.
//
// 브라우저 WebSocket 핸드셰이크는 커스텀 헤더를 지정할 수 없어 Authorization 을 쓸 수
// 없다. 쿼리파라미터는 액세스 로그에 토큰이 남아 기각했다(2026-07-07 D2).
//
// dev(USE_SSO!=true)에서는 검증 없이 fallbackUser 또는 DEV_USER 를 돌려준다 —
// Middleware 의 dev 분기와 같은 규칙이다.
func AuthenticateToken(token, fallbackUser string) (string, error) {
	if os.Getenv("USE_SSO") != "true" {
		if fallbackUser != "" {
			return fallbackUser, nil
		}
		return devUser(), nil
	}
	if token == "" {
		return "", errors.New("token required")
	}
	// 신뢰 컴포넌트가 WS 로 붙는 경우 — 서비스 토큰도 받아준다.
	if validServiceToken(token) {
		return fallbackUser, nil
	}
	loginID, err := introspect(token)
	if err != nil {
		return "", err
	}
	if loginID == "" {
		return "", errors.New("introspect returned empty loginid")
	}
	return loginID, nil
}

// validServiceToken — 상수시간 비교. SERVICE_TOKEN 미설정이면 항상 거부(서비스토큰 비활성).
func validServiceToken(got string) bool {
	want := os.Getenv("SERVICE_TOKEN")
	if want == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
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

func devUser() string {
	if u := os.Getenv("DEV_USER"); u != "" {
		return u
	}
	return "duke.kimm"
}

var introspectClient = &http.Client{
	Timeout: 3 * time.Second,
	Transport: &http.Transport{
		// MobilAve 자체서명 인증서 대비 — 기본은 검증, SSO_INSECURE_TLS=true 시에만 우회.
		TLSClientConfig: &tls.Config{InsecureSkipVerify: os.Getenv("SSO_INSECURE_TLS") == "true"},
	},
}

// introspect — MobilAve /Account/introspect?token= 호출. {loginid} 추출, 401/오류면 error.
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
