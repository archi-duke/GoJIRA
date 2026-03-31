package handlers

import (
	"encoding/json"
	"net/http"
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
func extractJiraCode(prj map[string]interface{}) string {
	jiraCodes, _ := prj["jiraCodes"].(map[string]interface{})
	if jiraCodes == nil {
		return ""
	}
	main, _ := jiraCodes["main"].(map[string]interface{})
	if main == nil {
		return ""
	}
	code, _ := main["jiraCode"].(string)
	return code
}

// parseXBody parses the XBody header as JSON.
func parseXBody(r *http.Request, v interface{}) error {
	xbody := r.Header.Get("XBody")
	if xbody == "" {
		return nil
	}
	return json.Unmarshal([]byte(xbody), v)
}
