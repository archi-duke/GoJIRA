// Gitea 클라이언트 단위 테스트.
//
// 실행: PLATFORM_ENV_FILE=testdata/test.env go test ./github/
// config.init() 이 필수 환경변수를 요구하며(미설정 시 log.Fatal) TestMain 보다 먼저
// 실행되므로, 더미 설정 경로를 환경변수로 넘겨야 한다. 테스트는 Gitea API 를
// httptest 로 스텁하고 Mongo 에는 접근하지 않는다.
package github

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"platform-api/config"
)

// "승인 부족" 거절만 force_merge 폴백 대상이어야 한다 — repo lock 등 다른 405 와
// 섞이면 protection 우회가 엉뚱한 상황에서 발동한다.
func TestIsApprovalShortfall(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"승인 부족", errString(`merge PR failed (405): {"message":"Does not have enough approvals"}`), true},
		{"repo lock", errString(`merge PR failed (405): {"message":"merge conflict"}`), false},
		{"충돌", errString(`merge PR failed (409): conflict`), false},
		{"nil", nil, false},
	}
	for _, c := range cases {
		if got := isApprovalShortfall(c.err); got != c.want {
			t.Errorf("%s: isApprovalShortfall = %v, want %v", c.name, got, c.want)
		}
	}
}

// 정상 경로: 사용자 토큰 + protection 준수(force_merge 없음) 로 머지돼야 한다.
// 머지 주체가 Gitea 이력에 실제 작업자로 남는 것이 이 전환의 목적이다.
func TestMergePRUsesUserTokenWithoutForce(t *testing.T) {
	var gotAuth string
	var gotBody map[string]interface{}
	srv := giteaStub(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotBody = decodeBody(r)
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	if err := MergePR("gojira", "doc", 7, "merge", "usertok"); err != nil {
		t.Fatalf("머지 실패: %v", err)
	}
	if gotAuth != "Bearer usertok" {
		t.Errorf("Authorization = %q — 사용자 토큰이어야 한다", gotAuth)
	}
	if _, ok := gotBody["force_merge"]; ok {
		t.Errorf("force_merge 가 실렸다: %v — 평시에는 protection 을 준수해야 한다", gotBody)
	}
}

// 승인 집계가 어긋나 거절되면 관리자 토큰 + force_merge 로 폴백해야 한다
// (머지가 전면 차단되는 회귀를 막는 안전망).
func TestMergePRFallsBackToForceOnApprovalShortfall(t *testing.T) {
	type attempt struct {
		auth  string
		force bool
	}
	var attempts []attempt
	srv := giteaStub(t, func(w http.ResponseWriter, r *http.Request) {
		body := decodeBody(r)
		_, forced := body["force_merge"]
		attempts = append(attempts, attempt{auth: r.Header.Get("Authorization"), force: forced})
		if !forced {
			w.WriteHeader(http.StatusMethodNotAllowed)
			_, _ = w.Write([]byte(`{"message":"Does not have enough approvals"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	if err := MergePR("gojira", "doc", 7, "merge", "usertok"); err != nil {
		t.Fatalf("폴백 머지 실패: %v", err)
	}
	if len(attempts) != 2 {
		t.Fatalf("시도 횟수 = %d, want 2 (%+v)", len(attempts), attempts)
	}
	if attempts[0].force {
		t.Error("1차 시도에 force_merge 가 실렸다")
	}
	if attempts[0].auth != "Bearer usertok" {
		t.Errorf("1차 Authorization = %q — 사용자 토큰이어야 한다", attempts[0].auth)
	}
	if !attempts[1].force {
		t.Error("2차 시도에 force_merge 가 없다")
	}
	if attempts[1].auth != "Bearer admintok" {
		t.Errorf("2차 Authorization = %q — 관리자 토큰이어야 한다", attempts[1].auth)
	}
}

// 승인 부족이 아닌 실패(충돌 등)는 폴백하지 않는다 — protection 우회를
// 엉뚱한 실패에까지 쓰면 안 된다.
func TestMergePRDoesNotFallBackOnOtherErrors(t *testing.T) {
	var calls int
	srv := giteaStub(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"merge conflict"}`))
	})
	defer srv.Close()

	if err := MergePR("gojira", "doc", 7, "merge", "usertok"); err == nil {
		t.Fatal("충돌인데 성공으로 보고됐다")
	}
	if calls != 1 {
		t.Errorf("호출 %d 회 — 폴백하지 않아야 한다", calls)
	}
}

// 승인 리뷰는 Gitea 가 인식하는 "APPROVED" 로 보내야 한다. "APPROVE"(GitHub 표기)를
// 그대로 보내면 PENDING 으로 저장되어 official 집계에서 빠지고, 그 결과 머지가
// 승인 부족으로 막힌다 — force_merge 우회가 생긴 원인이었다.
func TestCreateReviewNormalizesApproveEvent(t *testing.T) {
	var gotEvent string
	srv := giteaStub(t, func(w http.ResponseWriter, r *http.Request) {
		body := decodeBody(r)
		gotEvent, _ = body["event"].(string)
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	if err := CreateReview("gojira", "doc", 7, "APPROVE", "LGTM", "usertok"); err != nil {
		t.Fatalf("리뷰 생성 실패: %v", err)
	}
	if gotEvent != "APPROVED" {
		t.Errorf("event = %q, want %q", gotEvent, "APPROVED")
	}
}

// --- helpers ---

// giteaStub — config 를 Gitea 형태(/api/v1)로 바꿔치기한 테스트 서버.
// 관리자 토큰은 "admintok" 으로 고정해 폴백 주체를 검증할 수 있게 한다.
func giteaStub(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	prevAPI, prevTok := config.GitHubAPI, config.GitHubToken
	config.GitHubAPI = srv.URL + "/api/v1"
	config.GitHubToken = "admintok"
	t.Cleanup(func() {
		config.GitHubAPI, config.GitHubToken = prevAPI, prevTok
	})
	if !IsGitea() {
		t.Fatal("스텁이 Gitea 로 인식되지 않았다")
	}
	return srv
}

func decodeBody(r *http.Request) map[string]interface{} {
	raw, _ := io.ReadAll(r.Body)
	m := map[string]interface{}{}
	_ = json.Unmarshal(raw, &m)
	return m
}

type errString string

func (e errString) Error() string { return string(e) }
