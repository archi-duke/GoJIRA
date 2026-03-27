package handlers

import (
	"context"
	"net/http"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"gojira-api/db"
	"gojira-api/models"
)

// PUT /api/v{version}/Issue/relations
func RelationsHandler(w http.ResponseWriter, r *http.Request) {
	var req models.RelationsRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	issues, err := db.FindAllIssues(ctx, nil)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	linkCount := 0
	for _, issue := range issues {
		if fields, ok := issue["fields"].(bson.M); ok {
			if links, ok := fields["issuelinks"].(bson.A); ok {
				linkCount += len(links)
			}
		}
	}

	respondJSON(w, http.StatusOK, linkCount)
}
