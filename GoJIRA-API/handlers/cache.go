package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"gojira-api/db"
	"gojira-api/models"
)

// GET /api/v{version}/Issue/fetch — Pull from JIRA to MongoDB
func IssueFetchHandler(w http.ResponseWriter, r *http.Request) {
	var req models.FetchRequest
	parseXBody(r, &req)

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	count, err := db.PullFromJira(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	issues, _ := db.FindAllIssues(ctx, nil)
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message":  fmt.Sprintf("pulled %d issues from JIRA", count),
		"total":    len(issues),
		"upserted": count,
	})
}

// PUT /api/v{version}/Issue/cache — Push dirty issues from MongoDB to JIRA
func CachePushHandler(w http.ResponseWriter, r *http.Request) {
	var req models.CachePushRequest
	decodeJSON(r, &req)

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	count, err := db.PushToJira(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": fmt.Sprintf("pushed %d dirty issues to JIRA", count),
		"pushed":  count,
	})
}

// DELETE /api/v{version}/Issue/cache — Clear all issues in MongoDB
func CacheDeleteHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	result, err := db.Issues().DeleteMany(ctx, map[string]interface{}{})
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, fmt.Sprintf("cleared %d issues from DB", result.DeletedCount))
}
