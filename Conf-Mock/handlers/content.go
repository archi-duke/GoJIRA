package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"conf-mock/store"
)

// POST /rest/api/content
func CreateContent(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Type  string            `json:"type"`
		Title string            `json:"title"`
		Space map[string]string `json:"space"`
		Body  struct {
			Storage struct {
				Value          string `json:"value"`
				Representation string `json:"representation"`
			} `json:"storage"`
		} `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.Title == "" {
		respondError(w, http.StatusBadRequest, "title is required")
		return
	}

	spaceKey := body.Space["key"]
	if spaceKey == "" {
		respondError(w, http.StatusBadRequest, "space.key is required")
		return
	}

	page := store.Create(body.Title, spaceKey, body.Body.Storage.Value)
	respondJSON(w, http.StatusOK, page)
}

// GET /rest/api/content/{id}
func GetContent(w http.ResponseWriter, r *http.Request) {
	id := extractID(r.URL.Path)
	if id == "" {
		// List/search mode
		SearchContent(w, r)
		return
	}

	page := store.Get(id)
	if page == nil {
		respondError(w, http.StatusNotFound, "content not found")
		return
	}
	respondJSON(w, http.StatusOK, page)
}

// GET /rest/api/content?spaceKey=...&title=...
func SearchContent(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	spaceKey := q.Get("spaceKey")
	title := q.Get("title")
	start, _ := strconv.Atoi(q.Get("start"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 25
	}

	results, total := store.Search(spaceKey, title, start, limit)

	resp := map[string]interface{}{
		"results": results,
		"start":   start,
		"limit":   limit,
		"size":    len(results),
		"_links":  map[string]string{},
	}

	if start+limit < total {
		nextStart := start + limit
		resp["_links"] = map[string]string{
			"next": fmt.Sprintf("/rest/api/content?spaceKey=%s&title=%s&start=%d&limit=%d",
				spaceKey, title, nextStart, limit),
		}
	}

	respondJSON(w, http.StatusOK, resp)
}

// PUT /rest/api/content/{id}
func UpdateContent(w http.ResponseWriter, r *http.Request) {
	id := extractID(r.URL.Path)
	if id == "" {
		respondError(w, http.StatusBadRequest, "content id is required")
		return
	}

	var body struct {
		Title   string `json:"title"`
		Version struct {
			Number int `json:"number"`
		} `json:"version"`
		Body struct {
			Storage struct {
				Value string `json:"value"`
			} `json:"storage"`
		} `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	page, err := store.Update(id, body.Title, body.Body.Storage.Value, body.Version.Number)
	if err != nil {
		if err.Error() == "not found" {
			respondError(w, http.StatusNotFound, "content not found")
		} else {
			respondError(w, http.StatusConflict, err.Error())
		}
		return
	}
	respondJSON(w, http.StatusOK, page)
}

// DELETE /rest/api/content/{id}
func DeleteContent(w http.ResponseWriter, r *http.Request) {
	id := extractID(r.URL.Path)
	if id == "" {
		respondError(w, http.StatusBadRequest, "content id is required")
		return
	}

	if !store.Delete(id) {
		respondError(w, http.StatusNotFound, "content not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- helpers ---

func extractID(path string) string {
	// /rest/api/content/12345 → "12345"
	prefix := "/rest/api/content/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	id := strings.TrimPrefix(path, prefix)
	id = strings.TrimSuffix(id, "/")
	if id == "" || strings.Contains(id, "/") {
		return ""
	}
	return id
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"statusCode": status,
		"message":    msg,
	})
}
