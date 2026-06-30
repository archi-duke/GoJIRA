package handlers

import (
	"encoding/json"
	"net/http"

	"go.mongodb.org/mongo-driver/v2/bson"

	authmw "gojira-api/auth"
)

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, status int, msg string) {
	respondJSON(w, status, map[string]string{"error": msg})
}

func decodeJSON(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// extractJiraCode extracts jiraCodes.main.jiraCode from a project bson.M document.
// MongoDB v2 driver returns nested docs as bson.D, so handle both bson.M and bson.D.
func extractJiraCode(prj map[string]interface{}) string {
	return bsonGetStr(bsonGetDoc(bsonGetDoc(prj, "jiraCodes"), "main"), "jiraCode")
}

// bsonGetDoc retrieves a nested document field, handling both bson.M and bson.D.
func bsonGetDoc(doc interface{}, key string) interface{} {
	switch d := doc.(type) {
	case map[string]interface{}:
		return d[key]
	case bson.M:
		return d[key]
	case bson.D:
		for _, e := range d {
			if e.Key == key {
				return e.Value
			}
		}
	}
	return nil
}

// bsonGetStr retrieves a string field from a document, handling both bson.M and bson.D.
func bsonGetStr(doc interface{}, key string) string {
	switch d := doc.(type) {
	case map[string]interface{}:
		s, _ := d[key].(string)
		return s
	case bson.M:
		s, _ := d[key].(string)
		return s
	case bson.D:
		for _, e := range d {
			if e.Key == key {
				s, _ := e.Value.(string)
				return s
			}
		}
	}
	return ""
}

// parseXBody parses the XBody header as JSON.
func parseXBody(r *http.Request, v interface{}) error {
	xbody := r.Header.Get("XBody")
	if xbody == "" {
		return nil
	}
	return json.Unmarshal([]byte(xbody), v)
}

// resolveUser — 요청자 신원은 auth 미들웨어가 검증해 context 에 넣은 값만 신뢰한다.
// (client 가 보낸 body.User/X-User-Id/?user= 는 더 이상 신뢰하지 않음 — 사칭 차단.)
// 대신 행위 사용자(act-as)는 미들웨어의 서비스토큰 경로에서 X-User-Id 로 들어와 context 에 반영됨.
// bodyUser 인자는 시그니처 호환을 위해 남겨두되 사용하지 않는다.
func resolveUser(r *http.Request, bodyUser string) (string, bool) {
	if u := authmw.UserID(r.Context()); u != "" {
		return u, true
	}
	return "", false
}
