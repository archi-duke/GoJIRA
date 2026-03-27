package handlers

import (
	"context"
	"log"
	"net/http"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"gojira-api/db"
	"gojira-api/jiraclient"
	"gojira-api/models"
)

// POST /api/v{version}/Issue
func IssueCreateHandler(w http.ResponseWriter, r *http.Request) {
	var req models.IssueCreateRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Summary == "" {
		respondError(w, http.StatusBadRequest, "summary is required")
		return
	}

	// Create in JIRA first to get key
	fields := map[string]interface{}{
		"summary":   req.Summary,
		"issuetype": map[string]string{"name": req.IssueType},
	}
	if req.Desc != "" {
		fields["description"] = req.Desc
	}

	result, err := jiraclient.CreateIssue(fields)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	key, _ := result["key"].(string)

	// Save to MongoDB
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	doc := bson.M{
		"key":    key,
		"id":     result["id"],
		"self":   result["self"],
		"fields": fields,
		"_dirty": false, // already synced to JIRA
		"_syncedAt": time.Now(),
	}
	if req.ReqType != "" {
		fields["customfield_12555"] = req.ReqType
	}

	db.UpsertIssue(ctx, key, doc)

	respondJSON(w, http.StatusOK, result)
}

// GET /api/v{version}/Issue — read from MongoDB
func IssueGetHandler(w http.ResponseWriter, r *http.Request) {
	var req models.IssueGetRequest
	if err := parseXBody(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid XBody header")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var filter bson.M
	switch req.InfoType {
	case "designs":
		filter = bson.M{"fields.issuetype.name": bson.M{
			"$in": []string{"Architectueral Design", "Detaied Design"},
		}}
	case "nonTestIssues":
		filter = bson.M{"fields.issuetype.name": bson.M{"$ne": "Test"}}
	case "isBusy":
		respondJSON(w, http.StatusOK, false)
		return
	default:
		filter = nil
	}

	issues, err := db.FindAllIssues(ctx, filter)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Strip internal fields from response
	cleaned := make([]bson.M, 0, len(issues))
	for _, issue := range issues {
		delete(issue, "_id")
		delete(issue, "_dirty")
		delete(issue, "_syncedAt")
		cleaned = append(cleaned, issue)
	}

	respondJSON(w, http.StatusOK, cleaned)
}

// PUT /api/v{version}/Issue — update in MongoDB, mark dirty
func IssueUpdateHandler(w http.ResponseWriter, r *http.Request) {
	var req models.IssueUpdateRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Key == "" {
		respondError(w, http.StatusBadRequest, "key is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Build field updates with "fields." prefix
	updates := bson.M{}
	for k, v := range req.Data {
		updates["fields."+k] = v
	}
	updates["_dirty"] = true

	if err := db.UpdateIssueFields(ctx, req.Key, updates); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, "issue updated")
}

// DELETE /api/v{version}/Issue — delete from MongoDB and JIRA
func IssueDeleteHandler(w http.ResponseWriter, r *http.Request) {
	var req models.IssueDeleteRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Key == "" {
		respondError(w, http.StatusBadRequest, "key is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Delete from MongoDB
	if err := db.DeleteIssueByKey(ctx, req.Key); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Async delete from JIRA
	go func() {
		if err := jiraclient.DeleteIssue(req.Key); err != nil {
			log.Printf("async JIRA delete %s failed: %v", req.Key, err)
		}
	}()

	respondJSON(w, http.StatusOK, "issue deleted")
}
