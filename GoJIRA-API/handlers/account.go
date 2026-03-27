package handlers

import (
	"context"
	"net/http"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"gojira-api/db"
	"gojira-api/models"
)

// POST /api/v{version}/Jira/Account
func AccountCreateHandler(w http.ResponseWriter, r *http.Request) {
	var req models.AccountRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.JiraID == "" || req.Pswd == "" {
		respondError(w, http.StatusBadRequest, "jiraID and pswd are required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	doc := bson.M{
		"jiraID":    req.JiraID,
		"pswd":      req.Pswd,
		"createdAt": time.Now(),
	}
	if err := db.UpsertAccount(ctx, req.JiraID, doc); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, "account registered")
}
