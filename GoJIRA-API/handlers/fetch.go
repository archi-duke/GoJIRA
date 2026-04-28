package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
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

	var issueTypes []string
	if req.Query != "" {
		for _, t := range splitAndTrim(req.Query) {
			if t != "" {
				issueTypes = append(issueTypes, t)
			}
		}
	}

	count, _, err := db.PullFromJiraFiltered(ctx, req.PrjID, issueTypes)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message":  fmt.Sprintf("pulled %d issues from JIRA", count),
		"upserted": count,
	})
}

func splitAndTrim(s string) []string {
	parts := []string{}
	for _, p := range strings.Split(s, ",") {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return parts
}
