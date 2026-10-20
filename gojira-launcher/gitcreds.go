package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// 사용자별 Gitea 자격(username + PAT) 해석.
//
// 배경: 이전에는 launcher.json 의 git.user/git.token 에 사람이 PAT 를 손으로 적어두고
// 모든 clone/fetch/push 가 그 한 계정으로 나갔다. 공용 관리자 PAT(gojira-admin)를
// 적어두는 운영이 굳어, Gitea 상의 pusher 가 전원 동일 계정으로 기록되는 문제가 있었다.
//
// 지금은 요청 사용자(knoxId)의 개인 PAT 를 Platform-API 에서 런타임에 받아 쓴다.
// PAT 는 사용자 등록 시 Gitea 계정과 함께 자동 발급되어 서버에 저장돼 있다.
// 조회 실패(백엔드 미도달 / 토큰 미발급 / 구버전 백엔드)면 launcher.json 설정으로
// 폴백하므로 기존 환경에서도 동작이 깨지지 않는다.

// gitCred — 캐시 항목. token 이 빈 문자열이면 "조회했지만 없음"(네거티브) 을 뜻하고,
// 호출 측은 launcher.json 설정으로 폴백한다.
type gitCred struct {
	login   string
	token   string
	expires time.Time
}

const (
	// gitCredTTL — 조회 성공분 보관 기간. git 명령마다 HTTP 가 나가지 않게 한다.
	// 서버에서 PAT 를 재발급하면 이 시간 안에 반영된다.
	gitCredTTL = 10 * time.Minute
	// gitCredNegTTL — 실패분 보관 기간. 백엔드가 404/다운일 때 매 git 호출마다
	// 재시도해 지연이 누적되는 것을 막는다(사내망에서 특히 체감).
	gitCredNegTTL = 1 * time.Minute
)

// userGitCreds — user(knoxId) 의 Gitea 자격을 반환한다.
// 우선순위: 캐시 → Platform-API → launcher.json(git.user/git.token).
func (s *Server) userGitCreds(user string) (login, token string) {
	if user == "" {
		return s.cfg.Git.User, s.cfg.Git.Token
	}

	if v, ok := s.gitCreds.Load(user); ok {
		if c, ok := v.(gitCred); ok && time.Now().Before(c.expires) {
			if c.token != "" {
				return c.login, c.token
			}
			return s.cfg.Git.User, s.cfg.Git.Token
		}
	}

	fetchedLogin, fetchedToken, ok := s.fetchGitCreds(user)
	ttl := gitCredTTL
	if !ok {
		fetchedLogin, fetchedToken, ttl = "", "", gitCredNegTTL
	}
	s.gitCreds.Store(user, gitCred{login: fetchedLogin, token: fetchedToken, expires: time.Now().Add(ttl)})

	if fetchedToken != "" {
		return fetchedLogin, fetchedToken
	}
	return s.cfg.Git.User, s.cfg.Git.Token
}

// fetchGitCreds — Platform-API 에서 본인 Gitea 자격을 조회.
//
// 엔드포인트는 경로에 사용자 파라미터가 없고 검증된 신원의 자격만 돌려주므로,
// X-User-Id 는 SSO 비활성(dev) 환경에서의 신원 지정용이고 SSO 환경에서는
// Bearer 세션토큰이 실제 신원을 결정한다.
//
// 보안: 반환 토큰은 로그에 남기지 않는다(길이/성공 여부만 기록).
func (s *Server) fetchGitCreds(user string) (string, string, bool) {
	base := strings.TrimRight(s.cfg.Backend.PlatformAPI, "/")
	if base == "" {
		return "", "", false
	}

	req, err := http.NewRequest(http.MethodGet, base+"/Account/me/gitCredentials", nil)
	if err != nil {
		return "", "", false
	}
	req.Header.Set("X-User-Id", user)
	if t := s.ssoToken(); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}

	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		log.Printf("userGitCreds warn (%s): %v — launcher.json 설정으로 폴백", user, err)
		return "", "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		log.Printf("userGitCreds warn (%s): HTTP %d — launcher.json 설정으로 폴백", user, resp.StatusCode)
		return "", "", false
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	var payload struct {
		Login string `json:"login"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.Token == "" {
		log.Printf("userGitCreds warn (%s): 응답 파싱 실패 — launcher.json 설정으로 폴백", user)
		return "", "", false
	}

	login := payload.Login
	if login == "" {
		login = user
	}
	log.Printf("userGitCreds 조회 완료 (%s): login=%s token=%d자", user, login, len(payload.Token))
	return login, payload.Token, true
}
