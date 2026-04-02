package handlers

import (
	"net/http"

	"gojira-api/confluence"
)

type confluenceExportRequest struct {
	SpaceKey string `json:"spaceKey"`
	Title    string `json:"title"`
	Body     string `json:"body"`
}

// POST /api/v{version}/Page/confluence
func ConfluenceExportHandler(w http.ResponseWriter, r *http.Request) {
	var req confluenceExportRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.SpaceKey == "" || req.Title == "" || req.Body == "" {
		respondError(w, http.StatusBadRequest, "spaceKey, title, body are required")
		return
	}

	result, err := confluence.UpsertPage(req.SpaceKey, req.Title, req.Body)
	if err != nil {
		respondError(w, http.StatusBadGateway, "Confluence 내보내기 실패: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Confluence 페이지 생성/수정 완료",
		"page":    result,
	})
}
