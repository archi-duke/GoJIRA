package handlers

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"platform-api/config"
	"platform-api/db"
)

// 사내 사용자 조회 API(CommonAPI) 대리 호출.
//
// 사용자 등록 시 Knox ID 나 이름으로 임직원을 찾아 이름·이메일을 채우기 위한 것.
// 브라우저가 사내 API 를 직접 부르지 않고 Platform-API 가 대리한다 — 사내 API 주소와
// TLS 설정을 서버에만 두고, 플랫폼 등록 여부(registered)를 함께 내려주기 위함이다.
//
// 사외에서는 USER_API_BASE 가 비어 있어 조회가 503 으로 떨어지는 것이 정상이다.

// commonAPIUser — CommonAPI 응답 1건.
//
// 주의: 필드명은 CommonAPI 스펙에 맞춰야 한다. 사내 스펙 확인 후 json 태그를
// 교정할 것. 바깥(LookupUser)으로 내보내는 이름은 이 구조체 안에서만 매핑되므로
// 태그만 고치면 되고 핸들러·프런트는 건드릴 필요가 없다.
type commonAPIUser struct {
	LoginID        string `json:"loginId"`
	Name           string `json:"name"`
	NameEn         string `json:"nameEn"`
	Email          string `json:"email"`
	EmployeeNumber string `json:"employeeNumber"`
	Department     string `json:"department"`
	Company        string `json:"company"`
	Title          string `json:"title"`
}

// lookupResult — 프런트로 내려가는 형태.
type lookupResult struct {
	KnoxID         string `json:"knoxId"`
	Name           string `json:"name"`
	NameEn         string `json:"nameEn"`
	Email          string `json:"email"`
	EmployeeNumber string `json:"employeeNumber"`
	Department     string `json:"department"`
	Company        string `json:"company"`
	Title          string `json:"title"`
	Registered     bool   `json:"registered"`
}

func (u commonAPIUser) toResult(registered bool) lookupResult {
	return lookupResult{
		KnoxID:         u.LoginID,
		Name:           u.Name,
		NameEn:         u.NameEn,
		Email:          u.Email,
		EmployeeNumber: u.EmployeeNumber,
		Department:     u.Department,
		Company:        u.Company,
		Title:          u.Title,
		Registered:     registered,
	}
}

// userAPIClient — 사내 API 전용 클라이언트.
// UserAPIInsecureTLS 가 true 일 때만 검증을 우회한다(사내 CA 미신뢰 환경 대응).
func userAPIClient() *http.Client {
	c := &http.Client{Timeout: 10 * time.Second}
	if config.UserAPIInsecureTLS {
		c.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // #nosec G402 — 명시 설정 시에만
		}
	}
	return c
}

// errUserAPINotConfigured — USER_API_BASE 미설정. 사외 기본 상태다.
const msgUserAPINotConfigured = "사내 사용자 조회 API(USER_API_BASE)가 설정되지 않았습니다."

// callUserAPI — CommonAPI 를 호출해 원문 바이트를 돌려준다.
// 미존재는 CommonAPI 가 non-200 + "Not Found user" 텍스트로 답한다.
func callUserAPI(ctx context.Context, path string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, config.UserAPIBase+path, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := userAPIClient().Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return body, resp.StatusCode, nil
}

// UserLookupHandler — GET /Account/lookup/{knoxId}
// 사내 사용자 API 에서 임직원 1명을 조회하고 플랫폼 등록 여부를 덧붙인다.
func UserLookupHandler(w http.ResponseWriter, r *http.Request) {
	knoxID := strings.TrimSpace(chi.URLParam(r, "knoxId"))
	if knoxID == "" {
		respondError(w, http.StatusBadRequest, "knoxId 가 필요합니다")
		return
	}
	if config.UserAPIBase == "" {
		respondError(w, http.StatusServiceUnavailable, msgUserAPINotConfigured)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	body, status, err := callUserAPI(ctx, "/api/v1/knox/userID/"+url.PathEscape(knoxID))
	if err != nil {
		respondError(w, http.StatusBadGateway, "사내 사용자 조회 실패: "+err.Error())
		return
	}
	// CommonAPI 는 미존재를 non-200 + "Not Found user" 텍스트로 답한다.
	if status != http.StatusOK {
		respondError(w, http.StatusNotFound, fmt.Sprintf("'%s' 임직원 정보를 찾을 수 없습니다.", knoxID))
		return
	}

	var u commonAPIUser
	if err := json.Unmarshal(body, &u); err != nil || u.LoginID == "" {
		respondError(w, http.StatusNotFound, fmt.Sprintf("'%s' 임직원 정보를 찾을 수 없습니다.", knoxID))
		return
	}

	registered := db.ActiveKnoxIDs(ctx, []string{u.LoginID})[u.LoginID]
	respondJSON(w, http.StatusOK, u.toResult(registered))
}

// UserLookupByNameHandler — GET /Account/lookupByName?name=
// 동명이인을 전부(계열사 포함) 돌려준다. CommonAPI 는 항상 배열이고 없으면 [] 다.
func UserLookupByNameHandler(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		respondError(w, http.StatusBadRequest, "name 이 필요합니다")
		return
	}
	if config.UserAPIBase == "" {
		respondError(w, http.StatusServiceUnavailable, msgUserAPINotConfigured)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	body, status, err := callUserAPI(ctx, "/api/v1/knox/userName/"+url.PathEscape(name))
	if err != nil {
		respondError(w, http.StatusBadGateway, "사내 사용자 조회 실패: "+err.Error())
		return
	}
	if status != http.StatusOK {
		respondJSON(w, http.StatusOK, []lookupResult{})
		return
	}

	var users []commonAPIUser
	if err := json.Unmarshal(body, &users); err != nil {
		respondJSON(w, http.StatusOK, []lookupResult{})
		return
	}

	// loginId 가 빈 항목은 식별이 불가능하므로 제외한다.
	ids := make([]string, 0, len(users))
	for _, u := range users {
		if u.LoginID != "" {
			ids = append(ids, u.LoginID)
		}
	}
	registered := db.ActiveKnoxIDs(ctx, ids)

	results := make([]lookupResult, 0, len(ids))
	for _, u := range users {
		if u.LoginID == "" {
			continue
		}
		results = append(results, u.toResult(registered[u.LoginID]))
	}
	respondJSON(w, http.StatusOK, results)
}
