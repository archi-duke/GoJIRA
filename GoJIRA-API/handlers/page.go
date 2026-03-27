package handlers

import (
	"net/http"

	"gojira-api/models"
)

// POST /api/v{version}/Page/images - download images, delete non-requested local files
func PageImagesPostHandler(w http.ResponseWriter, r *http.Request) {
	var req models.PageImagesRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Mock: just acknowledge the request
	respondJSON(w, http.StatusOK, "images downloaded (clean mode)")
}

// PUT /api/v{version}/Page/images - download images, keep existing local files
func PageImagesPutHandler(w http.ResponseWriter, r *http.Request) {
	var req models.PageImagesRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Mock: just acknowledge the request
	respondJSON(w, http.StatusOK, "images downloaded (keep mode)")
}
