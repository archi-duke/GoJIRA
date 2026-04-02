package handlers

import (
	"context"
	"net/http"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"gojira-api/db"
	"gojira-api/models"
)

// POST /api/v{version}/Issue/label
func LabelCreateHandler(w http.ResponseWriter, r *http.Request) {
	var req models.LabelRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Key == "" || req.LabelContent == "" {
		respondError(w, http.StatusBadRequest, "key and labelContent are required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Add label to MongoDB issue (push to array), mark dirty
	_, err := db.Issues().UpdateOne(ctx,
		bson.M{"key": req.Key},
		bson.M{
			"$addToSet": bson.M{"fields.labels": req.LabelContent},
			"$set":      bson.M{"_dirty": true},
		},
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	db.SyncIssueToJira(req.Key)
	respondJSON(w, http.StatusOK, "label added")
}

// DELETE /api/v{version}/Issue/label
func LabelDeleteHandler(w http.ResponseWriter, r *http.Request) {
	var req models.LabelRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Key == "" || req.LabelContent == "" {
		respondError(w, http.StatusBadRequest, "key and labelContent are required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	_, err := db.Issues().UpdateOne(ctx,
		bson.M{"key": req.Key},
		bson.M{
			"$pull": bson.M{"fields.labels": req.LabelContent},
			"$set":  bson.M{"_dirty": true},
		},
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	db.SyncIssueToJira(req.Key)
	respondJSON(w, http.StatusOK, "label removed")
}
