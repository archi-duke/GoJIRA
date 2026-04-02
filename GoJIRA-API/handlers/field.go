package handlers

import (
	"context"
	"net/http"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"gojira-api/db"
	"gojira-api/models"
)

// PUT /api/v{version}/Issue/field
func FieldUpdateHandler(w http.ResponseWriter, r *http.Request) {
	var req models.FieldUpdateRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Key == "" || req.Field == "" {
		respondError(w, http.StatusBadRequest, "key and field are required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	updates := bson.M{
		"fields." + req.Field: req.Value,
		"_dirty":              true,
	}

	if err := db.UpdateIssueFields(ctx, req.Key, updates); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	db.SyncIssueToJira(req.Key)
	respondJSON(w, http.StatusOK, true)
}
