// 사내 사용자 조회 핸들러 테스트.
//
//	실행: MONGO_URI=x PLATFORM_GITHUB_TOKEN=x PLATFORM_GITHUB_ORG=x \
//	      GITHUB_URL=http://x/ GOJIRA_API_URL=x go test ./handlers
//
// (config.init() 이 필수 환경변수를 요구하며 TestMain 보다 먼저 실행된다)
//
// Mongo 에 접속하지 않는 경로만 다룬다 — 미설정(503)·입력 검증(400)·응답 매핑.
package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"platform-api/config"
)

// 사외 기본 상태(USER_API_BASE 미설정)에서는 503 과 안내 문구를 돌려줘야 한다.
// 이 값이 사내 주소로 기본 설정되는 일이 없도록 하는 회귀 방지용이기도 하다.
func TestUserLookupUnconfiguredReturns503(t *testing.T) {
	restore := setUserAPIBase("")
	defer restore()

	r := chi.NewRouter()
	r.Get("/lookup/{knoxId}", UserLookupHandler)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/lookup/hong.gildong", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "USER_API_BASE") {
		t.Errorf("안내 문구에 USER_API_BASE 가 없다: %s", rec.Body.String())
	}
}

func TestUserLookupByNameUnconfiguredReturns503(t *testing.T) {
	restore := setUserAPIBase("")
	defer restore()

	rec := httptest.NewRecorder()
	UserLookupByNameHandler(rec, httptest.NewRequest(http.MethodGet, "/lookupByName?name=%ED%99%8D%EA%B8%B8%EB%8F%99", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

// 이름이 비면 사내 API 를 부르기 전에 400 이어야 한다.
func TestUserLookupByNameRequiresName(t *testing.T) {
	restore := setUserAPIBase("http://user-api.example.com")
	defer restore()

	rec := httptest.NewRecorder()
	UserLookupByNameHandler(rec, httptest.NewRequest(http.MethodGet, "/lookupByName?name=", nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// CommonAPI 응답 → 프런트 응답 매핑. knoxId 는 CommonAPI 의 loginId 에서 온다.
func TestCommonAPIUserToResult(t *testing.T) {
	u := commonAPIUser{
		LoginID: "hong.gildong", Name: "홍길동", NameEn: "Gildong Hong",
		Email: "hong@example.com", EmployeeNumber: "12345678",
		Department: "개발팀", Company: "example", Title: "책임",
	}
	got := u.toResult(true)

	if got.KnoxID != "hong.gildong" {
		t.Errorf("KnoxID = %q — loginId 가 매핑돼야 한다", got.KnoxID)
	}
	if !got.Registered {
		t.Error("Registered 가 반영되지 않았다")
	}

	// JSON 키가 프런트 계약과 일치해야 한다.
	raw, _ := json.Marshal(got)
	var m map[string]interface{}
	_ = json.Unmarshal(raw, &m)
	for _, k := range []string{"knoxId", "name", "nameEn", "email", "employeeNumber", "department", "company", "title", "registered"} {
		if _, ok := m[k]; !ok {
			t.Errorf("응답에 %q 키가 없다", k)
		}
	}
}

func setUserAPIBase(v string) func() {
	prev := config.UserAPIBase
	config.UserAPIBase = v
	return func() { config.UserAPIBase = prev }
}
