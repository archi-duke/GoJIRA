package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"gojira-api/jiraclient"
	"gojira-api/models"
)

// POST /api/v{version}/Issue/link
func LinkCreateHandler(w http.ResponseWriter, r *http.Request) {
	var req models.LinkCreateRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.InwardKey == "" || req.OutwardKey == "" {
		respondError(w, http.StatusBadRequest, "inwardKey and outwardKey are required")
		return
	}

	linkType := req.LinkType
	if linkType == "" {
		linkType = "Relates"
	}

	if err := jiraclient.CreateIssueLink(req.InwardKey, req.OutwardKey, linkType); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, "link created")
}

// DELETE /api/v{version}/Issue/link
func LinkDeleteHandler(w http.ResponseWriter, r *http.Request) {
	var req models.LinkDeleteRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.LinkID == "" {
		respondError(w, http.StatusBadRequest, "linkId is required")
		return
	}

	if err := jiraclient.DeleteIssueLink(req.LinkID); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, "link deleted")
}

// DELETE /api/v{version}/Issue/link/{linkId}/of/{key}
func LinkDeleteByKeyHandler(w http.ResponseWriter, r *http.Request) {
	linkID := chi.URLParam(r, "linkId")
	// key is available but link deletion only needs linkID
	if linkID == "" {
		respondError(w, http.StatusBadRequest, "linkId is required")
		return
	}

	if err := jiraclient.DeleteIssueLink(linkID); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, "link deleted")
}
