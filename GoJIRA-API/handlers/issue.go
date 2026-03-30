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

	prjKey := req.ProjectKey
	if prjKey == "" {
		prjKey = req.PrjID
	}

	// Save to MongoDB first (with _dirty flag for JIRA sync)
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	doc := bson.M{
		"key":       "",
		"issueType": req.IssueType,
		"subKey":    req.SubKey,
		"summary":   req.Summary,
		"labels":    req.Labels,
		"reqtype":   req.ReqType,
		"_dirty":    true,
		"_syncedAt": nil,
	}
	if req.Desc != "" {
		doc["description"] = req.Desc
	}

	// Use subKey as temporary key until JIRA assigns a real one
	tempKey := prjKey + "-" + req.SubKey
	doc["key"] = tempKey
	db.UpsertIssue(ctx, tempKey, doc)

	// Create in JIRA asynchronously
	go func() {
		fields := map[string]interface{}{
			"summary":   req.Summary,
			"issuetype": map[string]string{"name": req.IssueType},
		}
		if req.Desc != "" {
			fields["description"] = req.Desc
		}
		if len(req.Labels) > 0 {
			fields["labels"] = req.Labels
		}
		if req.ReqType != "" {
			fields["customfield_12555"] = map[string]string{"value": req.ReqType}
		}
		if req.SubKey != "" {
			fields["customfield_15209"] = req.SubKey
		}
		if prjKey != "" {
			fields["project"] = map[string]string{"key": prjKey}
		}

		result, err := jiraclient.CreateIssue(fields)
		if err != nil {
			log.Printf("async create JIRA issue failed: %v", err)
			return
		}

		jiraKey, _ := result["key"].(string)
		if jiraKey == "" {
			return
		}

		bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Remove temp doc and re-fetch from JIRA with real key
		db.DeleteIssueByKey(bgCtx, tempKey)

		jiraIssue, err := jiraclient.GetIssue(jiraKey)
		if err != nil {
			// Fallback: save with real key
			doc["key"] = jiraKey
			doc["_dirty"] = false
			doc["_syncedAt"] = time.Now()
			db.UpsertIssue(bgCtx, jiraKey, doc)
		} else {
			newDoc := db.TransformToCache(jiraIssue)
			newDoc["_dirty"] = false
			newDoc["_syncedAt"] = time.Now()
			db.UpsertIssue(bgCtx, jiraKey, newDoc)
		}
		log.Printf("async: created JIRA issue %s (subKey: %s)", jiraKey, req.SubKey)
	}()

	respondJSON(w, http.StatusOK, map[string]string{"key": tempKey, "subKey": req.SubKey})
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
		filter = bson.M{"issueType": bson.M{
			"$in": []string{"Architectueral Design", "Detaied Design"},
		}}
	case "nonTestIssues":
		filter = bson.M{"issueType": bson.M{"$ne": "Test"}}
	case "isBusy":
		respondJSON(w, http.StatusOK, false)
		return
	default:
		filter = nil
	}

	// Filter by project if prjID is provided (match key prefix)
	if req.PrjID != "" {
		prjFilter := bson.M{"key": bson.M{"$regex": "^" + req.PrjID + "-"}}
		if filter != nil {
			filter = bson.M{"$and": []bson.M{filter, prjFilter}}
		} else {
			filter = prjFilter
		}
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

	// Build field updates (flat cache schema — no "fields." prefix)
	updates := bson.M{}
	for k, v := range req.Data {
		updates[k] = v
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
